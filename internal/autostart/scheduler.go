package autostart

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	imageReleaseTimeout = 10 * time.Second
	imageReleasePoll    = 100 * time.Millisecond
)

// scheduler uses the logged-in user's token, without a password or elevation.
type scheduler struct{ host }

type scheduledCommand struct {
	Operation string
	Binary    string
	ParentPID int
}

func (s scheduler) binary() string {
	local := s.getenv("LOCALAPPDATA")
	if !filepath.IsAbs(local) {
		local = filepath.Join(s.home, "AppData", "Local")
	}

	return filepath.Join(local, name, "bin", name+".exe")
}

func (s scheduler) install(ctx context.Context, exe string) (Installed, error) {
	staged, err := s.stage(exe)
	if err != nil {
		return Installed{}, fmt.Errorf("copy the binary: %w", err)
	}

	if staged != "" {
		defer func() { _ = os.Remove(staged) }()
	}

	if _, err := s.command(ctx, "stop"); err != nil {
		return Installed{}, err
	}

	if staged != "" {
		if err := replaceStopped(ctx, staged, s.binary()); err != nil {
			return Installed{}, fmt.Errorf("replace the stopped binary: %w", err)
		}
	}

	task, err := s.command(ctx, "install")
	if err != nil {
		return Installed{}, err
	}

	return Installed{Binary: s.binary(), Service: strings.TrimSpace(task)}, nil
}

// replaceStopped waits for Windows to release a task's image after Stop has
// returned. It never removes the old copy before the replacement succeeds.
func replaceStopped(ctx context.Context, src, dst string) error {
	deadline := time.Now().Add(imageReleaseTimeout)

	for {
		err := os.Rename(src, dst)
		if !errors.Is(err, os.ErrPermission) || time.Now().After(deadline) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(imageReleasePoll):
		}
	}
}

// stage copies before stopping a working task. Reinstalling from its own copy
// only restarts the task: Windows cannot replace the installer's running image.
func (s scheduler) stage(exe string) (string, error) {
	if sameFile(exe, s.binary()) {
		return "", nil
	}

	dir := filepath.Dir(s.binary())
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // this user's binary directory
		return "", err
	}

	tmp, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return "", err
	}

	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}

	if err := copyBinary(exe, path); err != nil {
		_ = os.Remove(path)
		return "", err
	}

	return path, nil
}

func (s scheduler) uninstall(ctx context.Context) error {
	_, err := s.command(ctx, "uninstall")

	return err
}

func (s scheduler) running(ctx context.Context) bool {
	out, err := s.command(ctx, "running")

	return err == nil && strings.TrimSpace(out) == "True"
}

func (s scheduler) command(ctx context.Context, operation string) (string, error) {
	script, err := render("scheduler.ps1", scheduledCommand{
		Operation: operation,
		Binary:    s.binary(),
		ParentPID: os.Getpid(),
	})
	if err != nil {
		return "", err
	}

	return s.run(
		ctx,
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-WindowStyle",
		"Hidden",
		"-OutputFormat",
		"Text",
		"-EncodedCommand",
		encodedCommand(script),
	)
}

// encodedCommand keeps PowerShell's command-line parser away from paths and
// uses the UTF-16LE encoding Windows PowerShell requires.
func encodedCommand(script string) string {
	units := utf16.Encode([]rune(script))

	data := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[i*2:], unit)
	}

	return base64.StdEncoding.EncodeToString(data)
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
