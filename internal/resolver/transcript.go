package resolver

import "github.com/kryptamine/herdr-auto-title/internal/state"

// Transcript derives the activity from what the agent's own session says it is
// about. It sits below TerminalTitle because an agent that titles its window
// has already said the same thing, sooner; this answers when it has not.
type Transcript struct{ agentFormat string }

var _ Source = Transcript{}

func NewTranscript(agentFormat string) Transcript { return Transcript{agentFormat: agentFormat} }

func (Transcript) Name() string    { return "transcript" }
func (Transcript) Confidence() int { return ConfidenceTranscript }

func (t Transcript) Resolve(pane *state.PaneState) (Parts, bool) {
	if !pane.HasAgent() {
		return Parts{}, false
	}

	return activityFrom(pane, pane.AgentTopic, t.agentFormat)
}
