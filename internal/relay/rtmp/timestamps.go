package rtmp

// A shared audio/video timeline uses serial-number arithmetic for 32-bit RTMP
// timestamps. Packets before the join keyframe are dropped, rather than wrapping
// negative startup offsets into a timestamp almost 50 days in the future.
type timestampTimeline struct {
	initialized bool
	last        uint32
	elapsed     int64
}

func (t *timestampTimeline) normalize(timestamp uint32) (uint32, bool) {
	if !t.initialized {
		t.initialized, t.last = true, timestamp
		return 0, true
	}
	delta := int64(int32(timestamp - t.last))
	value := t.elapsed + delta
	if value < 0 {
		return 0, false
	}
	if delta > 0 {
		t.last, t.elapsed = timestamp, value
	}
	return uint32(value), true
}
