package resolver

import (
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// The sidebar is a column of rows that often begin alike, so a row cut from the
// end would leave a screenful of the same repository name. Whole parts go from
// the front instead, and only what survives is cut.
func TestFormatTailDropsWholePartsFromTheFront(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		parts  Parts
		maxLen int
		want   string
	}{
		{
			name:   "everything fits",
			parts:  Parts{Context: "api", Branch: "feat/oauth"},
			maxLen: 20,
			want:   "api › feat/oauth",
		},
		{
			name: "the context goes first",
			parts: Parts{
				Context:  "demo-repo",
				Branch:   "feat/billing",
				Activity: "Fix invoice totals",
			},
			maxLen: 20,
			want:   "Fix invoice totals",
		},
		{
			name:   "only the context goes when that is enough",
			parts:  Parts{Context: "demo-repo", Branch: "feat/billing"},
			maxLen: 14,
			want:   "feat/billing",
		},
		{
			// The decisive one: dropping the context alone is enough, so the
			// branch must survive. Dropping every leading part at once would
			// leave the activity by itself and still fit.
			name:   "only what has to go, goes",
			parts:  Parts{Context: "api", Branch: "feat/oauth", Activity: "nvim"},
			maxLen: 18,
			want:   "feat/oauth › nvim",
		},
		{
			name:   "a double-width part is measured in columns, not runes",
			parts:  Parts{Context: "api", Activity: "安裝設定檔案"},
			maxLen: 12,
			want:   "安裝設定檔案",
		},
		{
			name:   "the last part is kept even when it must be cut",
			parts:  Parts{Context: "api", Activity: "Reconcile the ledger"},
			maxLen: 10,
			want:   "Reconcile",
		},
		{
			name:   "a lone part is cut rather than dropped",
			parts:  Parts{Context: "a-very-long-project-name"},
			maxLen: 8,
			want:   "a-very-l",
		},
		{
			name:   "nothing to say",
			parts:  Parts{},
			maxLen: 20,
			want:   "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := FormatTail(test.parts, test.maxLen); got != test.want {
				t.Errorf("FormatTail() = %q, want %q", got, test.want)
			}
		})
	}
}

// Format is what a tab keeps: the front matters there, because a tab bar holds
// titles that differ from their first column.
func TestFormatStillCutsFromTheEnd(t *testing.T) {
	t.Parallel()

	parts := Parts{Context: "demo-repo", Branch: "feat/billing", Activity: "Fix invoice totals"}

	tail := FormatTail(parts, 20)
	head := Format(parts, 20)

	if tail == head {
		t.Fatalf("the two should differ, both gave %q", tail)
	}

	if got := head[:4]; got != "demo" {
		t.Errorf("Format dropped its front: %q", head)
	}
}

// Places is the shipped chain minus one source. Each of the rest is checked by
// giving a pane only what that source reads, and the process is checked by
// giving it one and finding it unused.
func TestPlacesKeepsEverySourceButTheProcess(t *testing.T) {
	t.Parallel()

	places := Places(Options{MaxLength: 60, BranchMax: DefaultBranchMaxLength})

	tests := []struct {
		name string
		pane *state.PaneState
		want string
	}{
		{
			name: "cwd",
			pane: &state.PaneState{Dir: herdrtest.Dir("work", "api")},
			want: "cwd",
		},
		{
			name: "git",
			pane: &state.PaneState{
				Dir: herdrtest.Dir("work", "api"),
				Git: git.Checkout{Branch: "feat/oauth", Default: "main"},
			},
			want: "git",
		},
		{
			name: "terminal_title",
			pane: &state.PaneState{
				Dir:           herdrtest.Dir("work", "api"),
				TerminalTitle: "Fix invoice totals",
			},
			want: "terminal_title",
		},
		{
			name: "transcript",
			pane: &state.PaneState{
				Dir: herdrtest.Dir(
					"work",
					"api",
				),
				Agent:      "claude",
				AgentTopic: "Fix invoice totals",
			},
			want: "transcript",
		},
		{
			name: "agent",
			pane: &state.PaneState{
				Dir: herdrtest.Dir(
					"work",
					"api",
				),
				Agent:      "claude",
				AgentTitle: "Fix invoice totals",
			},
			want: "agent",
		},
		{
			name: "ssh",
			pane: &state.PaneState{
				Dir:       herdrtest.Dir("work", "api"),
				Processes: []state.Process{{Name: "ssh", Args: []string{"ssh", "prod-01"}}},
			},
			want: "ssh",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := places.ResolveWorkspace(state.WorkspaceState{Context: test.pane})
			if got.Reason != test.want {
				t.Errorf("Reason = %q, want %q (name %q)", got.Reason, test.want, got.Name)
			}
		})
	}

	// The process is left out of the chain, and the terminal title above it
	// must not bring it back as the kind it would bind to a tab's title.
	for _, test := range []struct {
		name string
		pane *state.PaneState
	}{
		{
			name: "not the process",
			pane: &state.PaneState{
				Dir:       herdrtest.Dir("work", "api"),
				Processes: []state.Process{{Name: "npm", Args: []string{"npm", "run", "build"}}},
			},
		},
		{
			name: "not the process behind a terminal title",
			pane: &state.PaneState{
				Dir:           herdrtest.Dir("work", "api"),
				TerminalTitle: "auth.ts",
				Processes:     []state.Process{{Name: "npm", Args: []string{"npm", "run", "build"}}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := places.ResolveWorkspace(state.WorkspaceState{Context: test.pane})
			if got.Reason == "process" || strings.Contains(got.Name, "npm") {
				t.Errorf("the row took the foreground process: %+v", got)
			}
		})
	}
}

// A terminal title binds the pane's kind to a tab's title, `nvim › auth.ts`.
// The row reads the same title without the kind: a program is the one thing a
// row must not be named after, however the name reaches it.
func TestTheRowDoesNotCarryTheForegroundProcessThroughTheTerminalTitle(t *testing.T) {
	t.Parallel()

	places := Places(Options{MaxLength: 60, BranchMax: DefaultBranchMaxLength})

	pane := &state.PaneState{
		Dir:           herdrtest.Dir("work", "dashboard"),
		TerminalTitle: "auth.ts",
		Processes:     []state.Process{{Name: "nvim", Args: []string{"nvim", "auth.ts"}}},
	}

	first := places.ResolveWorkspace(state.WorkspaceState{Context: pane})
	if strings.Contains(first.Name, "nvim") {
		t.Errorf("the row carries the foreground process: %q", first.Name)
	}

	pane.Processes = []state.Process{{Name: "less", Args: []string{"less", "auth.ts"}}}

	second := places.ResolveWorkspace(state.WorkspaceState{Context: pane})
	if strings.Contains(second.Name, "less") {
		t.Errorf("the row carries the foreground process: %q", second.Name)
	}

	if first.Name != second.Name {
		t.Errorf("the row followed the process: %q then %q", first.Name, second.Name)
	}
}

// The tab's chain is untouched by that: a tab under the same pane still reads
// the program the title came from.
func TestATabStillCarriesTheForegroundProcessThroughTheTerminalTitle(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{
		Dir:           herdrtest.Dir("work", "dashboard"),
		TerminalTitle: "auth.ts",
		Processes:     []state.Process{{Name: "nvim", Args: []string{"nvim", "auth.ts"}}},
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "dashboard › nvim › auth.ts"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

// A workspace whose tab holds no pane, and one whose pane says nothing, are
// both left alone rather than named the generic fallback a tab would take.
func TestResolveWorkspaceSaysNothingRatherThanShell(t *testing.T) {
	t.Parallel()

	places := Places(Options{MaxLength: 20})

	for _, test := range []struct {
		name string
		ws   state.WorkspaceState
	}{
		{name: "no pane", ws: state.WorkspaceState{}},
		{name: "a pane with nothing to read", ws: state.WorkspaceState{Context: &state.PaneState{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := places.ResolveWorkspace(test.ws); got.Name != "" {
				t.Errorf("named %q, want the empty decision", got.Name)
			}
		})
	}
}

// A part counted wide before sanitizing can turn out to fit after it, and a row
// that fits must not lose its context to a run of spaces.
func TestFormatTailMeasuresWhatItWouldWrite(t *testing.T) {
	t.Parallel()

	got := FormatTail(Parts{Context: "a     b", Activity: "x"}, 10)
	if want := "a b › x"; got != want {
		t.Errorf("FormatTail() = %q, want %q", got, want)
	}
}

// The row is stored sanitized and a tab's parts are not, so a part carrying
// anything Sanitize rewrites has to be compared in the same shape or it misses
// its own segment and is said twice.
func TestATabDropsAPartTheRowSanitized(t *testing.T) {
	t.Parallel()

	row := FormatTail(Parts{Context: "api     service"}, 20)
	if row != "api service" {
		t.Fatalf("the row sanitized to %q, which is not what this case needs", row)
	}

	got := withoutRow(Parts{Context: "api     service", Activity: "nvim"}, rowParts(row))
	if got.Context != "" {
		t.Errorf("the tab kept a context the row already carries: %q", got.Context)
	}
}

// A directory whose name carries the separator is written to the row as one
// part, and the tab under it must still see that part as its own context: read
// back as two segments, it matches neither and the tab repeats the row. The
// activity is repeated by design; the context is what must go.
func TestATabDoesNotRepeatARowWhoseDirectoryCarriesTheSeparator(t *testing.T) {
	t.Parallel()

	dir := herdrtest.Dir("work", "a › b")
	pane := &state.PaneState{Dir: dir, TerminalTitle: "nvim"}

	row := Places(Options{MaxLength: DefaultWorkspaceMaxLength, BranchMax: DefaultBranchMaxLength}).
		ResolveWorkspace(state.WorkspaceState{Context: pane})
	if row.Name != "a › b › nvim" {
		t.Fatalf("the row was named %q, which is not what this case needs", row.Name)
	}

	tab := tabWithPane(pane)
	tab.WorkspaceName = row.Name

	if got, want := namingChain().Resolve(tab).Name, "nvim"; got != want {
		t.Errorf("name = %q under row %q, want %q", got, row.Name, want)
	}
}

// The row's segments have no positions, so a tab drops its agent's name, like
// its context and its branch, against any segment. Left alone, the label is
// matched whole and against the context only, as before the row was ever named.
func TestATabDropsItsAgentAgainstAnySegmentOfTheRowOnlyWhenNamingWorkspaces(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{Dir: api, Agent: "claude", AgentTitle: "Fix totals"}

	on := tabWithPane(pane)
	on.WorkspaceName = "claude › Fix totals"

	if got, want := namingChain().Resolve(on).Name, "api › Fix totals"; got != want {
		t.Errorf("naming workspaces: name = %q, want %q", got, want)
	}

	off := tabWithPane(pane)
	off.WorkspaceName = "claude › Fix totals"

	if got, want := defaultChain().Resolve(off).Name, "api › claude › Fix totals"; got != want {
		t.Errorf("left alone: name = %q, want %q", got, want)
	}
}

// namingChain is the shipped chain with workspace naming turned on.
func namingChain() *Deterministic {
	return Default(Options{
		MaxLength:       DefaultMaxLength,
		BranchMax:       DefaultBranchMaxLength,
		NamesWorkspaces: true,
	})
}

// A row Herdr labelled with a basename has one segment, like a row this wrote
// and cut down to its branch, and nothing tells the two apart. The one segment
// is matched against every part, so a branch spelled like the directory goes.
func TestARowWithOneSegmentIsMatchedAgainstEveryPart(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{
		Dir:           api,
		Git:           git.Checkout{Branch: "release", Default: "main"},
		TerminalTitle: "Fix OAuth redirect",
	}
	tab := tabWithPane(pane)
	tab.WorkspaceName = "release"

	if got, want := namingChain().Resolve(tab).Name, "api › Fix OAuth redirect"; got != want {
		t.Errorf("naming workspaces: name = %q, want %q", got, want)
	}

	if got, want := defaultChain().Resolve(tab).
		Name, "api › release › Fix OAuth redirect"; got != want {
		t.Errorf("left alone: name = %q, want %q", got, want)
	}
}

// The row shows the agent's name whenever the tab bar would, and a tab under
// such a row must not say it again. The row here is too long for its context,
// so the agent is the one part left for the tab to repeat.
func TestATabDoesNotRepeatTheAgentTheRowCarries(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{
		ID:         "wE:p1",
		Dir:        herdrtest.Dir("work", "dashboard"),
		Focused:    true,
		Agent:      "claude",
		AgentTitle: "Fix",
	}

	row := Places(Options{MaxLength: DefaultWorkspaceMaxLength, BranchMax: DefaultBranchMaxLength}).
		ResolveWorkspace(state.WorkspaceState{Context: pane})
	if row.Name != "claude › Fix" {
		t.Fatalf("the row reads %q, which is not what this case needs", row.Name)
	}

	tab := tabWithPane(pane)
	tab.WorkspaceName = row.Name

	chain := Default(Options{
		MaxLength:       DefaultMaxLength,
		BranchMax:       DefaultBranchMaxLength,
		NamesWorkspaces: true,
	})

	if got, want := chain.Resolve(tab).Name, "dashboard › Fix"; got != want {
		t.Errorf("tab named %q, want %q", got, want)
	}
}

// A sole tab with only a context and a branch is said in full by the row named
// after it. Dropping both would leave the tab the generic fallback under a row
// that is its own title, which loses more than it saves.
func TestATabSaidWhollyByTheRowKeepsItsTitle(t *testing.T) {
	t.Parallel()

	tab := tabWithPane(&state.PaneState{Dir: dashboard})
	tab.WorkspaceName = "dashboard"

	names := Default(Options{
		MaxLength:       DefaultMaxLength,
		BranchMax:       DefaultBranchMaxLength,
		NamesWorkspaces: true,
	})

	got := names.Resolve(tab)
	if got.Name != "dashboard" {
		t.Errorf("name = %q, want dashboard", got.Name)
	}
}
