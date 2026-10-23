//go:build !windows

package logging

import (
	"os/signal"
	"syscall"
)

// Losing a log pipe must not terminate all streams with SIGPIPE.
func IgnoreBrokenPipe() { signal.Ignore(syscall.SIGPIPE) }
