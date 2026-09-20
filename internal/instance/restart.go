package instance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// Restart starts exe as a fresh instance, detached from this process, and
// returns the pid naming the session once it is polling and the instance it
// displaced has left: its own, or a newer restart's that took it over.
func Restart(ctx context.Context, path, exe string, timeout time.Duration) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	// Without a claim the running instance would never leave, and the new one
	// could not be waited for: two would run, so none is started.
	if path == "" {
		return 0, errors.New("no configuration directory to claim the session in")
	}

	old := holder(path)

	proc, err := start(exe)
	if err != nil {
		return 0, fmt.Errorf("start %s: %w", exe, err)
	}

	// Waited on so an instance that dies at once is reported as such, and so
	// that it is reaped, which is what lets alive see it gone.
	exited := make(chan *os.ProcessState, 1)

	go func() {
		state, _ := proc.Wait()
		exited <- state
	}()

	var state *os.ProcessState

	done := waitUntil(ctx, timeout, func() bool {
		select {
		case state = <-exited:
			return true
		default:
			return pending(path, proc.Pid, old) == nil
		}
	})

	switch {
	case state != nil:
		// An instance started a moment later claims the session and this one
		// leaves for it, which is a restart that happened, not one that failed.
		if newer := holder(path); newer != 0 && newer != old {
			return newer, nil
		}

		return 0, fmt.Errorf(
			"the new instance %d exited before it read the session: %v",
			proc.Pid,
			state,
		)
	case done:
		return proc.Pid, nil
	case ctx.Err() != nil:
		return 0, fmt.Errorf(
			"%w; the new instance %d is left to take over on its own",
			ctx.Err(),
			proc.Pid,
		)
	default:
		return 0, fmt.Errorf("gave up after %s: %w", timeout, pending(path, proc.Pid, old))
	}
}

// pending is what a restart is still waiting for, or nil once the new instance
// is polling and the one it displaced has gone.
func pending(path string, pid, old int) error {
	switch {
	case holder(path) != pid:
		return fmt.Errorf("the new instance %d has not claimed the session", pid)
	case !marked(path, pid):
		return fmt.Errorf("the new instance %d has not read the session", pid)
	case old != 0 && alive(old):
		return fmt.Errorf(
			"the new instance %d is polling, but the one before it, %d, is still running; `herdr server stop` ends it",
			pid,
			old,
		)
	default:
		return nil
	}
}

// start runs exe with no arguments, holding nothing of this process: stdio on
// the null device, because Herdr reads an action's output to its end, and no
// terminal or process group, so the instance outlives the action that made it.
func start(exe string) (*os.Process, error) {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer null.Close()

	// The program is this binary's own path and the arguments are fixed, so
	// nothing terminal-derived is run here.
	return os.StartProcess(exe, []string{exe}, &os.ProcAttr{
		Env:   os.Environ(),
		Files: []*os.File{null, null, null},
		Sys:   detached(),
	})
}
