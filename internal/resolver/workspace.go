package resolver

import (
	"strings"

	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// DefaultWorkspaceMaxLength bounds a generated workspace label: twenty columns
// is what a default-width sidebar was measured to hold, and the sidebar is
// narrower than the tab bar DefaultMaxLength is sized for.
const DefaultWorkspaceMaxLength = 20

// WorkspaceResolver names the row Herdr shows above a workspace's tabs.
type WorkspaceResolver interface {
	ResolveWorkspace(ws state.WorkspaceState) Decision
}

var _ WorkspaceResolver = (*Deterministic)(nil)

// Places builds the resolver a workspace row is named by: the shipped chain
// without the foreground process, and the terminal title read without it too.
// Why the row must not follow the process is in title-resolution.md.
func Places(opts Options) *Deterministic {
	return New(opts,
		NewAgent(),
		NewUnboundTerminalTitle(),
		NewTranscript(),
		NewSSH(),
		NewGit(opts.BranchMax),
		NewCWD(opts.Home),
	)
}

// ResolveWorkspace names a workspace after the one tab it holds. An empty name
// leaves the row alone, which is the answer wherever there is nothing to say.
func (d *Deterministic) ResolveWorkspace(ws state.WorkspaceState) Decision {
	// collect takes a nil pane and answers nothing, so a workspace whose tab
	// holds no pane falls out here with an empty name.
	found := d.collect(ws.Context)

	name := FormatTail(withoutRepetition(found.parts), d.maxLength)
	if name == "" {
		return Decision{}
	}

	return Decision{Name: name, Confidence: found.confidence, Reason: found.reason}
}

// FormatTail assembles a label that keeps its end: the parts nearest the front
// go whole -- context first, then branch -- and only what is left is cut, from
// the end, by Sanitize. Why a row is not cut as a tab is: title-resolution.md.
func FormatTail(parts Parts, maxLen int) string {
	ordered := []string{parts.Context, parts.Branch, parts.Agent, parts.Activity}

	kept := make([]string, 0, len(ordered))

	for _, part := range ordered {
		if part != "" {
			kept = append(kept, part)
		}
	}

	// Measured on what would actually be written rather than on the raw parts:
	// Sanitize collapses runs of whitespace, so a part counted wide here can
	// turn out to fit, and a row would lose its context to a space.
	for len(kept) > 1 && uniseg.StringWidth(Sanitize(strings.Join(kept, Separator), 0)) > maxLen {
		kept = kept[1:]
	}

	return Sanitize(strings.Join(kept, Separator), maxLen)
}
