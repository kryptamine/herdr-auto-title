package app

import (
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// tabsIn assembles the snapshot's tabs with their panes, and reads nothing:
// what a pane is running, has checked out and is talking about each costs a
// request or a file, and the reads are spent on the panes that earn them.
func (a *App) tabsIn(snapshot herdr.Snapshot) []state.TabState {
	workspaces := workspaceLabelsIn(snapshot.Workspaces)
	handles := handlesIn(snapshot.Agents)

	byTab := make(map[string][]*state.PaneState, len(snapshot.Tabs))

	for _, pane := range snapshot.Panes {
		byTab[pane.TabID] = append(
			byTab[pane.TabID],
			state.PaneFrom(pane, handles[pane.PaneID], a.changes.ChangedAt(pane.PaneID)),
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
				a.preferAgent,
			),
		)
	}

	return tabs
}

// handlesIn maps each pane to the name its agent was given, leaving out the
// agents nobody named: Herdr omits their name rather than sending it empty.
func handlesIn(agents []herdr.AgentInfo) map[string]string {
	handles := make(map[string]string, len(agents))
	for _, agent := range agents {
		if agent.Name != "" {
			handles[agent.PaneID] = agent.Name
		}
	}

	return handles
}
