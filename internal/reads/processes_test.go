package reads

import (
	"sync"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

var p1 = []herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1"}}

var nvim = []herdr.PaneProcessInfoProcess{{Name: "nvim"}}

func TestAReadSurvivesAPollThatChangedNothing(t *testing.T) {
	t.Parallel()

	c := newProcessCache()
	c.observe(p1, map[string]bool{"wE:p1": true})
	c.record("wE:p1", nvim)

	c.observe(p1, nil)

	got, read := c.lookup("wE:p1")
	if !read || len(got) != 1 || got[0].Name != "nvim" {
		t.Errorf("processes = %v, %v, want nvim remembered", got, read)
	}
}

func TestAPaneThatDrewForgetsWhatWasRunning(t *testing.T) {
	t.Parallel()

	c := newProcessCache()
	c.observe(p1, nil)
	c.record("wE:p1", nvim)

	c.observe(p1, map[string]bool{"wE:p1": true})

	if _, read := c.lookup("wE:p1"); read {
		t.Error("a pane that drew still answers with what it used to run")
	}
}

func TestAPaneTheSessionDroppedIsForgotten(t *testing.T) {
	t.Parallel()

	c := newProcessCache()
	c.observe(p1, nil)
	c.record("wE:p1", nvim)

	c.observe(nil, nil)
	c.observe(p1, nil)

	if _, read := c.lookup("wE:p1"); read {
		t.Error("a pane that left the session came back with what it ran before")
	}
}

func TestAnOldReadIsMadeAgain(t *testing.T) {
	t.Parallel()

	// A command starting just after a read moves no revision until the pane
	// draws, so a remembered read is not trusted forever.
	c := newProcessCache()
	c.observe(p1, nil)
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
				c.observe(p1, map[string]bool{"wE:p1": n%2 == 0})
				c.record("wE:p1", nvim)
				c.lookup("wE:p1")
			}
		}()
	}

	wg.Wait()
}
