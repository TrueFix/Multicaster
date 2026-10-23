package stream

import (
	"testing"
	"time"

	"github.com/bluenviron/gortmplib/pkg/amf0"
	flv "github.com/yutopp/go-flv/tag"
)

func TestMetadataReplayedOnEveryJoin(t *testing.T) {
	ps := NewPubsub("live")
	defer ps.Close()
	metadata, err := NewMetadata(amf0.Data{"onMetaData", amf0.ECMAArray{{Key: "width", Value: float64(1920)}}})
	if err != nil {
		t.Fatal(err)
	}
	ps.Pub().Publish(&flv.FlvTag{TagType: flv.TagTypeScriptData, Data: metadata})
	for _, ts := range []uint32{1000, 2000} {
		events := make(chan *flv.FlvTag, 4)
		sub := ps.Subscribe(func(tag *flv.FlvTag) error { events <- tag; return nil })
		ps.Pub().Publish(video(ts, true))
		for i := range 2 {
			select {
			case tag := <-events:
				if tag.Timestamp != ts {
					t.Error("cached metadata has stale timestamp")
				}
				if _, ok := tag.Data.(*Metadata); (i == 0) != ok {
					t.Error("metadata must precede the join keyframe")
				}
			case <-time.After(time.Second):
				t.Error("missing metadata or keyframe on join")
			}
		}
		ps.Unsubscribe(sub)
		sub.Wait()
	}
}
