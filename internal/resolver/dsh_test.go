package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func dshPane(title string) *state.PaneState {
	return &state.PaneState{
		Dir:           dashboard,
		TerminalTitle: title,
		Agent:         dshAgent,
		AgentStatus:   herdr.AgentStatusWorking,
	}
}

// DeepSeek Harness alternates `✦` and `⠂`/`⠐` in front of `🐋 <title>`, and
// Herdr strips only the braille spinner, so the frame changed with the state.
func TestEveryDshStateReadsAsOneTitle(t *testing.T) {
	t.Parallel()

	const want = "dashboard › dsh-tui › Rework the poll loop"

	for _, title := range []string{
		"✦ 🐋 Rework the poll loop",
		"⠂ 🐋 Rework the poll loop",
		"⠐ 🐋 Rework the poll loop",
		"🐋 Rework the poll loop",
	} {
		if got := defaultChain().Resolve(tabWithPane(dshPane(title))).Name; got != want {
			t.Errorf("%q: name = %q, want %q", title, got, want)
		}
	}
}

// Until a session has a title DeepSeek Harness sets only the frame, which
// otherwise names the tab `✦ 🐋` or `🐋`.
func TestAnUntitledDshSessionDeclinesItsTitle(t *testing.T) {
	t.Parallel()

	for _, title := range []string{"✦ 🐋", "✦ 🐋 ", "⠂ 🐋", "⠐ 🐋", "🐋", "🐋 "} {
		got := defaultChain().Resolve(tabWithPane(dshPane(title)))
		if got.Name != "dashboard › dsh-tui" {
			t.Errorf("%q: name = %q, want %q", title, got.Name, "dashboard › dsh-tui")
		}

		if got.Reason == "terminal_title" {
			t.Errorf("%q: reason = %q, want the frame declined", title, got.Reason)
		}
	}
}

// Only DeepSeek Harness's own frame comes off: a title that merely starts with
// its idle state, its mark or another glyph before the mark keeps every word.
func TestATitleDshDidNotFrameIsReadAsItIs(t *testing.T) {
	t.Parallel()

	for _, title := range []string{"Deploying staging", "✦ release notes", "🐋-watcher", "A 🐋 sighting"} {
		if got, want := defaultChain().Resolve(tabWithPane(dshPane(title))).
			Name, "dashboard › dsh-tui › "+title; got != want {
			t.Errorf("name = %q, want %q", got, want)
		}
	}
}

// The frame is read off a dsh-tui pane alone; another agent's title is its own.
func TestAnotherAgentsDshTitleIsReadAsItIs(t *testing.T) {
	t.Parallel()

	pane := dshPane("✦ 🐋 Rework the poll loop")
	pane.Agent = "claude"

	if got, want := defaultChain().Resolve(tabWithPane(pane)).
		Name, "dashboard › claude › ✦ 🐋 Rework the poll loop"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
}
