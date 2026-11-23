package resolver

import (
	"maps"
	"slices"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// twin builds a tab in workspace whose one pane in dir titles its terminal like
// every other twin, under its own IDs so two of them can be told apart.
func twin(id, workspace, dir string) state.TabState {
	tab := state.TabFrom(herdr.TabInfo{TabID: id}, workspace, 1, []*state.PaneState{{
		ID:            id + ":p1",
		Dir:           dir,
		TerminalTitle: "Fix OAuth redirect",
		Focused:       true,
	}}, false)

	return tab
}

// session gives the tab's pane an agent session reported by id.
func session(tab state.TabState, id string) state.TabState {
	tab.Context.AgentSession = &herdr.AgentSessionInfo{
		Agent: "claude", Kind: herdr.SessionRefID, Value: id,
	}

	return tab
}

// qualified resolves each tab the way a poll does once Qualifiers has run.
func qualified(chain *Deterministic, tabs ...state.TabState) []string {
	qualifiers := chain.Qualifiers(tabs)

	names := make([]string, len(tabs))
	for i, tab := range tabs {
		tab.Qualifier = qualifiers[tab.ID]
		names[i] = chain.Resolve(tab).Name
	}

	return names
}

func TestQualifiersLeaveDistinctTitlesAlone(t *testing.T) {
	t.Parallel()

	got := defaultChain().Qualifiers([]state.TabState{
		twin("w1:t1", "", dashboard),
		twin("w2:t1", "", api),
	})
	if len(got) != 0 {
		t.Errorf("qualifiers = %v, want none", got)
	}
}

func TestTwinsInTheirOwnWorkspacesGetTheirDirectoryBack(t *testing.T) {
	t.Parallel()

	// Each tab dropped its directory for repeating its workspace, so two tabs
	// whose agents chose the same title read the same everywhere one is listed.
	got := qualified(defaultChain(),
		twin("w1:t1", "dashboard", dashboard),
		twin("w2:t1", "api", api),
	)

	want := []string{"dashboard › Fix OAuth redirect", "api › Fix OAuth redirect"}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestOnlyTheTwinWithADirectoryIsQualified(t *testing.T) {
	t.Parallel()

	// A pane with no project directory, as in the home directory, has no
	// context to add, and once its twin says where it is the two differ.
	got := qualified(defaultChain(),
		twin("w1:t1", "dashboard", dashboard),
		twin("w2:t1", "finance", ""),
	)

	want := []string{"dashboard › Fix OAuth redirect", "Fix OAuth redirect"}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestAHiddenDirectoryStaysHiddenFromTwins(t *testing.T) {
	t.Parallel()

	// DIRECTORY_MAX=0 asked never to show the directory, so twins in two
	// directories go straight to their sessions.
	got := qualified(directoryChain(0, true),
		session(twin("w1:t1", "", dashboard), "3fa9c1d2-0000-4000-8000-000000000001"),
		session(twin("w1:t2", "", api), "7b20e4aa-0000-4000-8000-000000000002"),
	)

	want := []string{"3fa9c1d2 › Fix OAuth redirect", "7b20e4aa › Fix OAuth redirect"}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestHiddenDirectoryTwinsWithoutSessionsKeepTheirTitle(t *testing.T) {
	t.Parallel()

	got := directoryChain(0, true).Qualifiers([]state.TabState{
		twin("w1:t1", "", dashboard),
		twin("w1:t2", "", api),
	})
	if len(got) != 0 {
		t.Errorf("qualifiers = %v, want none", got)
	}
}

func TestTwinsInOneDirectoryAreToldApartByTheirSession(t *testing.T) {
	t.Parallel()

	got := qualified(defaultChain(),
		session(
			twin("w1:t1", "dashboard", dashboard),
			"3fa9c1d2-0000-4000-8000-000000000001",
		),
		session(
			twin("w2:t1", "dashboard", dashboard),
			"7b20e4aa-0000-4000-8000-000000000002",
		),
	)

	want := []string{"3fa9c1d2 › Fix OAuth redirect", "7b20e4aa › Fix OAuth redirect"}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestTwinsWithNothingToTellThemApartKeepTheirTitle(t *testing.T) {
	t.Parallel()

	// Two shells in one directory: there is nothing true to add, and a made-up
	// difference would be worse than none.
	got := defaultChain().Qualifiers([]state.TabState{
		twin("w1:t1", "dashboard", dashboard),
		twin("w1:t2", "dashboard", dashboard),
	})
	if len(got) != 0 {
		t.Errorf("qualifiers = %v, want none", got)
	}
}

func TestAThirdTabSharingADirectoryFallsBackToSessions(t *testing.T) {
	t.Parallel()

	// A directory that two of three twins share tells only one of them apart,
	// so the session, which all three hold, is what qualifies.
	got := qualified(defaultChain(),
		session(
			twin("w1:t1", "dashboard", dashboard),
			"3fa9c1d2-0000-4000-8000-000000000001",
		),
		session(
			twin("w2:t1", "dashboard", dashboard),
			"7b20e4aa-0000-4000-8000-000000000002",
		),
		session(
			twin("w3:t1", "api", api),
			"c0ffee00-0000-4000-8000-000000000003",
		),
	)

	want := []string{
		"3fa9c1d2 › Fix OAuth redirect",
		"7b20e4aa › Fix OAuth redirect",
		"c0ffee00 › Fix OAuth redirect",
	}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestAQualifiedTabIsNamedTheSameEveryTime(t *testing.T) {
	t.Parallel()

	tabs := []state.TabState{
		twin("w1:t1", "dashboard", dashboard),
		twin("w2:t1", "api", api),
	}

	first := defaultChain().Qualifiers(tabs)
	for range 20 {
		if again := defaultChain().Qualifiers(tabs); !maps.Equal(again, first) {
			t.Fatalf("qualifiers = %v, then %v", first, again)
		}
	}
}

func TestAQualifierMakesNoNewTwin(t *testing.T) {
	t.Parallel()

	// C already says dashboard, so giving A its directory back would only
	// move the twin from B to C.
	tabs := []state.TabState{
		twin("w1:t1", "dashboard", dashboard),
		twin("w2:t1", "api", api),
		twin("w3:t1", "other", dashboard),
	}

	got := qualified(defaultChain(), tabs...)
	if got[0] == got[2] {
		t.Errorf("names = %q, want A and C apart", got)
	}

	got = qualified(defaultChain(),
		session(tabs[0], "3fa9c1d2-0000-4000-8000-000000000001"),
		session(tabs[1], "7b20e4aa-0000-4000-8000-000000000002"),
		tabs[2],
	)

	want := []string{
		"3fa9c1d2 › Fix OAuth redirect",
		"7b20e4aa › Fix OAuth redirect",
		"dashboard › Fix OAuth redirect",
	}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestTheDirectoryOutranksTheSession(t *testing.T) {
	t.Parallel()

	// Both would tell the twins apart; the directory is the one a reader knows.
	got := qualified(defaultChain(),
		session(twin("w1:t1", "dashboard", dashboard), "3fa9c1d2-0000-4000-8000-000000000001"),
		session(twin("w2:t1", "api", api), "7b20e4aa-0000-4000-8000-000000000002"),
	)

	want := []string{"dashboard › Fix OAuth redirect", "api › Fix OAuth redirect"}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestAQualifierKeepsToTheDirectoryBound(t *testing.T) {
	t.Parallel()

	// The bound is what leaves the activity its width.
	got := qualified(directoryChain(4, false),
		twin("w1:t1", "dashboard", dashboard),
		twin("w2:t1", "api", api),
	)

	want := []string{"dash › Fix OAuth redirect", "api › Fix OAuth redirect"}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestABoundThatJoinsTwinsGivesWayToTheWholeDirectory(t *testing.T) {
	t.Parallel()

	web := herdrtest.Dir("work", "dashboard-web")
	admin := herdrtest.Dir("work", "dashboard-admin")

	got := qualified(directoryChain(9, false),
		twin("w1:t1", "dashboard-web", web),
		twin("w2:t1", "dashboard-admin", admin),
	)

	want := []string{"dashboard-web › Fix OAuth redirect", "dashboard-admin › Fix OAuth redirect"}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestTwinGroupsClaimTitlesInTitleOrder(t *testing.T) {
	t.Parallel()

	// Both groups could take `dashboard › …`; the one whose title sorts first
	// decides, so the outcome does not hang on the order of a map.
	got := qualified(defaultChain(),
		session(twin("w1:t1", "dashboard", dashboard), "a0000001-0000-4000-8000-000000000001"),
		session(twin("w2:t1", "api", api), "b0000002-0000-4000-8000-000000000002"),
		session(twin("w3:t1", "other", dashboard), "c0000003-0000-4000-8000-000000000003"),
		session(twin("w4:t1", "other", dashboard), "d0000004-0000-4000-8000-000000000004"),
	)

	want := []string{
		"a0000001 › Fix OAuth redirect",
		"b0000002 › Fix OAuth redirect",
		"c0000003 › Fix OAuth redirect",
		"d0000004 › Fix OAuth redirect",
	}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestAGroupNothingTellsApartGivesBackTheTitlesItTried(t *testing.T) {
	t.Parallel()

	// The first group's sessions share their first eight characters, so no
	// rung works; the titles it tried must not block the next group.
	got := qualified(defaultChain(),
		session(twin("w1:t1", "dashboard", dashboard), "3fa9c1d2-0000-4000-8000-000000000001"),
		session(twin("w1:t2", "dashboard", dashboard), "3fa9c1d2-0000-4000-8000-000000000002"),
		session(twin("w2:t1", "other", api), "3fa9c1d2-0000-4000-8000-000000000003"),
		session(twin("w2:t2", "other", api), "7b20e4aa-0000-4000-8000-000000000004"),
	)

	want := []string{
		"Fix OAuth redirect",
		"Fix OAuth redirect",
		"3fa9c1d2 › Fix OAuth redirect",
		"7b20e4aa › Fix OAuth redirect",
	}
	if !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}
