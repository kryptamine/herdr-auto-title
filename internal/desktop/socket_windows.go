package desktop

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
)

const pipePrefix = `\\.\pipe\`

func dial(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if strings.HasPrefix(path, pipePrefix) {
		//nolint:gosec // the endpoint is the local core's configured pipe
		return os.OpenFile(path, os.O_RDWR, 0)
	}

	var dialer net.Dialer

	return dialer.DialContext(ctx, "unix", path)
}

// coreSocketPath matches herdr-core's default session, including its ASCII-only
// account spelling. A terminal's HC_TERMINAL_SOCKET may name another session.
func coreSocketPath(_ string) string {
	user := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}

		return -1
	}, os.Getenv("USERNAME"))

	return pipePrefix + "herdr-core-" + user + "-default.sock"
}
