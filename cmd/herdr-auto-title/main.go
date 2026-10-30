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
)

func main() {
	if err := execute(os.Args[1:]); err != nil {
		slog.Error("auto title stopped", "error", err)
		os.Exit(1)
	}
}

func execute(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch {
	case len(args) == 0:
		return run(ctx)
	case len(args) == 1 && args[0] == "restart":
		return restart(ctx)
	case len(args) == 1 && args[0] == "--version":
		fmt.Println("herdr-auto-title", version)

		return nil
	default:
		return errors.New("usage: herdr-auto-title [restart | --version]")
	}
}

func run(ctx context.Context) error {
	cfg, log := app.Configure()

	client, err := herdr.New()
	if err != nil {
		return err
	}

	app.Serve(ctx, cfg, log, client)

	return nil
}

func restart(ctx context.Context) error {
	_, log := app.Configure()

	client, err := herdr.New()
	if err != nil {
		return err
	}

	return app.Restart(ctx, log, client)
}
