// Package app polls the Herdr session and keeps every tab's title in step with
// what that tab is doing.
package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// pollTimeout bounds one poll: a snapshot and the renames it decides on.
const pollTimeout = 5 * time.Second

// App is one run of the Auto Title loop.
type App struct {
	cfg     Config
	log     *slog.Logger
	titles  resolver.TitleResolver
	changes *state.Changes
	manual  *state.Manual
	// topics reads what an agent's own session is about. It keeps its place in
	// each transcript, so a poll reads only what was appended since the last.
	topics *claude.Reader
	// failures is the run of polls that have failed in a row, which decides
	// how loudly the next one is reported.
	failures failureLog
}

// New builds the application. The connection is supplied to Run, so the same
// App can be driven by a real socket or by a stub in tests.
func New(cfg Config, log *slog.Logger, titles resolver.TitleResolver) *App {
	return &App{
		cfg:     cfg,
		log:     log,
		titles:  titles,
		changes: state.NewChanges(),
		manual:  state.LoadManual(cfg.ManualPath),
		topics:  claude.NewReader(),
	}
}

// Run polls the session until the context is cancelled. Herdr's event stream is
// deliberately not used, and the measurements that settled that are in
// docs/architecture/poll-loop.md.
func (a *App) Run(ctx context.Context, client herdr.Client) {
	// Name what already exists before waiting for the first tick.
	a.poll(ctx, client)

	ticker := time.NewTicker(a.cfg.Poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("shutting down")
			return
		case <-ticker.C:
			a.poll(ctx, client)
		}
	}
}

// poll is one turn of the loop. No failure is fatal, the first one included: a
// plugin that gave up would stay dead, and Herdr's socket can be a moment
// behind the process it just launched.
func (a *App) poll(ctx context.Context, client herdr.Client) {
	err := a.readAndRename(ctx, client)
	if ctx.Err() != nil {
		return
	}

	if err != nil {
		if run := a.failures.failed(); run > 0 {
			a.log.Warn("poll failed", "error", err, "in a row", run)
		}

		return
	}

	if run := a.failures.recovered(); run > 0 {
		a.log.Info("the session is answering again", "polls missed", run)
	}
}

// readAndRename reads the session and renames every tab whose title no longer
// fits.
func (a *App) readAndRename(ctx context.Context, client herdr.Client) error {
	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	snapshot, err := herdr.SessionSnapshot(ctx, client)
	if err != nil {
		return err
	}

	a.changes.Observe(snapshot.Panes)
	a.topics.Retain(sessionsIn(snapshot.Panes))
	// Taken from the snapshot rather than from the tabs below, because this is
	// what decides which of them are locked, and a locked tab is never read.
	a.manual.Retain(labelsIn(snapshot.Tabs))

	tabs := a.tabsIn(snapshot)
	// One memo for this poll, because the tabs of a project share a directory.
	checkouts := make(checkoutMemo, len(tabs))

	for _, tab := range tabs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if a.manual.Locked(tab.ID) {
			continue
		}

		// The reads happen here rather than during assembly: they are what a
		// poll spends, and this is where it is known they will be used. The
		// resolver picks the same pane again, which the choice being made from
		// state alone is what guarantees.
		a.readInto(ctx, client, state.SelectContextPane(tab), checkouts)

		decision := a.titles.Resolve(tab)
		if a.manual.Observe(state.SightingFrom(tab, decision.Name)) {
			a.log.Info("leaving a tab the user renamed", "tab_id", tab.ID, "name", tab.CurrentName)
			continue
		}

		if decision.Name == "" || decision.Name == tab.CurrentName {
			continue
		}

		a.rename(ctx, client, tab, decision)
	}

	// Reached only when every tab was seen. Deferring this would settle after a
	// poll cut short, and the tabs it missed would look new and already named.
	a.manual.Settled()

	return nil
}

// rename gives the tab the name the resolver chose. Nothing that goes wrong
// here is worth cutting the poll short: the next one decides again from state
// it has read again.
func (a *App) rename(
	ctx context.Context,
	client herdr.Client,
	tab state.TabState,
	decision resolver.Decision,
) {
	if err := herdr.RenameTab(ctx, client, tab.ID, decision.Name); err != nil {
		if herdr.ErrorCode(err) == herdr.CodeTabNotFound {
			// The tab closed between the snapshot and the rename. The next
			// poll will not see it at all.
			a.log.Debug("tab closed before it could be renamed", "tab_id", tab.ID)
			return
		}

		a.log.Warn("rename failed", "tab_id", tab.ID, "name", decision.Name, "error", err)

		return
	}

	// Recorded before the log line so the next poll cannot read this rename as
	// the user's.
	a.manual.Applied(tab.ID, decision.Name)
	a.log.Info("tab renamed",
		"tab_id", tab.ID,
		"old", tab.CurrentName,
		"new", decision.Name,
		"reason", decision.Reason,
		"confidence", decision.Confidence,
	)
}

// labelsIn indexes the session's tabs by id for the manual-name bookkeeping,
// which needs both an id that is gone and a label that has moved on.
func labelsIn(tabs []herdr.TabInfo) map[string]string {
	labels := make(map[string]string, len(tabs))
	for _, tab := range tabs {
		labels[tab.TabID] = tab.Label
	}

	return labels
}
