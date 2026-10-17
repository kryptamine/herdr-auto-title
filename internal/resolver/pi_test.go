package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// piPane is a Pi pane in dashboard whose terminal reads title, which is the
// only thing Pi says about itself until its session is named.
func piPane(title string) *state.PaneState {
	return &state.PaneState{
		Dir:           dashboard,
		TerminalTitle: title,
		Agent:         "pi",
		AgentStatus:   herdr.AgentStatusWorking,
	}
}

func TestAnUnnamedPiSessionTitlesOnlyItsDirectory(t *testing.T) {
	t.Parallel()

	// Pi's title says which program sits where, and the context already says
	// where, so the title must not become the activity.
	got := defaultChain().Resolve(tabWithPane(piPane("π - dashboard")))

	if want := "dashboard › pi"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason == "terminal_title" {
		t.Errorf("reason = %q, want the title declined", got.Reason)
	}
}

func TestANamedPiSessionIsTitledByItsName(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(piPane("π - Rework the poll loop - dashboard")))

	if want := "dashboard › pi › Rework the poll loop"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "terminal_title" {
		t.Errorf("reason = %q, want terminal_title", got.Reason)
	}
}

func TestAPiSessionNameKeepsItsOwnDashes(t *testing.T) {
	t.Parallel()

	// Only the glyph in front and the directory behind are Pi's; a dash inside
	// the name is the user's.
	got := defaultChain().Resolve(tabWithPane(piPane("π - oauth - redirect fix - dashboard")))

	if want := "dashboard › pi › oauth - redirect fix"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestATitleAPiExtensionSetIsReadAsItIs(t *testing.T) {
	t.Parallel()

	// An extension may title the terminal itself, and then nothing frames it.
	got := defaultChain().Resolve(tabWithPane(piPane("Deploying staging")))

	if want := "dashboard › pi › Deploying staging"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAnUnnamedPiSessionFallsThroughToItsTranscript(t *testing.T) {
	t.Parallel()

	// The case the frame check exists for: with the title declined, what the
	// session file says is the first thing left that names the tab.
	pane := piPane("π - dashboard")
	pane.AgentTopic = "rework the poll loop"

	got := defaultChain().Resolve(tabWithPane(pane))

	if want := "dashboard › pi › rework the poll loop"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "transcript" {
		t.Errorf("reason = %q, want transcript", got.Reason)
	}
}

func TestAnotherAgentsTitleIsNotReadAsPis(t *testing.T) {
	t.Parallel()

	// The frame is Pi's grammar, not a general one: another agent's title that
	// happens to start the same way is still that agent's whole statement.
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           dashboard,
		TerminalTitle: "π - the math service",
		Agent:         "claude",
		AgentStatus:   herdr.AgentStatusWorking,
	}))

	if want := "dashboard › claude › π - the math service"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}
