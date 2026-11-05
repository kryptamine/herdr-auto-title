//go:build !windows

package desktop

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

func dial(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "unix", path)
}

// coreSocketPath follows the rules in herdr-core's docs/security.md for its
// default session.
func coreSocketPath(home string) string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		candidate := fmt.Sprintf("/run/user/%d", os.Getuid())
		if _, err := os.Stat(candidate); err == nil {
			runtimeDir = candidate
		}
	}

	if runtimeDir != "" {
		return filepath.Join(runtimeDir, "herdr-core", "default.sock")
	}

	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(state, "herdr-core", "default", "default.sock")
}
