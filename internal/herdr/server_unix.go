//go:build !windows

package herdr

import (
	"fmt"
	"os"
	"syscall"
)

// serverIdentity is the socket file's device and inode. A server binding the
// path creates the file anew, so a successor's differs from its predecessor's,
// and "" says no server holds the path.
func serverIdentity(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}

	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
}
