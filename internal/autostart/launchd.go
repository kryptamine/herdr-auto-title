package autostart

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// launchdLabel names the agent to launchd, and its plist.
const launchdLabel = "io.github.kryptamine.herdr-auto-title"

// bootstrapTries is how many times bootstrap is asked before an install fails.
const bootstrapTries = 3

// launchd starts the desktop mode as a LaunchAgent of the logged-in user.
type launchd struct{ host }

func (l launchd) binary() string {
	return filepath.Join(l.home, "Library", "Application Support", name, "bin", name)
}

func (l launchd) plist() string {
	return filepath.Join(l.home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func (l launchd) log() string {
	return filepath.Join(l.home, "Library", "Logs", name+".log")
}

func (l launchd) domain() string { return "gui/" + strconv.Itoa(l.uid) }

func (l launchd) target() string { return l.domain() + "/" + launchdLabel }

func (l launchd) install(ctx context.Context, exe string) (Installed, error) {
	if err := copyBinary(exe, l.binary()); err != nil {
		return Installed{}, fmt.Errorf("copy the binary: %w", err)
	}

	plist, err := render("launchd.plist", launchdAgent{
		Label:  launchdLabel,
		Binary: l.binary(),
		Log:    l.log(),
	})
	if err == nil {
		err = writeFile(l.plist(), plist)
	}

	if err != nil {
		return Installed{}, fmt.Errorf("write the launch agent: %w", err)
	}

	// bootstrap refuses a label already loaded, and a reload is what makes it
	// read a changed plist and start the new copy.
	if err := l.stop(ctx); err != nil {
		return Installed{}, err
	}

	if err := l.bootstrap(ctx); err != nil {
		return Installed{}, err
	}

	return Installed{Binary: l.binary(), Service: l.plist()}, nil
}

func (l launchd) uninstall(ctx context.Context) error {
	if err := l.stop(ctx); err != nil {
		return err
	}

	if err := remove(l.plist(), l.log()); err != nil {
		return err
	}

	return removeBinary(l.binary())
}

// bootstrap loads the agent, which starts it. An agent just unloaded can still
// be leaving, which bootstrap refuses, so a refusal is tried again a few times
// rather than leaving no agent at all.
func (l launchd) bootstrap(ctx context.Context) error {
	var err error

	for range bootstrapTries {
		if _, err = l.run(ctx, "launchctl", "bootstrap", l.domain(), l.plist()); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.retry):
		}
	}

	return err
}

// stop unloads the agent if launchd has it loaded, which ends the process.
func (l launchd) stop(ctx context.Context) error {
	if _, err := l.run(ctx, "launchctl", "print", l.target()); err != nil {
		return nil //nolint:nilerr // print fails for an agent not loaded, which is stopped
	}

	_, err := l.run(ctx, "launchctl", "bootout", l.target())

	return err
}

// running is whether launchd has the agent loaded and its process up: a loaded
// agent that failed waits out its throttle as "not running".
func (l launchd) running(ctx context.Context) bool {
	out, err := l.run(ctx, "launchctl", "print", l.target())

	return err == nil && strings.Contains(out, "state = running")
}
