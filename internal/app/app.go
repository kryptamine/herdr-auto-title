// Package app polls the Herdr session and keeps every tab's title in step with
// what that tab is doing.
package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/git"
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

// failureLog decides how often a run of failing polls is worth mentioning. At
// two polls a second, logging every one turns an hour of Herdr being down into
// thousands of identical lines; logging as the run doubles costs a dozen.
type failureLog struct {
	run  int
	next int
}

// failed records a failed poll and returns the length of the run when it is
// worth logging, or zero when it is not.
func (f *failureLog) failed() int {
	f.run++
	if f.run < f.next {
		return 0
	}

	f.next = f.run * 2

	return f.run
}

// recovered records a successful poll and returns how many polls the run of
// failures it ended cost, or zero when nothing was wrong.
func (f *failureLog) recovered() int {
	run := f.run
	f.run, f.next = 0, 0

	return run
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

	tabs := a.tabsIn(ctx, client, snapshot)
	a.manual.Retain(labelsOf(tabs))

	for _, tab := range tabs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if a.manual.Locked(tab.ID) {
			continue
		}

		decision := a.titles.Resolve(tab)
		if a.manual.Observe(state.SightingFrom(tab, decision.Name)) {
			a.log.Info("leaving a tab the user renamed", "tab_id", tab.ID, "name", tab.CurrentName)
			continue
		}

		if decision.Name == "" || decision.Name == tab.CurrentName {
			continue
		}

		if err := herdr.RenameTab(ctx, client, tab.ID, decision.Name); err != nil {
			if herdr.ErrorCode(err) == herdr.CodeTabNotFound {
				// The tab closed between the snapshot and the rename. The next
				// poll will not see it at all.
				a.log.Debug("tab closed before it could be renamed", "tab_id", tab.ID)
				continue
			}

			a.log.Warn("rename failed", "tab_id", tab.ID, "name", decision.Name, "error", err)

			continue
		}

		// Recorded before the log line so the next poll cannot read this
		// rename as the user's.
		a.manual.Applied(tab.ID, decision.Name)
		a.log.Info("tab renamed",
			"tab_id", tab.ID,
			"old", tab.CurrentName,
			"new", decision.Name,
			"reason", decision.Reason,
			"confidence", decision.Confidence,
		)
	}

	// Reached only when every tab was seen. Deferring this would settle after a
	// poll cut short, and the tabs it missed would look new and already named.
	a.manual.Settled()

	return nil
}

// labelsOf indexes tabs by id for the manual-name bookkeeping, which needs both
// an id that is gone and a label that has moved on.
func labelsOf(tabs []state.TabState) map[string]string {
	labels := make(map[string]string, len(tabs))
	for _, tab := range tabs {
		labels[tab.ID] = tab.CurrentName
	}

	return labels
}

// processesIn reports what a pane is running, reusing the last read while the
// pane's revision holds and that read is recent. Neither test is exact, which
// is why there are two — see docs/architecture/poll-loop.md.
func (a *App) processesIn(
	ctx context.Context,
	client herdr.Client,
	paneID string,
) []herdr.PaneProcessInfoProcess {
	if processes, read := a.changes.Processes(paneID); read {
		return processes
	}

	processes, err := herdr.PaneProcesses(ctx, client, paneID)
	if err != nil {
		if herdr.ErrorCode(err) != herdr.CodePaneNotFound && ctx.Err() == nil {
			a.log.Debug("could not read what a pane is running", "pane_id", paneID, "error", err)
		}

		return nil
	}

	a.changes.Ran(paneID, processes)

	return processes
}

// checkoutIn reports what the repository holding the pane has checked out.
// Unlike a process read it is not cached, and why not is in
// docs/architecture/title-resolution.md.
func (a *App) checkoutIn(ctx context.Context, pane herdr.PaneInfo) git.Checkout {
	// A branch width of zero is how branches are turned off, and a read whose
	// answer is thrown away is still a read on every pane twice a second.
	if a.cfg.BranchMax <= 0 {
		return git.Checkout{}
	}

	if spentPoll(ctx) {
		return git.Checkout{}
	}

	checkout, _ := git.Read(pane.Dir())

	return checkout
}

// topicIn reports what the session the pane's agent is holding says it is
// about. Only Claude Code's transcripts are understood, and only Herdr's
// integration hook says which session a pane holds.
func (a *App) topicIn(ctx context.Context, pane herdr.PaneInfo) string {
	if !a.cfg.ReadTranscripts || spentPoll(ctx) {
		return ""
	}

	sessionID, ok := pane.AgentSession.IDFor(claude.Agent)
	if !ok {
		return ""
	}

	return a.topics.Topic(sessionID, pane.Dir()).Text()
}

// spentPoll reports that this poll is past its deadline, in which case the tab
// loop will discard it. The reads it guards go to the filesystem, which takes
// no context, so the only way to bound them is not to start them.
func spentPoll(ctx context.Context) bool {
	return ctx.Err() != nil
}

// sessionsIn lists the agent sessions the snapshot holds, which is what the
// transcript reader keeps its places for.
func sessionsIn(panes []herdr.PaneInfo) []string {
	sessions := make([]string, 0, len(panes))
	for _, pane := range panes {
		if pane.AgentSession != nil {
			sessions = append(sessions, pane.AgentSession.Value)
		}
	}

	return sessions
}

// tabsIn assembles the snapshot's tabs with their panes. What is running in a
// pane needs a request of its own, which processesIn makes only when the last
// answer will no longer do.
func (a *App) tabsIn(
	ctx context.Context,
	client herdr.Client,
	snapshot herdr.Snapshot,
) []state.TabState {
	workspaces := make(map[string]string, len(snapshot.Workspaces))

	for _, workspace := range snapshot.Workspaces {
		workspaces[workspace.WorkspaceID] = workspace.Label
	}

	byTab := make(map[string][]*state.PaneState, len(snapshot.Tabs))

	for _, pane := range snapshot.Panes {
		byTab[pane.TabID] = append(byTab[pane.TabID], state.PaneFrom(pane, state.Reads{
			Processes: a.processesIn(ctx, client, pane.PaneID),
			Git:       a.checkoutIn(ctx, pane),
			Topic:     a.topicIn(ctx, pane),
			ChangedAt: a.changes.ChangedAt(pane.PaneID),
		}))
	}

	// An unnamed tab carries its place in the workspace, and the snapshot lists
	// tabs in display order, so counting them gives that label.
	positions := make(map[string]int, len(snapshot.Workspaces))
	tabs := make([]state.TabState, 0, len(snapshot.Tabs))

	for _, info := range snapshot.Tabs {
		positions[info.WorkspaceID]++

		tabs = append(
			tabs,
			state.TabFrom(
				info,
				workspaces[info.WorkspaceID],
				positions[info.WorkspaceID],
				byTab[info.TabID],
			),
		)
	}

	return tabs
}
