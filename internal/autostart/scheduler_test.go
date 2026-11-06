package autostart

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

func decodeCommand(t *testing.T, args []string) string {
	t.Helper()

	if len(args) != 8 || args[6] != "-EncodedCommand" {
		t.Fatalf("unexpected PowerShell arguments: %q", args)
	}

	script, err := decodeEncodedCommand(args[7])
	if err != nil {
		t.Fatal(err)
	}

	return script
}

func decodeEncodedCommand(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[i*2:])
	}

	return string(utf16.Decode(units)), nil
}

func TestSchedulerInstallStopsBeforeReplacing(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "a user's space")

	s := scheduler{(&fakeHost{}).host(home, nil)}
	if err := copyBinary(fakeBinary(t, "v1"), s.binary()); err != nil {
		t.Fatal(err)
	}

	var operations []string

	s.run = func(_ context.Context, program string, args ...string) (string, error) {
		if program != "powershell.exe" {
			t.Errorf("unexpected program %q", program)
		}

		script := decodeCommand(t, args)
		if !strings.Contains(script, "$binary = "+powershellQuote(s.binary())) {
			t.Error("the binary path did not reach PowerShell literally")
		}

		switch {
		case strings.Contains(script, "$operation = 'stop'"):
			operations = append(operations, "stop")

			if readFile(t, s.binary()) != "v1" {
				t.Error("the running copy was replaced before stopping the task")
			}
		case strings.Contains(script, "$operation = 'install'"):
			operations = append(operations, "install")

			if readFile(t, s.binary()) != "v2" {
				t.Error("the task would start the old copy")
			}
		default:
			t.Error("unexpected operation")
		}

		return "herdr-auto-title-S-1-test\r\n", nil
	}

	got, err := s.install(t.Context(), fakeBinary(t, "v2"))
	if err != nil {
		t.Fatal(err)
	}

	if got.Binary != s.binary() || got.Service != "herdr-auto-title-S-1-test" {
		t.Errorf("installed %+v", got)
	}

	if !slices.Equal(operations, []string{"stop", "install"}) {
		t.Errorf("operations = %v", operations)
	}
}

func TestSchedulerReinstallFromItsCopy(t *testing.T) {
	t.Parallel()

	s := scheduler{(&fakeHost{}).host(t.TempDir(), nil)}
	if err := copyBinary(fakeBinary(t, "v1"), s.binary()); err != nil {
		t.Fatal(err)
	}

	before, err := os.Stat(s.binary())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.install(t.Context(), s.binary()); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(s.binary())
	if err != nil {
		t.Fatal(err)
	}

	if !os.SameFile(before, after) || readFile(t, s.binary()) != "v1" {
		t.Error("reinstall replaced its own running image")
	}
}

func TestSchedulerCopyFailureDoesNotStopTheTask(t *testing.T) {
	t.Parallel()

	fake := &fakeHost{}

	s := scheduler{fake.host(t.TempDir(), nil)}
	if _, err := s.install(t.Context(), filepath.Join(t.TempDir(), "missing.exe")); err == nil {
		t.Fatal("a missing source was accepted")
	}

	if len(fake.runs) != 0 {
		t.Error("the service manager was called before the copy succeeded")
	}
}

func TestSchedulerStopFailurePreservesTheCopy(t *testing.T) {
	t.Parallel()

	s := scheduler{(&fakeHost{}).host(t.TempDir(), nil)}
	if err := copyBinary(fakeBinary(t, "v1"), s.binary()); err != nil {
		t.Fatal(err)
	}

	s.run = func(context.Context, string, ...string) (string, error) {
		return "", errors.New("task access denied")
	}
	if _, err := s.install(t.Context(), fakeBinary(t, "v2")); err == nil {
		t.Error("a failed stop was reported as installed")
	}

	if readFile(t, s.binary()) != "v1" {
		t.Error("a failed stop replaced the copy")
	}

	files, err := os.ReadDir(filepath.Dir(s.binary()))
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Errorf("staged files were left behind: %v", files)
	}
}

func TestSchedulerRunning(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		output string
		err    error
		want   bool
	}{
		{"True\r\n", nil, true},
		{"False\r\n", nil, false},
		{"", errors.New("scheduler unavailable"), false},
	} {
		s := scheduler{(&fakeHost{}).host(t.TempDir(), nil)}

		s.run = func(context.Context, string, ...string) (string, error) {
			return tc.output, tc.err
		}
		if got := s.running(t.Context()); got != tc.want {
			t.Errorf("output %q: running = %v", tc.output, got)
		}
	}
}

func TestSchedulerUsesLocalAppData(t *testing.T) {
	t.Parallel()

	home, local := t.TempDir(), t.TempDir()

	s := scheduler{(&fakeHost{}).host(home, map[string]string{"LOCALAPPDATA": local})}
	if s.binary() != filepath.Join(local, name, "bin", name+".exe") {
		t.Errorf("binary at %s", s.binary())
	}
}

func TestPowerShellCommandPreservesLiteralPaths(t *testing.T) {
	t.Parallel()

	path := "C:\\Users\\O'Brien $x `test` & space\\\u7528\u6237\\a.exe"
	script := "$binary = " + powershellQuote(path)

	args := []string{
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-OutputFormat", "Text", "-EncodedCommand", encodedCommand(script),
	}
	if got := decodeCommand(
		t,
		args,
	); got != "$binary = 'C:\\Users\\O''Brien $x `test` & space\\\u7528\u6237\\a.exe'" {
		t.Errorf("literal changed: %s", got)
	}
}
