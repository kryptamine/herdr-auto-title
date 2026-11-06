package autostart

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec" //nolint:depguard // launchctl and systemctl, given fixed arguments and no shell
	"runtime"
	"strings"
	"time"
)

// bootstrapRetry is how long launchd is given between two refused bootstraps.
const bootstrapRetry = 500 * time.Millisecond

// current is this OS's service. Windows is a change of its own.
func current() (service, error) {
	h, err := newHost()
	if err != nil {
		return nil, err
	}

	switch runtime.GOOS {
	case "darwin":
		return launchd{h}, nil
	case "linux":
		return systemd{h}, nil
	default:
		return nil, fmt.Errorf("starting at login is not supported on %s yet", runtime.GOOS)
	}
}

func newHost() (host, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return host{}, fmt.Errorf("locate the home directory: %w", err)
	}

	return host{
		home:   home,
		uid:    os.Getuid(),
		getenv: os.Getenv,
		run:    run,
		retry:  bootstrapRetry,
	}, nil
}

// run runs a service manager's command and returns what it printed, which on
// a failure goes into the error, since that is where the manager says why.
func run(ctx context.Context, name string, args ...string) (string, error) {
	//nolint:forbidigo,gosec // the arguments are fixed paths and labels, and no shell
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"%s %s: %w: %s",
			name,
			strings.Join(args, " "),
			err,
			bytes.TrimSpace(out),
		)
	}

	return string(out), nil
}
