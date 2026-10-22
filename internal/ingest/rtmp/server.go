package rtmpingest

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	mp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/gortmplib"
	msg "github.com/bluenviron/gortmplib/pkg/message"
	"github.com/truefix/multicaster/internal/engine"
	flv "github.com/yutopp/go-flv/tag"
)

// Run owns connection lifetimes. Malformed clients and parser panics are isolated
// to their connection rather than being able to terminate all relay workers.
func Run(ctx context.Context, ln net.Listener, eng *engine.Engine, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	stop := context.AfterFunc(ctx, func() { ln.Close() })
	defer stop()
	defer func() { cancel(); ln.Close(); wg.Wait() }()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			defer conn.Close()
			owns := false
			defer func() {
				if recover() != nil {
					logger.Error("RTMP connection parser panicked; connection closed")
				}
				if owns {
					eng.OnPublishStop()
					logger.Info("ingest stopped")
				}
			}()
			after := context.AfterFunc(ctx, func() { conn.Close() })
			defer after()
			if tcp, ok := conn.(*net.TCPConn); ok {
				tcp.SetKeepAlive(true)
				tcp.SetKeepAlivePeriod(15 * time.Second)
			}
			deadline := time.Now().Add(10 * time.Second)
			conn.SetDeadline(deadline)
			wire := &idleConn{Conn: conn, authDeadline: deadline}
			server := &gortmplib.ServerConn{RW: wire}
			if server.Initialize() != nil || server.AcceptConn() != nil {
				return
			}
			if !server.Publish || server.URL == nil {
				return
			}
			path := strings.TrimPrefix(server.URL.Path, "/")
			app, key, ok := strings.Cut(path, "/")
			if !ok || app != eng.Config().IngestApp() || !eng.ValidStreamKey(key) {
				server.RejectAction()
				return
			}
			ps, err := eng.OnPublishStart(key)
			if err != nil {
				server.RejectAction()
				return
			}
			owns = true
			if server.AcceptAction() != nil {
				return
			}
			wire.authDeadline = time.Time{}
			conn.SetDeadline(time.Time{})
			pub := ps.Pub()
			logger.Info("ingest started")
			for {
				message, err := server.Read()
				if err != nil {
					return
				}
				tag, err := mediaTag(message)
				if err != nil {
					logger.Warn("incompatible or invalid input; closing publisher")
					return
				}
				if tag != nil {
					eng.OnMediaBytes(payloadSize(tag))
					pub.Publish(tag)
				}
				if command, ok := message.(*msg.CommandAMF0); ok && (command.Name == "deleteStream" || command.Name == "FCUnpublish") {
					return
				}
			}
		}()
	}
}

type idleConn struct {
	net.Conn
	authDeadline time.Time
}

func (c *idleConn) Read(b []byte) (int, error) {
	deadline := time.Now().Add(30 * time.Second)
	if !c.authDeadline.IsZero() && c.authDeadline.Before(deadline) {
		deadline = c.authDeadline
	}
	c.SetReadDeadline(deadline)
	return c.Conn.Read(b)
}
func (c *idleConn) Write(b []byte) (int, error) {
	c.SetWriteDeadline(time.Now().Add(8 * time.Second))
	return c.Conn.Write(b)
}
func mediaTag(message msg.Message) (*flv.FlvTag, error) {
	switch d := message.(type) {
	case *msg.Video:
		if d.FrameType == msg.VideoFrameTypeCommand {
			return nil, nil
		}
		if d.Codec != 7 {
			return nil, fmt.Errorf("H.264 video required")
		}
		var body []byte
		if d.PacketType == 0 {
			if d.AVCConfig == nil {
				return nil, fmt.Errorf("missing AVC configuration")
			}
			var b bytes.Buffer
			if _, err := mp4.Marshal(&b, d.AVCConfig, mp4.Context{}); err != nil {
				return nil, err
			}
			body = b.Bytes()
		} else {
			if len(d.AU) > 8*1024*1024 {
				return nil, fmt.Errorf("video packet exceeds 8 MiB")
			}
			body = bytes.Clone(d.AU)
		}
		return &flv.FlvTag{TagType: flv.TagTypeVideo, Timestamp: uint32(d.DTS / time.Millisecond), Data: &flv.VideoData{FrameType: flv.FrameType(d.FrameType), CodecID: 7, AVCPacketType: flv.AVCPacketType(d.PacketType), CompositionTime: int32(d.PTSDelta / time.Millisecond), Data: bytes.NewBuffer(body)}}, nil
	case *msg.Audio:
		if d.Codec == 0 {
			return nil, nil
		}
		if d.Codec != 10 {
			return nil, fmt.Errorf("AAC audio required")
		}
		var body []byte
		if d.AACType == 0 {
			if d.AACConfig == nil {
				return nil, fmt.Errorf("missing AAC configuration")
			}
			var err error
			body, err = d.AACConfig.Marshal()
			if err != nil {
				return nil, err
			}
		} else {
			if len(d.AU) > 1024*1024 {
				return nil, fmt.Errorf("AAC packet exceeds 1 MiB")
			}
			body = bytes.Clone(d.AU)
		}
		soundType := flv.SoundTypeMono
		if d.IsStereo {
			soundType = flv.SoundTypeStereo
		}
		return &flv.FlvTag{TagType: flv.TagTypeAudio, Timestamp: uint32(d.DTS / time.Millisecond), Data: &flv.AudioData{SoundFormat: 10, SoundRate: flv.SoundRate(d.Rate), SoundSize: flv.SoundSize(d.Depth), SoundType: soundType, AACPacketType: flv.AACPacketType(d.AACType), Data: bytes.NewBuffer(body)}}, nil
	}
	return nil, nil
}
func payloadSize(t *flv.FlvTag) int {
	switch d := t.Data.(type) {
	case *flv.VideoData:
		return d.Data.(*bytes.Buffer).Len()
	case *flv.AudioData:
		return d.Data.(*bytes.Buffer).Len()
	}
	return 0
}
