package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func newManual(t *testing.T) *Manual {
	t.Helper()
	m := LoadManual(filepath.Join(t.TempDir(), "manual-names.json"))
	// Most tests are about a session already under way.
	m.Settled()

	return m
}

// sighting is the common case: the first tab in a workspace, which the
// resolver would name `dashboard`.
func sighting(current string) Sighting {
	return Sighting{ID: "wE:t1", Current: current, Desired: "dashboard", Default: "1"}
}

func TestSightingFromDerivesTheDefaultLabel(t *testing.T) {
	// A tab nobody has named wears its position, so the third tab of a
	// workspace is `3` however high the ids around it have climbed.
	tab := TabFrom(herdr.TabInfo{TabID: "wE:t9", Label: "Important work"}, "work", 3, nil, false)

	got := SightingFrom(tab, "dashboard")
	want := Sighting{
		ID:      "wE:t9",
		Current: "Important work",
		Desired: "dashboard",
		Default: "3",
	}

	if got != want {
		t.Errorf("sighting = %+v, want %+v", got, want)
	}
}

func TestTheFirstPollNeverLocks(t *testing.T) {
	// The trap this rule exists for: on the first poll almost every tab carries
	// a label that is not yet what the resolver would produce. Locking on that
	// would claim the whole session the moment the plugin starts.
	m := LoadManual("")

	for _, s := range []Sighting{
		{ID: "wE:t1", Current: "1", Desired: "dashboard", Default: "1"},
		{ID: "wE:t2", Current: "Important work", Desired: "api", Default: "2"},
		{ID: "wE:t3", Current: "nvim › stale.go", Desired: "nvim › fresh.go", Default: "3"},
	} {
		if m.Tabs.Observe(s) {
			t.Errorf("tab %s was locked on the first poll", s.ID)
		}
	}
}

func TestATabTurningUpAlreadyNamedIsTheUsers(t *testing.T) {
	// The case that made this rule necessary: a tab created and named faster
	// than the next poll. Auto Title never saw it carrying its position, so
	// the name it carries is not Auto Title's.
	m := newManual(t)

	if !m.Tabs.Observe(
		Sighting{ID: "wE:t9", Current: "My thing", Desired: "dashboard", Default: "9"},
	) {
		t.Fatal("a tab that appeared already named was not read as the user's")
	}

	if !m.Tabs.Locked("wE:t9") {
		t.Error("the tab is not locked")
	}
}

func TestATabTurningUpUnnamedIsNotTheUsers(t *testing.T) {
	// Herdr names a new tab after its position. Nobody has claimed this one.
	m := newManual(t)

	if m.Tabs.Observe(Sighting{ID: "wE:t9", Current: "9", Desired: "dashboard", Default: "9"}) {
		t.Error("an unnamed new tab was locked")
	}
}

func TestATabFallingBackToItsDefaultLabelIsNotTheUsers(t *testing.T) {
	// The default label is not only how a tab starts out: it comes back, and it
	// slides down for every tab that closes to the left. Locking there would
	// freeze the tab at a number for the rest of the session.
	m := newManual(t)
	m.Tabs.Observe(sighting("1"))
	m.Tabs.Applied("wE:t1", "dashboard")

	if m.Tabs.Observe(sighting("1")) {
		t.Fatal("a tab back on its default label was read as the user's")
	}

	if m.Tabs.Locked("wE:t1") {
		t.Error("the tab is locked")
	}
}

func TestATabWhoseNameWasClearedIsNotTheUsers(t *testing.T) {
	// Clearing a tab's name empties its label rather than putting the position
	// back, so an empty label is Herdr's other way of saying nobody named it —
	// see docs/architecture/herdr-socket-api.md.
	m := newManual(t)
	m.Tabs.Observe(sighting("1"))
	m.Tabs.Applied("wE:t1", "dashboard")

	if m.Tabs.Observe(sighting("")) {
		t.Fatal("a tab whose name was cleared was read as the user's")
	}

	if m.Tabs.Locked("wE:t1") {
		t.Error("the tab is locked")
	}
}

func TestARenameByTheUserLocksTheTab(t *testing.T) {
	m := newManual(t)
	m.Tabs.Observe(sighting("1"))
	m.Tabs.Applied("wE:t1", "dashboard")

	if !m.Tabs.Observe(sighting("Important work")) {
		t.Fatal("a label the plugin neither set nor wanted was not read as the user's")
	}

	if !m.Tabs.Locked("wE:t1") {
		t.Error("the tab is not locked")
	}
}

func TestARenameByThePluginDoesNotLock(t *testing.T) {
	m := newManual(t)
	m.Tabs.Observe(sighting("1"))
	m.Tabs.Applied("wE:t1", "dashboard")

	if m.Tabs.Observe(sighting("dashboard")) {
		t.Error("the plugin's own rename was read as the user's")
	}
}

func TestALabelThatHasNotMovedIsNobodysDoing(t *testing.T) {
	m := newManual(t)
	m.Tabs.Observe(sighting("Important work"))

	// Same label on the next poll: nothing happened, whatever it says.
	if m.Tabs.Observe(sighting("Important work")) {
		t.Error("an unchanged label was read as a rename")
	}
}

func TestALabelMatchingWhatWeWouldSetDoesNotLock(t *testing.T) {
	// Indistinguishable from the plugin's own work, and harmless either way.
	m := newManual(t)
	m.Tabs.Observe(sighting("1"))

	if m.Tabs.Observe(sighting("dashboard")) {
		t.Error("a label matching the resolved one locked the tab")
	}
}

func TestLocksSurviveAReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manual-names.json")

	m := LoadManual(path)
	m.Tabs.Observe(sighting("1"))

	if !m.Tabs.Observe(sighting("Important work")) {
		t.Fatal("the tab was not locked")
	}

	if !LoadManual(path).Tabs.Locked("wE:t1") {
		t.Error("the lock did not survive a restart")
	}
}

func TestAReloadedLockIsReleasedWhenTheLabelMovedOn(t *testing.T) {
	// Herdr's tab ids belong to a session, so a stored wE:t1 may be an
	// unrelated tab by the time it is read back. Only the label makes it the
	// same tab.
	path := filepath.Join(t.TempDir(), "manual-names.json")

	m := LoadManual(path)
	m.Tabs.Observe(sighting("1"))
	m.Tabs.Observe(sighting("Important work"))

	reloaded := LoadManual(path)
	reloaded.Tabs.Retain(map[string]string{"wE:t1": "2"})

	if reloaded.Tabs.Locked("wE:t1") {
		t.Error("a lock was kept for a tab that no longer carries its name")
	}

	if LoadManual(path).Tabs.Locked("wE:t1") {
		t.Error("the released lock was not written out")
	}
}

func TestRetainDropsTabsTheSessionNoLongerHolds(t *testing.T) {
	m := newManual(t)
	m.Tabs.Observe(sighting("1"))
	m.Tabs.Observe(sighting("Important work"))

	m.Tabs.Retain(map[string]string{})

	if m.Tabs.Locked("wE:t1") {
		t.Error("a closed tab is still locked")
	}

	// Its baseline went too, so a tab reusing the id starts clean and is
	// judged on what it carries rather than on what the old tab did.
	if m.Tabs.Observe(Sighting{ID: "wE:t1", Current: "1", Desired: "dashboard", Default: "1"}) {
		t.Error("an unnamed tab reusing the id was locked")
	}
}

func TestAnUnreadableStoreIsNotFatal(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "manual-names.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := LoadManual(path)
	if m.Tabs.Locked("wE:t1") {
		t.Error("a corrupt store produced a lock")
	}
	// And it still works from there.
	m.Tabs.Observe(sighting("1"))

	if !m.Tabs.Observe(sighting("Important work")) {
		t.Error("locking stopped working after a corrupt store")
	}
}

func TestWithoutAPathLocksStayInMemory(t *testing.T) {
	m := LoadManual("")
	m.Tabs.Observe(sighting("1"))

	if !m.Tabs.Observe(sighting("Important work")) {
		t.Error("locking needs a file")
	}

	if !m.Tabs.Locked("wE:t1") {
		t.Error("the lock was not kept")
	}
}

// paneSighting is the pane counterpart of sighting: a pane the resolver would
// name `dashboard`, and which carries no label until somebody sets one.
func paneSighting(current string) Sighting {
	return Sighting{ID: "wE:p1", Current: current, Desired: "dashboard"}
}

func TestPaneSightingFromHasNoDefaultLabel(t *testing.T) {
	// A pane has one spelling for unnamed and no second one. pane.rename clears
	// an empty label rather than storing it, and Herdr omits the field entirely
	// until a pane is named, so there is no position to compare against.
	got := PaneSightingFrom(&PaneState{ID: "wE:p1", CurrentName: "Important work"}, "dashboard")
	want := Sighting{ID: "wE:p1", Current: "Important work", Desired: "dashboard"}

	if got != want {
		t.Errorf("sighting = %+v, want %+v", got, want)
	}
}

func TestAPaneTheUserRenamedIsLocked(t *testing.T) {
	m := LoadManual("")
	m.Settled()

	if !m.Panes.Observe(paneSighting("Important work")) {
		t.Fatal("a pane carrying a name nobody set was not claimed")
	}

	if !m.Panes.Locked("wE:p1") {
		t.Error("the pane was not locked")
	}
	// The tab of the same poll is a claim of its own: locking one must not
	// lock the other, and the two id spaces are Herdr's, not this package's.
	if m.Tabs.Locked("wE:p1") {
		t.Error("claiming a pane claimed a tab of the same id")
	}
}

func TestAPaneAutoTitleNamedIsNotTheUsers(t *testing.T) {
	m := LoadManual("")
	m.Settled()
	m.Panes.Applied("wE:p1", "dashboard")

	if m.Panes.Observe(paneSighting("dashboard")) {
		t.Error("the plugin's own pane rename read as the user's")
	}
}

func TestClearingAPaneLabelHandsItBack(t *testing.T) {
	m := LoadManual("")
	m.Settled()
	m.Panes.Observe(paneSighting("Important work"))

	// Herdr reports a cleared pane as carrying no label at all.
	m.Panes.Retain(map[string]string{"wE:p1": ""})

	if m.Panes.Locked("wE:p1") {
		t.Error("clearing the label did not hand the pane back")
	}
}

func TestPaneLocksSurviveAReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manual-names.json")

	m := LoadManual(path)
	m.Settled()
	m.Panes.Observe(paneSighting("Important work"))

	if !LoadManual(path).Panes.Locked("wE:p1") {
		t.Error("the pane lock was not persisted")
	}
	// Tab locks are written under a key of their own, so a store holding one
	// kind still reads back the other.
	m.Tabs.Observe(sighting("1"))
	m.Tabs.Observe(sighting("My tab"))

	reloaded := LoadManual(path)
	if !reloaded.Tabs.Locked("wE:t1") || !reloaded.Panes.Locked("wE:p1") {
		t.Error("the two kinds of lock did not survive the same store")
	}
}
