package session

import (
	"sort"
	"sync"
	"time"
)

type IngestState string

const (
	IngestOffline IngestState = "offline"
	IngestLive    IngestState = "live"
)

type DestStatus string

const (
	DestOff        DestStatus = "off"
	DestConnecting DestStatus = "connecting"
	DestLive       DestStatus = "live"
	DestError      DestStatus = "error"
)

type WarningCode string

const (
	WarningNone             WarningCode = ""
	WarningIngestOnly       WarningCode = "ingest_only"
	WarningNoDestinations   WarningCode = "no_destinations_enabled"
	WarningStreamingNowhere WarningCode = "streaming_nowhere"
)

type DestinationRuntime struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Enabled   bool       `json:"enabled"`
	Status    DestStatus `json:"status"`
	LastError string     `json:"last_error,omitempty"`
}

type Snapshot struct {
	PreviewEnabled   bool                 `json:"preview_enabled"`
	CDNStatus        string               `json:"cdn_status"`
	CDNError         string               `json:"cdn_error,omitempty"`
	CDNURL           string               `json:"cdn_url,omitempty"`
	Ingest           IngestState          `json:"ingest"`
	IngestSince      *time.Time           `json:"ingest_since,omitempty"`
	BytesIn          uint64               `json:"bytes_in"`
	BitrateKbps      float64              `json:"bitrate_kbps"`
	RestreamEnabled  bool                 `json:"restream_enabled"`
	WebStreamEnabled bool                 `json:"web_stream_enabled"`
	PerformanceMode  bool                 `json:"performance_mode"`
	Warning          WarningCode          `json:"warning"`
	WarningMessage   string               `json:"warning_message,omitempty"`
	Destinations     []DestinationRuntime `json:"destinations"`
	HLSPlaylistURL   string               `json:"hls_playlist_url,omitempty"`
	HLSStatus        string               `json:"hls_status"`
	HLSError         string               `json:"hls_error,omitempty"`
}

type Tracker struct {
	mu sync.RWMutex

	ingest      IngestState
	ingestSince time.Time
	bytesIn     uint64
	lastBytes   uint64
	lastSample  time.Time
	bitrateKbps float64

	dest map[string]*DestinationRuntime
}

func NewTracker() *Tracker {
	return &Tracker{
		ingest: IngestOffline,
		dest:   make(map[string]*DestinationRuntime),
	}
}

func (t *Tracker) SetIngestLive(live bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if live && t.ingest != IngestLive {
		t.ingest = IngestLive
		now := time.Now()
		t.ingestSince = now
		t.lastSample = now
		t.lastBytes = t.bytesIn
	} else if !live {
		t.ingest = IngestOffline
		t.ingestSince = time.Time{}
		t.bitrateKbps = 0
	}
}

func (t *Tracker) AddBytes(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bytesIn += uint64(n)
	now := time.Now()
	if !t.lastSample.IsZero() && now.Sub(t.lastSample) >= time.Second {
		delta := float64(t.bytesIn - t.lastBytes)
		sec := now.Sub(t.lastSample).Seconds()
		if sec > 0 {
			t.bitrateKbps = (delta * 8) / sec / 1000
		}
		t.lastSample = now
		t.lastBytes = t.bytesIn
	}
}

func (t *Tracker) SyncDestinations(ids []DestinationRuntime) {
	t.mu.Lock()
	defer t.mu.Unlock()
	next := make(map[string]*DestinationRuntime, len(ids))
	for _, d := range ids {
		cp := d
		if old, ok := t.dest[d.ID]; ok {
			cp.Status = old.Status
			cp.LastError = old.LastError
		}
		next[d.ID] = &cp
	}
	t.dest = next
}

func (t *Tracker) SetDestStatus(id string, status DestStatus, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	d, ok := t.dest[id]
	if !ok {
		return
	}
	d.Status = status
	d.LastError = errMsg
}

func (t *Tracker) Snapshot(
	restreamEnabled, webStreamEnabled, performanceMode bool,
	hlsPath string,
) Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	dests := make([]DestinationRuntime, 0, len(t.dest))
	var enabledCount int
	var liveCount int
	for _, d := range t.dest {
		dests = append(dests, *d)
		if d.Enabled {
			enabledCount++
			if d.Status == DestLive {
				liveCount++
			}
		}
	}
	sort.Slice(dests, func(i, j int) bool { return dests[i].Name < dests[j].Name })

	warn, msg := computeWarning(t.ingest, restreamEnabled, webStreamEnabled, enabledCount, liveCount)

	var since *time.Time
	if t.ingest == IngestLive && !t.ingestSince.IsZero() {
		s := t.ingestSince
		since = &s
	}

	snap := Snapshot{
		Ingest:           t.ingest,
		IngestSince:      since,
		BytesIn:          t.bytesIn,
		BitrateKbps:      t.bitrateKbps,
		RestreamEnabled:  restreamEnabled,
		WebStreamEnabled: webStreamEnabled,
		PerformanceMode:  performanceMode,
		Warning:          warn,
		WarningMessage:   msg,
		Destinations:     dests,
	}
	if webStreamEnabled && hlsPath != "" {
		snap.HLSPlaylistURL = hlsPath
	}
	return snap
}

func computeWarning(
	ingest IngestState,
	restream, webStream bool,
	enabledCount, liveCount int,
) (WarningCode, string) {
	if ingest != IngestLive {
		return WarningNone, ""
	}
	if !restream && !webStream {
		return WarningIngestOnly, "Live input detected but restream and web stream are both off."
	}
	if restream && enabledCount == 0 {
		return WarningNoDestinations, "Restream is on but no platforms are enabled."
	}
	if restream && enabledCount > 0 && liveCount == 0 {
		return WarningStreamingNowhere, "Restream is on but no destination is receiving video yet."
	}
	return WarningNone, ""
}
