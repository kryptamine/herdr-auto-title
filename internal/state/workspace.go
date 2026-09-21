package state

import (
	"path/filepath"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// WorkspaceState is one workspace as a poll last read it. Herdr shows its label
// above the workspace's tabs, and derives that label from the directory the
// workspace was created in.
type WorkspaceState struct {
	ID string
	// CurrentName lets a poll skip a rename that would change nothing.
	CurrentName string
	// DefaultName is what Herdr calls a workspace nobody has renamed: the
	// basename of the directory it holds. Unlike a tab, a workspace never wears
	// its position (herdr-socket-api.md).
	DefaultName string
	// Context is the pane the workspace is named after: the context pane of
	// the only tab it holds. Nil when that tab holds no pane; the resolver then
	// has nothing to name the row after and leaves it alone.
	Context *PaneState
}

// WorkspaceFrom builds a workspace from its snapshot entry and the pane it is
// judged through. A nil pane leaves DefaultName empty: with no directory there
// is no basename to tell an owner's label from.
func WorkspaceFrom(info herdr.WorkspaceInfo, context *PaneState) WorkspaceState {
	ws := WorkspaceState{
		ID:          info.WorkspaceID,
		CurrentName: info.Label,
		Context:     context,
	}
	if context != nil {
		ws.DefaultName = dirBase(context.Dir)
	}

	return ws
}

// dirBase is the basename Herdr derives a workspace label from. A relative path
// or a root -- `/`, or a drive's `C:\` -- yields nothing, which reads as "no
// default to compare against" rather than as a label.
func dirBase(dir string) string {
	clean := cleanDir(dir)
	root := filepath.VolumeName(clean) + string(filepath.Separator)

	if clean == "" || !filepath.IsAbs(clean) || clean == root {
		return ""
	}

	return filepath.Base(clean)
}

// WorkspaceSightingFrom is what a poll saw of a workspace. Its default cannot
// move on its own as a tab's position can -- Herdr writes it once from the
// directory -- so a label that differs is one the user wrote.
func WorkspaceSightingFrom(ws WorkspaceState, desired string) Sighting {
	return Sighting{
		ID:      ws.ID,
		Current: ws.CurrentName,
		Desired: desired,
		Default: ws.DefaultName,
	}
}
