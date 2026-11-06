package autostart

import (
	"context"
	"fmt"
	"log/slog"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	desktopRestartDelay = 5 * time.Second
	processWaitMS       = 100
	// procThreadAttributeJobList assigns ownership atomically with process creation.
	procThreadAttributeJobList = 0x2000d
)

// Serve owns the desktop poller in a kill-on-close job and restarts failures.
// Task Scheduler's failure retry does not reliably retry a process's exit code:
// docs/architecture/desktop-api.md#starting-at-login.
func Serve(ctx context.Context, exe string) error {
	_, _, _ = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole").Call()

	for ctx.Err() == nil {
		code, err := runDesktopProcess(ctx, exe)
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancelling the service is a successful stop
		}

		if err == nil && code == 0 {
			return nil
		}

		if err != nil {
			slog.Error("desktop poller failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(desktopRestartDelay):
		}
	}

	return nil
}

func runDesktopProcess(ctx context.Context, exe string) (uint32, error) {
	job, err := desktopJob()
	if err != nil {
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(job) }()

	process, err := startDesktopProcess(job, exe)
	if err != nil {
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(process) }()

	for ctx.Err() == nil {
		event, err := windows.WaitForSingleObject(process, processWaitMS)
		if err != nil {
			return 0, err
		}

		if event == windows.WAIT_OBJECT_0 {
			var code uint32

			err := windows.GetExitCodeProcess(process, &code)

			return code, err
		}
	}

	return 0, ctx.Err()
}

func desktopJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION

	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	//nolint:gosec // the Windows binding consumes this exact native structure synchronously
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}

	return job, nil
}

func startDesktopProcess(job windows.Handle, exe string) (windows.Handle, error) {
	app, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}

	command, err := windows.UTF16PtrFromString(syscall.EscapeArg(exe) + " desktop")
	if err != nil {
		return 0, err
	}

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return 0, err
	}
	defer attributes.Delete()

	//nolint:gosec // the attribute owns a reference to this live job handle through CreateProcess
	if err := attributes.Update(
		procThreadAttributeJobList,
		unsafe.Pointer(&job),
		unsafe.Sizeof(job),
	); err != nil {
		return 0, err
	}

	var info windows.ProcessInformation

	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))

	err = windows.CreateProcess(app, command, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_NO_WINDOW, nil, nil,
		&startup.StartupInfo, &info)
	if err != nil {
		return 0, fmt.Errorf("start the desktop poller: %w", err)
	}

	_ = windows.CloseHandle(info.Thread)

	return info.Process, nil
}
