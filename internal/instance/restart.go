package instance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// restartTimeout bounds a restart: the displaced instance gets ten seconds to
// leave and the new one a first poll after that.
const restartTimeout = 15 * time.Second

// Restart starts exe as a fresh instance, detached from this process, and
// returns its pid once it has read the session and the instance it displaced
// has left.
func Restart(ctx context.Context, path, exe string) (int, error) {
	return restart(ctx, path, exe, restartTimeout)
}

func restart(ctx context.Context, path, exe string, timeout time.Duration) (int, error) {
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

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf(
				"%w; the new instance %d is left to take over on its own",
				ctx.Err(),
				proc.Pid,
			)
		case state := <-exited:
			return 0, fmt.Errorf(
				"the new instance %d exited before it read the session: %v",
				proc.Pid,
				state,
			)
		case <-deadline.C:
			return 0, stalled(path, proc.Pid, old, timeout)
		case <-ticker.C:
			if holder(path) == proc.Pid && ready(path) && (old == 0 || !alive(old)) {
				return proc.Pid, nil
			}
		}
	}
}

// stalled says which of the three things a restart waits for did not happen.
func stalled(path string, pid, old int, timeout time.Duration) error {
	switch {
	case holder(path) != pid:
		return fmt.Errorf("the new instance %d has not claimed the session after %s", pid, timeout)
	case !ready(path):
		return fmt.Errorf("the new instance %d has not read the session after %s", pid, timeout)
	default:
		return fmt.Errorf(
			"the new instance %d is polling, but the one before it, %d, is still running; `herdr server stop` ends it",
			pid,
			old,
		)
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
