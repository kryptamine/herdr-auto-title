// Command herdr-auto-title keeps every tab's title in step with what the tab is
// doing, for as long as it runs. `restart` starts a fresh one in its place, and
// `desktop install` has the OS run the desktop mode from login.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kryptamine/herdr-auto-title/internal/app"
	"github.com/kryptamine/herdr-auto-title/internal/autostart"
	"github.com/kryptamine/herdr-auto-title/internal/desktop"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

var errUsage = errors.New(
	"usage: herdr-auto-title [desktop [install | uninstall | status] | restart | --version]",
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
	case len(args) >= 1 && args[0] == "desktop":
		return desktopCommand(ctx, args[1:])
	case len(args) == 1 && args[0] == "restart":
		return restart(ctx)
	case len(args) == 1 && args[0] == "--version":
		fmt.Println("herdr-auto-title", version)

		return nil
	default:
		return errUsage
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

func desktopCommand(ctx context.Context, args []string) error {
	switch {
	case len(args) == 0:
		return runDesktop(ctx)
	case len(args) == 1 && args[0] == "install":
		return install(ctx)
	case len(args) == 1 && args[0] == "uninstall":
		return autostart.Uninstall(ctx)
	case len(args) == 1 && args[0] == "status":
		return status(ctx, os.Stdout)
	default:
		return errUsage
	}
}

func runDesktop(ctx context.Context) error {
	cfg, log := app.Configure()

	client, err := desktop.New(cfg.ClaudeDirs)
	if err != nil {
		return err
	}

	app.Serve(ctx, cfg.ForDesktop(), log, client)

	return nil
}

func install(ctx context.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	installed, err := autostart.Install(ctx, exe)
	if err != nil {
		return err
	}

	fmt.Println("installed", installed.Binary)
	fmt.Println("started at login by", installed.Service)

	return nil
}

// desktopStatus is what `desktop status` prints, for the gallery mod to read.
type desktopStatus struct {
	Version string `json:"version"`
	Running bool   `json:"running"`
}

func status(ctx context.Context, w io.Writer) error {
	running, err := autostart.Running(ctx)
	if err != nil {
		return err
	}

	return json.NewEncoder(w).Encode(desktopStatus{Version: version, Running: running})
}

func restart(ctx context.Context) error {
	_, log := app.Configure()

	client, err := herdr.New()
	if err != nil {
		return err
	}

	return app.Restart(ctx, log, client)
}
