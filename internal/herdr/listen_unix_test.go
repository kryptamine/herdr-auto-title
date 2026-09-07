//go:build !windows

package herdr

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

type socketListener struct{ net.Listener }

func (l socketListener) accept() (io.ReadWriteCloser, error) { return l.Accept() }
func (l socketListener) close()                              { l.Close() }

// listen opens a Unix socket for the client to dial.
func listen(t *testing.T) (listener, string) {
	t.Helper()

	// t.TempDir() names the directory after the test, which measured 122 bytes
	// here — past the 104 a socket's sun_path holds.
	//nolint:usetesting // t.TempDir() overruns sun_path, as measured above
	dir, err := os.MkdirTemp("", "at")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(dir) })

	path := filepath.Join(dir, "h.sock")

	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen on %s: %v", path, err)
	}

	return socketListener{ln}, path
}
