package rtmp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/bits"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/bytecounter"
	"github.com/bluenviron/gortmplib/pkg/handshake"
	msg "github.com/bluenviron/gortmplib/pkg/message"
	"github.com/truefix/multicaster/internal/session"
	"github.com/truefix/multicaster/internal/stream"
	flv "github.com/yutopp/go-flv/tag"
)

// Use the wire codec directly: ServerConn itself always allocates stream 1,
// which would conceal regressions in negotiated-ID handling.
func newWirePeer(t *testing.T, serve func(*msg.ReadWriter) error) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		ln.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("RTMP peer did not stop")
		}
	})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		stop := context.AfterFunc(ctx, func() { conn.Close() })
		defer stop()
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		bc := bytecounter.NewReadWriter(conn)
		if _, _, err := handshake.DoServer(bc, false); err != nil {
			done <- err
			return
		}
		done <- serve(msg.NewReadWriter(bc, bc, false))
	}()
	return ln.Addr().String()
}

func acceptWirePublish(wire *msg.ReadWriter, id float64, publishCode string) error {
	for {
		m, err := wire.Read()
		if err != nil {
			return err
		}
		cmd, ok := m.(*msg.CommandAMF0)
		if !ok {
			continue
		}
		switch cmd.Name {
		case "connect":
			err = wire.Write(&msg.CommandAMF0{ChunkStreamID: 3, Name: "_result", CommandID: cmd.CommandID, Arguments: []any{nil, amf0.Object{{Key: "level", Value: "status"}, {Key: "code", Value: "NetConnection.Connect.Success"}}}})
		case "releaseStream", "FCPublish":
			// Optional command responses must not be mistaken for createStream.
			name := "_result"
			if cmd.Name == "releaseStream" {
				name = "_error"
			}
			err = wire.Write(&msg.CommandAMF0{ChunkStreamID: 3, Name: name, CommandID: cmd.CommandID, Arguments: []any{nil}})
		case "createStream":
			err = wire.Write(&msg.CommandAMF0{ChunkStreamID: 3, Name: "_result", CommandID: cmd.CommandID, Arguments: []any{nil, id}})
			if math.IsNaN(id) || id < 1 || id > math.MaxUint32 || id != math.Trunc(id) {
				return err
			}
		case "publish":
			if cmd.MessageStreamID != bits.ReverseBytes32(uint32(id)) {
				return fmt.Errorf("publish used stream ID %#x, wanted numeric %v", cmd.MessageStreamID, id)
			}
			level := "status"
			if publishCode != "NetStream.Publish.Start" {
				level = "error"
			}
			return wire.Write(&msg.CommandAMF0{ChunkStreamID: 3, MessageStreamID: cmd.MessageStreamID, Name: "onStatus", Arguments: []any{nil, amf0.Object{{Key: "level", Value: level}, {Key: "code", Value: publishCode}}}})
		}
		if err != nil {
			return err
		}
	}
}

func TestNegotiatedStreamIDMediaMetadataAndShutdown(t *testing.T) {
	const id = 0x1234
	wireID := bits.ReverseBytes32(id)
	video, audio := make(chan struct{}, 1), make(chan struct{}, 1)
	updated := make(chan struct{})
	finished := make(chan struct{})
	address := newWirePeer(t, func(wire *msg.ReadWriter) error {
		if err := acceptWirePublish(wire, id, "NetStream.Publish.Start"); err != nil {
			return err
		}
		var gotMeta, gotVideo, gotAudio, gotEOS, gotUnpublish bool
		metadataCount := 0
		for {
			m, err := wire.Read()
			if err != nil {
				return err
			}
			switch m := m.(type) {
			case *msg.DataAMF0:
				if m.MessageStreamID != wireID || len(m.Payload) != 3 || m.Payload[0] != "@setDataFrame" || m.Payload[1] != "onMetaData" {
					return fmt.Errorf("incorrect metadata: %+v", m)
				}
				gotMeta = true
				metadataCount++
				if metadataCount == 2 {
					object, ok := m.Payload[2].(amf0.ECMAArray)
					if !ok {
						return fmt.Errorf("metadata array changed type")
					}
					width, _ := amf0.Object(object).GetFloat64("width")
					if width != 1280 {
						return fmt.Errorf("metadata update was not forwarded")
					}
					close(updated)
				}
			case *msg.Video:
				if m.MessageStreamID != wireID {
					return fmt.Errorf("video has wrong stream ID")
				}
				if m.PacketType == msg.VideoPacketTypeAU && !gotVideo {
					if !gotMeta || m.DTS != 0 {
						return fmt.Errorf("missing metadata or incorrect video epoch: %v", m.DTS)
					}
					gotVideo = true
					video <- struct{}{}
				}
				if m.PacketType == msg.VideoPacketTypeEOS {
					gotEOS = true
				}
			case *msg.Audio:
				if m.MessageStreamID != wireID || m.DTS != 20*time.Millisecond {
					return fmt.Errorf("incorrect audio stream or timestamp: %+v", m)
				}
				gotAudio = true
				select {
				case audio <- struct{}{}:
				default:
				}
			case *msg.CommandAMF0:
				if m.Name == "FCUnpublish" {
					gotUnpublish = true
				}
				if m.Name == "deleteStream" {
					if m.MessageStreamID != 0 || len(m.Arguments) != 2 || m.Arguments[1] != float64(id) || !gotEOS || !gotUnpublish || !gotAudio {
						return fmt.Errorf("incorrect shutdown: %+v", m)
					}
					close(finished)
					return nil
				}
			}
		}
	})
	w, _, pub := newTestWorker(t, address)
	t.Cleanup(w.Stop)
	metadata, err := stream.NewMetadata(amf0.Data{"@setDataFrame", "onMetaData", amf0.ECMAArray{{Key: "width", Value: float64(1920)}}})
	if err != nil {
		t.Fatal(err)
	}
	pub.Publish(&flv.FlvTag{TagType: flv.TagTypeScriptData, Data: metadata})
	w.Start(context.Background())
	// Joining must replay metadata before the first keyframe, with media at 0.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(3 * time.Second)
waitVideo:
	for {
		select {
		case <-video:
			break waitVideo
		case <-ticker.C:
			pub.Publish(&flv.FlvTag{Timestamp: 1000, Data: &flv.VideoData{CodecID: 7, FrameType: flv.FrameTypeKeyFrame, AVCPacketType: flv.AVCPacketTypeNALU, Data: bytes.NewBuffer([]byte{0, 0, 0, 1, 0x65})}})
		case <-timeout:
			t.Fatal("no video received")
		}
	}
	for _, ts := range []uint32{990, 1020} {
		pub.Publish(&flv.FlvTag{Timestamp: ts, Data: &flv.AudioData{SoundFormat: 10, SoundRate: 3, SoundSize: 1, SoundType: 1, AACPacketType: flv.AACPacketTypeRaw, Data: bytes.NewBuffer([]byte{1, 2, 3})}})
	}
	receive(t, audio, "non-underflowing audio")
	metadata, err = stream.NewMetadata(amf0.Data{"@setDataFrame", "onMetaData", amf0.ECMAArray{{Key: "width", Value: float64(1280)}}})
	if err != nil {
		t.Fatal(err)
	}
	pub.Publish(&flv.FlvTag{TagType: flv.TagTypeScriptData, Data: metadata})
	receive(t, updated, "metadata update after nonzero media timestamps")
	w.Stop()
	receive(t, finished, "negotiated-ID shutdown")
}

func TestProviderStatusStopsLiveWorker(t *testing.T) {
	for _, code := range []string{"NetStream.Publish.BadName", "NetStream.Unpublish.Success", "unknown-private-code"} {
		t.Run(code, func(t *testing.T) {
			sent := make(chan struct{})
			address := newWirePeer(t, func(wire *msg.ReadWriter) error {
				if err := acceptWirePublish(wire, 7, "NetStream.Publish.Start"); err != nil {
					return err
				}
				for {
					m, err := wire.Read()
					if err != nil {
						return err
					}
					if _, ok := m.(*msg.Video); !ok {
						continue
					}
					if err := wire.Write(&msg.CommandAMF0{ChunkStreamID: 3, MessageStreamID: bits.ReverseBytes32(7), Name: "onStatus", Arguments: []any{nil, amf0.ECMAArray{{Key: "level", Value: "error"}, {Key: "code", Value: code}, {Key: "description", Value: "private stream-key"}}}}); err != nil {
						return err
					}
					close(sent)
					// Keep the socket open. Only interpreting onStatus can end it.
					for {
						_, err := wire.Read()
						if err != nil {
							if errors.Is(err, io.EOF) {
								return nil
							}
							return err
						}
					}
				}
			})
			w, _, pub := newTestWorker(t, address)
			t.Cleanup(w.Stop)
			w.Start(context.Background())
			publishUntilMedia(t, pub, sent)
			deadline := time.After(500 * time.Millisecond)
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for {
				status := w.tracker.Snapshot(true, false, false, "").Destinations[0]
				if status.Status == session.DestError {
					if bytes.Contains([]byte(status.LastError), []byte("private")) {
						t.Fatal("provider credentials exposed")
					}
					return
				}
				select {
				case <-tick.C:
				case <-deadline:
					t.Fatal("publishing error left dashboard live")
				}
			}
		})
	}
}

func TestInvalidStreamIDsAndPublishRejection(t *testing.T) {
	for _, id := range []float64{0, -1, 1.5, math.NaN(), math.Inf(1), float64(math.MaxUint32) + 1, 3} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			address := newWirePeer(t, func(wire *msg.ReadWriter) error { return acceptWirePublish(wire, id, "NetStream.Publish.BadName") })
			u, _ := url.Parse("rtmp://" + address + "/live#key")
			client := &publishClient{}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := client.initialize(ctx, u, (&net.Dialer{}).DialContext)
			if client.conn != nil {
				client.Close()
			}
			if err == nil {
				t.Fatal("accepted invalid stream ID or rejected publish")
			}
		})
	}
}

func TestProviderErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name, level, code string
		stream            uint32
		terminal          bool
	}{
		{"onStatus", "status", "NetStream.Publish.Start", 0, false},
		{"onStatus", "status", "NetStream.Unpublish.Success", 0, true},
		{"onStatus", "status", "NetConnection.Connect.Closed", 0, true},
		{"onStatus", "error", "secret", 0, true},
		{"_error", "", "", 0, true},
		{"onStatus", "error", "secret", 99, false},
	} {
		cmd := &msg.CommandAMF0{Name: tc.name, MessageStreamID: tc.stream, Arguments: []any{nil, amf0.Object{{Key: "level", Value: tc.level}, {Key: "code", Value: tc.code}}}}
		err := providerError(cmd, bits.ReverseBytes32(7))
		if (err != nil) != tc.terminal {
			t.Errorf("%+v: %v", tc, err)
		}
		if err != nil && bytes.Contains([]byte(err.Error()), []byte("secret")) {
			t.Fatal("unsafe server status text")
		}
	}
}

func TestInitializeCancellationInterruptsHandshake(t *testing.T) {
	for _, scheme := range []string{"rtmp", "rtmps"} {
		t.Run(scheme, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()
			done := make(chan struct{})
			go func() { defer close(done); io.Copy(io.Discard, serverConn) }()
			defer func() { serverConn.Close(); <-done }()
			u, _ := url.Parse(scheme + "://example.test/live#key")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			client := &publishClient{}
			err := client.initialize(ctx, u, func(_ context.Context, network, address string) (net.Conn, error) {
				want := "example.test:1935"
				if scheme == "rtmps" {
					want = "example.test:443"
				}
				if network != "tcp" || address != want {
					t.Errorf("dialed %s %s", network, address)
				}
				return clientConn, nil
			})
			if err == nil || ctx.Err() == nil {
				t.Fatalf("stalled handshake did not honor cancellation: %v", err)
			}
		})
	}
}
