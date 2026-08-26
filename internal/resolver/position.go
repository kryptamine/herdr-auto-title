package resolver

import (
	"strconv"

	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// positionMark separates a tab's position from the name that follows it. It is
// deliberately not Separator: the position is not one of the things a title
// says about a tab, and the parts separator would read as if it were.
const positionMark = " · "

// Numbered prefixes every title with its tab's position, which is the key that
// switches to that tab. It wraps a resolver rather than being part of one: the
// position says nothing about what a tab holds, so no source can supply it.
type Numbered struct {
	inner *Deterministic
}

var _ TitleResolver = (*Numbered)(nil)

// NewNumbered wraps inner. It takes no width of its own: the position is
// counted against inner's bound rather than added on top of it, and two
// numbers to keep in step would only be one to get wrong.
func NewNumbered(inner *Deterministic) *Numbered {
	return &Numbered{inner: inner}
}

// Resolve names the tab and puts its position in front, unless the tab bar is
// too narrow to carry both.
func (n *Numbered) Resolve(tab state.TabState) Decision {
	decision := n.inner.Resolve(tab)

	// The prefix goes in front because truncation cuts the tail: a position at
	// the end is the first thing a title too wide for the tab bar would lose.
	prefix := strconv.Itoa(tab.Position) + positionMark
	room := n.inner.maxLength - uniseg.StringWidth(prefix)
	// Sanitize takes a width of zero as "no bound at all", and a title reduced
	// to nothing has lost more than the position is worth.
	if room <= 0 {
		return decision
	}

	name := Sanitize(decision.Name, room)
	if name == "" {
		return decision
	}

	decision.Name = prefix + name

	return decision
}
