package resolver

import "github.com/kryptamine/herdr-auto-title/internal/state"

// Agent derives the activity from what the pane's agent says it is working on,
// which outranks every other source. Many agents leave the field empty and fall
// through to TerminalTitle instead.
type Agent struct{ agentFormat string }

var _ Source = Agent{}

func NewAgent(agentFormat string) Agent { return Agent{agentFormat: agentFormat} }

func (Agent) Name() string    { return "agent" }
func (Agent) Confidence() int { return ConfidenceAgent }

func (a Agent) Resolve(pane *state.PaneState) (Parts, bool) {
	if !pane.HasAgent() {
		return Parts{}, false
	}

	return activityFrom(pane, pane.AgentTitle, a.agentFormat)
}
