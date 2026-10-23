package rtmpingest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/bluenviron/gortmplib/pkg/amf0"
	msg "github.com/bluenviron/gortmplib/pkg/message"
	"github.com/truefix/multicaster/internal/stream"
)

func TestMetadataIngest(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		payload := amf0.Data{"onMetaData", amf0.ECMAArray{{Key: "width", Value: float64(1920)}}}
		if wrapped {
			payload = append(amf0.Data{"@setDataFrame"}, payload...)
		}
		tag, err := mediaTag(&msg.DataAMF0{Payload: payload})
		if err != nil || tag == nil {
			t.Fatalf("metadata rejected: %v", err)
		}
		metadata, ok := tag.Data.(*stream.Metadata)
		if !ok {
			t.Fatalf("wrong metadata type %T", tag.Data)
		}
		got, err := metadata.Payload()
		if err != nil || !reflect.DeepEqual(got, payload) {
			t.Fatalf("metadata altered: %v, %v", got, err)
		}
		// Decoded graphs must not expose the immutable reconnect cache.
		got[0] = "modified"
		again, _ := metadata.Payload()
		if !reflect.DeepEqual(again, payload) {
			t.Fatal("metadata cache was mutated")
		}
	}
	for _, payload := range []amf0.Data{nil, {"onCuePoint", amf0.Object{}}, {"@setDataFrame"}} {
		tag, err := mediaTag(&msg.DataAMF0{Payload: payload})
		if err != nil || tag != nil {
			t.Fatalf("unexpected non-metadata result: %v %v", tag, err)
		}
	}
	if _, err := mediaTag(&msg.DataAMF0{Payload: amf0.Data{"onMetaData", strings.Repeat("x", 1024*1024)}}); err == nil {
		t.Fatal("oversized metadata accepted")
	}
}
