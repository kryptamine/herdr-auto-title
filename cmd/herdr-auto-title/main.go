// Command herdr-auto-title is the Herdr Auto Title plugin process: started once,
// it stays alive and keeps every tab's title in step with what that tab is
// doing. `herdr-auto-title restart` starts a fresh one in its place.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kryptamine/herdr-auto-title/internal/app"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/instance"
)

func main() {
	var err error

	switch {
	case len(os.Args) == 1:
		err = run()
	case len(os.Args) == 2 && os.Args[1] == "restart":
		err = restart()
	default:
		err = errors.New("usage: herdr-auto-title [restart]")
	}

	if err != nil {
		slog.Error("auto title stopped", "error", err)
		os.Exit(1)
	}
}

// setup reads the configuration, makes a logger at the level it asks for, and
// a client for the session this process was started in.
func setup() (app.Config, *slog.Logger, *herdr.SocketClient, error) {
	cfg, warnings := app.LoadConfig()

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	for _, warning := range warnings {
		log.Warn(warning)
	}

	client, err := herdr.New()

	return cfg, log, client, err
}

func run() error {
	cfg, log, client, err := setup()
	if err != nil {
		return err
	}

	log.Info("starting auto title", "poll", cfg.Poll, "max_length", cfg.MaxLength)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	claim, err := instance.Take(instance.File(app.StateDir(), client.Path()))
	if err != nil {
		log.Warn(
			"the session could not be claimed, so a restart will not end this instance",
			"error",
			err,
		)
	}
	defer claim.Release()

	// The displaced instance looks at the claim before every poll, and leaves
	// on its own; the manual-name locks are loaded only once it has.
	if old := claim.Displaced(); old != 0 {
		log.Info("waiting for the instance this one replaces to leave", "pid", old)

		if !claim.AwaitDisplaced(ctx, app.LeaveTimeout) {
			log.Warn("the instance this one replaces is still running", "pid", old)
		}
	}

	titles, panes := app.Resolvers(cfg)
	app.New(cfg, log, titles, panes, claim).Run(ctx, client)

	return nil
}

// restart starts a fresh instance in this one's session and says how that went,
// in the log Herdr keeps of the action and in a notification.
func restart() error {
	_, log, client, err := setup()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	pid, err := instance.Restart(ctx, instance.File(app.StateDir(), client.Path()), exe)

	title, body := "Auto Title restarted", fmt.Sprintf("pid %d is naming the session", pid)
	if err != nil {
		title, body = "Auto Title did not restart", err.Error()
	}

	shown, reason, notifyErr := herdr.ShowNotification(ctx, client, title, body)

	switch {
	case notifyErr != nil:
		log.Warn("notification failed", "error", notifyErr)
	case !shown:
		log.Debug("notification not shown", "reason", reason)
	}

	if err != nil {
		return err
	}

	log.Info(title, "pid", pid)

	return nil
}
