package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// openCodePane is an OpenCode pane in dashboard whose terminal reads title.
func openCodePane(title string) *state.PaneState {
	return &state.PaneState{
		Dir:           dashboard,
		TerminalTitle: title,
		Agent:         "opencode",
		AgentStatus:   herdr.AgentStatusIdle,
	}
}

func TestAnOpenCodeTitleNamesTheSessionWithoutItsPrefix(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(openCodePane("OC | Fix the login redirect")))

	if want := "dashboard › opencode › Fix the login redirect"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAnUntitledOpenCodeSessionIsNamedAfterTheAgent(t *testing.T) {
	t.Parallel()

	// OpenCode titles its home screen and an unnamed session with its own name.
	got := defaultChain().Resolve(tabWithPane(openCodePane("OpenCode")))

	if want := "dashboard › opencode"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAnEmptyOpenCodeTitleIsDeclined(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(openCodePane("OC | ")))

	if got.Reason == "terminal_title" {
		t.Errorf("name = %q, want the frame alone declined", got.Name)
	}
}

func TestAnotherAgentsOCTitleIsReadAsItIs(t *testing.T) {
	t.Parallel()

	pane := openCodePane("OC | Fix the login redirect")
	pane.Agent = "claude"
	got := defaultChain().Resolve(tabWithPane(pane))

	if want := "dashboard › claude › OC | Fix the login redirect"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}
