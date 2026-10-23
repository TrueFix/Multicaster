package logging

import (
	"bytes"
	"io"
	"sync"
	"time"
)

// BufferedWriter keeps console backpressure out of the media path. It has one
// writer goroutine, 64 records, and a 64 KiB per-record limit. A blocked console
// drops excess records instead of retaining unbounded memory or stalling video.
type BufferedWriter struct {
	mu     sync.Mutex
	out    io.Writer
	queue  chan []byte
	done   chan struct{}
	closed bool
}

func NewBufferedWriter(out io.Writer) *BufferedWriter {
	w := &BufferedWriter{out: out, queue: make(chan []byte, 64), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		for record := range w.queue {
			_, _ = out.Write(record)
		}
	}()
	return w
}
func (w *BufferedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed && len(p) <= 64<<10 && len(w.queue) < cap(w.queue) {
		w.queue <- bytes.Clone(p)
	}
	return len(p), nil
}
func (w *BufferedWriter) Close() {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.queue)
	}
	w.mu.Unlock()
	select {
	case <-w.done:
	case <-time.After(200 * time.Millisecond):
	}
}
