package resolver

import (
	"path/filepath"
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// piGlyph and piJoin are how Pi frames the one title it sets: `π - <dir>`
// unnamed, `π - <name> - <dir>` once named.
const (
	piGlyph = "π"
	piJoin  = " - "
)

// withoutPiFrame reports what a Pi pane's title says beyond the program and
// its directory, and false while it says nothing else. A title Pi did not
// frame passes through as it is.
func withoutPiFrame(pane *state.PaneState, title string) (string, bool) {
	rest, framed := strings.CutPrefix(strings.TrimSpace(title), piGlyph+piJoin)
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
