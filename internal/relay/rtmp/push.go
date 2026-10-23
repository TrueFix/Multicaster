package rtmp

import (
	"bytes"
	"context"
	"errors"
	mp4 "github.com/abema/go-mp4"
	msg "github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/truefix/multicaster/internal/logging"
	"github.com/truefix/multicaster/internal/session"
	"github.com/truefix/multicaster/internal/stream"
	flv "github.com/yutopp/go-flv/tag"
	"io"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"time"
)

type PushWorker struct {
	ID, Name, URL string
	hub           *stream.Hub
	tracker       *session.Tracker
	logger        *slog.Logger
	mu            sync.Mutex
	cancel        context.CancelFunc
	done          chan struct{}
}

func NewPushWorker(id, name, raw string, h *stream.Hub, t *session.Tracker, l *slog.Logger, _ bool) *PushWorker {
	return &PushWorker{ID: id, Name: name, URL: raw, hub: h, tracker: t, logger: l}
}
func (w *PushWorker) Start(parent context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() { defer close(w.done); w.run(ctx) }()
}
func (w *PushWorker) Stop() {
	w.RequestStop()
	w.WaitStopped()
}

// Split cancellation from joining so a master stop drains all providers in
// parallel instead of spending a full stop timeout on each failed endpoint.
func (w *PushWorker) RequestStop() {
	w.mu.Lock()
	cancel := w.cancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
func (w *PushWorker) WaitStopped() {
	w.mu.Lock()
	done := w.done
	w.mu.Unlock()
	if done != nil {
		<-done
	}
	w.tracker.SetDestStatus(w.ID, session.DestOff, "")
}
func (w *PushWorker) run(ctx context.Context) {
	delay := time.Second
	events := logging.NewPlatformEvents(w.logger, w.Name)
	for ctx.Err() == nil {
		w.tracker.SetDestStatus(w.ID, session.DestConnecting, "")
		started := time.Now()
		err := w.connect(ctx, events.Live)
		if ctx.Err() != nil {
			return
		}
		w.tracker.SetDestStatus(w.ID, session.DestError, "Connection interrupted; retrying. Check URL, key and network.")
		// Protocol errors can embed the complete publish URL, including query secrets.
		// Keep credentials out of logs rather than attempting partial redaction.
		events.Unavailable(delay)
		_ = err
		if time.Since(started) > 30*time.Second {
			delay = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, 30*time.Second)
	}
}
func (w *PushWorker) connect(ctx context.Context, onLive func()) (connectErr error) {
	var transport net.Conn
	defer func() {
		if recover() != nil {
			if transport != nil {
				transport.Close()
			}
			connectErr = errors.New("provider client panicked; retrying")
		}
	}()
	u, err := url.Parse(w.URL)
	if err != nil {
		return err
	}
	ep, err := ParseURL(w.URL)
	if err != nil {
		return err
	}
	u.Path = "/" + ep.App
	u.RawPath = ""
	u.Fragment = ep.StreamKey
	if u.RawQuery != "" {
		u.Fragment += "?" + u.RawQuery
		u.RawQuery = ""
	}
	client := &publishClient{}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{KeepAlive: 15 * time.Second}).DialContext(ctx, network, address)
		if err == nil {
			transport = conn
		}
		return conn, err
	}
	connectCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	err = client.initialize(connectCtx, u, dial)
	cancel()
	if err != nil {
		if transport != nil {
			transport.Close()
		}
		return err
	}
	shutdown := &publishShutdown{client: client, transport: transport}
	stopDrain := context.AfterFunc(ctx, shutdown.begin)
	var ps *stream.Pubsub
	var sub *stream.Sub
	var remoteDone <-chan struct{}
	var videoSent, writeFailed bool
	var lastVideoTimestamp uint32
	defer func() {
		intentionalStop := ctx.Err() != nil || !w.hub.Active()
		if !intentionalStop {
			transport.Close()
		}
		shutdown.begin()
		defer func() {
			shutdown.close()
			if remoteDone != nil {
				<-remoteDone
			}
		}()
		defer stopDrain()
		if sub != nil {
			ps.Unsubscribe(sub)
			sub.Wait()
		}
		// A failed/partial media write cannot safely be followed by commands on
		// the same RTMP chunk stream. In that case just close the transport.
		remoteClosed := false
		if remoteDone != nil {
			select {
			case <-remoteDone:
				remoteClosed = true
			default:
			}
		}
		// Automatic reconnection should not explicitly end the platform's
		// broadcast during its reconnect grace period. Only deliberate stops or
		// the end of our input send the unpublish sequence.
		if intentionalStop && !writeFailed && !remoteClosed && time.Now().Before(shutdown.deadline) {
			client.NetConn().SetWriteDeadline(shutdown.deadline)
			_ = unpublish(client, u.Fragment, lastVideoTimestamp, videoSent)
		}
	}()
	remoteDone = closedClient(client)
	ps = w.hub.Pubsub()
	if ps == nil {
		return errors.New("input offline")
	}
	failed := make(chan error, 1)
	var timeline timestampTimeline
	streaming := false
	sub = ps.Subscribe(func(tag *flv.FlvTag) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// DataAMF0 carries no DTS in gortmplib. It must neither establish the
		// media epoch nor be discarded as a late packet with timestamp zero.
		if _, metadata := tag.Data.(*stream.Metadata); !metadata {
			var keep bool
			tag.Timestamp, keep = timeline.normalize(tag.Timestamp)
			if !keep {
				return nil
			}
		}
		client.NetConn().SetWriteDeadline(time.Now().Add(8 * time.Second))
		err := writeTag(client, tag)
		if err != nil {
			writeFailed = true
			select {
			case failed <- err:
			default:
			}
			return err
		}
		if _, ok := tag.Data.(*flv.VideoData); ok {
			videoSent = true
			lastVideoTimestamp = tag.Timestamp
		}
		media := false
		switch d := tag.Data.(type) {
		case *flv.VideoData:
			media = d.AVCPacketType == flv.AVCPacketTypeNALU
		case *flv.AudioData:
			media = d.AACPacketType == flv.AACPacketTypeRaw
		}
		if !streaming && media {
			w.tracker.SetDestStatus(w.ID, session.DestLive, "")
			onLive()
			streaming = true
		}
		return nil
	})
	select {
	case <-remoteDone:
		return errors.New("remote provider disconnected")
	case <-ctx.Done():
		return ctx.Err()
	case <-sub.Done():
		return errors.New("provider buffer exhausted or input ended")
	case err := <-failed:
		return err
	}
}
func closedClient(c *publishClient) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { recover(); c.Close() }()
		for {
			m, err := c.Read()
			if err != nil || providerError(m, c.messageStreamID()) != nil {
				c.Close()
				return
			}
		}
	}()
	return done
}
func writeTag(c *publishClient, t *flv.FlvTag) error {
	dts := time.Duration(t.Timestamp) * time.Millisecond
	switch d := t.Data.(type) {
	case *flv.VideoData:
		b, err := payloadBytes(d.Data)
		if err != nil {
			return err
		}
		v := &msg.Video{ChunkStreamID: 6, MessageStreamID: c.messageStreamID(), DTS: dts, FrameType: msg.VideoFrameType(d.FrameType), Codec: 7, PacketType: msg.VideoPacketType(d.AVCPacketType), PTSDelta: time.Duration(d.CompositionTime) * time.Millisecond}
		if d.AVCPacketType == 0 {
			var cfg mp4.AVCDecoderConfiguration
			cfg.SetType(mp4.BoxTypeAvcC())
			if _, err = mp4.Unmarshal(bytes.NewReader(b), uint64(len(b)), &cfg, mp4.Context{}); err != nil {
				return err
			}
			v.AVCConfig = &cfg
		} else {
			v.AU = b
		}
		return c.Write(v)
	case *flv.AudioData:
		b, err := payloadBytes(d.Data)
		if err != nil {
			return err
		}
		a := &msg.Audio{ChunkStreamID: 4, MessageStreamID: c.messageStreamID(), DTS: dts, Codec: 10, Rate: msg.AudioRate(d.SoundRate), Depth: msg.AudioDepth(d.SoundSize), IsStereo: d.SoundType == 1, AACType: msg.AudioAACType(d.AACPacketType)}
		if d.AACPacketType == 0 {
			var cfg mpeg4audio.AudioSpecificConfig
			if err = cfg.Unmarshal(b); err != nil {
				return err
			}
			a.AACConfig = &cfg
		} else {
			a.AU = b
		}
		return c.Write(a)
	case *stream.Metadata:
		payload, err := d.Payload()
		if err != nil {
			return err
		}
		return c.Write(&msg.DataAMF0{ChunkStreamID: 5, MessageStreamID: c.messageStreamID(), Payload: payload})
	}
	return nil
}

func payloadBytes(r io.Reader) ([]byte, error) {
	if b, ok := r.(*bytes.Buffer); ok {
		return b.Bytes(), nil
	}
	return io.ReadAll(r)
}
