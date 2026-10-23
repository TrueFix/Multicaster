package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConsoleOutputIsCompactAndEscapesValues(t *testing.T) {
	var out bytes.Buffer
	l, err := New(&out, "text", "info")
	if err != nil {
		t.Fatal(err)
	}
	l.Debug("hidden debug")
	l.Info("OBS connected", "platform", "My channel\nforged")
	text := out.String()
	if strings.Contains(text, "time=") || strings.Contains(text, "level=") || strings.Contains(text, "msg=") || strings.Contains(text, "hidden debug") {
		t.Fatalf("noisy output: %s", text)
	}
	if strings.Count(text, "\n") != 1 || !strings.Contains(text, `platform="My channel\nforged"`) {
		t.Fatalf("unsafe or invalid field formatting: %q", text)
	}
}
func TestStartupSummaryPrecedesRuntimeEvents(t *testing.T) {
	var out bytes.Buffer
	err := PrintStartup(&out, "text", Startup{Version: "0.1.0", Platform: "linux/amd64", Dashboard: "http://localhost:8080/app/", RTMP: "rtmp://localhost:1935/live", Config: "/config/config.yaml", Enabled: 3, Total: 4})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := New(&out, "text", "info")
	l.Info("OBS connected")
	text := out.String()
	if !strings.Contains(text, "Multicaster v0.1.0") || !strings.Contains(text, "3/4 platforms enabled") || strings.Contains(text, "Set password") || strings.Index(text, "Ready — waiting for OBS") > strings.Index(text, "OBS connected") {
		t.Fatalf("unexpected startup summary: %s", text)
	}
}
func TestJSONStartupAndRuntimeRemainMachineReadable(t *testing.T) {
	var out bytes.Buffer
	PrintStartup(&out, "json", Startup{Version: "0.1.0"})
	l, _ := New(&out, "json", "info")
	l.Warn("Platform unavailable", "platform", "Example")
	decoder := json.NewDecoder(&out)
	for range 2 {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record["msg"] == nil || record["level"] == nil {
			t.Fatal("invalid JSON log schema")
		}
	}
}
func TestRepeatedRetriesOnlyLogOneWarningAndRecovery(t *testing.T) {
	var out bytes.Buffer
	l, _ := New(&out, "text", "info")
	events := NewPlatformEvents(l, "Example")
	now := time.Unix(100, 0)
	events.now = func() time.Time { return now }
	for range 30 {
		events.Unavailable(time.Second)
		now = now.Add(time.Second)
	}
	events.Live()
	for range 10 {
		events.Unavailable(time.Second)
		events.Live()
	}
	if strings.Count(out.String(), "WARN") != 1 || strings.Count(out.String(), "Platform reconnected") != 1 {
		t.Fatalf("retries flooded logs: %s", out.String())
	}
	now = now.Add(5 * time.Minute)
	events.Unavailable(time.Second)
	if strings.Count(out.String(), "WARN") != 2 {
		t.Fatal("persistent outage reminder was lost")
	}
}
func TestConcurrentConsoleRecordsDoNotInterleave(t *testing.T) {
	var out bytes.Buffer
	l, _ := New(&out, "text", "info")
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() { defer wg.Done(); l.With("worker", i).WithGroup("output").Info("Event", "ready", true) }()
	}
	wg.Wait()
	if strings.Count(out.String(), "\n") != 40 {
		t.Fatal("log records interleaved")
	}
	if !strings.Contains(out.String(), "output.ready=true") {
		t.Fatal("grouped attributes lost")
	}
}

type blockedOutput struct {
	entered, release chan struct{}
	once             sync.Once
}

func (b *blockedOutput) Write(p []byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return len(p), nil
}
func TestBlockedConsoleCannotBlockMediaLogging(t *testing.T) {
	out := &blockedOutput{entered: make(chan struct{}), release: make(chan struct{})}
	writer := NewBufferedWriter(out)
	defer func() { close(out.release); writer.Close() }()
	writer.Write([]byte("first\n"))
	<-out.entered
	finished := make(chan struct{})
	go func() {
		for range 10000 {
			writer.Write([]byte("another event\n"))
		}
		writer.Close()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("blocked output stalled callers")
	}
	writer.mu.Lock()
	pending := len(writer.queue)
	writer.mu.Unlock()
	if pending > 64 {
		t.Fatal("log queue is unbounded")
	}
}
