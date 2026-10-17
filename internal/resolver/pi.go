package resolver

import (
	"path/filepath"
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// piAgent is the label Herdr gives Pi. piGlyph and piJoin are how Pi frames
// the one title it sets: `π - <dir>` unnamed, `π - <name> - <dir>` once named.
const (
	piAgent = "pi"
	piGlyph = "π"
	piJoin  = " - "
)

// withoutPiFrame reports what a Pi pane's title says beyond the program and
// its directory, and false while it says nothing else. A title Pi did not
// frame, or any other agent's, passes through as it is.
func withoutPiFrame(pane *state.PaneState, title string) (string, bool) {
	if pane.Agent != piAgent {
		return title, true
	}

	rest, framed := strings.CutPrefix(strings.TrimSpace(title), piGlyph+piJoin)
	if !framed {
		return title, true
	}

	if pane.Dir != "" {
		dir := filepath.Base(pane.Dir)
		if rest == dir {
			return "", false
		}

		if name, named := strings.CutSuffix(rest, piJoin+dir); named {
			rest = name
		}
	}

	return rest, rest != ""
}
