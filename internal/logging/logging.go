// Package logging provides compact console output and structured JSON logs.
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

func New(out io.Writer, format, level string) (*slog.Logger, error) {
	var minimum slog.Level
	switch strings.ToLower(level) {
	case "debug":
		minimum = slog.LevelDebug
	case "info":
		minimum = slog.LevelInfo
	case "warn", "warning":
		minimum = slog.LevelWarn
	case "error":
		minimum = slog.LevelError
	default:
		return nil, fmt.Errorf("log level must be debug, info, warn or error")
	}
	switch format {
	case "text":
		return slog.New(&consoleHandler{out: out, mu: &sync.Mutex{}, minimum: minimum}), nil
	case "json":
		return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: minimum})), nil
	default:
		return nil, fmt.Errorf("log format must be text or json")
	}
}

type boundAttr struct {
	prefix string
	attr   slog.Attr
}
type consoleHandler struct {
	out     io.Writer
	mu      *sync.Mutex
	minimum slog.Level
	prefix  string
	attrs   []boundAttr
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool { return level >= h.minimum }
func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append([]boundAttr(nil), h.attrs...)
	for _, attr := range attrs {
		next.attrs = append(next.attrs, boundAttr{h.prefix, attr})
	}
	return &next
}
func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.prefix += name + "."
	return &next
}
func (h *consoleHandler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	fmt.Fprintf(&line, "%s %-5s %s", record.Time.Local().Format("15:04:05"), record.Level.String(), strings.NewReplacer("\r", " ", "\n", " ").Replace(record.Message))
	for _, attr := range h.attrs {
		writeAttr(&line, attr.prefix, attr.attr)
	}
	record.Attrs(func(attr slog.Attr) bool { writeAttr(&line, h.prefix, attr); return true })
	line.WriteByte('\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, line.String())
	return err
}
func writeAttr(out *strings.Builder, prefix string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		if attr.Key != "" {
			prefix += attr.Key + "."
		}
		for _, child := range attr.Value.Group() {
			writeAttr(out, prefix, child)
		}
		return
	}
	out.WriteString("  ")
	out.WriteString(prefix)
	out.WriteString(attr.Key)
	out.WriteByte('=')
	value := attr.Value.String()
	if value == "" || strings.ContainsAny(value, " \t\r\n\"=") {
		out.WriteString(strconv.Quote(value))
	} else {
		out.WriteString(value)
	}
}

type Startup struct {
	Version, Platform, Dashboard, RTMP, Config, SetupURL string
	NewConfig                                            bool
	Enabled, Total                                       int
	WebStream                                            bool
	Restream                                             bool
}

func PrintStartup(out io.Writer, format string, s Startup) error {
	if format == "json" {
		return json.NewEncoder(out).Encode(struct {
			Time      string `json:"time"`
			Level     string `json:"level"`
			Message   string `json:"msg"`
			Version   string `json:"version"`
			Platform  string `json:"platform"`
			Dashboard string `json:"dashboard"`
			RTMP      string `json:"rtmp"`
			Config    string `json:"config"`
			SetupURL  string `json:"setup_url,omitempty"`
			Enabled   int    `json:"enabled_platforms"`
			Total     int    `json:"configured_platforms"`
			WebStream bool   `json:"web_stream"`
			Restream  bool   `json:"restream"`
		}{time.Now().UTC().Format(time.RFC3339), "INFO", "Multicaster ready", s.Version, s.Platform, s.Dashboard, s.RTMP, s.Config, s.SetupURL, s.Enabled, s.Total, s.WebStream, s.Restream})
	}
	var block strings.Builder
	fmt.Fprintf(&block, "\nMulticaster v%s · %s\n", strings.TrimPrefix(s.Version, "v"), s.Platform)
	fmt.Fprintf(&block, "  Dashboard  %s\n  RTMP       %s\n  Config     %s", s.Dashboard, s.RTMP, s.Config)
	if s.NewConfig {
		block.WriteString(" (created)")
	}
	block.WriteByte('\n')
	web := "off"
	if s.WebStream {
		web = "on"
	}
	restream := "off"
	if s.Restream {
		restream = "on"
	}
	fmt.Fprintf(&block, "  Outputs    %d/%d platforms enabled · restream %s · web stream %s\n", s.Enabled, s.Total, restream, web)
	if s.SetupURL != "" {
		fmt.Fprintf(&block, "  Set password  %s\n", s.SetupURL)
	}
	block.WriteString("\nReady — waiting for OBS. Press Ctrl+C to stop.\n\n")
	_, err := io.WriteString(out, block.String())
	return err
}

// PlatformEvents emits a warning/recovery pair for an outage, not every retry.
// Status and retry timing remain independent from this console-only throttling.
type PlatformEvents struct {
	mu                  sync.Mutex
	logger              *slog.Logger
	name                string
	now                 func() time.Time
	nextWarning         time.Time
	failures            int
	announced, seenLive bool
}

func NewPlatformEvents(logger *slog.Logger, name string) *PlatformEvents {
	if logger == nil {
		logger = slog.Default()
	}
	return &PlatformEvents{logger: logger, name: name, now: time.Now}
}
func (p *PlatformEvents) Unavailable(retry time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failures++
	now := p.now()
	if p.nextWarning.IsZero() || !now.Before(p.nextWarning) {
		p.logger.Warn("Platform unavailable; reconnecting", "platform", p.name, "retry_in", retry)
		p.nextWarning = now.Add(5 * time.Minute)
		p.announced = true
	} else {
		p.logger.Debug("Platform retry scheduled", "platform", p.name, "attempt", p.failures, "retry_in", retry)
	}
}
func (p *PlatformEvents) Live() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.announced {
		p.logger.Info("Platform reconnected", "platform", p.name, "attempts", p.failures)
	} else if !p.seenLive {
		p.logger.Info("Platform live", "platform", p.name)
	}
	p.seenLive = true
	p.announced = false
	p.failures = 0
}
