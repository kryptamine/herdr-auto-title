package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// agentPane is a working agent that has titled its own work, the case the
// agent format shapes.
func agentPane() *state.PaneState {
	return &state.PaneState{
		Dir:         "/Users/dev/work/dashboard",
		Agent:       "claude",
		AgentStatus: herdr.AgentStatusWorking,
		AgentTitle:  "Implement OAuth scopes",
	}
}

func TestDefaultFormatKeepsTheAgentPrefix(t *testing.T) {
	got := Default(DefaultMaxLength, DefaultBranchMaxLength, DefaultAgentFormat).Resolve(tabWithPane(agentPane()))

	if want := "dashboard › claude › Implement OAuth scopes"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestFormatWithoutAgentDropsThePrefix(t *testing.T) {
	got := Default(DefaultMaxLength, DefaultBranchMaxLength, "{activity}").Resolve(tabWithPane(agentPane()))

	if want := "dashboard › Implement OAuth scopes"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	// The activity still comes from the agent; only the `claude ›` is gone.
	if got.Reason != "agent" {
		t.Errorf("reason = %q, want agent", got.Reason)
	}
}

func TestFormatCanReorderAndRestyleTheParts(t *testing.T) {
	pane := agentPane()
	pane.AgentTitle = "Fix bug"

	got := Default(DefaultMaxLength, DefaultBranchMaxLength, "{activity} ({agent})").Resolve(tabWithPane(pane))

	if want := "dashboard › Fix bug (claude)"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestTheFormatDoesNotTouchANonAgentPane(t *testing.T) {
	// A plain editor pane keeps the built-in separator whatever the agent
	// format says: the format is for agent panes only.
	pane := &state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "auth.provider.ts - Nvim",
		Processes:     running("nvim"),
	}

	got := Default(DefaultMaxLength, DefaultBranchMaxLength, "{activity}").Resolve(tabWithPane(pane))

	if want := "dashboard › nvim › auth.provider.ts"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}
