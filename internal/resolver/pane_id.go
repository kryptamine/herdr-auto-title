package resolver

import (
	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// PaneIDs prefixes every pane label with the pane's Herdr ID, the handle
// `herdr agent prompt` and every other pane command take. It wraps a resolver
// for the reason Numbered does: the ID says nothing about what a pane holds.
type PaneIDs struct {
	inner PaneResolver
	// maxLength is the bound inner was built with; the ID is counted against
	// it, as Numbered counts the position.
	maxLength int
}

var _ PaneResolver = (*PaneIDs)(nil)

// NewPaneIDs wraps inner, which must have been given the same maxLength.
// Zero or less takes the default, as New does.
func NewPaneIDs(inner PaneResolver, maxLength int) *PaneIDs {
	if maxLength <= 0 {
		maxLength = DefaultMaxLength
	}

	return &PaneIDs{inner: inner, maxLength: maxLength}
}

// ResolvePanes names each pane and puts its ID in front, unless the label is
// too narrow to carry both.
func (p *PaneIDs) ResolvePanes(tab state.TabState) []Decision {
	decisions := p.inner.ResolvePanes(tab)

	for i := range decisions {
		prefix := "[" + tab.Panes[i].ID + "] "
		room := p.maxLength - uniseg.StringWidth(prefix)
		// truncate takes a width of zero as "no bound at all".
		if room <= 0 {
			continue
		}

		if name := truncate(decisions[i].Name, room); name != "" {
			decisions[i].Name = prefix + name
		}
	}

	return decisions
}
