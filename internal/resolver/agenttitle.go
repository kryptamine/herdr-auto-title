package resolver

import (
	"github.com/kryptamine/herdr-auto-title/internal/grok"
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
	case grok.Agent:
		return withoutGrokFrame(pane, title)
	}

	return title, true
}
