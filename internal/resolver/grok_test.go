package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// grokPane is a Grok pane in dashboard whose terminal reads title, spelled as
// Herdr strips it, and whose summary said topic.
func grokPane(title, topic string) *state.PaneState {
	return &state.PaneState{
		Dir:           dashboard,
		TerminalTitle: title,
		Agent:         "grok",
		AgentStatus:   herdr.AgentStatusWorking,
		AgentTopic:    topic,
	}
}

func TestAnIdleGrokTitleNamesTheSession(t *testing.T) {
	t.Parallel()

	// The title follows /resume at once, while Herdr's session id does not.
	got := defaultChain().Resolve(tabWithPane(grokPane("Casual greeting - grok", "")))

	if want := "dashboard › grok › Casual greeting"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAWorkingGrokTitleLeavesItsStatusOut(t *testing.T) {
	t.Parallel()

	// A working Grok puts a spinner and its status in front of the name, and
	// the tab must not change with every step it takes.
	for _, pane := range []*state.PaneState{
		grokPane("- Thinking - Casual greeting - grok", ""),
		{
			Dir:              dashboard,
			TerminalTitleRaw: "⠙ - Waiting for response… - Casual greeting - grok",
			Agent:            "grok",
			AgentStatus:      herdr.AgentStatusWorking,
		},
	} {
		got := defaultChain().Resolve(tabWithPane(pane))

		if want := "dashboard › grok › Casual greeting"; got.Name != want {
			t.Errorf("name = %q, want %q", got.Name, want)
		}
	}
}

func TestACutGrokTitleYieldsToTheWholeNameItsSummaryRead(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(grokPane(
		"Fix the login redi… - grok",
		"Fix the login redirect",
	)))

	if got.Reason != "transcript" {
		t.Errorf("reason = %q, name = %q, want the summary's whole name", got.Reason, got.Name)
	}
}

func TestACutGrokTitleNamesThePaneWhenNothingElseDoes(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(grokPane("Fix the login redi… - grok", "")))

	if want := "dashboard › grok › Fix the login redi…"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAGrokTitleInAnotherShapeIsDeclined(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(grokPane("grok", "")))

	if got.Reason == "terminal_title" {
		t.Errorf("reason = %q, name = %q, want the title declined", got.Reason, got.Name)
	}
}
