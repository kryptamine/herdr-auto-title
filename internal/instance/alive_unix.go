//go:build !windows

package instance

import (
	"errors"
	"syscall"
)

// alive reports whether a process with this pid exists. Signal zero checks
// without sending, and being refused permission still means someone is there.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}
