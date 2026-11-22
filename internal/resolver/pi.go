package resolver

import (
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

	// A case-insensitive directory can reach Herdr with a different spelling
	// than the one Pi wrote into its terminal title. The frame is the same
	// directory either way, so compare it without regard to case.
	dir := pane.DirBase()
	if strings.EqualFold(rest, dir) {
		return "", false
	}

	suffix := piJoin + dir
	if len(rest) >= len(suffix) &&
		strings.EqualFold(rest[len(rest)-len(suffix):], suffix) {
		rest = rest[:len(rest)-len(suffix)]
	}

	return rest, rest != ""
}
