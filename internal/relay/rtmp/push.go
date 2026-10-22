package rtmp

import (
	"bytes"
	"context"
	"errors"
	mp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/gortmplib"
	msg "github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/truefix/multicaster/internal/session"
	"github.com/truefix/multicaster/internal/stream"
	flv "github.com/yutopp/go-flv/tag"
	"io"
	"log/slog"
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
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	w.tracker.SetDestStatus(w.ID, session.DestOff, "")
}
func (w *PushWorker) run(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		w.tracker.SetDestStatus(w.ID, session.DestConnecting, "")
		started := time.Now()
		err := w.connect(ctx)
		if ctx.Err() != nil {
			return
		}
		w.tracker.SetDestStatus(w.ID, session.DestError, "Connection interrupted; retrying. Check URL, key and network.")
		// Protocol errors can embed the complete publish URL, including query secrets.
		// Keep credentials out of logs rather than attempting partial redaction.
		w.logger.Warn("provider disconnected or rejected stream; retrying", "destination", w.Name)
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
func (w *PushWorker) connect(ctx context.Context) (connectErr error) {
	defer func() {
		if recover() != nil {
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
	client := &gortmplib.Client{URL: u, Publish: true}
	connectCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	err = client.Initialize(connectCtx)
	cancel()
	if err != nil {
		return err
	}
	defer client.Close()
	stopClose := context.AfterFunc(ctx, client.Close)
	defer stopClose()
	remoteDone := closedClient(client)
	ps := w.hub.Pubsub()
	if ps == nil {
		return errors.New("input offline")
	}
	failed := make(chan error, 1)
	var base uint32
	haveBase := false
	streaming := false
	sub := ps.Subscribe(func(tag *flv.FlvTag) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !haveBase {
			base = tag.Timestamp
			haveBase = true
		}
		tag.Timestamp -= base
		client.NetConn().SetWriteDeadline(time.Now().Add(8 * time.Second))
		err := writeTag(client, tag)
		if err != nil {
			select {
			case failed <- err:
			default:
			}
			return err
		}
		if !streaming {
			w.tracker.SetDestStatus(w.ID, session.DestLive, "")
			streaming = true
		}
		return nil
	})
	defer func() { client.Close(); ps.Unsubscribe(sub); sub.Wait() }()
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
func closedClient(c *gortmplib.Client) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { recover(); c.Close() }()
		for {
			if _, err := c.Read(); err != nil {
				c.Close()
				return
			}
		}
	}()
	return done
}
func writeTag(c *gortmplib.Client, t *flv.FlvTag) error {
	dts := time.Duration(t.Timestamp) * time.Millisecond
	switch d := t.Data.(type) {
	case *flv.VideoData:
		b, err := payloadBytes(d.Data)
		if err != nil {
			return err
		}
		v := &msg.Video{ChunkStreamID: 6, MessageStreamID: 0x1000000, DTS: dts, FrameType: msg.VideoFrameType(d.FrameType), Codec: 7, PacketType: msg.VideoPacketType(d.AVCPacketType), PTSDelta: time.Duration(d.CompositionTime) * time.Millisecond}
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
		a := &msg.Audio{ChunkStreamID: 4, MessageStreamID: 0x1000000, DTS: dts, Codec: 10, Rate: msg.AudioRate(d.SoundRate), Depth: msg.AudioDepth(d.SoundSize), IsStereo: d.SoundType == 1, AACType: msg.AudioAACType(d.AACPacketType)}
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
	}
	return nil
}

func payloadBytes(r io.Reader) ([]byte, error) {
	if b, ok := r.(*bytes.Buffer); ok {
		return b.Bytes(), nil
	}
	return io.ReadAll(r)
}
