package app

import (
	"context"

	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// tabsIn assembles the snapshot's tabs with their panes, and reads nothing:
// what a pane is running, has checked out and is talking about each costs a
// request or a file, and readInto spends that on the panes that earn it.
func (a *App) tabsIn(snapshot herdr.Snapshot) []state.TabState {
	workspaces := make(map[string]string, len(snapshot.Workspaces))

	for _, workspace := range snapshot.Workspaces {
		workspaces[workspace.WorkspaceID] = workspace.Label
	}

	byTab := make(map[string][]*state.PaneState, len(snapshot.Tabs))

	for _, pane := range snapshot.Panes {
		byTab[pane.TabID] = append(
			byTab[pane.TabID],
			state.PaneFrom(pane, a.changes.ChangedAt(pane.PaneID)),
		)
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

// readInto fills in what the snapshot could not say about the one pane a tab
// is named from. The other panes of that tab keep what the snapshot said,
// because nothing reads any more of them.
func (a *App) readInto(
	ctx context.Context,
	client herdr.Client,
	pane *state.PaneState,
	checkouts checkoutMemo,
) {
	if pane == nil {
		return
	}

	// What a pane is running settles which directory it speaks for, and the
	// reads below are of that directory, so this one comes first.
	pane.ReadProcesses(a.processesIn(ctx, client, pane.ID))

	pane.Read(state.Reads{
		Git:   a.checkoutIn(ctx, pane.Dir, checkouts),
		Topic: a.topicIn(ctx, pane),
	})
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
// Nothing is remembered past the poll, and why not is in
// docs/architecture/title-resolution.md.
func (a *App) checkoutIn(ctx context.Context, dir string, checkouts checkoutMemo) git.Checkout {
	// A branch width of zero is how branches are turned off, and a read whose
	// answer is thrown away is still a read on every pane twice a second.
	if a.cfg.BranchMax <= 0 {
		return git.Checkout{}
	}

	if spentPoll(ctx) {
		return git.Checkout{}
	}

	return checkouts.read(dir)
}

// checkoutMemo holds the checkouts one poll has read. The tabs of a project
// share a directory, and the memo dies with the poll, so collapsing their
// reads costs no staleness — see docs/architecture/title-resolution.md.
type checkoutMemo map[string]git.Checkout

// read reports what the repository holding dir has checked out, going to the
// filesystem at most once. A directory that holds no repository is remembered
// too: finding that out costs the same walk as finding one.
func (m checkoutMemo) read(dir string) git.Checkout {
	if checkout, known := m[dir]; known {
		return checkout
	}

	checkout := git.Read(dir)
	m[dir] = checkout

	return checkout
}

// topicIn reports what the session the pane's agent is holding says it is
// about. Only Claude Code's transcripts are understood, and only Herdr's
// integration hook says which session a pane holds.
func (a *App) topicIn(ctx context.Context, pane *state.PaneState) string {
	if !a.cfg.ReadTranscripts || spentPoll(ctx) {
		return ""
	}

	sessionID, ok := pane.AgentSession.IDFor(claude.Agent)
	if !ok {
		return ""
	}

	return a.topics.Topic(sessionID, pane.Dir).Text()
}

// spentPoll reports that this poll is past its deadline, in which case the tab
// loop will discard it. The reads it guards go to the filesystem, which takes
// no context, so the only way to bound them is not to start them.
func spentPoll(ctx context.Context) bool {
	return ctx.Err() != nil
}

func sessionsIn(panes []herdr.PaneInfo) []string {
	sessions := make([]string, 0, len(panes))
	for _, pane := range panes {
		if pane.AgentSession != nil {
			sessions = append(sessions, pane.AgentSession.Value)
		}
	}

	return sessions
}
