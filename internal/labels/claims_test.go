package labels

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func load(path string) *Labels {
	return Load(path, slog.New(slog.DiscardHandler))
}

func settledLabels(t *testing.T) *Labels {
	t.Helper()
	l := load(filepath.Join(t.TempDir(), "manual-names.json"))
	// Most tests are about a session already under way.
	l.settle()

	return l
}

// tabSighting is the common case: the first tab in a workspace, which the
// resolver would name `dashboard`.
func tabSighting(current string) sighting {
	return sighting{id: "wE:t1", current: current, desired: "dashboard", byDefault: "1"}
}

func TestATabSightingCarriesItsPositionAsItsDefault(t *testing.T) {
	t.Parallel()

	// A tab nobody has named wears its position, so the third tab of a
	// workspace is `3` however high the ids around it have climbed.
	tab := state.TabFrom(
		herdr.TabInfo{TabID: "wE:t9", Label: "Important work"},
		"work",
		3,
		nil,
		false,
	)

	got := sightingOfTab(tab, "dashboard")
	want := sighting{
		id:        "wE:t9",
		current:   "Important work",
		desired:   "dashboard",
		byDefault: "3",
	}

	if got != want {
		t.Errorf("sighting = %+v, want %+v", got, want)
	}
}

func TestTheFirstPollNeverLocks(t *testing.T) {
	t.Parallel()

	// The trap this rule exists for: on the first poll almost every tab carries
	// a label that is not yet what the resolver would produce. Locking on that
	// would claim the whole session the moment the plugin starts.
	l := load("")

	for _, s := range []sighting{
		{id: "wE:t1", current: "1", desired: "dashboard", byDefault: "1"},
		{id: "wE:t2", current: "Important work", desired: "api", byDefault: "2"},
		{id: "wE:t3", current: "nvim › stale.go", desired: "nvim › fresh.go", byDefault: "3"},
	} {
		if l.tabs.observe(s) == verdictClaimed {
			t.Errorf("tab %s was locked on the first poll", s.id)
		}
	}
}

func TestATabTurningUpAlreadyNamedIsTheUsers(t *testing.T) {
	t.Parallel()

	// The case that made this rule necessary: a tab created and named faster
	// than the next poll. Auto Title never saw it carrying its position, so
	// the name it carries is not Auto Title's.
	l := settledLabels(t)

	if l.tabs.observe(
		sighting{id: "wE:t9", current: "My thing", desired: "dashboard", byDefault: "9"},
	) != verdictClaimed {
		t.Fatal("a tab that appeared already named was not read as the user's")
	}

	if !l.tabs.isLocked("wE:t9") {
		t.Error("the tab is not locked")
	}
}

func TestATabTurningUpUnnamedIsNotTheUsers(t *testing.T) {
	t.Parallel()

	// Herdr names a new tab after its position. Nobody has claimed this one.
	l := settledLabels(t)

	if l.tabs.observe(
		sighting{id: "wE:t9", current: "9", desired: "dashboard", byDefault: "9"},
	) == verdictClaimed {
		t.Error("an unnamed new tab was locked")
	}
}

func TestATabFallingBackToItsDefaultLabelIsNotTheUsers(t *testing.T) {
	t.Parallel()

	// The default label is not only how a tab starts out: it comes back, and it
	// slides down for every tab that closes to the left. Locking there would
	// freeze the tab at a number for the rest of the session.
	l := settledLabels(t)
	l.tabs.observe(tabSighting("1"))
	l.tabs.applied("wE:t1", "dashboard")

	if l.tabs.observe(tabSighting("1")) == verdictClaimed {
		t.Fatal("a tab back on its default label was read as the user's")
	}

	if l.tabs.isLocked("wE:t1") {
		t.Error("the tab is locked")
	}
}

func TestATabWhoseNameWasClearedIsNotTheUsers(t *testing.T) {
	t.Parallel()

	// Clearing a tab's name empties its label rather than putting the position
	// back, so an empty label is Herdr's other way of saying nobody named it —
	// see docs/architecture/herdr-socket-api.md.
	l := settledLabels(t)
	l.tabs.observe(tabSighting("1"))
	l.tabs.applied("wE:t1", "dashboard")

	if l.tabs.observe(tabSighting("")) == verdictClaimed {
		t.Fatal("a tab whose name was cleared was read as the user's")
	}

	if l.tabs.isLocked("wE:t1") {
		t.Error("the tab is locked")
	}
}

func TestARenameByTheUserLocksTheTab(t *testing.T) {
	t.Parallel()

	l := settledLabels(t)
	l.tabs.observe(tabSighting("1"))
	l.tabs.applied("wE:t1", "dashboard")

	if l.tabs.observe(tabSighting("Important work")) != verdictClaimed {
		t.Fatal("a label the plugin neither set nor wanted was not read as the user's")
	}

	if !l.tabs.isLocked("wE:t1") {
		t.Error("the tab is not locked")
	}
}

func TestARenameLandingAfterItsCallFailedIsNotTheUsers(t *testing.T) {
	t.Parallel()

	// Herdr can apply a rename seconds after the call gave up on it, by when
	// the name wanted has moved on. Read as the user's, it froze the tab.
	l := settledLabels(t)
	l.tabs.observe(tabSighting("1"))
	l.tabs.sent("wE:t1", "api")

	if l.tabs.observe(tabSighting("api")) == verdictClaimed {
		t.Fatal("a label the plugin sent was read as the user's")
	}

	if l.tabs.isLocked("wE:t1") {
		t.Error("the tab is locked")
	}
}

func TestARenameByThePluginDoesNotLock(t *testing.T) {
	t.Parallel()

	l := settledLabels(t)
	l.tabs.observe(tabSighting("1"))
	l.tabs.applied("wE:t1", "dashboard")

	if l.tabs.observe(tabSighting("dashboard")) == verdictClaimed {
		t.Error("the plugin's own rename was read as the user's")
	}
}

func TestALabelThatHasNotMovedIsNobodysDoing(t *testing.T) {
	t.Parallel()

	l := settledLabels(t)
	l.tabs.observe(tabSighting("Important work"))

	// Same label on the next poll: nothing happened, whatever it says.
	if l.tabs.observe(tabSighting("Important work")) == verdictClaimed {
		t.Error("an unchanged label was read as a rename")
	}
}

func TestALabelMatchingWhatWeWouldSetDoesNotLock(t *testing.T) {
	t.Parallel()

	// Indistinguishable from the plugin's own work, and harmless either way.
	l := settledLabels(t)
	l.tabs.observe(tabSighting("1"))

	if l.tabs.observe(tabSighting("dashboard")) == verdictClaimed {
		t.Error("a label matching the resolved one locked the tab")
	}
}

func TestLocksSurviveAReload(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manual-names.json")

	l := load(path)
	l.tabs.observe(tabSighting("1"))

	if l.tabs.observe(tabSighting("Important work")) != verdictClaimed {
		t.Fatal("the tab was not locked")
	}

	if !load(path).tabs.isLocked("wE:t1") {
		t.Error("the lock did not survive a restart")
	}
}

func TestAReloadedLockIsReleasedWhenTheLabelMovedOn(t *testing.T) {
	t.Parallel()

	// Herdr's tab ids belong to a session, so a stored wE:t1 may be an
	// unrelated tab by the time it is read back. Only the label makes it the
	// same tab.
	path := filepath.Join(t.TempDir(), "manual-names.json")

	l := load(path)
	l.tabs.observe(tabSighting("1"))
	l.tabs.observe(tabSighting("Important work"))

	reloaded := load(path)
	reloaded.tabs.retain(map[string]string{"wE:t1": "2"})

	if reloaded.tabs.isLocked("wE:t1") {
		t.Error("a lock was kept for a tab that no longer carries its name")
	}

	if load(path).tabs.isLocked("wE:t1") {
		t.Error("the released lock was not written out")
	}
}

func TestRetainDropsTabsTheSessionNoLongerHolds(t *testing.T) {
	t.Parallel()

	l := settledLabels(t)
	l.tabs.observe(tabSighting("1"))
	l.tabs.observe(tabSighting("Important work"))

	l.tabs.retain(map[string]string{})

	if l.tabs.isLocked("wE:t1") {
		t.Error("a closed tab is still locked")
	}

	// Its baseline went too, so a tab reusing the id starts clean and is
	// judged on what it carries rather than on what the old tab did.
	if l.tabs.observe(
		sighting{id: "wE:t1", current: "1", desired: "dashboard", byDefault: "1"},
	) == verdictClaimed {
		t.Error("an unnamed tab reusing the id was locked")
	}
}

func TestAnUnreadableStoreIsNotFatal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	path := filepath.Join(dir, "manual-names.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	l := load(path)
	if l.tabs.isLocked("wE:t1") {
		t.Error("a corrupt store produced a lock")
	}
	// And it still works from there.
	l.tabs.observe(tabSighting("1"))

	if l.tabs.observe(tabSighting("Important work")) != verdictClaimed {
		t.Error("locking stopped working after a corrupt store")
	}
}

// A file an earlier version saved still carries its workspace keys. They must
// not cost the tab and pane locks beside them, and go at the next save.
func TestAFileWithWorkspaceLocksStillLoads(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manual-names.json")
	stored := `{"locked_tabs":{"wE:t1":"mine"},"locked_panes":{"wE:p1":"pane"},` +
		`"locked_workspaces":{"wE":"the migration"},"written_workspaces":{"wE":"x"}}`

	if err := os.WriteFile(path, []byte(stored), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	l := load(path)
	if !l.tabs.isLocked("wE:t1") || !l.panes.isLocked("wE:p1") {
		t.Fatal("the tab and pane locks were lost with the workspace keys")
	}

	l.tabs.retain(map[string]string{})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if strings.Contains(string(raw), "workspaces") {
		t.Errorf("the saved file still carries workspace keys: %s", raw)
	}
}

func TestWithoutAPathLocksStayInMemory(t *testing.T) {
	t.Parallel()

	l := load("")
	l.tabs.observe(tabSighting("1"))

	if l.tabs.observe(tabSighting("Important work")) != verdictClaimed {
		t.Error("locking needs a file")
	}

	if !l.tabs.isLocked("wE:t1") {
		t.Error("the lock was not kept")
	}
}

// paneSighting is the pane counterpart of tabSighting: a pane the resolver would
// name `dashboard`, and which carries no label until somebody sets one.
func paneSighting(current string) sighting {
	return sighting{id: "wE:p1", current: current, desired: "dashboard"}
}

func TestAPaneSightingHasNoDefaultLabel(t *testing.T) {
	t.Parallel()

	// A pane has one spelling for unnamed and no second one. pane.rename clears
	// an empty label rather than storing it, and Herdr omits the field entirely
	// until a pane is named, so there is no position to compare against.
	got := sightingOfPane(&state.PaneState{ID: "wE:p1", CurrentName: "Important work"}, "dashboard")
	want := sighting{id: "wE:p1", current: "Important work", desired: "dashboard"}

	if got != want {
		t.Errorf("sighting = %+v, want %+v", got, want)
	}
}

func TestAPaneTheUserRenamedIsLocked(t *testing.T) {
	t.Parallel()

	l := load("")
	l.settle()

	if l.panes.observe(paneSighting("Important work")) != verdictClaimed {
		t.Fatal("a pane carrying a name nobody set was not claimed")
	}

	if !l.panes.isLocked("wE:p1") {
		t.Error("the pane was not locked")
	}
	// The tab of the same poll is a claim of its own: locking one must not
	// lock the other, and the two id spaces are Herdr's, not this package's.
	if l.tabs.isLocked("wE:p1") {
		t.Error("claiming a pane claimed a tab of the same id")
	}
}

func TestAPaneAutoTitleNamedIsNotTheUsers(t *testing.T) {
	t.Parallel()

	l := load("")
	l.settle()
	l.panes.applied("wE:p1", "dashboard")

	if l.panes.observe(paneSighting("dashboard")) == verdictClaimed {
		t.Error("the plugin's own pane rename read as the user's")
	}
}

func TestClearingAPaneLabelHandsItBack(t *testing.T) {
	t.Parallel()

	l := load("")
	l.settle()
	l.panes.observe(paneSighting("Important work"))

	// Herdr reports a cleared pane as carrying no label at all.
	l.panes.retain(map[string]string{"wE:p1": ""})

	if l.panes.isLocked("wE:p1") {
		t.Error("clearing the label did not hand the pane back")
	}
}

func TestPaneLocksSurviveAReload(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manual-names.json")

	l := load(path)
	l.settle()
	l.panes.observe(paneSighting("Important work"))

	if !load(path).panes.isLocked("wE:p1") {
		t.Error("the pane lock was not persisted")
	}
	// Tab locks are written under a key of their own, so a store holding one
	// kind still reads back the other.
	l.tabs.observe(tabSighting("1"))
	l.tabs.observe(tabSighting("My tab"))

	reloaded := load(path)
	if !reloaded.tabs.isLocked("wE:t1") || !reloaded.panes.isLocked("wE:p1") {
		t.Error("the two kinds of lock did not survive the same store")
	}
}

// moved is a pane seen again under the id Herdr gave it in another workspace,
// wearing the label it carried under wE:p1.
func moved() sighting {
	return sighting{id: "wV:p9", current: "[wE:p1] api", desired: "[wV:p9] api"}
}

func TestAMovedPaneOutlivesAPollCutShort(t *testing.T) {
	t.Parallel()

	// A poll that times out before reaching the moved pane never settles, and
	// the label it left must still be there for the poll that does reach it.
	l := settledLabels(t)
	l.panes.observe(sighting{id: "wE:p1", current: "[wE:p1] api", desired: "[wE:p1] api"})

	l.panes.retain(map[string]string{"wV:p9": "[wE:p1] api"})
	l.panes.retain(map[string]string{"wV:p9": "[wE:p1] api"})

	if l.panes.observe(moved()) != verdictName {
		t.Error("a moved pane was claimed for the user after a poll cut short")
	}
}

func TestAMovedPaneIsForgottenOnceEverythingWasSeen(t *testing.T) {
	t.Parallel()

	// A poll that saw everything has seen any pane that moved, so a label worn
	// by a pane appearing later is somebody else's.
	l := settledLabels(t)
	l.panes.observe(sighting{id: "wE:p1", current: "[wE:p1] api", desired: "[wE:p1] api"})

	l.panes.retain(map[string]string{})
	l.settle()
	l.panes.retain(map[string]string{"wV:p9": "[wE:p1] api"})

	if l.panes.observe(moved()) != verdictClaimed {
		t.Error("a departed label was still taken as moved after a settled poll")
	}
}

func TestATabIsNotFollowedAcrossIDs(t *testing.T) {
	t.Parallel()

	// Herdr moves a tab only within its workspace and keeps its id, so a tab
	// wearing a departed tab's label is judged as any tab turning up named.
	l := settledLabels(t)
	l.tabs.observe(sighting{id: "wE:t1", current: "api", desired: "api", byDefault: "1"})

	l.tabs.retain(map[string]string{"wV:t9": "api"})

	if l.tabs.observe(
		sighting{id: "wV:t9", current: "api", desired: "web", byDefault: "1"},
	) != verdictClaimed {
		t.Error("a tab was taken as moved")
	}
}
