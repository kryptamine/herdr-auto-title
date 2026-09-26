package reads

import (
	"sync"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// processRefresh is how long a process read is reused for. A pane's revision
// cannot carry this alone — measured, it moved for only four of nine process
// changes — so see docs/architecture/poll-loop.md before raising it.
const processRefresh = 2 * time.Second

// processCache remembers what each pane was running when it was last asked.
// Pane revisions are monotonic, so one that differs from the last seen says
// the pane drew since, and what was read of it no longer holds.
type processCache struct {
	mu    sync.Mutex
	panes map[string]processRead
	now   func() time.Time
}

// processRead is what pane.process_info answered at a revision, and readAt
// when it did — an empty list is a real answer, a zero time is not.
type processRead struct {
	revision  uint64
	processes []herdr.PaneProcessInfoProcess
	readAt    time.Time
}

func newProcessCache() *processCache {
	return &processCache{
		panes: make(map[string]processRead),
		now:   time.Now,
	}
}

// observe forgets what was read of a pane whose revision moved, and of panes
// the session no longer holds.
func (c *processCache) observe(panes []herdr.PaneInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[string]processRead, len(panes))
	for _, pane := range panes {
		previous, known := c.panes[pane.PaneID]
		// Any difference, not just an advance: a revision that went backwards
		// is a new pane wearing an id Herdr has handed out again.
		switch {
		case !known, pane.Revision != previous.revision:
			seen[pane.PaneID] = processRead{revision: pane.Revision}
		default:
			seen[pane.PaneID] = previous
		}
	}

	c.panes = seen
}

// lookup returns what the pane was running when it was last read, or false
// when it has not been read since its revision moved, or when that read is old
// enough to be worth making again.
func (c *processCache) lookup(paneID string) ([]herdr.PaneProcessInfoProcess, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	read := c.panes[paneID]
	if read.readAt.IsZero() || c.now().Sub(read.readAt) >= processRefresh {
		return nil, false
	}

	return read.processes, true
}

// record keeps what a pane was running at the revision the last observe saw.
// A pane that has since gone is not resurrected.
func (c *processCache) record(paneID string, processes []herdr.PaneProcessInfoProcess) {
	c.mu.Lock()
	defer c.mu.Unlock()

	read, known := c.panes[paneID]
	if !known {
		return
	}

	read.processes, read.readAt = processes, c.now()
	c.panes[paneID] = read
}
