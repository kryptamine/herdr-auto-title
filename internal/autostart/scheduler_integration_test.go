//go:build windows && autostart_integration

package autostart

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestMain makes the copied test binary a harmless task action; it never opens
// Herdr's sockets. The tag keeps real task registration out of the normal gate.
func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "desktop" && os.Args[2] == "service" {
		os.Exit(schedulerHelper("service"))
	}

	if len(os.Args) == 2 && !strings.HasPrefix(os.Args[1], "-") {
		os.Exit(schedulerHelper(os.Args[1]))
	}

	os.Exit(m.Run())
}

func schedulerHelper(operation string) int {
	exe, err := os.Executable()
	if err != nil {
		return 1
	}

	local := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	s := fixtureScheduler(local)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch operation {
	case "self-install":
		_, err = s.install(ctx, exe)
	case "self-uninstall":
		err = s.uninstall(ctx)
	case "desktop":
		return fixtureDesktop(local)
	case "service":
		if err := os.WriteFile(
			filepath.Join(local, "service.pid"),
			[]byte(strconv.Itoa(os.Getpid())),
			0o600,
		); err != nil {
			return 1
		}

		err = Serve(ctx, exe)
	default:
		return 1
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

func fixtureDesktop(local string) int {
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(filepath.Join(local, "worker.pid"), []byte(pid), 0o600); err != nil {
		return 1
	}

	for {
		if data, err := os.ReadFile(filepath.Join(local, "exit-code")); err == nil {
			code, err := strconv.Atoi(string(data))
			if err != nil {
				return 1
			}

			return code
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func fixtureScheduler(local string) scheduler {
	return scheduler{host{
		home: local,
		getenv: func(key string) string {
			if key == "LOCALAPPDATA" {
				return local
			}

			return ""
		},
		run: func(ctx context.Context, program string, args ...string) (string, error) {
			// Only the task name differs; the installed action and all operations are real.
			return fixtureCommand(ctx, local, program, args)
		},
	}}
}

func fixtureCommand(ctx context.Context, local, program string, args []string) (string, error) {
	data, err := decodeEncodedCommand(args[len(args)-1])
	if err != nil {
		return "", err
	}

	taskName := fmt.Sprintf("herdr-auto-title-test-%x", sha256.Sum256([]byte(local)))

	anchor := "$taskName = 'herdr-auto-title-' + $sid"
	if strings.Count(data, anchor) != 1 {
		return "", errors.New("refusing to run a fixture without an isolated task name")
	}

	script := strings.Replace(data, anchor,
		"$taskName = "+powershellQuote(taskName), 1)
	args[len(args)-1] = encodedCommand(script)

	return run(ctx, program, args...)
}

func waitForScheduler(t *testing.T, ready func() bool) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}

		time.Sleep(250 * time.Millisecond)
	}

	t.Fatal("the scheduled task did not reach the expected state")
}

func workerPID(local string) int {
	data, _ := os.ReadFile(filepath.Join(local, "worker.pid"))
	pid, _ := strconv.Atoi(string(data))

	return pid
}

func TestSchedulerLifecycle(t *testing.T) {
	t.Parallel()

	local := filepath.Join(t.TempDir(), "a user's space \u7528\u6237")
	s := fixtureScheduler(local)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		if err := s.uninstall(ctx); err != nil {
			t.Errorf("remove the fixture task: %v", err)
		}
	})

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	if s.running(t.Context()) {
		t.Fatal("the fixture task already exists")
	}

	if _, err := s.install(t.Context(), exe); err != nil {
		t.Fatal(err)
	}

	waitForScheduler(t, func() bool { return workerPID(local) != 0 })

	first := workerPID(local)

	if !s.running(t.Context()) {
		t.Fatal("the installed task is not running")
	}

	if _, err := s.install(t.Context(), exe); err != nil {
		t.Fatal(err)
	}

	waitForScheduler(t, func() bool { return workerPID(local) != first })
	t.Log("install and replacement of a running copy passed")

	before := workerPID(local)
	if err := os.WriteFile(filepath.Join(local, "exit-code"), []byte("7"), 0o600); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	if err := os.Remove(filepath.Join(local, "exit-code")); err != nil {
		t.Fatal(err)
	}

	waitForScheduler(t, func() bool { return workerPID(local) != before })
	t.Log("the task restarted a failed child")

	before = workerPID(local)

	process, err := os.FindProcess(before)
	if err != nil {
		t.Fatal(err)
	}

	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}

	_ = process.Release()

	waitForScheduler(t, func() bool { return workerPID(local) != before })
	t.Log("the task restarted a killed child")

	if err := os.WriteFile(filepath.Join(local, "exit-code"), []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}

	waitForScheduler(t, func() bool { return !s.running(t.Context()) })

	cleanPID := workerPID(local)

	time.Sleep(10 * time.Second)

	if s.running(t.Context()) || workerPID(local) != cleanPID {
		t.Fatal("the task restarted a clean exit")
	}

	t.Log("a clean exit stayed stopped beyond the restart interval")

	if err := os.Remove(filepath.Join(local, "exit-code")); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t.Context(), s.binary(), "self-install"); err != nil {
		t.Fatal(err)
	}

	waitForScheduler(t, func() bool { return workerPID(local) != cleanPID })
	assertSupervisorOwnsChild(t, local)

	if _, err := run(t.Context(), s.binary(), "self-install"); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t.Context(), s.binary(), "self-uninstall"); err != nil {
		t.Fatal(err)
	}

	waitForScheduler(t, func() bool {
		_, err := os.Stat(filepath.Dir(s.binary()))
		return os.IsNotExist(err)
	})

	if s.running(t.Context()) {
		t.Fatal("the removed task is still running")
	}

	if err := s.uninstall(t.Context()); err != nil {
		t.Fatal(err)
	}

	t.Log("self-install, self-uninstall and repeated uninstall passed")
}

func assertSupervisorOwnsChild(t *testing.T, local string) {
	t.Helper()

	child, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(workerPID(local)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(child) }()

	data, err := os.ReadFile(filepath.Join(local, "service.pid"))
	if err != nil {
		t.Fatal(err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Release() }()

	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}

	event, err := windows.WaitForSingleObject(child, 5000)
	if err != nil || event != windows.WAIT_OBJECT_0 {
		t.Fatalf("killing the supervisor orphaned its child: event %d, error %v", event, err)
	}

	t.Log("killing the supervisor also terminated its owned child")
}
