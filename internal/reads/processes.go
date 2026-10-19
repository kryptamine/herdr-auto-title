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

// processCache remembers what each pane was running when it was last asked,
// until the pane leaves the session.
type processCache struct {
	mu    sync.Mutex
	panes map[string]processRead
	now   func() time.Time
}

// processRead is what pane.process_info answered, and readAt when it did —
// an empty list is a real answer, a zero time is not. A pane that drew since
// is stale: its read is worth making again, and kept for when that fails.
type processRead struct {
	processes []herdr.PaneProcessInfoProcess
	readAt    time.Time
	stale     bool
}

func newProcessCache() *processCache {
	return &processCache{
		panes: make(map[string]processRead),
		now:   time.Now,
	}
}

// observe marks what was read of the panes that drew as stale, and forgets
// panes the session no longer holds.
func (c *processCache) observe(panes []herdr.PaneInfo, drew map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[string]processRead, len(panes))
	for _, pane := range panes {
		read := c.panes[pane.PaneID]
		if drew[pane.PaneID] {
			read.stale = true
		}

		seen[pane.PaneID] = read
	}

	c.panes = seen
}

// lookup returns what the pane was running when it was last read, or false
// when it has not been read since it drew, or when that read is old enough to
// be worth making again.
func (c *processCache) lookup(paneID string) ([]herdr.PaneProcessInfoProcess, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	read := c.panes[paneID]
	if read.stale || read.readAt.IsZero() || c.now().Sub(read.readAt) >= processRefresh {
		return nil, false
	}

	return read.processes, true
}

// last returns what the pane was running when it was last read, however long
// ago, or false when it never has been.
func (c *processCache) last(paneID string) ([]herdr.PaneProcessInfoProcess, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	read := c.panes[paneID]

	return read.processes, !read.readAt.IsZero()
}

// record keeps what a pane was running. A pane the last observe did not see
// is not resurrected.
func (c *processCache) record(paneID string, processes []herdr.PaneProcessInfoProcess) {
	c.mu.Lock()
	defer c.mu.Unlock()

	read, known := c.panes[paneID]
	if !known {
		return
	}

	read.processes, read.readAt, read.stale = processes, c.now(), false
	c.panes[paneID] = read
}
