package rtmp

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	msg "github.com/bluenviron/gortmplib/pkg/message"
	"github.com/truefix/multicaster/internal/session"
	"github.com/truefix/multicaster/internal/stream"
	flv "github.com/yutopp/go-flv/tag"
)

type receivedSession struct {
	messages []msg.Message
	err      error
}

// gortmplib's test server assigns numeric ID 1; its codec uses wire-order IDs.
const publishMessageStreamID = 0x1000000

type publishProbe struct {
	address      string
	ready, media chan struct{}
	sessions     chan receivedSession
	done         chan struct{}
}

func newPublishProbe(t *testing.T, count int) *publishProbe {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	probe := &publishProbe{address: listener.Addr().String(), ready: make(chan struct{}, count), media: make(chan struct{}, count), sessions: make(chan receivedSession, count), done: make(chan struct{})}
	t.Cleanup(func() {
		listener.Close()
		select {
		case <-probe.done:
		case <-time.After(3 * time.Second):
			t.Error("test receiver did not stop")
		}
	})
	go func() {
		defer close(probe.done)
		for range count {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			func() {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(10 * time.Second))
				server := &gortmplib.ServerConn{RW: conn}
				var result receivedSession
				if err := server.Initialize(); err != nil {
					result.err = err
					probe.sessions <- result
					return
				}
				if err := server.AcceptConn(); err != nil {
					result.err = err
					probe.sessions <- result
					return
				}
				if err := server.AcceptAction(); err != nil {
					result.err = err
					probe.sessions <- result
					return
				}
				if err := server.Write(&msg.UserControlPingRequest{ServerTime: 123}); err != nil {
					result.err = err
					probe.sessions <- result
					return
				}
				gotMedia := false
				for {
					message, err := server.Read()
					if err != nil {
						result.err = err
						probe.sessions <- result
						return
					}
					result.messages = append(result.messages, message)
					switch value := message.(type) {
					case *msg.UserControlPingResponse:
						probe.ready <- struct{}{}
					case *msg.Video:
						if value.PacketType == msg.VideoPacketTypeAU && !gotMedia {
							gotMedia = true
							probe.media <- struct{}{}
						}
					}
				}
			}()
		}
	}()
	return probe
}
func newTestWorker(t *testing.T, address string) (*PushWorker, *stream.Hub, *stream.Pub) {
	t.Helper()
	hub := stream.NewHub()
	ps, err := hub.Start("input")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hub.Stop)
	tracker := session.NewTracker()
	tracker.SyncDestinations([]session.DestinationRuntime{{ID: "provider", Enabled: true}})
	worker := NewPushWorker("provider", "Test", "rtmp://"+address+"/live/test-key?token=private-test-query", hub, tracker, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	return worker, hub, ps.Pub()
}
func receive(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + description)
	}
}
func publishUntilMedia(t *testing.T, pub *stream.Pub, ch <-chan struct{}) {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(15 * time.Millisecond)
	defer tick.Stop()
	var ts uint32
	for {
		select {
		case <-ch:
			return
		case <-timeout.C:
			t.Fatal("receiver did not get video")
		case <-tick.C:
			ts += 40
			pub.Publish(&flv.FlvTag{Timestamp: ts, Data: &flv.VideoData{CodecID: 7, FrameType: flv.FrameTypeKeyFrame, AVCPacketType: flv.AVCPacketTypeNALU, Data: bytes.NewBuffer([]byte{0, 0, 0, 1, 0x65})}})
		}
	}
}
func assertUnpublished(t *testing.T, probe *publishProbe, hasVideo bool) {
	t.Helper()
	var result receivedSession
	select {
	case result = <-probe.sessions:
	case <-time.After(3 * time.Second):
		t.Fatal("publisher socket was left connected")
	}
	if result.err != io.EOF {
		t.Fatalf("receiver did not get a clean connection end: %v", result.err)
	}
	var terminal []string
	var lastVideo time.Duration
	for _, message := range result.messages {
		switch value := message.(type) {
		case *msg.Video:
			if value.PacketType == msg.VideoPacketTypeEOS {
				terminal = append(terminal, "eos")
				if value.MessageStreamID != publishMessageStreamID || value.DTS != lastVideo {
					t.Fatal("EOS used the wrong stream or timestamp")
				}
			} else if value.PacketType == msg.VideoPacketTypeAU {
				if len(terminal) > 0 {
					t.Fatal("media was sent after unpublish began")
				}
				lastVideo = value.DTS
			}
		case *msg.CommandAMF0:
			if value.Name == "FCUnpublish" {
				terminal = append(terminal, value.Name)
				if value.MessageStreamID != 0 || len(value.Arguments) != 2 || value.Arguments[0] != nil || value.Arguments[1] != "test-key?token=private-test-query" {
					t.Fatalf("incorrect unpublish command: %+v", value)
				}
			}
			if value.Name == "deleteStream" {
				terminal = append(terminal, value.Name)
				if value.MessageStreamID != 0 || len(value.Arguments) != 2 || value.Arguments[0] != nil || value.Arguments[1] != float64(1) {
					t.Fatalf("incorrect deleteStream command: %+v", value)
				}
			}
		}
	}
	want := []string{"FCUnpublish", "deleteStream"}
	if hasVideo {
		want = append([]string{"eos"}, want...)
	}
	if len(terminal) != len(want) {
		t.Fatalf("missing stop signals: got %v, want %v", terminal, want)
	}
	for i := range want {
		if terminal[i] != want[i] {
			t.Fatalf("incorrect shutdown order: %v", terminal)
		}
	}
}
func TestStopUnpublishesAndReenableCreatesNewSession(t *testing.T) {
	probe := newPublishProbe(t, 2)
	worker, hub, pub := newTestWorker(t, probe.address)
	worker.Start(context.Background())
	defer worker.Stop()
	receive(t, probe.ready, "publisher readiness")
	publishUntilMedia(t, pub, probe.media)
	worker.Stop()
	assertUnpublished(t, probe, true)
	next := NewPushWorker("provider", "Test", worker.URL, hub, worker.tracker, worker.logger, false)
	next.Start(context.Background())
	defer next.Stop()
	receive(t, probe.ready, "fresh publisher readiness")
	publishUntilMedia(t, pub, probe.media)
	next.Stop()
	assertUnpublished(t, probe, true)
}
func TestParentCancellationUnpublishesBeforeAnyMedia(t *testing.T) {
	probe := newPublishProbe(t, 1)
	worker, _, _ := newTestWorker(t, probe.address)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx)
	defer worker.Stop()
	receive(t, probe.ready, "publisher readiness")
	cancel()
	worker.WaitStopped()
	assertUnpublished(t, probe, false)
}

func TestAutomaticRetryDoesNotExplicitlyEndBroadcast(t *testing.T) {
	probe := newPublishProbe(t, 1)
	worker, hub, pub := newTestWorker(t, probe.address)
	worker.Start(context.Background())
	defer worker.Stop()
	receive(t, probe.ready, "publisher readiness")
	publishUntilMedia(t, pub, probe.media)
	hub.Pubsub().Close()
	var result receivedSession
	select {
	case result = <-probe.sessions:
	case <-time.After(3 * time.Second):
		t.Fatal("failed consumer session did not close")
	}
	worker.Stop()
	for _, message := range result.messages {
		switch value := message.(type) {
		case *msg.Video:
			if value.PacketType == msg.VideoPacketTypeEOS {
				t.Fatal("automatic retry sent a deliberate end signal")
			}
		case *msg.CommandAMF0:
			if value.Name == "FCUnpublish" || value.Name == "deleteStream" {
				t.Fatal("automatic retry ended the platform broadcast")
			}
		}
	}
}

// A stalled receiver must not make a dashboard stop wait for the usual 8-second
// media-write timeout. The 1-second TCP watchdog bounds graceful cleanup.
func TestStopInterruptsBlockedMediaWrite(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ready := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	defer func() {
		once.Do(func() { close(release) })
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Error("stalled receiver did not close")
		}
	}()
	go func() {
		defer close(finished)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if tcp, ok := conn.(*net.TCPConn); ok {
			tcp.SetReadBuffer(1024)
		}
		server := &gortmplib.ServerConn{RW: conn}
		if server.Initialize() != nil || server.AcceptConn() != nil || server.AcceptAction() != nil {
			return
		}
		close(ready)
		<-release
	}()
	worker, _, pub := newTestWorker(t, listener.Addr().String())
	worker.Start(context.Background())
	defer worker.Stop()
	receive(t, ready, "stalled publisher acceptance")
	payload := make([]byte, 1024*1024)
	for i := range 4 {
		pub.Publish(&flv.FlvTag{Timestamp: uint32(i * 40), Data: &flv.VideoData{CodecID: 7, FrameType: flv.FrameTypeKeyFrame, AVCPacketType: flv.AVCPacketTypeNALU, Data: bytes.NewBuffer(payload)}})
		time.Sleep(5 * time.Millisecond)
	}
	started := time.Now()
	worker.Stop()
	if elapsed := time.Since(started); elapsed > gracefulStopTimeout+1500*time.Millisecond {
		t.Fatalf("stop blocked for %s", elapsed)
	}
	once.Do(func() { close(release) })
}
