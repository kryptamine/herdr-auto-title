package resolver

import (
	"path/filepath"
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// piGlyph, cavemanPrefix and piJoin are how Pi and Caveman Code frame the
// title each sets: `<prefix> - <dir>` unnamed, `<prefix> - <name> - <dir>`
// once named.
const (
	piGlyph       = "π"
	cavemanPrefix = "cave"
	piJoin        = " - "
)

// withoutPiFrame reports what a Pi or Caveman Code pane's title says beyond
// the program and its directory, and false while it says nothing else. A title
// the agent did not frame passes through as it is.
func withoutPiFrame(pane *state.PaneState, title, prefix string) (string, bool) {
	rest, framed := strings.CutPrefix(strings.TrimSpace(title), prefix+piJoin)
	if !framed {
		return title, true
	}

	// Without the pane's directory the frame's tail cannot be told from the
	// name, so the session file names the pane instead.
	if pane.Dir == "" {
		return "", false
	}

	dir := filepath.Base(pane.Dir)
	if rest == dir {
		return "", false
	}

	if name, named := strings.CutSuffix(rest, piJoin+dir); named {
		rest = name
	}

	return rest, rest != ""
}
