package resolver

import "github.com/kryptamine/herdr-auto-title/internal/state"

// Transcript derives the activity from what the agent's own session says it is
// about. It sits below TerminalTitle because an agent that titles its window
// has already said the same thing, sooner; this answers when it has not.
type Transcript struct{}

var _ Source = Transcript{}

func NewTranscript() Transcript { return Transcript{} }

func (Transcript) Name() string    { return "transcript" }
func (Transcript) Confidence() int { return ConfidenceTranscript }

func (Transcript) Resolve(pane *state.PaneState) (Parts, bool) {
	if !pane.HasAgent() || pane.StableTopic {
		return Parts{}, false
	}

	return activityFrom(pane, pane.AgentTopic)
}

// StableTopic derives the activity from a session name that should outrank the
// terminal title. See docs/architecture/title-resolution.md.
type StableTopic struct{}

var _ Source = StableTopic{}

func NewStableTopic() StableTopic { return StableTopic{} }

func (StableTopic) Name() string    { return "session" }
func (StableTopic) Confidence() int { return ConfidenceStableTopic }

func (StableTopic) Resolve(pane *state.PaneState) (Parts, bool) {
	if !pane.HasAgent() || !pane.StableTopic {
		return Parts{}, false
	}

	return activityFrom(pane, pane.AgentTopic)
}
