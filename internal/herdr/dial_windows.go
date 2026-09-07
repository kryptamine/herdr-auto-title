package herdr

import (
	"context"
	"io"
	"os"
)

// pipePrefix is the named pipe namespace. On Windows HERDR_SOCKET_PATH names a
// plain file, and the pipe Herdr listens on carries that whole path as its
// name, drive letter and backslashes included.
const pipePrefix = `\\.\pipe\`

// dial opens one connection to the pipe behind path. A pipe takes no deadline,
// but closing it does unblock a pending read, which is all cancellation needs.
func dial(_ context.Context, path string) (io.ReadWriteCloser, error) {
	//nolint:gosec // the path is HERDR_SOCKET_PATH, never terminal-derived
	return os.OpenFile(pipePrefix+path, os.O_RDWR, 0)
}
