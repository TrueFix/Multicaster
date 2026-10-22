// Package updates reads versioned release manifests. Checking or downloading a
// release never replaces the running binary or interrupts live streams.
package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// VERSION, frontend/package.json and this fallback are checked before tagging.
// Release builds stamp all four variables through linker flags.
var Version = "0.1.0"
var Commit = "development"
var BuildDate = "unknown"
var DefaultManifestURL = "https://github.com/TrueFix/Multicaster/releases/latest/download/update-manifest.json"

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func ValidRepository(s string) bool { return s == "" || repositoryPattern.MatchString(s) }
func ValidateURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("enter a valid HTTPS update-manifest URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())) {
		return nil
	}
	return fmt.Errorf("release manifests and downloads require HTTPS")
}

// Manifest is compatible with the release feed used by TrueFix/Localert.
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Manifest struct {
	Version     string           `json:"version"`
	Notes       string           `json:"notes,omitempty"`
	PublishedAt string           `json:"published_at,omitempty"`
	Commit      string           `json:"commit,omitempty"`
	ReleaseURL  string           `json:"release_url,omitempty"`
	Assets      map[string]Asset `json:"assets"`
}
type Info struct {
	Version     string     `json:"version"`
	Commit      string     `json:"commit"`
	BuildDate   string     `json:"build_date"`
	Platform    string     `json:"platform"`
	Repository  string     `json:"repository"`
	ManifestURL string     `json:"manifest_url"`
	Latest      string     `json:"latest,omitempty"`
	Available   bool       `json:"available"`
	URL         string     `json:"url,omitempty"`
	DownloadURL string     `json:"download_url,omitempty"`
	SHA256      string     `json:"sha256,omitempty"`
	Size        int64      `json:"size,omitempty"`
	Notes       string     `json:"notes,omitempty"`
	Message     string     `json:"message"`
	CheckedAt   *time.Time `json:"checked_at,omitempty"`
}

func Check(ctx context.Context, repo, feed string, check bool) (Info, error) {
	i := Info{Version: Version, Commit: Commit, BuildDate: BuildDate, Platform: runtime.GOOS + "/" + runtime.GOARCH, Repository: repo, Message: "The official release feed is ready. Check for updates when you're ready."}
	if !ValidRepository(repo) {
		return i, fmt.Errorf("repository must be owner/name")
	}
	if repo != "" {
		feed = "https://github.com/" + repo + "/releases/latest/download/update-manifest.json"
		i.URL = "https://github.com/" + repo + "/releases"
	} else {
		i.URL = "https://github.com/TrueFix/Multicaster/releases"
	}
	i.ManifestURL = feed
	if feed == "" {
		i.Message = "Update checks are disabled. Add a manifest URL or repository to enable them."
		return i, nil
	}
	if err := ValidateURL(feed); err != nil {
		return i, err
	}
	if !check {
		return i, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", feed, nil)
	if err != nil {
		return i, fmt.Errorf("invalid manifest URL")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Multicaster/"+Version)
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many release redirects")
		}
		return ValidateURL(r.URL.String())
	}}
	res, err := client.Do(req)
	if err != nil {
		return i, fmt.Errorf("could not reach the update feed; try again later")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return i, fmt.Errorf("update feed returned HTTP %d; check that a release has been published", res.StatusCode)
	}
	manifest, asset, err := DecodeManifest(io.LimitReader(res.Body, (256<<10)+1), res.Request.URL, runtime.GOOS+"-"+runtime.GOARCH)
	if err != nil {
		return i, err
	}
	now := time.Now().UTC()
	i.CheckedAt = &now
	i.Latest = strings.TrimPrefix(manifest.Version, "v")
	i.Notes = manifest.Notes
	i.DownloadURL = asset.URL
	i.SHA256 = asset.SHA256
	i.Size = asset.Size
	if manifest.ReleaseURL != "" {
		if err := ValidateURL(manifest.ReleaseURL); err != nil {
			return i, err
		}
		i.URL = manifest.ReleaseURL
	}
	current := "v" + strings.TrimPrefix(Version, "v")
	latest := "v" + i.Latest
	i.Available = semver.IsValid(current) && semver.Compare(latest, current) > 0
	switch {
	case !semver.IsValid(current):
		i.Message = "Development build. Latest published release: " + i.Latest
	case i.Available:
		i.Message = "A new release is available. Download it when your stream has finished."
	case semver.Compare(latest, current) < 0:
		i.Message = "Your installed version is newer than this feed's release."
	default:
		i.Message = "You're up to date."
	}
	return i, nil
}
func DecodeManifest(reader io.Reader, base *url.URL, platform string) (Manifest, Asset, error) {
	var m Manifest
	body, err := io.ReadAll(io.LimitReader(reader, (256<<10)+1))
	if err != nil {
		return m, Asset{}, fmt.Errorf("could not read the release manifest")
	}
	if len(body) > 256<<10 {
		return m, Asset{}, fmt.Errorf("release manifest is too large")
	}
	if json.Unmarshal(body, &m) != nil {
		return m, Asset{}, fmt.Errorf("invalid release manifest")
	}
	if !semver.IsValid("v" + strings.TrimPrefix(m.Version, "v")) {
		return m, Asset{}, fmt.Errorf("manifest version is not a valid semantic version")
	}
	a, ok := m.Assets[platform]
	if !ok {
		return m, a, fmt.Errorf("this release has no binary for %s", platform)
	}
	sum, err := hex.DecodeString(a.SHA256)
	if err != nil || len(sum) != sha256.Size {
		return m, a, fmt.Errorf("release binary requires a SHA-256 checksum")
	}
	if a.Size <= 0 || a.Size > 128<<20 {
		return m, a, fmt.Errorf("release binary must be between 1 byte and 128 MiB")
	}
	reference, err := url.Parse(a.URL)
	if err != nil || a.URL == "" {
		return m, a, fmt.Errorf("release binary requires a download URL")
	}
	a.URL = base.ResolveReference(reference).String()
	if err = ValidateURL(a.URL); err != nil {
		return m, a, err
	}
	return m, a, nil
}
