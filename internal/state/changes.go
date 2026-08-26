package state

import (
	"sync"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// processRefresh is how long a process read is reused for. A pane's revision
// cannot carry this alone — measured, it moved for only four of nine process
// changes — so see docs/architecture/poll-loop.md before raising it.
const processRefresh = 2 * time.Second

// Changes remembers what a snapshot cannot say: when each pane last changed,
// and what it was running when it was last asked. Pane revisions are monotonic,
// so comparing one poll's with the last says which panes drew.
type Changes struct {
	mu    sync.Mutex
	panes map[string]paneChange
	now   func() time.Time
}

type paneChange struct {
	revision uint64
	at       time.Time

	// processes is what pane.process_info answered at this revision, and
	// readAt when it did — an empty list is a real answer, a zero time is not.
	processes []herdr.PaneProcessInfoProcess
	readAt    time.Time
}

// NewChanges returns an empty history.
func NewChanges() *Changes {
	return &Changes{
		panes: make(map[string]paneChange),
		now:   time.Now,
	}
}

// Observe records a poll: panes whose revision moved changed just now and are
// no longer described by what was read of them, and panes the session no longer
// holds are forgotten.
func (c *Changes) Observe(panes []herdr.PaneInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()

	seen := make(map[string]paneChange, len(panes))
	for _, pane := range panes {
		previous, known := c.panes[pane.PaneID]
		// Any difference, not just an advance: a revision that went backwards
		// is a new pane wearing an id Herdr has handed out again.
		switch {
		case !known, pane.Revision != previous.revision:
			seen[pane.PaneID] = paneChange{revision: pane.Revision, at: now}
		default:
			seen[pane.PaneID] = previous
		}
	}

	c.panes = seen
}

// ChangedAt reports when a pane was last seen to change, or the zero time for
// a pane no poll has covered yet.
func (c *Changes) ChangedAt(paneID string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.panes[paneID].at
}

// Processes returns what the pane was running when it was last read, or false
// when it has not been read since its revision moved, or when that read is old
// enough to be worth making again.
func (c *Changes) Processes(paneID string) ([]herdr.PaneProcessInfoProcess, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	change := c.panes[paneID]
	if change.readAt.IsZero() || c.now().Sub(change.readAt) >= processRefresh {
		return nil, false
	}

	return change.processes, true
}

// Ran records what a pane was running at the revision the last Observe saw. A
// pane that has since gone is not resurrected.
func (c *Changes) Ran(paneID string, processes []herdr.PaneProcessInfoProcess) {
	c.mu.Lock()
	defer c.mu.Unlock()

	change, known := c.panes[paneID]
	if !known {
		return
	}

	change.processes, change.readAt = processes, c.now()
	c.panes[paneID] = change
}
