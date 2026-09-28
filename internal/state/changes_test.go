package state

import (
	"sync"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func pane(paneID string, revision uint64) herdr.PaneInfo {
	return herdr.PaneInfo{PaneID: paneID, TabID: "wE:t1", Revision: revision}
}

func TestAFirstSightingCountsAsAChange(t *testing.T) {
	t.Parallel()

	c := NewChanges()
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})

	if c.ChangedAt("wE:p1").IsZero() {
		t.Error("a pane seen for the first time has no change time")
	}

	if !c.ChangedAt("wE:p9").IsZero() {
		t.Error("a pane never seen reports a change time")
	}
}

func TestOnlyAnAdvancedRevisionIsAChange(t *testing.T) {
	t.Parallel()

	c := NewChanges()
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})
	first := c.ChangedAt("wE:p1")

	// Polls where nothing moved must not look like changes, or every pane
	// would always be the most recently changed one.
	c.now = func() time.Time { return first.Add(time.Hour) }
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})

	if got := c.ChangedAt("wE:p1"); !got.Equal(first) {
		t.Errorf("an unchanged pane moved to %v, want %v", got, first)
	}

	c.Observe([]herdr.PaneInfo{pane("wE:p1", 8)})

	if got := c.ChangedAt("wE:p1"); !got.After(first) {
		t.Error("an advanced revision was not recorded as a change")
	}
}

func TestARevisionThatWentBackwardsIsAChange(t *testing.T) {
	t.Parallel()

	// Revisions are monotonic per pane, so a lower one means Herdr handed the
	// id to a pane that is not the one seen before.
	c := NewChanges()
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})

	if drew := c.Observe([]herdr.PaneInfo{pane("wE:p1", 2)}); !drew["wE:p1"] {
		t.Error("a reused pane id was taken for the pane before it")
	}
}

func TestObserveReturnsThePanesThatChanged(t *testing.T) {
	t.Parallel()

	// What was read of a pane is thrown away on exactly this answer, so it must
	// agree with ChangedAt on which panes drew.
	c := NewChanges()

	drew := c.Observe([]herdr.PaneInfo{pane("wE:p1", 1), pane("wE:p2", 1)})
	if !drew["wE:p1"] || !drew["wE:p2"] {
		t.Errorf("drew = %v, want both panes on their first sighting", drew)
	}

	drew = c.Observe([]herdr.PaneInfo{pane("wE:p1", 1), pane("wE:p2", 2)})
	if drew["wE:p1"] || !drew["wE:p2"] {
		t.Errorf("drew = %v, want only the pane whose revision moved", drew)
	}
}

func TestPanesTheSessionDroppedAreForgotten(t *testing.T) {
	t.Parallel()

	c := NewChanges()
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 1), pane("wE:p2", 1)})
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 1)})

	if !c.ChangedAt("wE:p2").IsZero() {
		t.Error("a pane the session no longer holds is still remembered")
	}

	if c.ChangedAt("wE:p1").IsZero() {
		t.Error("a surviving pane lost its history")
	}
}

func TestChangesAreSafeUnderConcurrentUse(t *testing.T) {
	t.Parallel()

	c := NewChanges()

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for n := range 200 {
				c.Observe([]herdr.PaneInfo{pane("wE:p1", uint64(n))})
				c.ChangedAt("wE:p1")
			}
		}()
	}

	wg.Wait()
}
