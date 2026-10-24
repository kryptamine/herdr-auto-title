package resolver

import (
	"github.com/kryptamine/herdr-auto-title/internal/codex"
	"github.com/kryptamine/herdr-auto-title/internal/pi"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// agentTitle reports what a pane's terminal title says about the work, and
// false when its agent uses the title for something else. Such a pane is then
// named by the agent's own session, one rung down.
func agentTitle(pane *state.PaneState, title string) (string, bool) {
	switch pane.Agent {
	case pi.Agent:
		return withoutPiFrame(pane, title)
	case codex.Agent:
		// The title cuts the name at 48 characters; a session that was read
		// holds it whole.
		if pane.AgentTopic != "" {
			return "", false
		}

		return codex.ThreadName(title)
	}

	return title, true
}
