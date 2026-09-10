// Package app polls the Herdr session and keeps every tab's title in step with
// what that tab is doing.
package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// pollTimeout bounds one poll: a snapshot and the renames it decides on.
const pollTimeout = 5 * time.Second

// App is one run of the Auto Title loop.
type App struct {
	// pollEvery is how often the session is read, which is all the loop itself
	// decides anything by.
	pollEvery time.Duration
	log       *slog.Logger
	titles    resolver.TitleResolver
	changes   *state.Changes
	manual    *state.Manual
	reads     *paneReader
	scroll    *scroller
	// failures is the run of polls that have failed in a row, which decides
	// how loudly the next one is reported.
	failures failureLog
	// server identifies the Herdr this instance answers to, learned from the
	// first poll that could read it. Another server on the socket has started
	// an instance of its own, and this one leaves rather than double it.
	server string
}

// New builds the application. The client belongs to Run rather than to the
// App, so one App can be driven by any connection.
func New(cfg Config, log *slog.Logger) *App {
	changes := state.NewChanges()
	scroll := newScroller(cfg.ScrollStep)

	opts := resolver.Options{
		MaxLength:     cfg.MaxLength,
		BranchMax:     cfg.BranchMax,
		HideAgentName: !cfg.ShowAgentName,
		ShowPosition:  cfg.ShowPosition,
	}
	if cfg.Scroll {
		opts.Fit = scroll.Fit
	}

	return &App{
		pollEvery: cfg.Poll,
		log:       log,
		titles:    resolver.Default(opts),
		changes:   changes,
		manual:    state.LoadManual(cfg.ManualPath),
		reads:     newPaneReader(cfg, log, changes),
		scroll:    scroll,
	}
}

// Run polls the session until the context is cancelled. Herdr's event stream is
// deliberately not used, and the measurements that settled that are in
// docs/architecture/poll-loop.md.
func (a *App) Run(ctx context.Context, client herdr.Client) {
	// Name what already exists before waiting for the first tick.
	if !a.poll(ctx, client) {
		return
	}

	ticker := time.NewTicker(a.pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("shutting down")
			return
		case <-ticker.C:
			if !a.poll(ctx, client) {
				return
			}
		}
	}
}

// poll is one turn of the loop, and reports whether there should be another.
// No failure is fatal — Herdr's socket can lag the process it just launched —
// but another server on the socket ends the run: docs/architecture/poll-loop.md.
func (a *App) poll(ctx context.Context, client herdr.Client) bool {
	if a.superseded(client) {
		a.log.Info("another server holds the socket and starts an auto title of its own, leaving")
		return false
	}

	err := a.readAndRename(ctx, client)
	if ctx.Err() != nil {
		return true
	}

	if err != nil {
		if run := a.failures.failed(); run > 0 {
			a.log.Warn("poll failed", "error", err, "in a row", run)
		}

		return true
	}

	if run := a.failures.recovered(); run > 0 {
		a.log.Info("the session is answering again", "polls missed", run)
	}

	return true
}

// superseded reports whether the socket has passed to a server other than the
// one this instance first saw. While no server can be read nothing is decided:
// Herdr comes and goes, and only a successor is a reason to leave.
func (a *App) superseded(client herdr.Client) bool {
	current := client.Server()

	switch {
	case current == "":
		return false
	case a.server == "":
		a.server = current
		return false
	default:
		return current != a.server
	}
}

func (a *App) readAndRename(ctx context.Context, client herdr.Client) error {
	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	snapshot, err := herdr.SessionSnapshot(ctx, client)
	if err != nil {
		return err
	}

	a.changes.Observe(snapshot.Panes)
	// Taken from the snapshot rather than from the tabs below, because this is
	// what decides which of them are locked, and a locked tab is never read.
	live := labelsIn(snapshot.Tabs)
	a.manual.Retain(live)
	a.scroll.Retain(live)

	tabs := a.tabsIn(snapshot)
	reads := a.reads.forPoll(snapshot.Panes)

	for _, tab := range tabs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if a.manual.Locked(tab.ID) {
			continue
		}

		// Read here rather than during assembly: the reads are what a poll
		// spends, and only a tab that will be renamed is worth them. The
		// resolver picks the same pane, because the choice is made from state.
		reads.fill(ctx, client, state.SelectContextPane(tab))

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

	// A slide is not a rename: the title is the one the last poll set, one
	// step further along.
	if a.scroll.sliding(tab.ID) {
		a.log.Debug("title slid", "tab_id", tab.ID, "label", decision.Name)
		return
	}

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
