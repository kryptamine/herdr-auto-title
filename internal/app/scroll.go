package app

import (
	"maps"
	"sync"

	"github.com/kryptamine/herdr-auto-title/internal/resolver"
)

// scroller remembers, per tab, how far the name it was last given has slid.
// The tick moves once per poll, so the poll interval is the sliding speed.
type scroller struct {
	mu sync.Mutex
	// step is how many columns a title moves per poll.
	step int
	tabs map[string]sliding
}

type sliding struct {
	name string
	tick int
	// moved says the last fit showed this name further along than its head.
	moved bool
}

func newScroller(step int) *scroller {
	return &scroller{step: step, tabs: make(map[string]sliding)}
}

// Fit slides a name too wide for width, starting over whenever the name
// itself changes.
func (s *scroller) Fit(tabID, name string, width int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.tabs[tabID]
	if current.name == name {
		current.tick++
	} else {
		current = sliding{name: name}
	}

	shown := resolver.Slide(name, width, current.tick, s.step)
	current.moved = current.tick > 0 && shown != name
	s.tabs[tabID] = current

	return shown
}

// sliding reports whether the tab's title is the one it was last given, moved
// along, rather than a new one.
func (s *scroller) sliding(tabID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.tabs[tabID].moved
}

// Retain forgets the tabs the session no longer holds.
func (s *scroller) Retain(live map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	maps.DeleteFunc(s.tabs, func(tabID string, _ sliding) bool {
		_, alive := live[tabID]
		return !alive
	})
}
