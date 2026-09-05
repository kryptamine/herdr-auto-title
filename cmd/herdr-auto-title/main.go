// Command herdr-auto-title is the Herdr Auto Title plugin process: launched once
// through a startup hook, it stays alive and keeps every tab's title in step
// with what that tab is doing.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kryptamine/herdr-auto-title/internal/app"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
)

func main() {
	if err := run(); err != nil {
		slog.Error("auto title stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
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

	log.Info("starting auto title", "poll", cfg.Poll, "max_length", cfg.MaxLength)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := herdr.New()
	if err != nil {
		return err
	}

	chain := resolver.Default(resolver.Options{
		MaxLength:     cfg.MaxLength,
		BranchMax:     cfg.BranchMax,
		HideAgentName: !cfg.ShowAgentName,
	})

	var titles resolver.TitleResolver = chain
	if cfg.ShowPosition {
		titles = resolver.NewNumbered(chain, cfg.MaxLength)
	}

	app.New(cfg, log, titles).Run(ctx, client)

	return nil
}
