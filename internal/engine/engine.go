package engine

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/truefix/multicaster/internal/config"
	"github.com/truefix/multicaster/internal/hls"
	"github.com/truefix/multicaster/internal/relay/rtmp"
	"github.com/truefix/multicaster/internal/session"
	"github.com/truefix/multicaster/internal/storage"
	"github.com/truefix/multicaster/internal/stream"
)

// lifecycle serializes control changes; the media path never takes this lock.
type Engine struct {
	lifecycle                         sync.Mutex
	cfgMu                             sync.RWMutex
	cfg                               *config.Config
	cfgPath                           string
	HLS                               hls.Service
	Storage                           storage.Publisher
	storageActive, hlsActive, hlsPerf bool
	storageConfig                     config.Storage
	hub                               *stream.Hub
	tracker                           *session.Tracker
	logger                            *slog.Logger
	workers                           map[string]*rtmp.PushWorker
	runCtx                            context.Context
	runCancel                         context.CancelFunc
}

func New(c *config.Config, path string, l *slog.Logger) *Engine {
	if l == nil {
		l = slog.Default()
	}
	e := &Engine{cfg: c, cfgPath: path, hub: stream.NewHub(), tracker: session.NewTracker(), logger: l, workers: map[string]*rtmp.PushWorker{}}
	e.syncTrackerDestinations()
	return e
}
func (e *Engine) Config() *config.Config {
	e.cfgMu.RLock()
	defer e.cfgMu.RUnlock()
	c := *e.cfg
	c.Destinations = append([]config.Destination{}, c.Destinations...)
	return &c
}
func (e *Engine) ConfigPath() string        { return e.cfgPath }
func (e *Engine) Hub() *stream.Hub          { return e.hub }
func (e *Engine) Tracker() *session.Tracker { return e.tracker }
func (e *Engine) Snapshot(path string) session.Snapshot {
	c := e.Config()
	snap := e.tracker.Snapshot(c.Settings.RestreamEnabled, c.Settings.WebStreamEnabled, c.Settings.PerformanceMode, path)
	snap.PreviewEnabled = c.Settings.PreviewEnabled
	return snap
}
func (e *Engine) ValidStreamKey(key string) bool {
	c := e.Config()
	return key != "" && subtle.ConstantTimeCompare([]byte(key), []byte(c.Auth.StreamKey)) == 1
}
func (e *Engine) OnPublishStart(name string) (*stream.Pubsub, error) {
	e.lifecycle.Lock()
	defer e.lifecycle.Unlock()
	ps, err := e.hub.Start(name)
	if err != nil {
		return nil, err
	}
	e.runCtx, e.runCancel = context.WithCancel(context.Background())
	e.tracker.SetIngestLive(true)
	e.syncOutputs()
	return ps, nil
}
func (e *Engine) OnPublishStop() {
	e.lifecycle.Lock()
	defer e.lifecycle.Unlock()
	if e.runCancel != nil {
		e.runCancel()
		e.runCancel = nil
	}
	e.hub.Stop()
	e.tracker.SetIngestLive(false)
	e.stopAllWorkers()
	e.Storage.Stop()
	e.storageActive = false
	e.HLS.Stop()
	e.hlsActive = false
}
func (e *Engine) OnMediaBytes(n int) { e.tracker.AddBytes(n) }

// Save before applying: disk failure must never alter a running stream.
func (e *Engine) mutate(reconcile bool, change func(*config.Config) error) error {
	e.lifecycle.Lock()
	defer e.lifecycle.Unlock()
	next := e.Config()
	if err := change(next); err != nil {
		return err
	}
	if e.cfgPath != "" {
		if err := next.Save(e.cfgPath); err != nil {
			return fmt.Errorf("save configuration: %w", err)
		}
	}
	e.cfgMu.Lock()
	e.cfg = next
	e.cfgMu.Unlock()
	if reconcile {
		e.syncTrackerDestinations()
		e.syncOutputs()
	}
	return nil
}
func (e *Engine) UpdateAuth(a config.AuthConfig) error {
	return e.mutate(false, func(c *config.Config) error { c.Auth = a; return nil })
}
func (e *Engine) PatchSettings(p config.SettingsPatch) (config.Settings, error) {
	var result config.Settings
	err := e.mutate(true, func(c *config.Config) error {
		s := &c.Settings
		if p.UpdateManifestURL != nil {
			s.UpdateManifestURL = *p.UpdateManifestURL
		}
		if p.RestreamEnabled != nil {
			s.RestreamEnabled = *p.RestreamEnabled
		}
		if p.WebStreamEnabled != nil {
			s.WebStreamEnabled = *p.WebStreamEnabled
		}
		if p.PerformanceMode != nil {
			s.PerformanceMode = *p.PerformanceMode
		}
		if p.UpdateRepository != nil {
			s.UpdateRepository = *p.UpdateRepository
		}
		if p.PreviewEnabled != nil {
			s.PreviewEnabled = *p.PreviewEnabled
		}
		result = *s
		return nil
	})
	return result, err
}
func (e *Engine) UpdateSettings(s config.Settings) error {
	return e.mutate(true, func(c *config.Config) error { c.Settings = s; return nil })
}
func (e *Engine) AddDestination(d config.Destination) (config.Destination, error) {
	if err := validateDestination(d); err != nil {
		return d, err
	}
	d.ID = config.NewDestID(0)
	d.Type = "rtmp_push"
	err := e.mutate(true, func(c *config.Config) error {
		if len(c.Destinations) >= 32 {
			return fmt.Errorf("at most 32 platforms are supported per instance")
		}
		c.Destinations = append(c.Destinations, d)
		return nil
	})
	return d, err
}
func (e *Engine) PatchDestination(id string, enabled *bool, name, url *string) error {
	return e.mutate(true, func(c *config.Config) error {
		for i := range c.Destinations {
			d := &c.Destinations[i]
			if d.ID != id {
				continue
			}
			if enabled != nil {
				d.Enabled = *enabled
			}
			if name != nil {
				d.Name = *name
			}
			if url != nil {
				d.URL = *url
			}
			return validateDestination(*d)
		}
		return ErrNotFound
	})
}
func (e *Engine) DeleteDestination(id string) error {
	return e.mutate(true, func(c *config.Config) error {
		for i, d := range c.Destinations {
			if d.ID == id {
				c.Destinations = append(c.Destinations[:i], c.Destinations[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}
func (e *Engine) UpdateStorage(s config.Storage) error {
	return e.mutate(true, func(c *config.Config) error {
		if s.SecretKey == "" {
			s.SecretKey = c.Storage.SecretKey
		}
		if err := storage.Validate(s); err != nil {
			return err
		}
		c.Storage = s
		return nil
	})
}
func validateDestination(d config.Destination) error {
	if strings.TrimSpace(d.Name) == "" || len(d.Name) > 320 {
		return fmt.Errorf("platform name is required and must be at most 320 bytes")
	}
	if d.Type != "" && d.Type != "rtmp_push" {
		return fmt.Errorf("only RTMP/RTMPS outputs are supported")
	}
	_, err := rtmp.ParseURL(d.URL)
	return err
}

var ErrNotFound = errors.New("destination not found")

func (e *Engine) syncTrackerDestinations() {
	c := e.Config()
	dests := make([]session.DestinationRuntime, 0, len(c.Destinations))
	for _, d := range c.Destinations {
		dests = append(dests, session.DestinationRuntime{ID: d.ID, Name: d.Name, Type: d.Type, Enabled: d.Enabled, Status: session.DestOff})
	}
	e.tracker.SyncDestinations(dests)
}
func (e *Engine) syncOutputs() {
	c := e.Config()
	s := c.Settings
	if !e.hub.Active() {
		e.stopAllWorkers()
		return
	}
	want := map[string]config.Destination{}
	if s.RestreamEnabled {
		for _, d := range c.Destinations {
			if d.Enabled && d.Type == "rtmp_push" {
				want[d.ID] = d
			}
		}
	}
	for id, w := range e.workers {
		d, ok := want[id]
		if !ok || w.URL != d.URL {
			w.Stop()
			delete(e.workers, id)
		}
	}
	for id, d := range want {
		if _, ok := e.workers[id]; !ok {
			w := rtmp.NewPushWorker(d.ID, d.Name, d.URL, e.hub, e.tracker, e.logger, s.PerformanceMode)
			e.workers[id] = w
			w.Start(e.runCtx)
		}
	}
	if s.WebStreamEnabled {
		if !e.hlsActive || e.hlsPerf != s.PerformanceMode {
			e.Storage.Stop()
			e.storageActive = false
			e.HLS.Start(e.hub.Pubsub(), s.PerformanceMode)
			e.hlsActive = true
			e.hlsPerf = s.PerformanceMode
		}
	} else if e.hlsActive {
		e.Storage.Stop()
		e.storageActive = false
		e.HLS.Stop()
		e.hlsActive = false
	}
	if s.WebStreamEnabled && c.Storage.Enabled {
		if err := storage.Validate(c.Storage); err == nil {
			if !e.storageActive || e.storageConfig != c.Storage {
				e.Storage.Start(c.Storage, e.HLS.ReadResource)
				e.storageActive = true
				e.storageConfig = c.Storage
			}
		}
	} else if e.storageActive {
		e.Storage.Stop()
		e.storageActive = false
	}
}
func (e *Engine) stopAllWorkers() {
	for id, w := range e.workers {
		w.Stop()
		delete(e.workers, id)
	}
}
