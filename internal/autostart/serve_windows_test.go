//go:build windows && !autostart_integration

package autostart

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "desktop" {
		for {
			time.Sleep(time.Hour)
		}
	}

	os.Exit(m.Run())
}

func TestDesktopJobKillsItsChildOnClose(t *testing.T) {
	t.Parallel()

	job, err := desktopJob()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = windows.CloseHandle(job) })

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	process, err := startDesktopProcess(job, exe)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = windows.CloseHandle(process) })

	if err := windows.CloseHandle(job); err != nil {
		t.Fatal(err)
	}

	job = 0

	event, err := windows.WaitForSingleObject(process, 5000)
	if err != nil || event != windows.WAIT_OBJECT_0 {
		t.Fatalf("the child outlived its job: event %d, error %v", event, err)
	}
}

func TestDesktopProcessCancellation(t *testing.T) {
	t.Parallel()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	if _, err := runDesktopProcess(ctx, exe); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled process returned %v", err)
	}
}
