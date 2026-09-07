//go:build !windows

package herdr

import (
	"context"
	"io"
	"net"
)

// dial opens one connection to the Unix socket at path.
func dial(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	var d net.Dialer

	return d.DialContext(ctx, "unix", path)
}
