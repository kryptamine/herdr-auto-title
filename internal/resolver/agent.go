package resolver

import (
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// Agent derives the activity from what the pane's agent says it is working on,
// which outranks every other source. Many agents leave the field empty and fall
// through to TerminalTitle instead.
type Agent struct{}

var _ Source = Agent{}

// NewAgent builds the source.
func NewAgent() Agent { return Agent{} }

func (Agent) Name() string    { return "agent" }
func (Agent) Confidence() int { return ConfidenceAgent }

func (Agent) Resolve(pane *state.PaneState) (Parts, bool) {
	if !pane.HasAgent() {
		return Parts{}, false
	}

	// Truncation belongs to the assembled name, so no limit is applied here.
	activity, ok := Meaningful(Sanitize(pane.AgentTitle, 0))
	if !ok {
		return Parts{}, false
	}

	// An agent with nothing to report often echoes its own name. That is as
	// generic as anything in the table, but the name differs per agent, so it
	// is compared against the pane instead of being listed.
	if strings.EqualFold(activity, pane.Agent) || strings.EqualFold(activity, pane.DisplayAgent) {
		return Parts{}, false
	}

	return Parts{Activity: qualify(activity, paneKind(pane))}, true
}
