package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/instance"
)

// Configure reads the configuration and makes a logger at the level it asks for.
func Configure() (Config, *slog.Logger) {
	cfg, warnings := loadConfig()

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	for _, warning := range warnings {
		log.Warn(warning)
	}

	return cfg, log
}

// Serve claims the session and names its tabs until ctx ends or a newer instance
// or server takes over.
func Serve(ctx context.Context, cfg Config, log *slog.Logger, client herdr.Client) {
	log.Info("starting auto title", "poll", cfg.Poll, "max_length", cfg.MaxLength)

	// Take waits for the instance it displaces, which looks at the claim
	// before every poll; the manual-name locks are loaded only once it has.
	claim, stayed, err := instance.Take(ctx, instance.File(stateDir(), client.Path()), leaveTimeout)
	if err != nil {
		log.Warn(
			"the session could not be claimed, so a restart will not end this instance",
			"error",
			err,
		)
	}

	if stayed != 0 {
		log.Warn("the instance this one replaces is still running", "pid", stayed)
	}

	titles, panes, topics := resolvers(cfg)
	newApp(cfg, log, titles, panes, topics, claim).Run(ctx, client)
}

// Restart starts a fresh instance in this one's session and says how that went,
// in the log Herdr keeps of the action and in a notification.
func Restart(ctx context.Context, log *slog.Logger, client *herdr.SocketClient) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// The instance being replaced has leaveTimeout to go, and the new one a
	// first poll after that.
	pid, err := instance.Restart(
		ctx,
		instance.File(stateDir(), client.Path()),
		exe,
		leaveTimeout+pollTimeout,
	)

	title, body := "Auto Title restarted", fmt.Sprintf("pid %d is naming the session", pid)
	if err != nil {
		title, body = "Auto Title did not restart", err.Error()
	}

	notice, notifyErr := client.ShowNotification(ctx, title, body)

	switch {
	case notifyErr != nil:
		log.Warn("notification failed", "error", notifyErr)
	case !notice.Shown:
		log.Debug("notification not shown", "reason", notice.Reason)
	}

	if err != nil {
		return err
	}

	log.Info(title, "pid", pid)

	return nil
}
