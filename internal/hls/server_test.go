package hls

import (
	"context"
	ghls "github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"testing"
	"time"
)

func emptyMuxer(t *testing.T) *ghls.Muxer {
	t.Helper()
	m := &ghls.Muxer{Variant: ghls.MuxerVariantFMP4, Tracks: []*ghls.Track{{Codec: &codecs.H264{SPS: []byte{0x67, 0x42, 0, 0x1e, 0xe9, 1, 0x40, 0x7b, 0x20}, PPS: []byte{0x68, 0xce, 6, 0xe2}}, ClockRate: 90000}}}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	return m
}
func TestInitialPlaylistCancellationDoesNotBlockControlPlane(t *testing.T) {
	var s Service
	s.mux = emptyMuxer(t)
	defer s.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := s.ReadResource(ctx, "index.m3u8"); done <- err }()
	deadline := time.Now().Add(time.Second)
	for s.requests.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("playlist wait ignored cancellation")
	}
	s.Stop()
	deadline = time.Now().Add(time.Second)
	for s.requests.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.requests.Load() != 0 {
		t.Fatal("playlist goroutine was leaked")
	}
}
