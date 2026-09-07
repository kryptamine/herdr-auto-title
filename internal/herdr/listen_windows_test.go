package herdr

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

// The kernel32 calls a named pipe server needs and package syscall does not
// wrap.
var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procCreateNamedPipeW = kernel32.NewProc("CreateNamedPipeW")
	procConnectNamedPipe = kernel32.NewProc("ConnectNamedPipe")
)

const (
	pipeAccessDuplex       = 0x00000003
	pipeTypeByte           = 0x00000000
	pipeUnlimitedInstances = 255
	pipeBuffer             = 64 << 10
	// errPipeConnected is how ConnectNamedPipe reports a client that arrived
	// before it was called, which is a connection rather than a failure.
	errPipeConnected = syscall.Errno(535)
)

var errListenerClosed = errors.New("listener closed")

// pipeListener hands out one pipe instance per connection, as Herdr does. One
// instance is always waiting: a client finds nothing to open otherwise.
type pipeListener struct {
	name   *uint16
	path   string
	next   syscall.Handle
	closed chan struct{}
}

// listen names a pipe after a socket path, the way Herdr does, so the client
// under test dials exactly what it dials in a session.
func listen(t *testing.T) (listener, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "h.sock")

	name, err := syscall.UTF16PtrFromString(pipePrefix + path)
	if err != nil {
		t.Fatalf("pipe name: %v", err)
	}

	l := &pipeListener{name: name, path: path, closed: make(chan struct{})}

	if l.next, err = l.instance(); err != nil {
		t.Fatalf("create pipe %s: %v", path, err)
	}

	return l, path
}

func (l *pipeListener) instance() (syscall.Handle, error) {
	handle, _, err := procCreateNamedPipeW.Call(
		uintptr(unsafe.Pointer(l.name)),
		pipeAccessDuplex,
		pipeTypeByte,
		pipeUnlimitedInstances,
		pipeBuffer,
		pipeBuffer,
		0,
		0,
	)
	if syscall.Handle(handle) == syscall.InvalidHandle {
		return syscall.InvalidHandle, err
	}

	return syscall.Handle(handle), nil
}

// accept waits for a client on the instance that is up, and puts up the next
// one before handing this one out.
func (l *pipeListener) accept() (io.ReadWriteCloser, error) {
	handle := l.next

	ok, _, err := procConnectNamedPipe.Call(uintptr(handle), 0)
	if ok == 0 && !errors.Is(err, errPipeConnected) {
		syscall.CloseHandle(handle)

		return nil, err
	}

	select {
	case <-l.closed:
		syscall.CloseHandle(handle)

		return nil, errListenerClosed
	default:
	}

	if l.next, err = l.instance(); err != nil {
		syscall.CloseHandle(handle)

		return nil, err
	}

	return pipeConn{os.NewFile(uintptr(handle), l.path)}, nil
}

// close stops accepting. ConnectNamedPipe returns only when a client arrives,
// so one is sent to wake the accept that is waiting.
func (l *pipeListener) close() {
	close(l.closed)

	if f, err := os.OpenFile(pipePrefix+l.path, os.O_RDWR, 0); err == nil {
		f.Close()
	}
}

// pipeConn is a server end whose Close first waits for the client to read what
// was written: closing a pipe instance discards anything still unread.
type pipeConn struct{ *os.File }

func (c pipeConn) Close() error {
	_ = c.Sync()

	return c.File.Close()
}
