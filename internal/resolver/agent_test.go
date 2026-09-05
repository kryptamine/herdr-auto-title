package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func TestAgentTitleBeatsEverySourceBelowIt(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Claude Code",
		Agent:         "claude",
		AgentStatus:   herdr.AgentStatusWorking,
		AgentTitle:    "Implement OAuth scopes",
	}))

	if want := "dashboard › claude › Implement OAuth scopes"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "agent" {
		t.Errorf("reason = %q, want agent", got.Reason)
	}

	if got.Confidence != ConfidenceAgent {
		t.Errorf("confidence = %d, want %d", got.Confidence, ConfidenceAgent)
	}
}

func TestAgentTitleOutranksAMeaningfulTerminalTitle(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Fix OAuth redirect",
		Agent:         "claude",
		AgentStatus:   herdr.AgentStatusWorking,
		AgentTitle:    "Implement OAuth scopes",
	}))

	if want := "dashboard › claude › Implement OAuth scopes"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestGenericAgentNameFallsThrough(t *testing.T) {
	// An agent that has nothing to report titles itself. In a live session the
	// topic then arrives through the terminal title instead.
	for _, title := range []string{"Claude", "Claude Code", "Agent", "Coding Agent", ""} {
		t.Run(title, func(t *testing.T) {
			got := defaultChain().Resolve(tabWithPane(&state.PaneState{
				Dir:           "/Users/dev/work/dashboard",
				TerminalTitle: "Fix OAuth redirect",
				Agent:         "claude",
				AgentStatus:   herdr.AgentStatusWorking,
				AgentTitle:    title,
			}))

			if want := "dashboard › claude › Fix OAuth redirect"; got.Name != want {
				t.Errorf("name = %q, want %q", got.Name, want)
			}

			if got.Reason != "terminal_title" {
				t.Errorf("reason = %q, want terminal_title", got.Reason)
			}
		})
	}
}

func TestAgentEchoingItsOwnNameIsNotAgentContext(t *testing.T) {
	// Agents the generic table has never heard of must not pass their own name
	// off as a report of their work. The name still reaches the tab, but as the
	// kind of program running there rather than as what it is doing.
	pane := &state.PaneState{
		Dir:          "/Users/dev/work/dashboard",
		Agent:        "acme-bot",
		DisplayAgent: "Acme Bot",
		AgentStatus:  herdr.AgentStatusWorking,
		AgentTitle:   "Acme Bot",
	}

	if _, ok := NewAgent().Resolve(pane); ok {
		t.Error("the agent source claimed an agent naming itself")
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "dashboard › acme-bot"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "process" {
		t.Errorf("reason = %q, want process", got.Reason)
	}
}

func TestAnEchoedAgentNameIsDeclinedWhateverReportsIt(t *testing.T) {
	// A terminal title and a transcript topic carry the echo as readily as the
	// agent title does, and it says no more about the work there.
	pane := &state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		Agent:         "acme-bot",
		DisplayAgent:  "Acme Bot",
		AgentStatus:   herdr.AgentStatusWorking,
		TerminalTitle: "Acme Bot",
		AgentTopic:    "acme-bot",
	}

	if _, ok := NewTerminalTitle().Resolve(pane); ok {
		t.Error("the terminal title source claimed an agent naming itself")
	}

	if _, ok := NewTranscript().Resolve(pane); ok {
		t.Error("the transcript source claimed an agent naming itself")
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "dashboard › acme-bot"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAgentTitleWithoutAnAgentIsIgnored(t *testing.T) {
	// Herdr leaves the title on a pane whose agent it no longer recognizes;
	// without an agent it is not agent context.
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:        "/Users/dev/work/dashboard",
		AgentTitle: "Implement OAuth scopes",
	}))

	if got.Name != "dashboard" {
		t.Errorf("name = %q, want %q", got.Name, "dashboard")
	}
}

func TestAgentTitleWithNoDirectoryStandsAlone(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Agent:       "claude",
		AgentStatus: herdr.AgentStatusWorking,
		AgentTitle:  "Implement OAuth scopes",
	}))

	if want := "claude › Implement OAuth scopes"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestContextAndActivityComeFromTheSamePane(t *testing.T) {
	// The agent pane wins the selection; the other pane's directory must not
	// leak into the name and describe neither of them.
	tab := state.TabState{
		ID: "wE:t1",
		Panes: []*state.PaneState{
			{
				ID:            "wE:p1",
				Dir:           "/Users/dev/work/api",
				TerminalTitle: "Run migrations",
			},
			{
				ID:          "wE:p2",
				Dir:         "/Users/dev/work/dashboard",
				Agent:       "claude",
				AgentStatus: herdr.AgentStatusWorking,
				AgentTitle:  "Implement OAuth scopes",
			},
		},
	}

	got := defaultChain().Resolve(tab)
	if want := "dashboard › claude › Implement OAuth scopes"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAnAgentTabDoesNotRepeatItsOwnDirectory(t *testing.T) {
	// Claude Code titles its terminal after the project it was started in, so
	// the activity says what the context already does. The agent's name in
	// front of it must not hide that.
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "dashboard",
		Agent:         "claude",
		AgentStatus:   herdr.AgentStatusWorking,
	}))

	if want := "dashboard › claude"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func hiddenAgentChain() *Deterministic {
	return Default(Options{
		MaxLength:     DefaultMaxLength,
		BranchMax:     DefaultBranchMaxLength,
		HideAgentName: true,
	})
}

func TestAHiddenAgentNameLeavesTheWorkAlone(t *testing.T) {
	got := hiddenAgentChain().Resolve(tabWithPane(&state.PaneState{
		Dir:         "/Users/dev/work/dashboard",
		Agent:       "claude",
		AgentStatus: herdr.AgentStatusWorking,
		AgentTitle:  "Implement OAuth scopes",
	}))

	if want := "dashboard › Implement OAuth scopes"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAHiddenAgentNameIsAlsoStrippedFromTheWork(t *testing.T) {
	// An agent that signs its terminal title must not smuggle its name back in
	// as text once the name itself is turned off.
	got := hiddenAgentChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Claude — Implement OAuth scopes",
		Agent:         "claude",
		AgentStatus:   herdr.AgentStatusWorking,
	}))

	if want := "dashboard › Implement OAuth scopes"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestASilentAgentHiddenLeavesTheTabToItsDirectory(t *testing.T) {
	// The name is the whole title of a pane whose agent has reported nothing,
	// so turning it off has to leave that tab named like any other.
	pane := &state.PaneState{
		Dir:         "/Users/dev/work/dashboard",
		Agent:       "claude",
		AgentStatus: herdr.AgentStatusWorking,
	}

	if got := defaultChain().Resolve(tabWithPane(pane)); got.Name != "dashboard › claude" {
		t.Errorf("name = %q, want dashboard › claude", got.Name)
	}

	if got := hiddenAgentChain().Resolve(tabWithPane(pane)); got.Name != "dashboard" {
		t.Errorf("name = %q, want dashboard", got.Name)
	}
}

func TestASilentAgentHiddenWithNoDirectoryFallsBack(t *testing.T) {
	got := hiddenAgentChain().Resolve(tabWithPane(&state.PaneState{
		Agent:       "claude",
		AgentStatus: herdr.AgentStatusWorking,
	}))

	if got.Name != GenericFallback {
		t.Errorf("name = %q, want %q", got.Name, GenericFallback)
	}
}

func TestAHiddenAgentNameKeepsAWorkspaceDirectory(t *testing.T) {
	// The workspace is dropped only when something else is left to read, and a
	// name that is about to be hidden is not something else.
	tab := tabWithPane(&state.PaneState{
		Dir:         "/Users/dev/work/dashboard",
		Agent:       "claude",
		AgentStatus: herdr.AgentStatusWorking,
	})
	tab.WorkspaceName = "dashboard"

	if got := defaultChain().Resolve(tab); got.Name != "claude" {
		t.Errorf("name = %q, want claude", got.Name)
	}

	if got := hiddenAgentChain().Resolve(tab); got.Name != "dashboard" {
		t.Errorf("name = %q, want dashboard", got.Name)
	}
}
