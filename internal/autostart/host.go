package autostart

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec" //nolint:depguard // service managers and an embedded script, never terminal text
	"runtime"
	"strings"
	"time"
)

// bootstrapRetry is how long launchd is given between two refused bootstraps.
const bootstrapRetry = 500 * time.Millisecond

// current is this OS's service.
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
	case "windows":
		return scheduler{h}, nil
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
	//nolint:forbidigo,gosec // fixed service commands or an embedded script with literal paths
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)

	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(args) >= 2 && args[len(args)-2] == "-EncodedCommand" {
			args = append(args[:len(args)-1], "<script>")
		}

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
