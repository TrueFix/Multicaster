package storage

import (
	"context"
	"fmt"
	"github.com/truefix/multicaster/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUploadsMediaBeforeDependentPlaylists(t *testing.T) {
	var mu sync.Mutex
	var uploaded []string
	complete := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("upload was not signed")
		}
		mu.Lock()
		uploaded = append(uploaded, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(200)
		if strings.HasSuffix(r.URL.Path, "/index.m3u8") {
			once.Do(func() { close(complete) })
		}
	}))
	defer server.Close()
	source := map[string]string{"index.m3u8": "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=100000\nmain.m3u8\n", "main.m3u8": "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:2.0,\nsegment.mp4\n", "init.mp4": "init", "segment.mp4": "segment"}
	var publisher Publisher
	publisher.Start(config.Storage{Endpoint: server.URL, PublicURL: server.URL + "/bucket", Region: "auto", Bucket: "bucket", AccessKey: "test", SecretKey: "test"}, func(_ context.Context, name string) ([]byte, error) {
		s, ok := source[name]
		if !ok {
			return nil, fmt.Errorf("unexpected request: %s", name)
		}
		return []byte(s), nil
	})
	defer publisher.Stop()
	select {
	case <-complete:
	case <-time.After(3 * time.Second):
		t.Fatal("upload timed out")
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"/init.mp4", "/segment.mp4", "/main.m3u8", "/index.m3u8"}
	if len(uploaded) != len(want) {
		t.Fatalf("unexpected uploads: %v", uploaded)
	}
	for i, suffix := range want {
		if !strings.HasSuffix(uploaded[i], suffix) {
			t.Fatalf("unsafe publication order: %v", uploaded)
		}
	}
}

func TestPlaylistCannotReferenceExternalResources(t *testing.T) {
	if _, err := dependencies("index.m3u8", []byte("#EXTM3U\nhttps://other.example/stream.m3u8")); err == nil {
		t.Fatal("external playlist URI accepted")
	}
}
