package autostart

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// always fails a command however often it is run.
const always = 1 << 30

// fakeHost records every command and fails each named in fail that many times.
type fakeHost struct {
	runs []string
	fail map[string]int
}

func (f *fakeHost) host(home string, env map[string]string) host {
	return host{
		home:   home,
		uid:    501,
		getenv: func(key string) string { return env[key] },
		run: func(_ context.Context, name string, args ...string) error {
			line := strings.Join(append([]string{name}, args...), " ")

			f.runs = append(f.runs, line)
			if f.fail[line] > 0 {
				f.fail[line]--
				return errors.New(line + " failed")
			}

			return nil
		},
		retry: time.Millisecond,
	}
}

func fakeBinary(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func renderPlist(t *testing.T, binary, log string) string {
	t.Helper()

	plist, err := render(
		"launchd.plist",
		launchdAgent{Label: launchdLabel, Binary: binary, Log: log},
	)
	if err != nil {
		t.Fatal(err)
	}

	return plist
}

func assertGone(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there: %v", path, err)
		}
	}
}

func TestLaunchdInstallsAndStarts(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	fake := &fakeHost{fail: map[string]int{
		"launchctl print gui/501/" + launchdLabel: always,
	}}
	l := launchd{fake.host(home, nil)}

	got, err := l.install(t.Context(), fakeBinary(t, "v1"))
	if err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(home, "Library", "Application Support", name, "bin", name)
	plist := filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")

	if got.Binary != bin || got.Service != plist {
		t.Errorf("installed %+v", got)
	}

	if readFile(t, bin) != "v1" {
		t.Error("the binary was not copied")
	}

	if !strings.Contains(readFile(t, plist), "<string>"+xmlText(bin)+"</string>") {
		t.Error("the plist does not run the copy")
	}

	// Nothing loaded, so nothing to unload.
	want := []string{
		"launchctl print gui/501/" + launchdLabel,
		"launchctl bootstrap gui/501 " + plist,
	}
	if !slices.Equal(fake.runs, want) {
		t.Errorf("ran %q, want %q", fake.runs, want)
	}
}

// A second install is the upgrade: the running copy must be unloaded first, or
// bootstrap refuses and the old binary keeps running.
func TestLaunchdReinstallReplacesTheRunningCopy(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	fake := &fakeHost{}
	l := launchd{fake.host(home, nil)}

	if _, err := l.install(t.Context(), fakeBinary(t, "v1")); err != nil {
		t.Fatal(err)
	}

	fake.runs = nil

	got, err := l.install(t.Context(), fakeBinary(t, "v2"))
	if err != nil {
		t.Fatal(err)
	}

	if readFile(t, got.Binary) != "v2" {
		t.Error("the copy was not replaced")
	}

	want := []string{
		"launchctl print gui/501/" + launchdLabel,
		"launchctl bootout gui/501/" + launchdLabel,
		"launchctl bootstrap gui/501 " + got.Service,
	}
	if !slices.Equal(fake.runs, want) {
		t.Errorf("ran %q, want %q", fake.runs, want)
	}
}

// The installed copy may run the install itself, as an upgrade from the mod
// would; copying a file onto itself must not empty it.
func TestInstallFromTheCopyItselfKeepsIt(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	l := launchd{(&fakeHost{}).host(home, nil)}

	got, err := l.install(t.Context(), fakeBinary(t, "v1"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := l.install(t.Context(), got.Binary); err != nil {
		t.Fatal(err)
	}

	if readFile(t, got.Binary) != "v1" {
		t.Error("installing from the copy emptied it")
	}

	info, err := os.Stat(got.Binary)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the copy is not executable: %v", info.Mode())
	}
}

func TestLaunchdUninstallLeavesNothing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	fake := &fakeHost{}
	l := launchd{fake.host(home, nil)}

	got, err := l.install(t.Context(), fakeBinary(t, "v1"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(l.log()), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(l.log(), []byte("log"), 0o600); err != nil {
		t.Fatal(err)
	}

	fake.runs = nil

	if err := l.uninstall(t.Context()); err != nil {
		t.Fatal(err)
	}

	assertGone(t, got.Binary, filepath.Dir(got.Binary), got.Service, l.log())

	if !slices.Contains(fake.runs, "launchctl bootout gui/501/"+launchdLabel) {
		t.Errorf("the agent was not unloaded: %q", fake.runs)
	}

	// The directory above holds config.env and the manual names, which stay.
	if _, err := os.Stat(filepath.Dir(filepath.Dir(got.Binary))); err != nil {
		t.Error(err)
	}
}

func TestUninstallWithNothingInstalledSucceeds(t *testing.T) {
	t.Parallel()

	fake := &fakeHost{fail: map[string]int{
		"launchctl print gui/501/" + launchdLabel: always,
	}}

	if err := (launchd{fake.host(t.TempDir(), nil)}).uninstall(t.Context()); err != nil {
		t.Error(err)
	}

	fake.runs = nil

	// With no unit there is nothing to ask systemctl, which may not even exist.
	if err := (systemd{fake.host(t.TempDir(), nil)}).uninstall(t.Context()); err != nil {
		t.Error(err)
	}

	if len(fake.runs) != 0 {
		t.Errorf("ran %q with nothing installed", fake.runs)
	}
}

// An agent just unloaded may still be leaving, and a refused bootstrap then
// would leave no agent at all.
func TestLaunchdRetriesARefusedBootstrap(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	plist := filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
	bootstrap := "launchctl bootstrap gui/501 " + plist
	fake := &fakeHost{fail: map[string]int{bootstrap: bootstrapTries - 1}}

	if _, err := (launchd{fake.host(home, nil)}).install(
		t.Context(),
		fakeBinary(t, "v1"),
	); err != nil {
		t.Fatal(err)
	}

	tries := 0

	for _, line := range fake.runs {
		if line == bootstrap {
			tries++
		}
	}

	if tries != bootstrapTries {
		t.Errorf("bootstrap ran %d times, want %d", tries, bootstrapTries)
	}
}

// A failed bootstrap is an install that did not happen, and must say so.
func TestLaunchdReportsAFailedBootstrap(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	plist := filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
	fake := &fakeHost{fail: map[string]int{
		"launchctl bootstrap gui/501 " + plist: always,
	}}

	_, err := (launchd{fake.host(home, nil)}).install(t.Context(), fakeBinary(t, "v1"))
	if err == nil {
		t.Error("a failed bootstrap was reported as installed")
	}
}

// launchd must not start an instance again after a clean exit, which is how a
// newer one taking the session over makes this one leave.
func TestLaunchdRestartsOnlyAFailure(t *testing.T) {
	t.Parallel()

	plist := renderPlist(t, "/a/b", "/a/log")
	keepAlive := "<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>"

	if !strings.Contains(plist, keepAlive) {
		t.Errorf("KeepAlive is not limited to failures:\n%s", plist)
	}
}

func TestLaunchdEscapesPaths(t *testing.T) {
	t.Parallel()

	plist := renderPlist(t, "/Users/a&b/<bin>", "/log")
	if !strings.Contains(plist, "<string>/Users/a&amp;b/&lt;bin&gt;</string>") {
		t.Errorf("the path was not escaped:\n%s", plist)
	}
}

func TestSystemdInstallsAndStarts(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	fake := &fakeHost{}
	s := systemd{fake.host(home, nil)}

	got, err := s.install(t.Context(), fakeBinary(t, "v1"))
	if err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(home, ".local", "share", name, name)
	unit := filepath.Join(home, ".config", "systemd", "user", systemdUnit)

	if got.Binary != bin || got.Service != unit {
		t.Errorf("installed %+v", got)
	}

	if !strings.Contains(readFile(t, unit), "ExecStart="+systemdQuote(bin)+" desktop\n") {
		t.Error("the unit does not run the copy")
	}

	// restart rather than start, so a second install runs the new copy.
	want := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable " + systemdUnit,
		"systemctl --user restart " + systemdUnit,
	}
	if !slices.Equal(fake.runs, want) {
		t.Errorf("ran %q, want %q", fake.runs, want)
	}
}

func TestSystemdFollowsXDG(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	data, config := filepath.Join(home, "data"), filepath.Join(home, "config")
	s := systemd{(&fakeHost{}).host(home, map[string]string{
		"XDG_DATA_HOME":   data,
		"XDG_CONFIG_HOME": config,
	})}

	if got := s.binary(); got != filepath.Join(data, name, name) {
		t.Errorf("binary at %s", got)
	}

	if got := s.unit(); got != filepath.Join(config, "systemd", "user", systemdUnit) {
		t.Errorf("unit at %s", got)
	}

	// The specification says to ignore a relative value.
	relative := systemd{(&fakeHost{}).host(home, map[string]string{"XDG_DATA_HOME": "data"})}
	if got := relative.binary(); got != filepath.Join(home, ".local", "share", name, name) {
		t.Errorf("a relative XDG_DATA_HOME was followed: %s", got)
	}
}

func TestSystemdUninstallLeavesNothing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	fake := &fakeHost{}
	s := systemd{fake.host(home, nil)}

	got, err := s.install(t.Context(), fakeBinary(t, "v1"))
	if err != nil {
		t.Fatal(err)
	}

	fake.runs = nil

	if err := s.uninstall(t.Context()); err != nil {
		t.Fatal(err)
	}

	assertGone(t, got.Binary, filepath.Dir(got.Binary), got.Service)

	want := []string{
		"systemctl --user disable --now " + systemdUnit,
		"systemctl --user daemon-reload",
	}
	if !slices.Equal(fake.runs, want) {
		t.Errorf("ran %q, want %q", fake.runs, want)
	}
}

// systemd must not start an instance again after a clean exit either.
func TestSystemdRestartsOnlyAFailure(t *testing.T) {
	t.Parallel()

	unit, err := render("systemd.service", systemdService{Binary: "/a/b"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(unit, "\nRestart=on-failure\n") {
		t.Errorf("Restart is not limited to failures:\n%s", unit)
	}
}

// systemd expands % and $ in ExecStart, and splits it on spaces.
func TestSystemdQuotesTheBinary(t *testing.T) {
	t.Parallel()

	got := systemdQuote(`/home/a b/100%/$x/"q"\`)
	want := `"/home/a b/100%%/$$x/\"q\"\\"`

	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
