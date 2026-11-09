package resolver

import (
	"path/filepath"
	"strings"

	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// ompAgent is the agent Herdr reports for an oh-my-pi pane.
const ompAgent = "omp"

// withoutOmpFrame reports the name in an omp pane's title, `π <state> <name>`,
// and false for the frame alone. The state is a spinner frame while omp works,
// redrawn every 80 ms, so a title read with it renames the tab on every frame.
func withoutOmpFrame(pane *state.PaneState, title string) (string, bool) {
	head, name, _ := strings.Cut(strings.TrimSpace(title), " ")

	switch head {
	case piGlyph + ":": // the frame with the state turned off
	case piGlyph:
		mark, rest, _ := strings.Cut(name, " ")
		if uniseg.GraphemeClusterCount(mark) > 1 {
			return title, true
		}

		name = rest
	default:
		return title, true
	}

	// Until the session has a name of its own, omp frames its directory.
	name = strings.TrimSpace(name)
	if name == "" || (pane.Dir != "" && name == filepath.Base(pane.Dir)) {
		return "", false
	}

	return name, true
}
