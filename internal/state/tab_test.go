package state

import (
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func TestSelectContextPanePrefersFocused(t *testing.T) {
	now := time.Now()
	tab := TabState{
		ID: "wE:t1",
		Panes: []*PaneState{
			{ID: "wE:p1", ChangedAt: now},
			{ID: "wE:p2", ChangedAt: now.Add(time.Minute), Focused: true},
			{ID: "wE:p3", ChangedAt: now.Add(time.Hour)},
		},
	}

	if got := SelectContextPane(tab); got == nil || got.ID != "wE:p2" {
		t.Fatalf("selected %v, want the focused pane wE:p2", got)
	}
}

func TestSelectContextPaneFallsBackToMostRecent(t *testing.T) {
	now := time.Now()
	tab := TabState{
		ID: "wE:t1",
		Panes: []*PaneState{
			{ID: "wE:p1", ChangedAt: now},
			{ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
			{ID: "wE:p3", ChangedAt: now.Add(time.Minute)},
		},
	}

	if got := SelectContextPane(tab); got == nil || got.ID != "wE:p2" {
		t.Fatalf("selected %v, want the most recently updated pane wE:p2", got)
	}
}

func TestTabFromKeepsItsLabel(t *testing.T) {
	tab := TabFrom(
		herdr.TabInfo{TabID: "wE:t1", Label: "dashboard"},
		"dashboard",
		1,
		[]*PaneState{{ID: "wE:p1", Dir: "/work/dashboard"}},
	)

	if tab.CurrentName != "dashboard" {
		t.Errorf("current name = %q, want dashboard", tab.CurrentName)
	}

	if tab.WorkspaceName != "dashboard" {
		t.Errorf("workspace name = %q, want dashboard", tab.WorkspaceName)
	}

	if tab.Position != 1 {
		t.Errorf("position = %d, want the tab's place in its workspace 1", tab.Position)
	}

	if len(tab.Panes) != 1 || tab.Panes[0].Dir != "/work/dashboard" {
		t.Errorf("panes = %+v, want one pane in /work/dashboard", tab.Panes)
	}
}

func TestPaneFromReadsAgentContext(t *testing.T) {
	stamp := time.Now()
	pane := PaneFrom(herdr.PaneInfo{
		PaneID:                "wE:p1",
		CWD:                   "/work/dashboard",
		TerminalTitle:         "\u2733 Claude Code",
		TerminalTitleStripped: "Claude Code",
		Title:                 "Implement OAuth scopes",
		Agent:                 "claude",
		DisplayAgent:          "Claude Code",
		AgentStatus:           herdr.AgentStatusWorking,
	}, stamp)
	pane.Read(Reads{Topic: "Rework the poll loop"})

	switch {
	case pane.TerminalTitle != "Claude Code":
		t.Errorf("terminal title = %q, want the stripped one", pane.TerminalTitle)
	case pane.TerminalTitleRaw != "\u2733 Claude Code":
		t.Errorf("raw terminal title = %q", pane.TerminalTitleRaw)
	case pane.AgentTitle != "Implement OAuth scopes":
		t.Errorf("agent title = %q", pane.AgentTitle)
	case pane.AgentTopic != "Rework the poll loop":
		t.Errorf("agent topic = %q", pane.AgentTopic)
	case !pane.AgentIsActive():
		t.Error("a working agent is not active")
	case !pane.ChangedAt.Equal(stamp):
		t.Errorf("changed at = %v, want %v", pane.ChangedAt, stamp)
	}
}

func TestPaneWithoutAnAgent(t *testing.T) {
	pane := PaneFrom(
		herdr.PaneInfo{PaneID: "wE:p1", AgentStatus: "unknown"},
		time.Time{},
	)
	if pane.HasAgent() || pane.AgentIsActive() {
		t.Errorf("pane %+v reported an agent", pane)
	}
}

func TestSelectContextPaneBreaksTiesOnID(t *testing.T) {
	stamp := time.Now()
	// Built through TabFrom, because that is where the order is imposed: the
	// snapshot lists panes in whatever order it pleases.
	tab := TabFrom(herdr.TabInfo{TabID: "wE:t1"}, "", 1, []*PaneState{
		{ID: "wE:p3", ChangedAt: stamp},
		{ID: "wE:p1", ChangedAt: stamp},
		{ID: "wE:p2", ChangedAt: stamp},
	})

	if got := SelectContextPane(tab); got == nil || got.ID != "wE:p1" {
		t.Fatalf("selected %v, want the lowest id wE:p1", got)
	}
}

func TestSelectContextPaneWithoutPanes(t *testing.T) {
	if got := SelectContextPane(TabState{ID: "wE:t1"}); got != nil {
		t.Fatalf("selected %v, want nil", got)
	}
}

func TestSelectContextPanePrefersAnActiveAgent(t *testing.T) {
	now := time.Now()
	tab := TabState{
		ID: "wE:t1",
		Panes: []*PaneState{
			// The agent runs in a split the user is not typing in, so a build
			// scrolling past in the pane below keeps winning on recency.
			{ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
			{ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
		},
	}

	if got := SelectContextPane(tab); got == nil || got.ID != "wE:p1" {
		t.Fatalf("selected %v, want the agent pane wE:p1", got)
	}
}

func TestSelectContextPaneIgnoresAnIdleAgent(t *testing.T) {
	now := time.Now()

	for _, status := range []string{"idle", "done", "unknown"} {
		t.Run(status, func(t *testing.T) {
			tab := TabState{
				ID: "wE:t1",
				Panes: []*PaneState{
					{ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: status},
					{ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
				},
			}

			if got := SelectContextPane(tab); got == nil || got.ID != "wE:p2" {
				t.Fatalf("selected %v, want the most recently updated pane wE:p2", got)
			}
		})
	}
}

func TestSelectContextPanePrefersTheFocusedPaneOverAnAgent(t *testing.T) {
	now := time.Now()
	tab := TabState{
		ID: "wE:t1",
		Panes: []*PaneState{
			{ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
			{ID: "wE:p2", ChangedAt: now, Focused: true},
		},
	}

	if got := SelectContextPane(tab); got == nil || got.ID != "wE:p2" {
		t.Fatalf("selected %v, want the focused pane wE:p2", got)
	}
}

func TestSelectContextPaneAmongSeveralAgents(t *testing.T) {
	now := time.Now()
	tab := TabState{
		ID: "wE:t1",
		Panes: []*PaneState{
			{ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
			{
				ID:          "wE:p2",
				ChangedAt:   now.Add(time.Hour),
				Agent:       "claude",
				AgentStatus: herdr.AgentStatusBlocked,
			},
			{ID: "wE:p3", ChangedAt: now.Add(2 * time.Hour)},
		},
	}

	if got := SelectContextPane(tab); got == nil || got.ID != "wE:p2" {
		t.Fatalf("selected %v, want the most recently updated agent pane wE:p2", got)
	}
}

func TestAgentIsActiveOnANilPane(t *testing.T) {
	var pane *PaneState
	if pane.HasAgent() || pane.AgentIsActive() {
		t.Fatal("a nil pane reported an agent")
	}
}

func TestPaneFromPrefersTheForegroundDirectory(t *testing.T) {
	// A subshell — `chezmoi cd`, `nix develop` — moves the foreground process
	// and leaves the pane's own shell where it was started.
	both := PaneFrom(herdr.PaneInfo{
		PaneID: "wE:p1", CWD: "/work/dashboard", ForegroundCWD: "/work/chezmoi",
	}, time.Time{})
	if both.Dir != "/work/chezmoi" {
		t.Errorf("dir = %q, want the foreground process's", both.Dir)
	}

	shell := PaneFrom(herdr.PaneInfo{
		PaneID: "wE:p1", CWD: "/work/api",
	}, time.Time{})
	if shell.Dir != "/work/api" {
		t.Errorf("dir = %q, want the shell's own", shell.Dir)
	}
}
