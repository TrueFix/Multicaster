package hls

import (
	"bytes"
	"context"
	"fmt"
	mp4 "github.com/abema/go-mp4"
	hls "github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/truefix/multicaster/internal/stream"
	flv "github.com/yutopp/go-flv/tag"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Service allocates its muxer only while enabled and ingesting. No encoding.
type Service struct {
	mu           sync.RWMutex
	mux          *hls.Muxer
	video, audio *hls.Track
	err          string
	waiting      bool
	cancel       context.CancelFunc
	done         chan struct{}
	requests     atomic.Int32
	viewers      atomic.Int32
}

type resourceResponse struct {
	header http.Header
	body   bytes.Buffer
	code   int
	err    error
}

func (r *resourceResponse) Header() http.Header  { return r.header }
func (r *resourceResponse) WriteHeader(code int) { r.code = code }
func (r *resourceResponse) Write(b []byte) (int, error) {
	if r.body.Len()+len(b) > 16*1024*1024 {
		r.err = fmt.Errorf("HLS resource too large")
		return 0, r.err
	}
	return r.body.Write(b)
}
func (s *Service) ReadResource(ctx context.Context, name string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://multicaster/"+name, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.readResponse(req)
	if err != nil {
		return nil, err
	}
	if res.err != nil {
		return nil, res.err
	}
	if res.code != 200 {
		return nil, fmt.Errorf("HLS resource is not ready (HTTP %d)", res.code)
	}
	return res.body.Bytes(), nil
}

// gohlslib's initial-playlist wait does not observe request cancellation. Bridge
// it through a bounded recorder so a disconnected viewer or cloud worker cannot
// block Stop. A 12-second watchdog closes only the stuck HLS muxer to release its
// waiters; RTMP outputs keep running and the supervisor retries on a keyframe.
func (s *Service) readResponse(r *http.Request) (*resourceResponse, error) {
	if s.requests.Add(1) > 8 {
		s.requests.Add(-1)
		return nil, fmt.Errorf("too many simultaneous web requests")
	}
	s.mu.RLock()
	m := s.mux
	s.mu.RUnlock()
	if m == nil {
		s.requests.Add(-1)
		return nil, fmt.Errorf("web stream is off or waiting for input")
	}
	res := &resourceResponse{header: make(http.Header), code: 200}
	done := make(chan struct{})
	timeout := time.AfterFunc(12*time.Second, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.mux == m {
			m.Close()
			s.mux = nil
			s.err = "Web stream timed out waiting for complete segments; retrying"
		}
	})
	go func() {
		defer close(done)
		defer s.requests.Add(-1)
		defer timeout.Stop()
		defer func() {
			if recover() != nil {
				res.err = fmt.Errorf("web request failed unexpectedly")
			}
		}()
		m.Handle(res, r)
		if res.body.Len() == 0 && res.code == 200 {
			res.code = 404
		}
	}()
	select {
	case <-r.Context().Done():
		return nil, r.Context().Err()
	case <-done:
		if res.err != nil {
			return nil, res.err
		}
		return res, nil
	}
}

func (s *Service) Start(ps *stream.Pubsub, perf bool) {
	s.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.waiting = true
	s.cancel = cancel
	s.done = make(chan struct{})
	done := s.done
	s.mu.Unlock()
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			sub := ps.Subscribe(func(t *flv.FlvTag) error {
				s.mu.Lock()
				defer s.mu.Unlock()
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if s.err != "" {
					return fmt.Errorf("%s", s.err)
				}
				return s.write(t, perf)
			})
			select {
			case <-ctx.Done():
			case <-sub.Done():
			}
			ps.Unsubscribe(sub)
			sub.Wait()
			s.mu.Lock()
			if s.mux != nil {
				s.mux.Close()
			}
			s.mux = nil
			s.audio = nil
			s.video = nil
			if ctx.Err() == nil {
				s.err = "Web playback interrupted; retrying on the next keyframe"
				if sub.Err() != nil {
					s.err = sub.Err().Error()
				}
			}
			s.mu.Unlock()
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			s.mu.Lock()
			s.err = ""
			s.mu.Unlock()
		}
	}()
}
func (s *Service) Stop() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	s.mu.Lock()
	if s.mux != nil {
		s.mux.Close()
	}
	s.mux = nil
	s.audio = nil
	s.video = nil
	s.waiting = false
	s.err = ""
	s.mu.Unlock()
}
func (s *Service) Status() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.err != "" {
		return "error", s.err
	}
	if s.mux != nil {
		return "live", ""
	}
	if s.waiting {
		return "starting", "Waiting for H.264/AAC headers and a keyframe"
	}
	return "off", ""
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.viewers.Add(1) > 4 {
		s.viewers.Add(-1)
		http.Error(w, "Too many simultaneous preview requests", 503)
		return
	}
	defer s.viewers.Add(-1)
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	res, err := s.readResponse(r)
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(8 * time.Second))
	for key, values := range res.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(res.code)
	if r.Method != "HEAD" {
		w.Write(res.body.Bytes())
	}
}
func (s *Service) write(t *flv.FlvTag, perf bool) error {
	switch d := t.Data.(type) {
	case *flv.AudioData:
		b, err := payloadBytes(d.Data)
		if err != nil {
			return err
		}
		if d.AACPacketType == 0 {
			var cfg mpeg4audio.AudioSpecificConfig
			if err = cfg.Unmarshal(b); err != nil {
				return err
			}
			if s.mux == nil {
				s.audio = &hls.Track{Codec: &codecs.MPEG4Audio{Config: cfg}, ClockRate: cfg.SampleRate}
			}
			return nil
		}
		if s.mux != nil && s.audio != nil {
			return s.mux.WriteMPEG4Audio(s.audio, time.Now(), int64(t.Timestamp)*int64(s.audio.ClockRate)/1000, [][]byte{b})
		}
	case *flv.VideoData:
		b, err := payloadBytes(d.Data)
		if err != nil {
			return err
		}
		if d.AVCPacketType == 0 {
			if s.mux != nil {
				return nil
			}
			var cfg mp4.AVCDecoderConfiguration
			cfg.SetType(mp4.BoxTypeAvcC())
			if _, err = mp4.Unmarshal(bytes.NewReader(b), uint64(len(b)), &cfg, mp4.Context{}); err != nil {
				return err
			}
			if len(cfg.SequenceParameterSets) == 0 || len(cfg.PictureParameterSets) == 0 {
				return fmt.Errorf("missing H.264 parameters")
			}
			s.video = &hls.Track{Codec: &codecs.H264{SPS: cfg.SequenceParameterSets[0].NALUnit, PPS: cfg.PictureParameterSets[0].NALUnit}, ClockRate: 90000}
			return nil
		}
		if d.AVCPacketType != 1 || s.video == nil {
			return nil
		}
		var au h264.AVCC
		if err = au.Unmarshal(b); err != nil {
			return err
		}
		if h264.IsRandomAccess(au) {
			cfg := s.video.Codec.(*codecs.H264)
			au = append(h264.AVCC{cfg.SPS, cfg.PPS}, au...)
		}
		if s.mux == nil {
			if !h264.IsRandomAccess(au) {
				return nil
			}
			duration := 2 * time.Second
			if perf {
				duration = 4 * time.Second
			}
			tracks := []*hls.Track{s.video}
			if s.audio != nil {
				tracks = append(tracks, s.audio)
			}
			s.mux = &hls.Muxer{Tracks: tracks, Variant: hls.MuxerVariantFMP4, SegmentCount: 6, SegmentMinDuration: duration, SegmentMaxSize: 16 * 1024 * 1024}
			if err = s.mux.Start(); err != nil {
				s.mux = nil
				return err
			}
		}
		return s.mux.WriteH264(s.video, time.Now(), (int64(t.Timestamp)+int64(d.CompositionTime))*90, au)
	}
	return nil
}

func payloadBytes(r io.Reader) ([]byte, error) {
	if b, ok := r.(*bytes.Buffer); ok {
		return b.Bytes(), nil
	}
	return io.ReadAll(r)
}
