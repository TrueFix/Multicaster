package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstRunCredentialsSurviveRestart(t *testing.T) {
	for _, key := range []string{"MULTICASTER_STREAM_KEY", "MULTICASTER_ADMIN_TOKEN", "MULTICASTER_DATA_DIR"} {
		t.Setenv(key, "")
	}
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	a, resolved, created, err := LoadOrCreate(path)
	if err != nil || !created {
		t.Fatalf("create: %v", err)
	}
	if len(a.Auth.StreamKey) < 32 || len(a.Auth.AdminToken) < 40 {
		t.Fatal("generated credentials too short")
	}
	b, _, created, err := LoadOrCreate(resolved)
	if err != nil || created {
		t.Fatalf("reload: %v", err)
	}
	if a.Auth != b.Auth {
		t.Fatal("credentials changed on restart")
	}
}

func TestMalformedExistingConfigIsNotReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := []byte("server: [broken")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := LoadOrCreate(path); err == nil {
		t.Fatal("expected parse error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(body) {
		t.Fatal("existing config was overwritten")
	}
}

func TestDefaultPathUsesOSConfigurationDirectory(t *testing.T) {
	t.Setenv("MULTICASTER_CONFIG", "")
	t.Setenv("MULTICASTER_DATA_DIR", "")
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skip(err)
	}
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(base, "multicaster", "config.yaml") {
		t.Fatalf("unexpected default path: %s", path)
	}
}

func TestLegacyConfigMigratesWithoutChangingStreamKey(t *testing.T) {
	for _, key := range []string{"MULTICASTER_CONFIG", "MULTICASTER_DATA_DIR", "MULTICASTER_STREAM_KEY", "MULTICASTER_ADMIN_TOKEN"} {
		t.Setenv(key, "")
	}
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("APPDATA", base)
	t.Setenv("HOME", base)
	work := t.TempDir()
	t.Chdir(work)
	legacy := filepath.Join(work, "data", "config.yaml")
	c := Default()
	c.Server.DataDir = filepath.Dir(legacy)
	c.Auth.StreamKey = "existing-obs-key"
	c.Auth.AdminToken = "existing-machine-key"
	if err := c.Save(legacy); err != nil {
		t.Fatal(err)
	}
	loaded, path, _, err := LoadOrCreate("")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.StreamKey != "existing-obs-key" || path == legacy {
		t.Fatal("migration lost credentials or stayed in working directory")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("legacy config was removed")
	}
	t.Chdir(t.TempDir())
	again, nextPath, _, err := LoadOrCreate("")
	if err != nil {
		t.Fatal(err)
	}
	if nextPath != path || again.Auth.StreamKey != loaded.Auth.StreamKey {
		t.Fatal("configuration depended on working directory")
	}
}
