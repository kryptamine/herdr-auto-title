package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func TestATopicNamesATabTheTerminalTitleCannot(t *testing.T) {
	t.Parallel()

	// The session that motivated the source: the agent never titled its
	// terminal, so without the transcript the tab is just `claude`.
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           dashboard,
		TerminalTitle: "Claude Code",
		Agent:         "claude",
		AgentStatus:   "idle",
		AgentTopic:    "grill-me",
	}))

	if want := "dashboard › claude › grill-me"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "transcript" {
		t.Errorf("reason = %q, want transcript", got.Reason)
	}

	if got.Confidence != ConfidenceTranscript {
		t.Errorf("confidence = %d, want %d", got.Confidence, ConfidenceTranscript)
	}
}

func TestATerminalTitleOutranksTheTranscript(t *testing.T) {
	t.Parallel()

	// Both say what the session is about, and the agent says it sooner.
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           dashboard,
		TerminalTitle: "Fix OAuth redirect",
		Agent:         "claude",
		AgentStatus:   herdr.AgentStatusWorking,
		AgentTopic:    "Something the session was called earlier",
	}))

	if want := "dashboard › claude › Fix OAuth redirect"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "terminal_title" {
		t.Errorf("reason = %q, want terminal_title", got.Reason)
	}
}

func TestATopicWithoutAnAgentIsIgnored(t *testing.T) {
	t.Parallel()

	// A pane whose agent Herdr no longer recognizes keeps whatever was read of
	// its session, and that is no longer what the pane is doing.
	if _, ok := NewTranscript().Resolve(&state.PaneState{
		Dir:        dashboard,
		AgentTopic: "grill-me",
	}); ok {
		t.Error("the transcript source claimed a pane with no agent")
	}
}

func TestATopicThatNamesTheAgentFallsThrough(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:         dashboard,
		Agent:       "claude",
		AgentStatus: "idle",
		AgentTopic:  "Claude Code",
	}))

	if want := "dashboard › claude"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

// The Grok case is the one where the session name outranks the window title:
// that title is the live status line, spinner included.
func TestAStableTopicOutranksTheTerminalTitle(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           dashboard,
		TerminalTitle: "Waiting on subagent… - grok",
		Agent:         "grok",
		AgentStatus:   herdr.AgentStatusWorking,
		AgentTopic:    "Fix the login redirect",
		StableTopic:   true,
	}))

	if want := "dashboard › grok › Fix the login redirect"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "session" {
		t.Errorf("reason = %q, want session", got.Reason)
	}
}
