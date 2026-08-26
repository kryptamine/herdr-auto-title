package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func TestATopicNamesATabTheTerminalTitleCannot(t *testing.T) {
	// The session that motivated the source: the agent never titled its
	// terminal, so without the transcript the tab is just `claude`.
	got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Claude Code",
		Agent:         "claude",
		AgentStatus:   herdr.AgentStatusIdle,
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
	// Both say what the session is about, and the agent says it sooner.
	got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
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
	// A pane whose agent Herdr no longer recognizes keeps whatever was read of
	// its session, and that is no longer what the pane is doing.
	if _, ok := NewTranscript().Resolve(&state.PaneState{
		Dir:        "/Users/dev/work/dashboard",
		AgentTopic: "grill-me",
	}); ok {
		t.Error("the transcript source claimed a pane with no agent")
	}
}

func TestATopicThatNamesTheAgentFallsThrough(t *testing.T) {
	got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(&state.PaneState{
		Dir:         "/Users/dev/work/dashboard",
		Agent:       "claude",
		AgentStatus: herdr.AgentStatusIdle,
		AgentTopic:  "Claude Code",
	}))

	if want := "dashboard › claude"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestTranscriptSourceOnANilPane(t *testing.T) {
	if _, ok := NewTranscript().Resolve(nil); ok {
		t.Fatal("resolved a nil pane")
	}
}
