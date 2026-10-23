package stream

import (
	"bytes"
	flv "github.com/yutopp/go-flv/tag"
	"testing"
	"time"
)

func video(ts uint32, key bool) *flv.FlvTag {
	frame := flv.FrameTypeInterFrame
	if key {
		frame = flv.FrameTypeKeyFrame
	}
	return &flv.FlvTag{Timestamp: ts, Data: &flv.VideoData{FrameType: frame, AVCPacketType: 1, Data: bytes.NewBuffer([]byte{1, 2, 3})}}
}
func TestLateSubscriberWaitsForKeyframeAndReceivesAudioHeader(t *testing.T) {
	ps := NewPubsub("live")
	defer ps.Close()
	pub := ps.Pub()
	pub.Publish(&flv.FlvTag{Data: &flv.VideoData{AVCPacketType: 0, Data: bytes.NewBuffer([]byte{7})}})
	pub.Publish(&flv.FlvTag{Data: &flv.AudioData{AACPacketType: 0, Data: bytes.NewBuffer([]byte{8})}})
	events := make(chan *flv.FlvTag, 8)
	sub := ps.Subscribe(func(tag *flv.FlvTag) error { events <- tag; return nil })
	defer ps.Unsubscribe(sub)
	pub.Publish(video(100, false))
	select {
	case <-events:
		t.Fatal("joined before keyframe")
	case <-time.After(10 * time.Millisecond):
	}
	pub.Publish(video(200, true))
	for i := 0; i < 3; i++ {
		select {
		case tag := <-events:
			if tag.Timestamp != 200 {
				t.Fatal("headers have old timestamps")
			}
			if i == 1 {
				if _, ok := tag.Data.(*flv.AudioData); !ok {
					t.Fatal("AAC header missing")
				}
			}
		case <-time.After(time.Second):
			t.Fatal("missing startup media")
		}
	}
}

func TestBlockedSubscriberDoesNotBlockPublisher(t *testing.T) {
	ps := NewPubsub("live")
	defer ps.Close()
	release := make(chan struct{})
	entered := make(chan struct{})
	sub := ps.Subscribe(func(*flv.FlvTag) error {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		return nil
	})
	defer func() { close(release); ps.Unsubscribe(sub); sub.Wait() }()
	pub := ps.Pub()
	pub.Publish(video(0, true))
	<-entered
	done := make(chan struct{})
	go func() {
		for i := 0; i < 300; i++ {
			pub.Publish(video(uint32(i+1), false))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publisher blocked by slow output")
	}
	select {
	case <-sub.Done():
	default:
		t.Fatal("overflowing subscriber was not disconnected")
	}
}

func TestConsumerPanicIsIsolatedAndReported(t *testing.T) {
	ps := NewPubsub("test")
	defer ps.Close()
	bad := ps.Subscribe(func(*flv.FlvTag) error { panic("decoder failed") })
	good := make(chan struct{}, 2)
	sub := ps.Subscribe(func(*flv.FlvTag) error { good <- struct{}{}; return nil })
	defer ps.Unsubscribe(sub)
	ps.Pub().Publish(video(100, true))
	select {
	case <-bad.Done():
	case <-time.After(time.Second):
		t.Fatal("panic was not contained")
	}
	bad.Wait()
	if bad.Err() == nil {
		t.Fatal("panic was silently discarded")
	}
	select {
	case <-good:
	case <-time.After(time.Second):
		t.Fatal("other output was interrupted")
	}
}

func TestVideoEndSequenceCannotStartSubscriber(t *testing.T) {
	ps := NewPubsub("live")
	defer ps.Close()
	events := make(chan struct{}, 1)
	sub := ps.Subscribe(func(*flv.FlvTag) error { events <- struct{}{}; return nil })
	defer ps.Unsubscribe(sub)
	ps.Pub().Publish(&flv.FlvTag{Data: &flv.VideoData{FrameType: flv.FrameTypeKeyFrame, AVCPacketType: flv.AVCPacketTypeEOS, Data: bytes.NewBuffer(nil)}})
	select {
	case <-events:
		t.Fatal("end-of-sequence was treated as a video keyframe")
	case <-time.After(20 * time.Millisecond):
	}
}
