package resolver

import (
	"strconv"

	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// Fit fits a name into width columns of the tab bar; a width of zero is no
// bound. The tab id lets a fit carry something between polls, which is what
// sliding needs and truncation does not.
type Fit func(tabID, name string, width int) string

func truncateFit(_, name string, width int) string {
	return truncate(name, width)
}

// fitter is the fit the options ask for, truncation when they ask for none.
func (o Options) fitter() Fit {
	if o.Fit == nil {
		return truncateFit
	}

	return o.Fit
}

// positionMark separates a tab's position from the name that follows it. It is
// deliberately not Separator: the position is not one of the things a title
// says about a tab, and the parts separator would read as if it were.
const positionMark = " · "

// Fitted fits a title to the tab bar and, when asked, puts the tab's position
// in front — the key that switches to that tab. It wraps a resolver rather
// than being part of one, so the sources stay about what a tab holds.
type Fitted struct {
	inner TitleResolver
	// maxLength bounds the whole: the position is counted against it rather
	// than added on top, so a numbered title is never wider than an unnumbered
	// one.
	maxLength    int
	fit          Fit
	showPosition bool
}

var _ TitleResolver = (*Fitted)(nil)

// NewFitted wraps inner, which must leave its titles unbounded: the fit here is
// the only one, so a sliding title moves once per poll. A width of zero or less
// takes the default.
func NewFitted(inner TitleResolver, opts Options) *Fitted {
	if opts.MaxLength <= 0 {
		opts.MaxLength = DefaultMaxLength
	}

	return &Fitted{
		inner:        inner,
		maxLength:    opts.MaxLength,
		fit:          opts.fitter(),
		showPosition: opts.ShowPosition,
	}
}

// Resolve names the tab, and puts its position in front unless the tab bar is
// too narrow to carry both.
func (f *Fitted) Resolve(tab state.TabState) Decision {
	decision := f.inner.Resolve(tab)

	// The prefix goes in front because fitting keeps the head: a position at
	// the end is the first thing a title too wide for the tab bar would lose.
	prefix := ""
	if f.showPosition {
		prefix = strconv.Itoa(tab.Position) + positionMark
	}

	room := f.maxLength - uniseg.StringWidth(prefix)
	// A title reduced to nothing has lost more than the position is worth.
	if room <= 0 {
		prefix, room = "", f.maxLength
	}

	name := f.fit(tab.ID, decision.Name, room)
	if name == "" {
		return genericFallback()
	}

	decision.Name = prefix + name

	return decision
}
