package systeminfo

import (
	"github.com/truefix/multicaster/internal/engine"
	"github.com/truefix/multicaster/internal/updates"
	"os"
	"runtime"
	"time"
)

var started = time.Now()

func Snapshot(e *engine.Engine) map[string]any {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(), "go_max_procs": runtime.GOMAXPROCS(0), "go_version": runtime.Version(), "version": updates.Version, "uptime_seconds": int64(time.Since(started).Seconds()), "pid": os.Getpid(), "heap_alloc_bytes": memory.HeapAlloc, "go_reserved_bytes": memory.Sys, "goroutines": runtime.NumGoroutine(), "config_path": e.ConfigPath(), "data_path": e.Config().Server.DataDir}
}
