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

func TestPanesTheSessionDroppedAreForgotten(t *testing.T) {
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

func TestAReadSurvivesAPollThatChangedNothing(t *testing.T) {
	c := NewChanges()
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})
	c.Ran("wE:p1", []herdr.PaneProcessInfoProcess{{Name: "nvim"}})

	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})

	got, read := c.Processes("wE:p1")
	if !read || len(got) != 1 || got[0].Name != "nvim" {
		t.Errorf("processes = %v, %v, want nvim remembered", got, read)
	}
}

func TestAMovedRevisionForgetsWhatWasRunning(t *testing.T) {
	c := NewChanges()
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})
	c.Ran("wE:p1", []herdr.PaneProcessInfoProcess{{Name: "nvim"}})

	c.Observe([]herdr.PaneInfo{pane("wE:p1", 8)})

	if _, read := c.Processes("wE:p1"); read {
		t.Error("a pane that moved still answers with what it used to run")
	}
}

func TestARevisionThatWentBackwardsIsANewPane(t *testing.T) {
	// Revisions are monotonic per pane, so a lower one means Herdr handed the
	// id to a pane that is not the one that was read.
	c := NewChanges()
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})
	c.Ran("wE:p1", []herdr.PaneProcessInfoProcess{{Name: "nvim"}})

	c.Observe([]herdr.PaneInfo{pane("wE:p1", 2)})

	if _, read := c.Processes("wE:p1"); read {
		t.Error("a reused pane id kept the processes of the pane before it")
	}
}

func TestAnOldReadIsMadeAgain(t *testing.T) {
	// A command starting just after a read moves no revision until the pane
	// draws, so a remembered read is not trusted forever.
	c := NewChanges()
	c.Observe([]herdr.PaneInfo{pane("wE:p1", 7)})
	c.Ran("wE:p1", []herdr.PaneProcessInfoProcess{{Name: "nvim"}})
	read := c.now()

	c.now = func() time.Time { return read.Add(processRefresh - time.Millisecond) }
	if _, ok := c.Processes("wE:p1"); !ok {
		t.Error("a read was discarded before it went stale")
	}

	c.now = func() time.Time { return read.Add(processRefresh) }
	if _, ok := c.Processes("wE:p1"); ok {
		t.Error("a stale read was still answered with")
	}
}

func TestAPaneTheSessionDroppedCannotBeRecorded(t *testing.T) {
	c := NewChanges()
	c.Ran("wE:p1", []herdr.PaneProcessInfoProcess{{Name: "nvim"}})

	if _, read := c.Processes("wE:p1"); read {
		t.Error("a pane no poll has seen was recorded anyway")
	}
}

func TestChangesAreSafeUnderConcurrentUse(t *testing.T) {
	c := NewChanges()

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for n := range 200 {
				c.Observe([]herdr.PaneInfo{pane("wE:p1", uint64(n))})
				c.ChangedAt("wE:p1")
				c.Ran("wE:p1", []herdr.PaneProcessInfoProcess{{Name: "nvim"}})
				c.Processes("wE:p1")
			}
		}()
	}

	wg.Wait()
}
