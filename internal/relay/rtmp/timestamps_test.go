package rtmp

import "testing"

func TestTimestampTimeline(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input []uint32
		want  []uint32
		keep  []bool
	}{
		{"join and interleaving", []uint32{1000, 990, 1020, 1010, 1040}, []uint32{0, 0, 20, 10, 40}, []bool{true, false, true, true, true}},
		{"input rollover", []uint32{0xfffffff0, 0x10, 0x08, 0x20}, []uint32{0, 32, 24, 48}, []bool{true, true, true, true}},
		{"long running and output rollover", []uint32{0, 0x70000000, 0xe0000000, 0x50000000}, []uint32{0, 0x70000000, 0xe0000000, 0x50000000}, []bool{true, true, true, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var timeline timestampTimeline
			for i, timestamp := range tc.input {
				got, keep := timeline.normalize(timestamp)
				if got != tc.want[i] || keep != tc.keep[i] {
					t.Fatalf("packet %d: got (%d,%t), want (%d,%t)", i, got, keep, tc.want[i], tc.keep[i])
				}
			}
		})
	}
}
