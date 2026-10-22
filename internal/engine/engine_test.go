package engine

import (
	"github.com/truefix/multicaster/internal/config"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFailedSaveCannotApplySettings(t *testing.T) {
	c := config.Default()
	c.Server.DataDir = t.TempDir()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	e := New(c, path, nil)
	off := false
	if _, err := e.PatchSettings(config.SettingsPatch{RestreamEnabled: &off}); err == nil {
		t.Fatal("save to directory should fail")
	}
	if !e.Config().Settings.RestreamEnabled {
		t.Fatal("failed save changed live settings")
	}
}
func TestConcurrentSettingsPatchesPreserveBothChanges(t *testing.T) {
	e := New(config.Default(), "", nil)
	on := true
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := e.PatchSettings(config.SettingsPatch{PerformanceMode: &on}); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := e.PatchSettings(config.SettingsPatch{PreviewEnabled: &on}); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	s := e.Config().Settings
	if !s.PerformanceMode || !s.PreviewEnabled {
		t.Fatal("concurrent patch lost an update")
	}
}
