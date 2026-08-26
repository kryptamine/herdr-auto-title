package app

import (
	"context"

	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

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
