package resolver

import (
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// statusMarks maps a tab's agent status to the mark in front of its title. A
// tab without an agent reports unknown, which has none.
var statusMarks = map[string]string{
	herdr.AgentStatusIdle:    "◉",
	herdr.AgentStatusWorking: "◐",
	herdr.AgentStatusBlocked: "●",
	herdr.AgentStatusDone:    "✓",
}

// StatusTitles puts a mark for what a tab's agents are doing in front of its
// title. It wraps a resolver for the reason Numbered does: the status is not
// one of the things a title says, so no source supplies it.
type StatusTitles struct {
	inner TitleResolver
	// maxLength is the bound inner was built with; the mark is counted against
	// it, as Numbered counts the position.
	maxLength int
}

var _ TitleResolver = (*StatusTitles)(nil)

// NewStatusTitles wraps inner, which must have been given the same maxLength.
// Zero or less takes the default, as New does.
func NewStatusTitles(inner TitleResolver, maxLength int) *StatusTitles {
	if maxLength <= 0 {
		maxLength = DefaultMaxLength
	}

	return &StatusTitles{inner: inner, maxLength: maxLength}
}

func (s *StatusTitles) Resolve(tab state.TabState) Decision {
	decision := s.inner.Resolve(tab)

	if mark, ok := statusMarks[tab.AgentStatus]; ok {
		decision.Name = withPrefix(mark+" ", decision.Name, s.maxLength)
	}

	return decision
}
