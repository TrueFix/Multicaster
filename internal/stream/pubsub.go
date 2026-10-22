package stream

import (
	"bytes"
	"errors"
	flvtag "github.com/yutopp/go-flv/tag"
	"sync"
	"sync/atomic"
)

var ErrAlreadyLive = errors.New("a stream is already being published")

type Hub struct {
	mu     sync.RWMutex
	pubsub *Pubsub
}

func NewHub() *Hub { return &Hub{} }
func (h *Hub) Start(name string) (*Pubsub, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pubsub != nil {
		return nil, ErrAlreadyLive
	}
	h.pubsub = NewPubsub(name)
	return h.pubsub, nil
}
func (h *Hub) Active() bool    { return h.Pubsub() != nil }
func (h *Hub) Pubsub() *Pubsub { h.mu.RLock(); defer h.mu.RUnlock(); return h.pubsub }
func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pubsub != nil {
		h.pubsub.Close()
		h.pubsub = nil
	}
}

// Each subscriber has a bounded queue. A blocked network output never blocks ingest.
// Overflow disconnects that consumer; it must rejoin on a fresh keyframe.
type Pubsub struct {
	mu                 sync.Mutex
	name               string
	closed             bool
	subs               map[*Sub]bool
	video, audio, meta *flvtag.FlvTag
}
type Pub struct{ pb *Pubsub }
type Sub struct {
	queue       chan *flvtag.FlvTag
	done        chan struct{}
	once        sync.Once
	ready       bool
	exited      chan struct{}
	queuedBytes atomic.Int64
	errMu       sync.RWMutex
	err         error
}

func NewPubsub(name string) *Pubsub { return &Pubsub{name: name, subs: map[*Sub]bool{}} }
func (p *Pubsub) Name() string      { return p.name }
func (p *Pubsub) Pub() *Pub         { return &Pub{pb: p} }
func (p *Pubsub) Subscribe(cb func(*flvtag.FlvTag) error) *Sub {
	s := &Sub{queue: make(chan *flvtag.FlvTag, 256), done: make(chan struct{}), exited: make(chan struct{})}
	p.mu.Lock()
	if p.closed {
		s.Close()
	} else {
		p.subs[s] = true
	}
	p.mu.Unlock()
	go func() {
		defer close(s.exited)
		defer s.Close()
		defer func() {
			if recover() != nil {
				s.setError(errors.New("media consumer panicked; output will restart"))
			}
			p.mu.Lock()
			delete(p.subs, s)
			p.mu.Unlock()
			for {
				select {
				case <-s.queue:
				default:
					s.queuedBytes.Store(0)
					return
				}
			}
		}()
		for {
			select {
			case <-s.done:
				return
			default:
			}
			select {
			case <-s.done:
				return
			case tag := <-s.queue:
				size := tagSize(tag)
				if err := cb(tag); err != nil {
					s.setError(err)
					return
				}
				s.queuedBytes.Add(-size)
			}
		}
	}()
	return s
}
func (s *Sub) Done() <-chan struct{} { return s.done }
func (s *Sub) Wait()                 { <-s.exited }
func (s *Sub) Err() error            { s.errMu.RLock(); defer s.errMu.RUnlock(); return s.err }
func (s *Sub) setError(err error)    { s.errMu.Lock(); s.err = err; s.errMu.Unlock() }
func (s *Sub) Close() error          { s.once.Do(func() { close(s.done) }); return nil }
func (p *Pubsub) Unsubscribe(s *Sub) { p.mu.Lock(); delete(p.subs, s); p.mu.Unlock(); s.Close() }
func (p *Pubsub) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for s := range p.subs {
		s.Close()
	}
	p.subs = map[*Sub]bool{}
	p.video = nil
	p.audio = nil
	p.meta = nil
	return nil
}
func (p *Pub) Publish(tag *flvtag.FlvTag) error {
	b := p.pb
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	key := false
	header := false
	switch d := tag.Data.(type) {
	case *flvtag.VideoData:
		header = d.AVCPacketType == flvtag.AVCPacketTypeSequenceHeader
		if header {
			b.video = tag
		}
		key = d.FrameType == flvtag.FrameTypeKeyFrame && !header
	case *flvtag.AudioData:
		if d.AACPacketType == flvtag.AACPacketTypeSequenceHeader {
			b.audio = tag
			header = true
		}
	case *flvtag.ScriptData:
		b.meta = tag
		header = true
	}
	for s := range b.subs {
		select {
		case <-s.done:
			delete(b.subs, s)
			continue
		default:
		}
		send := func(t *flvtag.FlvTag) {
			if t == nil {
				return
			}
			if s.queuedBytes.Add(tagSize(t)) > 8*1024*1024 {
				s.setError(errors.New("media consumer exceeded the 8 MiB queue limit"))
				s.Close()
				return
			}
			select {
			case s.queue <- CloneTag(t):
			default:
				s.setError(errors.New("media consumer queue overflow"))
				s.Close()
			}
		}
		if !s.ready {
			if !key {
				continue
			}
			for _, t := range []*flvtag.FlvTag{b.meta, b.video, b.audio} {
				if t != nil {
					c := CloneTag(t)
					c.Timestamp = tag.Timestamp
					send(c)
				}
			}
			s.ready = true
		}
		if !header || s.ready {
			send(tag)
		}
	}
	return nil
}

func tagSize(t *flvtag.FlvTag) int64 {
	switch d := t.Data.(type) {
	case *flvtag.VideoData:
		if b, ok := d.Data.(*bytes.Buffer); ok {
			return int64(b.Len())
		}
	case *flvtag.AudioData:
		if b, ok := d.Data.(*bytes.Buffer); ok {
			return int64(b.Len())
		}
	}
	return 0
}

// Payload bytes are immutable; clone only reader cursors rather than video buffers.
func CloneTag(t *flvtag.FlvTag) *flvtag.FlvTag {
	v := *t
	switch d := t.Data.(type) {
	case *flvtag.AudioData:
		c := *d
		if b, ok := d.Data.(*bytes.Buffer); ok {
			c.Data = bytes.NewBuffer(b.Bytes())
		}
		v.Data = &c
	case *flvtag.VideoData:
		c := *d
		if b, ok := d.Data.(*bytes.Buffer); ok {
			c.Data = bytes.NewBuffer(b.Bytes())
		}
		v.Data = &c
	case *flvtag.ScriptData:
		c := *d
		v.Data = &c
	}
	return &v
}
