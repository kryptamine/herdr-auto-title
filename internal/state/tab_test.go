package state

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
)

func TestTabContextPrefersFocused(t *testing.T) {
	now := time.Now()
	tab := tabOf([]*PaneState{
		{ID: "wE:p1", ChangedAt: now},
		{ID: "wE:p2", ChangedAt: now.Add(time.Minute), Focused: true},
		{ID: "wE:p3", ChangedAt: now.Add(time.Hour)},
	})

	if got := tab.Context; got == nil || got.ID != "wE:p2" {
		t.Fatalf("selected %v, want the focused pane wE:p2", got)
	}
}

func TestTabContextFallsBackToMostRecent(t *testing.T) {
	now := time.Now()
	tab := tabOf([]*PaneState{
		{ID: "wE:p1", ChangedAt: now},
		{ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
		{ID: "wE:p3", ChangedAt: now.Add(time.Minute)},
	})

	if got := tab.Context; got == nil || got.ID != "wE:p2" {
		t.Fatalf("selected %v, want the most recently updated pane wE:p2", got)
	}
}

func TestTabFromKeepsItsLabel(t *testing.T) {
	tab := TabFrom(
		herdr.TabInfo{TabID: "wE:t1", Label: "dashboard"},
		"dashboard",
		1,
		[]*PaneState{{ID: "wE:p1", Dir: "/work/dashboard"}},
		false,
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
	pane.AgentTopic = "Rework the poll loop"

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

func TestTabContextBreaksTiesOnID(t *testing.T) {
	stamp := time.Now()
	// Built through TabFrom, because that is where the order is imposed: the
	// snapshot lists panes in whatever order it pleases.
	tab := tabOf([]*PaneState{
		{ID: "wE:p3", ChangedAt: stamp},
		{ID: "wE:p1", ChangedAt: stamp},
		{ID: "wE:p2", ChangedAt: stamp},
	})

	if got := tab.Context; got == nil || got.ID != "wE:p1" {
		t.Fatalf("selected %v, want the lowest id wE:p1", got)
	}
}

func TestTabContextWithoutPanes(t *testing.T) {
	if got := tabOf(nil).Context; got != nil {
		t.Fatalf("selected %v, want nil", got)
	}
}

func TestTabContextPrefersAnActiveAgent(t *testing.T) {
	now := time.Now()
	tab := tabOf([]*PaneState{
		// The agent runs in a split the user is not typing in, so a build
		// scrolling past in the pane below keeps winning on recency.
		{ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
		{ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
	})

	if got := tab.Context; got == nil || got.ID != "wE:p1" {
		t.Fatalf("selected %v, want the agent pane wE:p1", got)
	}
}

func TestTabContextIgnoresAnIdleAgent(t *testing.T) {
	now := time.Now()

	for _, status := range []string{"idle", "done", "unknown"} {
		t.Run(status, func(t *testing.T) {
			tab := tabOf([]*PaneState{
				{ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: status},
				{ID: "wE:p2", ChangedAt: now.Add(time.Hour)},
			})

			if got := tab.Context; got == nil || got.ID != "wE:p2" {
				t.Fatalf("selected %v, want the most recently updated pane wE:p2", got)
			}
		})
	}
}

func TestTabContextPrefersTheFocusedPaneOverAnAgent(t *testing.T) {
	now := time.Now()
	tab := tabOf([]*PaneState{
		{ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
		{ID: "wE:p2", ChangedAt: now, Focused: true},
	})

	if got := tab.Context; got == nil || got.ID != "wE:p2" {
		t.Fatalf("selected %v, want the focused pane wE:p2", got)
	}
}

func TestTabContextAmongSeveralAgents(t *testing.T) {
	now := time.Now()
	tab := tabOf([]*PaneState{
		{ID: "wE:p1", ChangedAt: now, Agent: "claude", AgentStatus: herdr.AgentStatusWorking},
		{
			ID:          "wE:p2",
			ChangedAt:   now.Add(time.Hour),
			Agent:       "claude",
			AgentStatus: herdr.AgentStatusBlocked,
		},
		{ID: "wE:p3", ChangedAt: now.Add(2 * time.Hour)},
	})

	if got := tab.Context; got == nil || got.ID != "wE:p2" {
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
	dashboard, chezmoi := herdrtest.Dir("work", "dashboard"), herdrtest.Dir("work", "chezmoi")

	both := PaneFrom(herdr.PaneInfo{
		PaneID: "wE:p1", CWD: dashboard, ForegroundCWD: chezmoi,
	}, time.Time{})
	if both.Dir != chezmoi {
		t.Errorf("dir = %q, want the foreground process's", both.Dir)
	}

	api := herdrtest.Dir("work", "api")

	shell := PaneFrom(herdr.PaneInfo{PaneID: "wE:p1", CWD: api}, time.Time{})
	if shell.Dir != api {
		t.Errorf("dir = %q, want the shell's own", shell.Dir)
	}
}

func TestPaneDirTakesTheForegroundProcessesOwnDirectory(t *testing.T) {
	// A snapshot reports the deepest descendant's directory, which for an agent
	// is a server it spawned. The process list is deepest first.
	server, portal := herdrtest.Dir("opt", "gimp-mcp"), herdrtest.Dir("work", "self-care-portal")
	processes := []herdr.PaneProcessInfoProcess{
		{Name: "gimp-mcp", CWD: server},
		{Name: "claude", CWD: portal},
	}

	pane := PaneFrom(herdr.PaneInfo{
		PaneID: "wE:p1", Agent: "claude",
		CWD: herdrtest.Dir("work", "dashboard"), ForegroundCWD: server,
	}, time.Time{})
	pane.Dir = PaneDir(processes, pane.Dir)
	pane.Processes = ProcessesFrom(processes)

	if pane.Dir != portal {
		t.Errorf("dir = %q, want the foreground process's own", pane.Dir)
	}

	if len(pane.Processes) != 2 || pane.Processes[0].Name != "gimp-mcp" {
		t.Errorf("processes = %+v, want both, deepest first", pane.Processes)
	}
}

func TestAProcessIsNamedWithoutItsWindowsExtension(t *testing.T) {
	// Windows reports `pwsh.exe` where every other platform reports `pwsh`,
	// and the extension would keep a shell from being read as one.
	processes := ProcessesFrom([]herdr.PaneProcessInfoProcess{
		{Name: "pwsh.exe"}, {Name: "claude.EXE"}, {Name: "nvim"}, {Name: ".exe"},
	})

	for i, want := range []string{"pwsh", "claude", "nvim", ".exe"} {
		if processes[i].Name != want {
			t.Errorf("process %d = %q, want %q", i, processes[i].Name, want)
		}
	}
}

func TestAPaneDirectoryIsCleanedAsItArrives(t *testing.T) {
	// Windows reports a directory with a trailing separator, which no reader
	// of Dir should have to know; a pane without one keeps "" rather than the
	// "." filepath.Clean would make of it.
	dir := herdrtest.Dir("work", "dashboard")
	trailing := dir + string(filepath.Separator)

	pane := PaneFrom(herdr.PaneInfo{PaneID: "wE:p1", CWD: trailing}, time.Time{})
	if pane.Dir != dir {
		t.Errorf("dir = %q from the snapshot, want %q", pane.Dir, dir)
	}

	read := []herdr.PaneProcessInfoProcess{{Name: "pwsh.exe", CWD: trailing}}
	if got := PaneDir(read, ""); got != dir {
		t.Errorf("dir = %q from a process read, want %q", got, dir)
	}

	if none := PaneFrom(herdr.PaneInfo{PaneID: "wE:p2"}, time.Time{}); none.Dir != "" {
		t.Errorf("dir = %q for a pane without one, want it empty", none.Dir)
	}
}

func TestPaneDirKeepsTheSnapshotsGuessWhenItLearnsNone(t *testing.T) {
	if dir := PaneDir(nil, "/work/api"); dir != "/work/api" {
		t.Errorf("dir = %q for an unread pane, want the snapshot's", dir)
	}

	nameOnly := []herdr.PaneProcessInfoProcess{{Name: "nvim"}}
	if dir := PaneDir(nameOnly, "/work/api"); dir != "/work/api" {
		t.Errorf("dir = %q for a process with no directory, want the snapshot's", dir)
	}
}

func TestAPreferredAgentOutranksFocusInAnyState(t *testing.T) {
	// An editor focused beside the agent must not take the tab over, even from
	// an agent that has finished, which the default rules pass over.
	panes := []*PaneState{
		{ID: "wE:p1", Agent: "claude", AgentStatus: "done", ChangedAt: time.Unix(1, 0)},
		{ID: "wE:p2", Focused: true, ChangedAt: time.Unix(2, 0)},
	}

	if got := TabFrom(
		herdr.TabInfo{TabID: "wE:t1"},
		"",
		1,
		panes,
		true,
	).Context; got == nil ||
		got.ID != "wE:p1" {
		t.Fatalf("context = %v, want the agent pane wE:p1", got)
	}

	if got := tabOf(panes).Context; got == nil || got.ID != "wE:p2" {
		t.Fatalf("context = %v without the preference, want the focused pane wE:p2", got)
	}
}

// tabOf builds a tab without the agent preference, which is how it ships.
func tabOf(panes []*PaneState) TabState {
	return TabFrom(herdr.TabInfo{TabID: "wE:t1"}, "", 1, panes, false)
}
