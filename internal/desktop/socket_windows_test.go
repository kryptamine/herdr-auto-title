package desktop

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func TestCorePipeUsesTheSanitizedAccount(t *testing.T) {
	t.Setenv("USERNAME", "A.d-min_42")
	t.Setenv("HC_TERMINAL_SOCKET", `\\.\pipe\another-session.sock`)

	if got := coreSocketPath("ignored"); got != `\\.\pipe\herdr-core-Admin42-default.sock` {
		t.Fatalf("core endpoint = %q", got)
	}
}

func TestCanceledPipeDialDoesNotOpen(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := dial(ctx, pipePrefix+"missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dial: %v", err)
	}
}

func TestPipeRequestAndCancellation(t *testing.T) {
	t.Parallel()

	for _, stalled := range []bool{false, true} {
		t.Run(map[bool]string{false: "reply", true: "cancel"}[stalled], func(t *testing.T) {
			t.Parallel()
			path, received := servePipe(t, stalled)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if stalled {
				go func() { <-received; cancel() }()
			}

			var reply map[string]bool

			err := ask(ctx, path, map[string]bool{"probe": true}, &reply)
			if stalled {
				if !errors.Is(err, herdr.ErrUnanswered) {
					t.Fatalf("canceled request: %v", err)
				}
			} else if err != nil || !reply["ok"] {
				t.Fatalf("reply=%v error=%v", reply, err)
			}
		})
	}
}

func servePipe(t *testing.T, stalled bool) (string, <-chan struct{}) {
	t.Helper()
	path := pipePrefix + filepath.Join(t.TempDir(), "core.sock")

	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}

	kernel := syscall.NewLazyDLL("kernel32.dll")

	const duplex, buffer = 3, 4096

	handle, _, err := kernel.NewProc("CreateNamedPipeW").
		Call(uintptr(unsafe.Pointer(name)), duplex, 0, 1, buffer, buffer, 0, 0)
	if syscall.Handle(handle) == syscall.InvalidHandle {
		t.Fatal(err)
	}

	conn := os.NewFile(handle, path)
	received := make(chan struct{})
	done := make(chan struct{})

	t.Cleanup(func() {
		_ = conn.Close()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("pipe server did not exit")
		}
	})

	go func() {
		defer close(done)

		ok, _, err := kernel.NewProc("ConnectNamedPipe").Call(handle, 0)

		const alreadyConnected = syscall.Errno(535)
		if ok == 0 && !errors.Is(err, alreadyConnected) {
			t.Error(err)
			return
		}

		if _, err := bufio.NewReader(conn).ReadBytes('\n'); err != nil {
			t.Error(err)
			return
		}

		close(received)

		if stalled {
			_, _ = io.Copy(io.Discard, conn)
			return
		}

		_, _ = conn.WriteString("{\"ok\":true}\n")
		_ = conn.Sync()
	}()

	return path, received
}
