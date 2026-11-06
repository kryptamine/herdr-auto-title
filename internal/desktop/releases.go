package desktop

import (
	"context"
	"errors"
	"log/slog"

	"github.com/kryptamine/herdr-auto-title/internal/release"
)

// modName is the gallery mod's folder in the app's mods.
const modName = "auto-title"

// OfferReleases offers each release newer than current in the gallery mod's
// popup until ctx ends, keeping the one offered at path. No path, no offers.
func (c *Client) OfferReleases(ctx context.Context, log *slog.Logger, current, path string) {
	if path == "" {
		return
	}

	release.Watcher{
		Current:  current,
		Releases: release.Releases,
		Path:     path,
		Show:     func(ctx context.Context) error { return c.showMod(ctx, log) },
		Log:      log,
	}.Run(ctx)
}

// showMod opens the gallery mod's popup; the app toggles it, so one already
// open closes instead. One installed by hand has no mod, and the app's refusal
// says so for good, so it is not asked again.
func (c *Client) showMod(ctx context.Context, log *slog.Logger) error {
	request := map[string]any{"do": "mod.open", "args": map[string]string{"name": modName}}

	var reply struct {
		Error string `json:"error"`
	}

	if err := ask(ctx, c.app, request, &reply); err != nil {
		return err
	}

	err := appError("mod.open", reply.Error)
	if errors.Is(err, errRefused) {
		log.Info("the newer release can be installed with desktop install", "error", err)

		return nil
	}

	return err
}
