// Package app polls the Herdr session and keeps every tab's title in step with
// what that tab is doing, each pane's label too unless the configuration turns
// that off, and the workspace row above them when it is asked for.
package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/reads"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// PollTimeout bounds one poll: a snapshot and the renames it decides on.
const PollTimeout = 5 * time.Second

// LeaveTimeout is how long a displaced instance gets to leave: a poll past its
// deadline, then the wait for its next one, which is its own interval — up to
// five seconds of it, a slower instance being left with a warning.
const LeaveTimeout = PollTimeout + 5*time.Second

// Instance is this process's claim on the session: a newer claim ends the run,
// and the first snapshot read is reported to whoever waits for it — see
// docs/architecture/poll-loop.md.
type Instance interface {
	Taken() bool
	Ready()
}

// App is one run of the Auto Title loop.
type App struct {
	// pollEvery is how often the session is read, which is all the loop itself
	// decides anything by.
	pollEvery time.Duration
	log       *slog.Logger
	titles    resolver.TitleResolver
	// panes names each pane of a tab as well as the tab itself, and is nil
	// when the user turned that off.
	panes resolver.PaneResolver
	// preferAgent names a tab after its agent pane rather than its focused one.
	preferAgent bool
	// workspaces names the row above each workspace's tabs, and is nil when the
	// user has not asked for it.
	workspaces resolver.WorkspaceResolver
	changes    *state.Changes
	manual     *state.Manual
	reads      *reads.Reader
	// failures is the run of polls that have failed in a row, which decides
	// how loudly the next one is reported.
	failures failureLog
	// server identifies the Herdr this instance answers to, learned from the
	// first poll that could read it. Another server on the socket has started
	// an instance of its own, and this one leaves rather than double it.
	server   string
	instance Instance
}

// New builds the application. The client belongs to Run rather than to the
// App, so one App can be driven by any connection. A nil panes leaves every
// pane's label alone.
func New(
	cfg Config,
	log *slog.Logger,
	titles resolver.TitleResolver,
	panes resolver.PaneResolver,
	workspaces resolver.WorkspaceResolver,
	instance Instance,
) *App {
	return &App{
		pollEvery:   cfg.Poll,
		log:         log,
		titles:      titles,
		panes:       panes,
		workspaces:  workspaces,
		preferAgent: cfg.PreferAgentPane,
		changes:     state.NewChanges(),
		manual:      state.LoadManual(cfg.ManualPath),
		reads: reads.New(reads.Options{
			ClaudeDirs:      cfg.ClaudeDirs,
			BranchMax:       cfg.BranchMax,
			ReadTranscripts: cfg.ReadTranscripts,
		}, log),
		instance: instance,
	}
}

// Resolvers builds what the configuration asks titles to be resolved by: the
// shipped chain, its position in front when asked, panes named unless that is
// turned off, and workspaces named only when asked.
func Resolvers(
	cfg Config,
) (resolver.TitleResolver, resolver.PaneResolver, resolver.WorkspaceResolver) {
	chain := resolver.Default(resolver.Options{
		MaxLength:       cfg.MaxLength,
		BranchMax:       cfg.BranchMax,
		HideAgentName:   !cfg.ShowAgentName,
		NamesWorkspaces: cfg.namesWorkspaces(),
		Home:            cfg.Home,
	})

	var titles resolver.TitleResolver = chain
	if cfg.ShowPosition {
		titles = resolver.NewNumbered(chain, cfg.MaxLength)
	}

	workspaces := WorkspaceResolver(cfg)

	if !cfg.RenamePanes {
		return titles, nil, workspaces
	}

	return titles, chain, workspaces
}

// WorkspaceResolver is the chain a workspace row is named by, or nil when the
// user has not asked for the row to be named. Not the tabs' chain: a workspace
// is named for where the work is, under a bound the tab bar does not set.
func WorkspaceResolver(cfg Config) resolver.WorkspaceResolver {
	if !cfg.namesWorkspaces() {
		return nil
	}

	return resolver.Places(resolver.Options{
		MaxLength:     cfg.WorkspaceMaxLength,
		BranchMax:     cfg.BranchMax,
		HideAgentName: !cfg.ShowAgentName,
		Home:          cfg.Home,
	})
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
// but a successor ends the run, on the socket or on the claim: docs/architecture/poll-loop.md.
func (a *App) poll(ctx context.Context, client herdr.Client) bool {
	if reason := a.successor(client); reason != "" {
		a.log.Info(reason + ", leaving")
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

	a.instance.Ready()

	if run := a.failures.recovered(); run > 0 {
		a.log.Info("the session is answering again", "polls missed", run)
	}

	return true
}

// successor says why this instance should leave, or "" when it should go on.
// The claim answers for a new server too; the socket is what is left to read
// when there was nowhere to claim the session — docs/architecture/poll-loop.md.
func (a *App) successor(client herdr.Client) string {
	if a.instance.Taken() {
		return "a newer auto title has claimed the session"
	}

	if a.superseded(client) {
		return "another server holds the socket and starts an auto title of its own"
	}

	return ""
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
	ctx, cancel := context.WithTimeout(ctx, PollTimeout)
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
	a.manual.Workspaces.Retain(workspaceLabelsIn(snapshot.Workspaces))

	tabs := a.tabsIn(snapshot)
	poll := a.reads.Poll(client, snapshot.Panes)

	for _, tab := range tabs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		a.nameTab(ctx, client, poll, tab)

		if a.panes != nil {
			a.namePanes(ctx, client, poll, tab)
		}
	}

	if a.workspaces != nil {
		// Last, so a poll cut short gives up the row rather than a tab. Order
		// changes nothing else: a tab deduplicates against the row label in the
		// snapshot, and a rename issued here reaches it on the next poll.
		a.nameWorkspaces(ctx, client, poll, snapshot, tabs)

		// As the tab loop above returns on a cancelled context: a pass cut short
		// has not seen every workspace, and settling would leave the ones it
		// missed looking like workspaces never there on the first poll.
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	// Reached only when every tab was seen. Deferring this would settle after a
	// poll cut short, and the tabs it missed would look new and already named.
	a.manual.Settled()

	return nil
}

// nameWorkspaces judges every workspace's label and names the row above one
// holding exactly one tab after that tab. Other counts are left alone; the
// reads this costs are in poll-loop.md.
func (a *App) nameWorkspaces(
	ctx context.Context,
	client herdr.Client,
	poll *reads.Poll,
	snapshot herdr.Snapshot,
	tabs []state.TabState,
) {
	count, first := workspaceTabs(snapshot.Tabs)

	contexts := make(map[string]*state.PaneState, len(tabs))
	for _, tab := range tabs {
		contexts[tab.ID] = tab.Context
	}

	for _, info := range snapshot.Workspaces {
		if ctx.Err() != nil {
			return
		}

		id := info.WorkspaceID
		context := contexts[first[id]]

		// Read before judging, not after: the snapshot's directory is a
		// descendant's guess, and a row judged on it could be claimed for
		// wearing a basename it never wore.
		poll.Fill(ctx, context)

		workspace := state.WorkspaceFrom(info, context)

		// Not named, but still seen. Every workspace is judged, since one
		// holding two tabs today may hold one tomorrow; and a lock read back
		// from disk needs a sighting for the user's next rename to read as a move.
		if count[id] != 1 || a.manual.Workspaces.Locked(id) {
			a.manual.Workspaces.Observe(state.WorkspaceSightingFrom(workspace, ""))

			continue
		}

		decision := a.workspaces.ResolveWorkspace(workspace)
		a.apply(
			ctx,
			client,
			workspaceLabels,
			a.manual.Workspaces,
			state.WorkspaceSightingFrom(workspace, decision.Name),
			decision,
		)
	}
}

// workspaceTabs counts the tabs each workspace holds, and names the first of
// them, which is the tab a workspace is read through.
func workspaceTabs(tabs []herdr.TabInfo) (count map[string]int, first map[string]string) {
	count = make(map[string]int, len(tabs))
	first = make(map[string]string, len(tabs))

	for _, tab := range tabs {
		if count[tab.WorkspaceID]++; count[tab.WorkspaceID] == 1 {
			first[tab.WorkspaceID] = tab.TabID
		}
	}

	return count, first
}

// nameTab keeps one tab's label in step with what the tab is doing.
func (a *App) nameTab(
	ctx context.Context,
	client herdr.Client,
	poll *reads.Poll,
	tab state.TabState,
) {
	if a.manual.Tabs.Locked(tab.ID) {
		return
	}

	// Read here rather than during assembly: the reads are what a poll
	// spends, and only a tab that will be renamed is worth them.
	poll.Fill(ctx, tab.Context)

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
	poll *reads.Poll,
	tab state.TabState,
) {
	// Every pane is named against the tab's own pane, which is read even when
	// the tab is claimed; a poll never spends the same read twice.
	poll.Fill(ctx, tab.Context)

	for _, pane := range tab.Panes {
		if !a.manual.Panes.Locked(pane.ID) {
			poll.Fill(ctx, pane)
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

// labelKind is what differs between the things Auto Title names.
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
	workspaceLabels = labelKind{
		noun:   "workspace",
		rename: herdr.RenameWorkspace,
		gone:   herdr.CodeWorkspaceNotFound,
	}
)

// apply gives a tab, a pane or a workspace the name the resolver chose, unless
// the user put the label it carries there. Nothing that goes wrong here is
// worth cutting the poll short: the next one decides again from state read again.
func (a *App) apply(
	ctx context.Context,
	client herdr.Client,
	kind labelKind,
	claims *state.Claims,
	seen state.Sighting,
	decision resolver.Decision,
) {
	idKey := kind.noun + "_id"

	switch claims.Observe(seen) {
	case state.VerdictClaimed:
		a.log.Info("leaving a "+kind.noun+" the user renamed", idKey, seen.ID, "name", seen.Current)
		return
	case state.VerdictUnjudged:
		a.log.Debug("leaving a "+kind.noun+" not yet judged", idKey, seen.ID, "name", seen.Current)
		return
	case state.VerdictName:
	}

	if decision.Name == "" || decision.Name == seen.Current {
		return
	}

	if err := kind.rename(ctx, client, seen.ID, decision.Name); err != nil {
		if herdr.ErrorCode(err) == kind.gone {
			a.log.Debug(kind.noun+" closed before it could be renamed", idKey, seen.ID)
			return
		}

		if errors.Is(err, herdr.ErrUnanswered) {
			// Herdr may still apply it: docs/architecture/manual-rename-protection.md.
			claims.Sent(seen.ID, decision.Name)
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

// workspaceLabelsIn indexes the session's workspaces by id, for the bookkeeping
// labelsIn feeds. Herdr always answers a label here: a workspace nobody has
// renamed carries the basename of the directory it was created in.
func workspaceLabelsIn(workspaces []herdr.WorkspaceInfo) map[string]string {
	labels := make(map[string]string, len(workspaces))
	for _, workspace := range workspaces {
		labels[workspace.WorkspaceID] = workspace.Label
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
