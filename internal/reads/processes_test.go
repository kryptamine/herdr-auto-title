package reads

import (
	"sync"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func pane(revision uint64) herdr.PaneInfo {
	return herdr.PaneInfo{PaneID: "wE:p1", TabID: "wE:t1", Revision: revision}
}

var nvim = []herdr.PaneProcessInfoProcess{{Name: "nvim"}}

func TestAReadSurvivesAPollThatChangedNothing(t *testing.T) {
	t.Parallel()

	c := newProcessCache()
	c.observe([]herdr.PaneInfo{pane(7)})
	c.record("wE:p1", nvim)

	c.observe([]herdr.PaneInfo{pane(7)})

	got, read := c.lookup("wE:p1")
	if !read || len(got) != 1 || got[0].Name != "nvim" {
		t.Errorf("processes = %v, %v, want nvim remembered", got, read)
	}
}

func TestAMovedRevisionForgetsWhatWasRunning(t *testing.T) {
	t.Parallel()

	c := newProcessCache()
	c.observe([]herdr.PaneInfo{pane(7)})
	c.record("wE:p1", nvim)

	c.observe([]herdr.PaneInfo{pane(8)})

	if _, read := c.lookup("wE:p1"); read {
		t.Error("a pane that moved still answers with what it used to run")
	}
}

func TestARevisionThatWentBackwardsIsANewPane(t *testing.T) {
	t.Parallel()

	// Revisions are monotonic per pane, so a lower one means Herdr handed the
	// id to a pane that is not the one that was read.
	c := newProcessCache()
	c.observe([]herdr.PaneInfo{pane(7)})
	c.record("wE:p1", nvim)

	c.observe([]herdr.PaneInfo{pane(2)})

	if _, read := c.lookup("wE:p1"); read {
		t.Error("a reused pane id kept the processes of the pane before it")
	}
}

func TestAPaneTheSessionDroppedIsForgotten(t *testing.T) {
	t.Parallel()

	c := newProcessCache()
	c.observe([]herdr.PaneInfo{pane(7)})
	c.record("wE:p1", nvim)

	c.observe(nil)
	c.observe([]herdr.PaneInfo{pane(7)})

	if _, read := c.lookup("wE:p1"); read {
		t.Error("a pane that left the session came back with what it ran before")
	}
}

func TestAnOldReadIsMadeAgain(t *testing.T) {
	t.Parallel()

	// A command starting just after a read moves no revision until the pane
	// draws, so a remembered read is not trusted forever.
	c := newProcessCache()
	c.observe([]herdr.PaneInfo{pane(7)})
	c.record("wE:p1", nvim)
	read := c.now()

	c.now = func() time.Time { return read.Add(processRefresh - time.Millisecond) }
	if _, ok := c.lookup("wE:p1"); !ok {
		t.Error("a read was discarded before it went stale")
	}

	c.now = func() time.Time { return read.Add(processRefresh) }
	if _, ok := c.lookup("wE:p1"); ok {
		t.Error("a stale read was still answered with")
	}
}

func TestAPaneTheSessionDroppedCannotBeRecorded(t *testing.T) {
	t.Parallel()

	c := newProcessCache()
	c.record("wE:p1", nvim)

	if _, read := c.lookup("wE:p1"); read {
		t.Error("a pane no poll has seen was recorded anyway")
	}
}

func TestTheProcessCacheIsSafeUnderConcurrentUse(t *testing.T) {
	t.Parallel()

	c := newProcessCache()

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for n := range 200 {
				c.observe([]herdr.PaneInfo{pane(uint64(n))})
				c.record("wE:p1", nvim)
				c.lookup("wE:p1")
			}
		}()
	}

	wg.Wait()
}
