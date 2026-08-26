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
		Panes: map[string]*PaneState{
			"wE:p1": {ID: "wE:p1", ChangedAt: now},
			"wE:p2": {ID: "wE:p2", ChangedAt: now.Add(time.Minute), Focused: true},
			"wE:p3": {ID: "wE:p3", ChangedAt: now.Add(time.Hour)},
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
		Panes: map[string]*PaneState{
			"wE:p1": {ID: "wE:p1", ChangedAt: now},
			"wE:p2": {ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
			"wE:p3": {ID: "wE:p3", ChangedAt: now.Add(time.Minute)},
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
	if pane := tab.Panes["wE:p1"]; pane == nil || pane.Dir != "/work/dashboard" {
		t.Errorf("pane wE:p1 = %+v, want dir /work/dashboard", pane)
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
	}, Reads{Topic: "Rework the poll loop", ChangedAt: stamp})

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
	pane := PaneFrom(herdr.PaneInfo{PaneID: "wE:p1", AgentStatus: herdr.AgentStatusUnknown}, Reads{})
	if pane.HasAgent() || pane.AgentIsActive() {
		t.Errorf("pane %+v reported an agent", pane)
	}
}

func TestSelectContextPaneBreaksTiesOnID(t *testing.T) {
	stamp := time.Now()
	tab := TabState{
		ID: "wE:t1",
		Panes: map[string]*PaneState{
			"wE:p3": {ID: "wE:p3", ChangedAt: stamp},
			"wE:p1": {ID: "wE:p1", ChangedAt: stamp},
			"wE:p2": {ID: "wE:p2", ChangedAt: stamp},
		},
	}

	// Map iteration order varies; the choice must not.
	for i := range 50 {
		got := SelectContextPane(tab)
		if got == nil || got.ID != "wE:p1" {
			t.Fatalf("iteration %d selected %v, want wE:p1", i, got)
		}
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
		Panes: map[string]*PaneState{
			// The agent runs in a split the user is not typing in, so a build
			// scrolling past in the pane below keeps winning on recency.
			"wE:p1": {ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
			"wE:p2": {ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
		},
	}

	if got := SelectContextPane(tab); got == nil || got.ID != "wE:p1" {
		t.Fatalf("selected %v, want the agent pane wE:p1", got)
	}
}

func TestSelectContextPaneIgnoresAnIdleAgent(t *testing.T) {
	now := time.Now()
	for _, status := range []string{herdr.AgentStatusIdle, herdr.AgentStatusDone, herdr.AgentStatusUnknown} {
		t.Run(status, func(t *testing.T) {
			tab := TabState{
				ID: "wE:t1",
				Panes: map[string]*PaneState{
					"wE:p1": {ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: status},
					"wE:p2": {ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
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
		Panes: map[string]*PaneState{
			"wE:p1": {ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
			"wE:p2": {ID: "wE:p2", ChangedAt: now, Focused: true},
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
		Panes: map[string]*PaneState{
			"wE:p1": {ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
			"wE:p2": {ID: "wE:p2", ChangedAt: now.Add(time.Hour), Agent: "claude", AgentStatus: herdr.AgentStatusBlocked},
			"wE:p3": {ID: "wE:p3", ChangedAt: now.Add(2 * time.Hour)},
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

func TestPaneFromPrefersTheShellsDirectory(t *testing.T) {
	// foreground_cwd follows whatever is running right now, so it only speaks
	// when the shell's own directory is missing.
	both := PaneFrom(herdr.PaneInfo{
		PaneID: "wE:p1", CWD: "/work/dashboard", ForegroundCWD: "/tmp",
	}, Reads{})
	if both.Dir != "/work/dashboard" {
		t.Errorf("dir = %q, want the shell's own", both.Dir)
	}

	foreground := PaneFrom(herdr.PaneInfo{
		PaneID: "wE:p1", ForegroundCWD: "/work/api",
	}, Reads{})
	if foreground.Dir != "/work/api" {
		t.Errorf("dir = %q, want the foreground process's", foreground.Dir)
	}
}
