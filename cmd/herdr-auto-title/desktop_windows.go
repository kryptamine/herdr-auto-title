package main

import (
	"context"
	"os"

	"github.com/kryptamine/herdr-auto-title/internal/autostart"
)

func desktopService(ctx context.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	return autostart.Serve(ctx, exe)
}
