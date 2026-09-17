package state

import (
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// Herdr labels an unnamed workspace after the directory it holds, so that
// basename is the one thing a poll can tell the user's name apart by. These are
// the shapes that decision has.
func TestAWorkspaceIsClaimedOnSight(t *testing.T) {
	tests := []struct {
		name    string
		settled bool
		known   string
		sight   Sighting
		want    bool
	}{
		{
			name:  "first poll, still wearing its directory",
			sight: Sighting{ID: "wE", Current: "api", Desired: "api › feat/oauth", Default: "api"},
			want:  false,
		},
		{
			name: "first poll, a name its owner wrote",
			sight: Sighting{
				ID:      "wE",
				Current: "the migration",
				Desired: "api › feat/oauth",
				Default: "api",
			},
			want: true,
		},
		{
			// The one a tab would let through: indistinguishable from this
			// resolver's own work, but on a workspace it was there first.
			name: "first poll, a name this would have chosen too",
			sight: Sighting{
				ID:      "wE",
				Current: "api › feat/oauth",
				Desired: "api › feat/oauth",
				Default: "api",
			},
			want: true,
		},
		{
			name:    "opened after the first poll, wearing its directory",
			settled: true,
			sight: Sighting{
				ID:      "wE",
				Current: "api",
				Desired: "api › feat/oauth",
				Default: "api",
			},
			want: false,
		},
		{
			// Not claimed for being unseen: it has not been watched long
			// enough for its label to mean anything yet.
			name:    "opened after the first poll, already carrying a name",
			settled: true,
			sight: Sighting{
				ID:      "wE",
				Current: "the migration",
				Desired: "api › feat/oauth",
				Default: "api",
			},
			want: false,
		},
		{
			name:    "renamed by hand after this had named it",
			settled: true,
			known:   "api › feat/oauth",
			sight: Sighting{
				ID:      "wE",
				Current: "the migration",
				Desired: "api › feat/billing",
				Default: "api",
			},
			want: true,
		},
		{
			name:    "cleared back to the directory's own name",
			settled: true,
			known:   "api › feat/oauth",
			sight: Sighting{
				ID:      "wE",
				Current: "api",
				Desired: "api › feat/oauth",
				Default: "api",
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := LoadManual("")
			if test.known != "" {
				m.Workspaces.Applied(test.sight.ID, test.known)
			}

			if test.settled {
				m.Settled()
			}

			if got := m.Workspaces.Observe(test.sight); got != test.want {
				t.Errorf("Observe() = %v, want %v", got, test.want)
			}

			if got := m.Workspaces.Locked(test.sight.ID); got != test.want {
				t.Errorf("Locked() = %v, want %v", got, test.want)
			}
		})
	}
}

// A workspace whose tab holds no pane yet shows no directory, so the first poll
// has nothing to compare its label against. It is judged on the first poll that
// does, by the same test -- not claimed under whatever it wore meanwhile.
func TestAWorkspaceWithNoDefaultYetIsJudgedOnTheFirstPollThatHasOne(t *testing.T) {
	tests := []struct {
		name    string
		settled bool
		current string
		want    bool
	}{
		{name: "still wearing its directory", current: "api", want: false},
		{name: "a name its owner wrote", current: "the migration", want: true},
		{name: "still wearing its directory, judged after settling", settled: true, current: "api"},
		{
			name:    "a name its owner wrote, judged after settling",
			settled: true,
			current: "the migration",
			want:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := LoadManual("")

			if m.Workspaces.Observe(Sighting{ID: "wE", Current: test.current}) {
				t.Fatal("claimed a workspace with no default to compare against")
			}

			if test.settled {
				m.Settled()
			}

			s := Sighting{
				ID:      "wE",
				Current: test.current,
				Desired: "api › feat/oauth",
				Default: "api",
			}
			if got := m.Workspaces.Observe(s); got != test.want {
				t.Errorf("Observe() = %v, want %v", got, test.want)
			}

			if got := m.Workspaces.Locked("wE"); got != test.want {
				t.Errorf("Locked() = %v, want %v", got, test.want)
			}
		})
	}
}

// The deferral does not outlive the workspace: one closed while still waiting
// for a pane is forgotten, so a later workspace reusing its id starts afresh.
func TestAWorkspaceClosedWhileWaitingForAPaneIsForgotten(t *testing.T) {
	m := LoadManual("")
	m.Workspaces.Observe(Sighting{ID: "wE", Current: "the migration"})
	m.Settled()
	m.Workspaces.Retain(map[string]string{})

	s := Sighting{ID: "wE", Current: "the migration", Desired: "api › feat/oauth", Default: "api"}
	if m.Workspaces.Observe(s) {
		t.Error(
			"claimed a workspace opened after the first poll, on a deferral its predecessor left",
		)
	}
}

// A tab is not told apart on the first poll: its label says nothing about who
// wrote it, so the same sighting that claims a workspace leaves a tab alone.
func TestATabIsNotClaimedOnSight(t *testing.T) {
	m := LoadManual("")
	s := Sighting{ID: "wE:t1", Current: "the migration", Desired: "api", Default: "1"}

	if m.Tabs.Observe(s) {
		t.Error("the first poll claimed a tab")
	}
}

func TestWorkspaceFromCarriesTheDirectoryAsItsDefault(t *testing.T) {
	root := string(filepath.Separator)

	tests := []struct {
		name    string
		dir     string
		nilPane bool
		want    string
	}{
		{name: "an ordinary directory", dir: filepath.Join(root, "work", "api"), want: "api"},
		{name: "a trailing separator", dir: filepath.Join(root, "work", "api") + root, want: "api"},
		{name: "the filesystem root", dir: root, want: ""},
		{name: "a relative path", dir: filepath.Join("work", "api"), want: ""},
		{name: "no directory at all", dir: "", want: ""},
		{name: "no pane to read", nilPane: true, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var pane *PaneState
			if !test.nilPane {
				pane = &PaneState{ID: "wE:p1", Dir: test.dir}
			}

			ws := WorkspaceFrom(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: "shipping"}, pane)
			if ws.DefaultName != test.want {
				t.Errorf("DefaultName = %q, want %q", ws.DefaultName, test.want)
			}

			sight := WorkspaceSightingFrom(ws, "desired")
			if sight.ID != "wE" || sight.Current != "shipping" ||
				sight.Desired != "desired" || sight.Default != test.want {
				t.Errorf("sighting = %+v", sight)
			}
		})
	}
}

// Herdr runs the startup hook again at every start and live handoff, so a
// restarted plugin meets rows it wrote itself: neither the basename nor a name
// it remembers. The label last written is kept in the file for exactly this.
func TestARowThisWroteIsItsOwnAfterAReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manual-names.json")

	m := LoadManual(path)
	m.Workspaces.Observe(
		Sighting{ID: "wE", Current: "api", Desired: "api › feat/oauth", Default: "api"},
	)
	m.Workspaces.Applied("wE", "api › feat/oauth")
	m.Settled()

	reloaded := LoadManual(path)

	s := Sighting{
		ID:      "wE",
		Current: "api › feat/oauth",
		Desired: "api › feat/billing",
		Default: "api",
	}
	if reloaded.Workspaces.Observe(s) {
		t.Error("a row this wrote before the restart was claimed as the user's")
	}

	if reloaded.Workspaces.Locked("wE") {
		t.Error("Locked() = true for a row this wrote itself")
	}

	// Renamed by the user after that, it is claimed as any other.
	moved := Sighting{
		ID:      "wE",
		Current: "the migration",
		Desired: "api › feat/billing",
		Default: "api",
	}
	if !reloaded.Workspaces.Observe(moved) {
		t.Error("a row the user renamed after the restart was not claimed")
	}
}

// What was written for a workspace the session no longer holds is dropped, so
// a workspace later reusing its id is judged on its own label.
func TestWhatWasWrittenForAClosedWorkspaceIsForgotten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manual-names.json")

	m := LoadManual(path)
	m.Workspaces.Applied("wE", "api › feat/oauth")
	m.Workspaces.Retain(map[string]string{})

	reloaded := LoadManual(path)
	reloaded.Settled()

	s := Sighting{ID: "wE", Current: "api › feat/oauth", Desired: "api", Default: "api"}
	if reloaded.Workspaces.Pending("wE") || reloaded.Workspaces.Observe(s) {
		t.Error("a forgotten label still spoke for a workspace reusing its id")
	}

	if _, written := reloaded.Workspaces.written["wE"]; written {
		t.Error("the file still carries a label for a workspace that closed")
	}
}

// A row rename that got no answer and landed late is this plugin's work as much
// as one that answered, so a restart must find it recorded as such.
func TestARowRenameThatLandedLateIsItsOwnAfterAReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manual-names.json")

	m := LoadManual(path)
	m.Workspaces.Observe(
		Sighting{ID: "wE", Current: "api", Desired: "api › feat/oauth", Default: "api"},
	)
	m.Workspaces.Sent("wE", "api › feat/oauth")
	m.Settled()
	m.Workspaces.Observe(
		Sighting{
			ID:      "wE",
			Current: "api › feat/oauth",
			Desired: "api › feat/oauth",
			Default: "api",
		},
	)

	reloaded := LoadManual(path)

	s := Sighting{
		ID:      "wE",
		Current: "api › feat/oauth",
		Desired: "api › feat/billing",
		Default: "api",
	}
	if reloaded.Workspaces.Observe(s) || reloaded.Workspaces.Locked("wE") {
		t.Error("a rename that landed late was claimed as the user's after the restart")
	}
}
