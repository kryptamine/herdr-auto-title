package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// codexPane is a Codex pane in dashboard whose terminal reads title, which is
// all Codex says about itself until its session is read.
func codexPane(title string) *state.PaneState {
	return &state.PaneState{
		Dir:           dashboard,
		TerminalTitle: title,
		Agent:         "codex",
		AgentStatus:   herdr.AgentStatusWorking,
	}
}

func TestAnUnnamedCodexSessionTitlesOnlyItsProject(t *testing.T) {
	t.Parallel()

	// Codex's title says which project it sits in, and the context already
	// says where, so the title must not become the activity.
	got := defaultChain().Resolve(tabWithPane(codexPane("dashboard")))

	if want := "dashboard › codex"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason == "terminal_title" {
		t.Errorf("reason = %q, want the title declined", got.Reason)
	}
}

func TestANamedCodexSessionIsTitledByItsName(t *testing.T) {
	t.Parallel()

	// The project is the last item whatever it is called: Codex names it after
	// the repository root, which a pane in a subdirectory does not share.
	for _, title := range []string{
		"Fix login form | dashboard",
		"Fix login form | web-platform-mono...",
		"● Fix login form | dashboard",
		"[ ! ] Action Required | Fix login form | dashboard",
		"[ . ] Action Required | Fix login form | dashboard",
	} {
		got := defaultChain().Resolve(tabWithPane(codexPane(title)))

		if want := "dashboard › codex › Fix login form"; got.Name != want {
			t.Errorf("%q: name = %q, want %q", title, got.Name, want)
		}
	}
}

func TestACodexSessionNameKeepsItsOwnSeparators(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(codexPane("auth | redirect fix | dashboard")))

	if want := "dashboard › codex › auth | redirect fix"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAnUnnamedCodexSessionWaitingOnTheUserTitlesOnlyItsProject(t *testing.T) {
	t.Parallel()

	// The prefix blinks, so a title that kept it would rename the tab twice a
	// second while Codex waits.
	got := defaultChain().Resolve(tabWithPane(codexPane("[ ! ] Action Required | dashboard")))

	if got.Reason == "terminal_title" {
		t.Errorf("reason = %q, name = %q, want the title declined", got.Reason, got.Name)
	}
}

func TestACodexSessionThatWasReadIsTitledFromIt(t *testing.T) {
	t.Parallel()

	// The title cuts a long name; the session index holds it whole.
	pane := codexPane("Rework the poll loop so a pane that moved is read... | dashboard")
	pane.AgentTopic = "Rework the poll loop so a pane that moved is read again"

	got := defaultChain().Resolve(tabWithPane(pane))

	if got.Reason != "transcript" {
		t.Errorf("reason = %q, name = %q, want transcript", got.Reason, got.Name)
	}
}

func TestAnotherAgentsTitleIsNotReadAsCodexs(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           dashboard,
		TerminalTitle: "build | test",
		Agent:         "claude",
		AgentStatus:   herdr.AgentStatusWorking,
	}))

	if want := "dashboard › claude › build | test"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}
