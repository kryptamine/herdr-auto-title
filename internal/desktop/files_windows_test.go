package desktop

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"unsafe"
)

//nolint:paralleltest // duplicating this process's handles can delay other tests' file replacement
func TestWindowsNativeSessionPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "space \u00e9")
	transcript := filepath.Join(root, "sessions", "repo", sessionA, "events.jsonl")

	brain := filepath.Join(root, "antigravity-cli", "brain", sessionB)
	for _, dir := range []string{filepath.Dir(transcript), brain} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	file, err := os.Create(transcript)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = file.Close() })

	dir, err := os.Open(brain)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dir.Close() })

	finder := newSessionFinder(nil)

	pids := []int{os.Getpid()}
	if got, err := finder.grokSession(t.Context(), pids); err != nil || got != sessionA {
		t.Fatalf("Grok session = %q, %v", got, err)
	}

	if got, err := finder.antigravitySession(t.Context(), pids); err != nil || got != sessionB {
		t.Fatalf("Antigravity session = %q, %v", got, err)
	}
}

func TestWindowsHandleSnapshotBounds(t *testing.T) {
	t.Parallel()

	const header = 2 * unsafe.Sizeof(uintptr(0))
	for _, buffer := range [][]byte{nil, make([]byte, header-1), make([]byte, header)} {
		if len(buffer) == int(header) {
			buffer[0] = 1
		}

		if _, err := handlesIn(buffer); err == nil {
			t.Fatalf("accepted short snapshot of %d bytes", len(buffer))
		}
	}

	buffer := make([]byte, header+unsafe.Sizeof(handleEntry{}))
	buffer[0] = 1
	buffer[header] = 42

	got, err := handlesIn(buffer)
	if err != nil || !slices.Equal(got, []syscall.Handle{42}) {
		t.Fatalf("decoded handles = %v, %v", got, err)
	}
}

func TestWindowsDOSPaths(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		`\\?\C:\space name\events.jsonl`:    `C:\space name\events.jsonl`,
		`\\?\UNC\server\share\events.jsonl`: `\\server\share\events.jsonl`,
		`C:\events.jsonl`:                   `C:\events.jsonl`,
	} {
		if got := dosPath(input); got != want {
			t.Errorf("dosPath(%q) = %q, want %q", input, got, want)
		}
	}
}

//nolint:paralleltest // duplicating this process's handles can delay other tests' file replacement
func TestWindowsOpenFilesClosesItsHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held.jsonl")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = file.Close() })

	for range 20 {
		paths, err := openFiles(t.Context(), []int{os.Getpid()})
		if err != nil {
			t.Fatal(err)
		}

		count := 0

		for _, held := range paths {
			if held == path {
				count++
			}
		}

		if count != 1 {
			t.Fatalf("fixture file has %d handles, want only its original", count)
		}
	}
}
