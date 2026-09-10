package app

import (
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
)

// wide and wider are directories whose names alone are too wide for the
// default title, and vary enough that every window into them differs.
var (
	wide  = wideName("abcdefghij")
	wider = wideName("0123456789")
)

func wideName(piece string) string {
	return strings.Repeat(piece, 2*resolver.DefaultMaxLength/len(piece))
}

// head is what a title of dir shows before it starts to slide.
func head(dir string) string {
	return dir[:resolver.DefaultMaxLength]
}

func startScrolling(t *testing.T, tabs []herdr.TabInfo, panes []herdr.PaneInfo) *harness {
	t.Helper()
	return startStepping(t, tabs, panes, resolver.DefaultScrollStep)
}

func startStepping(
	t *testing.T,
	tabs []herdr.TabInfo,
	panes []herdr.PaneInfo,
	step int,
) *harness {
	t.Helper()

	cfg := testConfig()
	cfg.Scroll = true
	cfg.ScrollStep = step

	return startConfigured(t, herdrtest.New(tabs, panes), cfg)
}

// labels are the names the renames set, in the order they were issued.
func labels(renames []herdrtest.RenameCall) []string {
	names := make([]string, len(renames))
	for i, rename := range renames {
		names[i] = rename.Label
	}

	return names
}

// assertSlid checks the renames a sliding title issued: it opens on the head
// and every later one is the previous moved on by step columns. The fixtures
// are ASCII, so a byte is a column.
func assertSlid(t *testing.T, names []string, polls, step int) {
	t.Helper()

	// The head holds still for a moment first, so not every poll renames.
	if len(names) < 2 {
		t.Fatalf("issued %q over %d polls, want the title to move", names, polls)
	}

	if names[0] != head(wide) {
		t.Errorf("first rename = %q, want the head %q", names[0], head(wide))
	}

	assertMovedBy(t, names, step)
}

func assertMovedBy(t *testing.T, names []string, step int) {
	t.Helper()

	for i := 1; i < len(names); i++ {
		if names[i-1][step:] != names[i][:len(names[i])-step] {
			t.Errorf("rename %d = %q does not follow %q by %d columns",
				i, names[i], names[i-1], step)
		}
	}
}

func TestAPositionStaysPutWhileItsTitleSlides(t *testing.T) {
	// The position is the key that switches to the tab, so it is the one part
	// a title sliding past it must not carry off.
	cfg := testConfig()
	cfg.Scroll = true
	cfg.ShowPosition = true

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: herdrtest.Dir("work", wide), Focused: true},
		},
	), cfg)
	h.polls(12)

	const prefix = "1 · "

	names := labels(h.client.Renames())
	if len(names) < 2 {
		t.Fatalf("issued %q, want the title to move", names)
	}

	bodies := make([]string, len(names))

	for i, name := range names {
		if !strings.HasPrefix(name, prefix) {
			t.Fatalf("rename %d = %q, want %q still in front", i, name, prefix)
		}

		bodies[i] = strings.TrimPrefix(name, prefix)
	}

	assertMovedBy(t, bodies, resolver.DefaultScrollStep)
}

func TestATitleTooWideForTheBarSlidesAcrossIt(t *testing.T) {
	h := startScrolling(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: herdrtest.Dir("work", wide), Focused: true},
		},
	)

	const polls = 12

	h.polls(polls)

	assertSlid(t, labels(h.client.Renames()), polls, resolver.DefaultScrollStep)
}

func TestTheScrollStepIsHowFarAPollMovesTheTitle(t *testing.T) {
	// The speed knob the poll interval is not: a faster title still costs one
	// rename per poll.
	const (
		polls = 12
		step  = 3
	)

	h := startStepping(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: herdrtest.Dir("work", wide), Focused: true},
		},
		step,
	)
	h.polls(polls)

	names := labels(h.client.Renames())
	if len(names) > polls {
		t.Errorf("issued %d renames over %d polls, want no more than one each", len(names), polls)
	}

	assertSlid(t, names, polls, step)
}

func TestATitleThatFitsDoesNotSlide(t *testing.T) {
	h := startScrolling(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.polls(10)

	if renames := h.client.Renames(); len(renames) != 1 {
		t.Errorf("issued %v, want exactly one rename", renames)
	}
}

func TestASlidingTitleIsNotMistakenForTheUsers(t *testing.T) {
	// Every poll moves the label, which is exactly what a rename by the user
	// looks like; the plugin's own work must never lock the tab.
	h := startScrolling(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: herdrtest.Dir("work", wide), Focused: true},
		},
	)
	h.polls(10)

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: api,
	})
	h.poll()

	names := labels(h.client.Renames())
	if got := names[len(names)-1]; got != "api" {
		t.Errorf("last rename = %q, want api: the tab was locked", got)
	}
}

func TestASlidingTitleStartsOverWhenTheNameChanges(t *testing.T) {
	h := startScrolling(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: herdrtest.Dir("work", wide), Focused: true},
		},
	)
	h.polls(10)

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: herdrtest.Dir("work", wider),
	})
	h.poll()

	names := labels(h.client.Renames())
	if got, want := names[len(names)-1], head(wider); got != want {
		t.Errorf("first rename after the change = %q, want the head %q", got, want)
	}
}

func TestAClosedTabsSlideIsForgotten(t *testing.T) {
	// Herdr hands tab ids out again, so a new tab wearing an old id must start
	// at the head of its title rather than wherever the old one had got to.
	h := startScrolling(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: herdrtest.Dir("work", wide), Focused: true},
		},
	)
	h.polls(10)

	h.client.CloseTab("wE:t1")
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "1"})
	h.client.SetPane(
		herdr.PaneInfo{
			PaneID:  "wE:p2",
			TabID:   "wE:t1",
			CWD:     herdrtest.Dir("work", wide),
			Focused: true,
		},
	)
	h.poll()

	names := labels(h.client.Renames())
	if got, want := names[len(names)-1], head(wide); got != want {
		t.Errorf("first rename of the new tab = %q, want the head %q", got, want)
	}
}
