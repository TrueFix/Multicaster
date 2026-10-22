package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/google/uuid"
	"github.com/truefix/multicaster/internal/config"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Source reads a single generated HLS resource without a network roundtrip.
type Source func(context.Context, string) ([]byte, error)
type Publisher struct {
	mu                        sync.RWMutex
	cancel                    context.CancelFunc
	done                      chan struct{}
	status, message, playback string
}
type Snapshot struct {
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	PlaybackURL string `json:"playback_url,omitempty"`
}

func (p *Publisher) Snapshot() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return Snapshot{p.status, p.message, p.playback}
}
func Validate(c config.Storage) error {
	if !c.Enabled {
		return nil
	}
	for _, v := range []string{c.Endpoint, c.PublicURL} {
		u, e := url.Parse(v)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("endpoint and public URL must be HTTP(S) URLs without credentials or query strings")
		}
	}
	if c.Bucket == "" || strings.ContainsAny(c.Bucket, "/\\") || c.Region == "" || c.AccessKey == "" || c.SecretKey == "" {
		return fmt.Errorf("bucket, region, access key and secret key are required")
	}
	if strings.Contains(c.Prefix, "..") || strings.Contains(c.Prefix, "\\") {
		return fmt.Errorf("prefix must be a relative object path")
	}
	return nil
}
func (p *Publisher) Start(c config.Storage, source Source) {
	p.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	prefix := path.Join(strings.Trim(c.Prefix, "/"), uuid.NewString())
	p.mu.Lock()
	p.cancel = cancel
	p.done = make(chan struct{})
	p.status = "starting"
	p.message = "Waiting for the first complete HLS segments"
	p.playback = strings.TrimRight(c.PublicURL, "/") + "/" + prefix + "/index.m3u8"
	done := p.done
	p.mu.Unlock()
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			panicked := func() (panicked bool) {
				defer func() {
					if recover() != nil {
						panicked = true
					}
				}()
				p.run(ctx, c, prefix, source)
				return false
			}()
			if !panicked {
				return
			}
			p.set("error", "Cloud publisher interrupted; retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}
func (p *Publisher) Stop() {
	p.mu.Lock()
	cancel, done := p.cancel, p.done
	p.cancel = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	p.mu.Lock()
	p.status = "off"
	p.message = ""
	p.playback = ""
	p.mu.Unlock()
}
func (p *Publisher) set(status, message string) {
	p.mu.Lock()
	p.status = status
	p.message = message
	p.mu.Unlock()
}

var uriPattern = regexp.MustCompile(`URI="([^"]+)"`)

func dependencies(name string, data []byte) ([]string, error) {
	var refs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			for _, match := range uriPattern.FindAllStringSubmatch(line, -1) {
				refs = append(refs, match[1])
			}
		} else {
			refs = append(refs, line)
		}
	}
	for i, ref := range refs {
		u, e := url.Parse(ref)
		if e != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || strings.HasPrefix(ref, "/") {
			return nil, fmt.Errorf("unexpected HLS resource path")
		}
		refs[i] = path.Clean(path.Join(path.Dir(name), u.Path))
		if strings.HasPrefix(refs[i], "../") {
			return nil, fmt.Errorf("invalid HLS resource path")
		}
	}
	return refs, nil
}
func (p *Publisher) run(ctx context.Context, c config.Storage, prefix string, source Source) {
	client := &http.Client{Timeout: 12 * time.Second}
	signer := v4.NewSigner()
	previous := map[string]string{}
	for ctx.Err() == nil {
		next := map[string]string{}
		visiting := map[string]bool{}
		var publish func(string) error
		publish = func(name string) error {
			if _, ok := next[name]; ok {
				return nil
			}
			if visiting[name] {
				return fmt.Errorf("cyclic HLS playlist")
			}
			visiting[name] = true
			playlist := strings.HasSuffix(name, ".m3u8")
			if !playlist {
				if hash, ok := previous[name]; ok {
					next[name] = hash
					return nil
				}
			}
			body, err := source(ctx, name)
			if err != nil {
				return err
			}
			if len(body) > 16*1024*1024 {
				return fmt.Errorf("HLS object exceeds upload limit")
			}
			if playlist {
				refs, err := dependencies(name, body)
				if err != nil {
					return err
				}
				for _, ref := range refs {
					if err = publish(ref); err != nil {
						return err
					}
				}
			}
			digest := sha256.Sum256(body)
			hash := hex.EncodeToString(digest[:])
			if previous[name] == hash {
				next[name] = hash
				return nil
			}
			target, err := url.Parse(c.Endpoint)
			if err != nil {
				return err
			}
			target.Path = path.Join(target.Path, c.Bucket, prefix, name)
			req, err := http.NewRequestWithContext(ctx, "PUT", target.String(), bytes.NewReader(body))
			if err != nil {
				return err
			}
			contentType := "video/mp4"
			cache := "public, max-age=3600, immutable"
			if playlist {
				contentType = "application/vnd.apple.mpegurl"
				cache = "no-cache, max-age=0"
			}
			req.Header.Set("Content-Type", contentType)
			req.Header.Set("Cache-Control", cache)
			req.Header.Set("X-Amz-Content-Sha256", hash)
			if err = signer.SignHTTP(ctx, aws.Credentials{AccessKeyID: c.AccessKey, SecretAccessKey: c.SecretKey}, req, hash, "s3", c.Region, time.Now(), func(options *v4.SignerOptions) { options.DisableURIPathEscaping = true }); err != nil {
				return err
			}
			res, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("object storage is unreachable")
			}
			res.Body.Close()
			if res.StatusCode < 200 || res.StatusCode >= 300 {
				return fmt.Errorf("object storage rejected upload (HTTP %d)", res.StatusCode)
			}
			next[name] = hash
			return nil
		}
		err := publish("index.m3u8")
		delay := 2 * time.Second
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.set("error", err.Error())
			delay = 5 * time.Second
		} else {
			previous = next
			p.set("live", "")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}
