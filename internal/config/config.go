package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/truefix/multicaster/internal/updates"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Storage      Storage       `yaml:"storage" json:"storage"`
	Server       ServerConfig  `yaml:"server"`
	Settings     Settings      `yaml:"settings"`
	Auth         AuthConfig    `yaml:"auth"`
	HLS          HLSConfig     `yaml:"hls"`
	Destinations []Destination `yaml:"destinations"`
}

type Storage struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Endpoint  string `yaml:"endpoint" json:"endpoint"`
	Region    string `yaml:"region" json:"region"`
	Bucket    string `yaml:"bucket" json:"bucket"`
	Prefix    string `yaml:"prefix" json:"prefix"`
	PublicURL string `yaml:"public_url" json:"public_url"`
	AccessKey string `yaml:"access_key" json:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key,omitempty"`
}

type ServerConfig struct {
	RTMPListen string `yaml:"rtmp_listen"`
	HTTPListen string `yaml:"http_listen"`
	DataDir    string `yaml:"data_dir"`
}

type Settings struct {
	UpdateManifestURL string `yaml:"update_manifest_url" json:"update_manifest_url"`
	PreviewEnabled    bool   `yaml:"preview_enabled" json:"preview_enabled"`
	UpdateRepository  string `yaml:"update_repository" json:"update_repository"`
	RestreamEnabled   bool   `yaml:"restream_enabled" json:"restream_enabled"`
	WebStreamEnabled  bool   `yaml:"web_stream_enabled" json:"web_stream_enabled"`
	PerformanceMode   bool   `yaml:"performance_mode" json:"performance_mode"`
}

type SettingsPatch struct {
	UpdateManifestURL *string `json:"update_manifest_url"`
	UpdateRepository  *string `json:"update_repository"`
	RestreamEnabled   *bool   `json:"restream_enabled"`
	WebStreamEnabled  *bool   `json:"web_stream_enabled"`
	PerformanceMode   *bool   `json:"performance_mode"`
	PreviewEnabled    *bool   `json:"preview_enabled"`
}

type AuthConfig struct {
	PasswordHash string `yaml:"password_hash,omitempty"`
	SetupToken   string `yaml:"setup_token,omitempty"`
	StreamKey    string `yaml:"stream_key"`
	AdminToken   string `yaml:"admin_token"`
}

type HLSConfig struct {
	SegmentDuration time.Duration `yaml:"segment_duration"`
	PlaylistLength  int           `yaml:"playlist_length"`
}

type Destination struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Type    string `yaml:"type" json:"type"`
	URL     string `yaml:"url" json:"url"`
	Enabled bool   `yaml:"enabled" json:"enabled"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			RTMPListen: ":1935",
			HTTPListen: ":8080",
			DataDir:    "",
		},
		Settings: Settings{
			UpdateManifestURL: updates.DefaultManifestURL,
			RestreamEnabled:   true,
			WebStreamEnabled:  false,
			PerformanceMode:   false,
		},
		Auth: AuthConfig{
			StreamKey:  "",
			AdminToken: "",
		},
		HLS: HLSConfig{
			SegmentDuration: 2 * time.Second,
			PlaylistLength:  6,
		},
		Destinations: []Destination{},
	}
}

// DefaultPath is where config is stored when -config is not passed.
func DefaultPath() (string, error) {
	if v := os.Getenv("MULTICASTER_CONFIG"); v != "" {
		return v, nil
	}
	dataDir := os.Getenv("MULTICASTER_DATA_DIR")
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("locate user configuration directory: %w", err)
		}
		dataDir = filepath.Join(base, "multicaster")
	}
	return filepath.Join(dataDir, "config.yaml"), nil
}

func ResolvePath(path string) (string, error) {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return "", err
		}
	}
	return filepath.Abs(path)
}

// LoadOrCreate reads config from path, or writes a new default file if missing.
// Returns the resolved path, whether the file was created, and any error.
func LoadOrCreate(path string) (*Config, string, bool, error) {
	implicit := path == "" && os.Getenv("MULTICASTER_CONFIG") == "" && os.Getenv("MULTICASTER_DATA_DIR") == ""
	path, err := ResolvePath(path)
	if err != nil {
		return nil, "", false, err
	}

	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, path, false, err
		}
		cfg, err := newDefaultForPath(path)
		if err != nil {
			return nil, path, false, err
		}
		if implicit {
			legacy := filepath.Join("data", "config.yaml")
			if body, e := os.ReadFile(legacy); e == nil {
				if e = yaml.Unmarshal(body, cfg); e != nil {
					return nil, path, false, fmt.Errorf("read legacy config: %w", e)
				}
				if cfg.Server.DataDir != "" && !filepath.IsAbs(cfg.Server.DataDir) {
					cfg.Server.DataDir, e = filepath.Abs(cfg.Server.DataDir)
					if e != nil {
						return nil, path, false, e
					}
				}
			} else if !errors.Is(e, os.ErrNotExist) {
				return nil, path, false, e
			}
		}
		applyEnv(cfg)
		if err := cfg.ensureSecrets(); err != nil {
			return nil, path, false, err
		}
		if err := cfg.validate(); err != nil {
			return nil, path, false, err
		}
		if err := cfg.Save(path); err != nil {
			return nil, path, false, err
		}
		if err := cfg.validate(); err != nil {
			return nil, path, false, err
		}
		return cfg, path, true, nil
	}

	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, false, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, path, false, err
	}
	if cfg.Server.DataDir == "" {
		cfg.Server.DataDir = filepath.Dir(path)
	}
	applyEnv(cfg)
	if !filepath.IsAbs(cfg.Server.DataDir) {
		cfg.Server.DataDir = filepath.Join(filepath.Dir(path), cfg.Server.DataDir)
	}
	oldAuth := cfg.Auth
	if err := cfg.ensureSecrets(); err != nil {
		return nil, path, false, err
	}
	if err := cfg.validate(); err != nil {
		return nil, path, false, err
	}
	if cfg.Auth != oldAuth {
		if err := cfg.Save(path); err != nil {
			return nil, path, false, err
		}
	}
	return cfg, path, false, nil
}

func newDefaultForPath(configPath string) (*Config, error) {
	cfg := Default()
	dataDir, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return nil, err
	}
	cfg.Server.DataDir = dataDir
	return cfg, nil
}

func (c *Config) ensureSecrets() error {
	if c.Auth.PasswordHash == "" && c.Auth.SetupToken == "" {
		token, err := NewSecret()
		if err != nil {
			return err
		}
		c.Auth.SetupToken = token
	}
	if c.Auth.StreamKey == "" {
		key, err := randomToken(24)
		if err != nil {
			return err
		}
		c.Auth.StreamKey = key
	}
	if c.Auth.AdminToken == "" {
		token, err := randomToken(32)
		if err != nil {
			return err
		}
		c.Auth.AdminToken = token
	}
	return nil
}

func NewSecret() (string, error) { return randomToken(32) }

func randomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("MULTICASTER_STREAM_KEY"); v != "" {
		cfg.Auth.StreamKey = v
	}
	if v := os.Getenv("MULTICASTER_ADMIN_TOKEN"); v != "" {
		cfg.Auth.AdminToken = v
	}
	if v := os.Getenv("MULTICASTER_HTTP_LISTEN"); v != "" {
		cfg.Server.HTTPListen = v
	}
	if v := os.Getenv("MULTICASTER_RTMP_LISTEN"); v != "" {
		cfg.Server.RTMPListen = v
	}
	if v := os.Getenv("MULTICASTER_DATA_DIR"); v != "" {
		cfg.Server.DataDir = v
	}
}

func (c *Config) validate() error {
	for _, address := range []string{c.Server.HTTPListen, c.Server.RTMPListen} {
		if _, _, err := net.SplitHostPort(address); err != nil {
			return fmt.Errorf("invalid listen address: %w", err)
		}
	}
	if len(c.Destinations) > 32 {
		return fmt.Errorf("at most 32 destinations are supported per instance")
	}
	if c.Auth.StreamKey == "" {
		return fmt.Errorf("auth.stream_key must be set")
	}
	if c.Auth.AdminToken == "" {
		return fmt.Errorf("auth.admin_token must be set")
	}
	ids := map[string]bool{}
	for i, d := range c.Destinations {
		if d.ID == "" {
			c.Destinations[i].ID = fmt.Sprintf("dest-%d", i+1)
		}
		if ids[c.Destinations[i].ID] {
			return fmt.Errorf("duplicate destination ID")
		}
		ids[c.Destinations[i].ID] = true
		if d.Type == "" {
			c.Destinations[i].Type = "rtmp_push"
		}
		u, err := url.Parse(d.URL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "rtmp" && u.Scheme != "rtmps") {
			return fmt.Errorf("destination %q requires an RTMP/RTMPS URL", d.Name)
		}
	}
	return nil
}

func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(c.Server.DataDir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// IngestPath is the RTMP app name publishers use (OBS server URL path).
func (c *Config) IngestApp() string {
	return "live"
}

func (c *Config) PublishURL(host string) string {
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	if idx := strings.Index(host, ":"); idx > 0 {
		host = host[:idx]
	}
	return fmt.Sprintf("rtmp://%s/%s/%s", host, c.IngestApp(), c.Auth.StreamKey)
}
