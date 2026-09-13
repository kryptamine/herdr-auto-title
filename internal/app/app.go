// Package app polls the Herdr session and keeps every tab's title in step with
// what that tab is doing, and each pane's label too where the configuration
// asks for it.
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
	// panes names each pane of a tab as well as the tab itself, and is nil
	// unless the user asked for it.
	panes   resolver.PaneResolver
	changes *state.Changes
	manual  *state.Manual
	reads   *paneReader
	// failures is the run of polls that have failed in a row, which decides
	// how loudly the next one is reported.
	failures failureLog
	// server identifies the Herdr this instance answers to, learned from the
	// first poll that could read it. Another server on the socket has started
	// an instance of its own, and this one leaves rather than double it.
	server string
}

// New builds the application. The client belongs to Run rather than to the
// App, so one App can be driven by any connection. A nil panes leaves every
// pane's label alone.
func New(
	cfg Config,
	log *slog.Logger,
	titles resolver.TitleResolver,
	panes resolver.PaneResolver,
) *App {
	changes := state.NewChanges()

	return &App{
		pollEvery: cfg.Poll,
		log:       log,
		titles:    titles,
		panes:     panes,
		changes:   changes,
		manual:    state.LoadManual(cfg.ManualPath),
		reads:     newPaneReader(cfg, log, changes),
	}
}

// Resolvers builds what the configuration asks titles to be resolved by: the
// shipped chain, its position in front when asked, and panes named only when
// asked.
func Resolvers(cfg Config) (resolver.TitleResolver, resolver.PaneResolver) {
	chain := resolver.Default(resolver.Options{
		MaxLength:     cfg.MaxLength,
		BranchMax:     cfg.BranchMax,
		HideAgentName: !cfg.ShowAgentName,
	})

	var titles resolver.TitleResolver = chain
	if cfg.ShowPosition {
		titles = resolver.NewNumbered(chain, cfg.MaxLength)
	}

	if !cfg.RenamePanes {
		return titles, nil
	}

	return titles, chain
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
	a.manual.Tabs.Retain(labelsIn(snapshot.Tabs))
	a.manual.Panes.Retain(paneLabelsIn(snapshot.Panes))

	tabs := a.tabsIn(snapshot)
	reads := a.reads.forPoll(snapshot.Panes)

	for _, tab := range tabs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		a.nameTab(ctx, client, reads, tab)

		if a.panes != nil {
			a.namePanes(ctx, client, reads, tab)
		}
	}

	// Reached only when every tab was seen. Deferring this would settle after a
	// poll cut short, and the tabs it missed would look new and already named.
	a.manual.Settled()

	return nil
}

// nameTab keeps one tab's label in step with what the tab is doing.
func (a *App) nameTab(
	ctx context.Context,
	client herdr.Client,
	reads *paneReads,
	tab state.TabState,
) {
	if a.manual.Tabs.Locked(tab.ID) {
		return
	}

	// Read here rather than during assembly: the reads are what a poll
	// spends, and only a tab that will be renamed is worth them. The
	// resolver picks the same pane, because the choice is made from state.
	reads.fill(ctx, client, state.SelectContextPane(tab))

	decision := a.titles.Resolve(tab)
	a.apply(
		ctx,
		client,
		tabLabels,
		a.manual.Tabs,
		state.SightingFrom(tab, decision.Name),
		decision,
	)
}

// namePanes labels every pane of a tab, which is what Herdr's goto panel lists
// a pane by. It reads each pane rather than the one its tab speaks through, so
// it costs a read per pane — see docs/architecture/poll-loop.md.
func (a *App) namePanes(
	ctx context.Context,
	client herdr.Client,
	reads *paneReads,
	tab state.TabState,
) {
	// Every pane is named against the tab's own pane, which is read even when
	// the tab is claimed; a poll never spends the same read twice.
	reads.fill(ctx, client, state.SelectContextPane(tab))

	for _, pane := range tab.Panes {
		if !a.manual.Panes.Locked(pane.ID) {
			reads.fill(ctx, client, pane)
		}
	}

	for i, decision := range a.panes.ResolvePanes(tab) {
		if ctx.Err() != nil {
			return
		}

		pane := tab.Panes[i]
		if a.manual.Panes.Locked(pane.ID) {
			continue
		}

		a.apply(
			ctx,
			client,
			paneLabels,
			a.manual.Panes,
			state.PaneSightingFrom(pane, decision.Name),
			decision,
		)
	}
}

// labelKind is what differs between the two things Auto Title names.
type labelKind struct {
	noun   string
	rename func(ctx context.Context, c herdr.Client, id, label string) error
	// gone is the error Herdr answers when the thing closed between the
	// snapshot and the rename. The next poll will not see it at all.
	gone string
}

var (
	tabLabels = labelKind{
		noun:   "tab",
		rename: herdr.RenameTab,
		gone:   herdr.CodeTabNotFound,
	}
	paneLabels = labelKind{
		noun:   "pane",
		rename: herdr.RenamePane,
		gone:   herdr.CodePaneNotFound,
	}
)

// apply gives a tab or a pane the name the resolver chose, unless the user put
// the label it carries there. Nothing that goes wrong here is worth cutting the
// poll short: the next one decides again from state it has read again.
func (a *App) apply(
	ctx context.Context,
	client herdr.Client,
	kind labelKind,
	claims *state.Claims,
	seen state.Sighting,
	decision resolver.Decision,
) {
	idKey := kind.noun + "_id"

	if claims.Observe(seen) {
		a.log.Info("leaving a "+kind.noun+" the user renamed", idKey, seen.ID, "name", seen.Current)
		return
	}

	if decision.Name == "" || decision.Name == seen.Current {
		return
	}

	if err := kind.rename(ctx, client, seen.ID, decision.Name); err != nil {
		if herdr.ErrorCode(err) == kind.gone {
			a.log.Debug(kind.noun+" closed before it could be renamed", idKey, seen.ID)
			return
		}

		a.log.Warn(kind.noun+" rename failed", idKey, seen.ID, "name", decision.Name, "error", err)

		return
	}

	// Recorded before the log line so the next poll cannot read this rename as
	// the user's.
	claims.Applied(seen.ID, decision.Name)
	a.log.Info(kind.noun+" renamed",
		idKey, seen.ID,
		"old", seen.Current,
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

// paneLabelsIn indexes the session's panes by id, for the same bookkeeping
// labelsIn feeds. A pane Herdr has never been asked to name carries no label
// at all, which arrives here as the empty string.
func paneLabelsIn(panes []herdr.PaneInfo) map[string]string {
	labels := make(map[string]string, len(panes))
	for _, pane := range panes {
		labels[pane.PaneID] = pane.Label
	}

	return labels
}
