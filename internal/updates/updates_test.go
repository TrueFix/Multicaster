package updates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
)

func fixture() Manifest {
	return Manifest{Version: "0.2.0", ReleaseURL: "https://github.com/TrueFix/Multicaster/releases/tag/v0.2.0", Assets: map[string]Asset{runtime.GOOS + "-" + runtime.GOARCH: {URL: "multicaster-test", SHA256: strings.Repeat("a", 64), Size: 1024}}}
}
func TestManifestCheckUsesPlatformAsset(t *testing.T) {
	previous:=Version;Version="0.1.0";t.Cleanup(func(){Version=previous})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(fixture()) }))
	defer server.Close()
	info, err := Check(context.Background(), "", server.URL+"/update-manifest.json", true)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Latest != "0.2.0" || info.DownloadURL != server.URL+"/multicaster-test" || len(info.SHA256) != 64 {
		t.Fatalf("incorrect release info: %+v", info)
	}
}
func TestOfficialFeedConfiguredWithoutNetworkRequest(t *testing.T) {
	info, err := Check(context.Background(), "", DefaultManifestURL, false)
	if err != nil || info.ManifestURL != DefaultManifestURL {
		t.Fatalf("default feed: %+v %v", info, err)
	}
	disabled, err := Check(context.Background(), "", "", true)
	if err != nil || disabled.CheckedAt != nil || disabled.Available {
		t.Fatal("disabled update checks performed work")
	}
}
func TestInvalidManifestNeverOffersDownload(t *testing.T) {
	base, _ := url.Parse("https://example.com/release/update-manifest.json")
	platform := runtime.GOOS + "-" + runtime.GOARCH
	for _, change := range []func(*Manifest){func(m *Manifest) { m.Version = "not-a-version" }, func(m *Manifest) { a := m.Assets[platform]; a.SHA256 = "bad"; m.Assets[platform] = a }, func(m *Manifest) {
		a := m.Assets[platform]
		a.URL = "http://not-secure.example/binary"
		m.Assets[platform] = a
	}, func(m *Manifest) { a := m.Assets[platform]; a.Size = 129 << 20; m.Assets[platform] = a }} {
		m := fixture()
		change(&m)
		body, _ := json.Marshal(m)
		if _, _, err := DecodeManifest(strings.NewReader(string(body)), base, platform); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}
