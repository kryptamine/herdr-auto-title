// Package autostart has the OS start the desktop mode at login and start it
// again if it fails, from a copy of the binary of its own, and takes all of
// that away again: docs/architecture/desktop-api.md#starting-at-login.
package autostart

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// name is the binary's own name, and the service's.
const name = "herdr-auto-title"

// executable is the copy's mode: the service manager runs it as this user.
const executable = 0o755

// Installed says where an install put the binary and what starts it.
type Installed struct {
	Binary  string
	Service string
}

// service is one OS's way of starting a program at login.
type service interface {
	install(ctx context.Context, exe string) (Installed, error)
	uninstall(ctx context.Context) error
}

// host is what a service reads of the machine and runs on it, so a test can
// hand it a temporary home and record the commands instead.
type host struct {
	home   string
	uid    int
	getenv func(string) string
	run    func(ctx context.Context, name string, args ...string) error
	retry  time.Duration
}

// Install copies exe to where the service runs it from, and has the OS start
// it now and at every login. Run again, it replaces the copy and restarts it.
func Install(ctx context.Context, exe string) (Installed, error) {
	s, err := current()
	if err != nil {
		return Installed{}, err
	}

	return s.install(ctx, exe)
}

// Uninstall stops the service and removes it and the binary's copy. What it
// finds already gone is not an error.
func Uninstall(ctx context.Context) error {
	s, err := current()
	if err != nil {
		return err
	}

	return s.uninstall(ctx)
}

// copyBinary puts exe at dst through a rename, so a copy that is running keeps
// its file, and an install run from the copy itself does not truncate it.
func copyBinary(exe, dst string) (err error) {
	//nolint:gosec // the directory is Auto Title's own, never terminal-derived
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	src, err := os.Open(exe) //nolint:gosec // the running binary's own path
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+name+"-*")
	if err != nil {
		return err
	}

	defer func() {
		_ = tmp.Close()

		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()

	if _, err := io.Copy(tmp, src); err != nil {
		return err
	}

	if err := tmp.Chmod(executable); err != nil {
		return err
	}

	// Closed here as well, because closing a written file can report the write.
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), dst)
}

// writeFile writes a service definition, readable by the service manager.
func writeFile(path, content string) error {
	//nolint:gosec // the directory is the service manager's, never terminal-derived
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // as the manager expects
}

// remove deletes each path, ignoring one already gone.
func remove(paths ...string) error {
	var errs []error

	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// removeBinary deletes the copy, and its directory once nothing else is in it.
func removeBinary(path string) error {
	if err := remove(path); err != nil {
		return err
	}

	_ = os.Remove(filepath.Dir(path))

	return nil
}
