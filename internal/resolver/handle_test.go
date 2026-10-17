package resolver

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func handleChain(maxLength int) *Deterministic {
	return Default(
		Options{MaxLength: maxLength, BranchMax: DefaultBranchMaxLength, ShowHandle: true},
	)
}

// handleTab is a split whose second pane, in a directory of its own, holds the
// agent named handle.
func handleTab(handle, title string) state.TabState {
	return tabOf([]*state.PaneState{
		{ID: "wE:p1", Dir: dashboard, Focused: true},
		{ID: "wE:p2", Dir: api, Handle: handle, TerminalTitle: title},
	})
}

func TestAHandleLeadsOnlyThePaneWhoseAgentHasOne(t *testing.T) {
	t.Parallel()

	got := handleChain(DefaultMaxLength).ResolvePanes(handleTab("driver", ""))

	want := []string{"dashboard", "driver › api"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("pane %d = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestAHandleNeverReachesATabTitle(t *testing.T) {
	t.Parallel()

	tab := tabOf([]*state.PaneState{{ID: "wE:p1", Dir: api, Focused: true, Handle: "driver"}})

	if got := handleChain(DefaultMaxLength).Resolve(tab).Name; got != "api" {
		t.Errorf("tab = %q, want %q", got, "api")
	}
}

func TestAHandleIsLeftOutUnlessAskedFor(t *testing.T) {
	t.Parallel()

	if got := defaultChain().ResolvePanes(handleTab("driver", ""))[1].Name; got != "api" {
		t.Errorf("pane = %q, want %q", got, "api")
	}
}

func TestAHandleKeepsAPaneThatSharesItsTabsContext(t *testing.T) {
	t.Parallel()

	// A pane its tab already describes in full keeps its context rather than
	// being reduced to the handle alone.
	tab := tabOf([]*state.PaneState{{ID: "wE:p1", Dir: api, Focused: true, Handle: "driver"}})

	if got := handleChain(DefaultMaxLength).ResolvePanes(tab)[0].Name; got != "driver › api" {
		t.Errorf("pane = %q, want %q", got, "driver › api")
	}
}

func TestAHandleTheLabelAlreadySaysIsNotRepeated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, handle, title, want string
	}{
		{"the context", "api", "fixing the login", "api › fixing the login"},
		{"the context in another case", "API", "", "API"},
		{"the activity", "Fixing", "fixing", "api › fixing"},
		{
			"the context behind an invisible character",
			"api\u200b",
			"fixing the login",
			"api › fixing the login",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := handleChain(DefaultMaxLength).ResolvePanes(handleTab(tc.handle, tc.title))[1].Name
			if got != tc.want {
				t.Errorf("pane = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAHandleNamedAfterTheAgentsKindReplacesIt(t *testing.T) {
	t.Parallel()

	tab := tabOf([]*state.PaneState{
		{ID: "wE:p1", Dir: dashboard, Focused: true},
		{ID: "wE:p2", Dir: api, Agent: "claude", Handle: "Claude"},
	})

	if got := handleChain(DefaultMaxLength).ResolvePanes(tab)[1].Name; got != "Claude › api" {
		t.Errorf("pane = %q, want %q", got, "Claude › api")
	}
}

func TestAHandleNamedAfterADirectoryTheTabSharesSurvives(t *testing.T) {
	t.Parallel()

	// A worktree named after the ticket its agent holds: the tab above already
	// says the directory, which must not take the handle down with it.
	tab := tabOf([]*state.PaneState{{
		ID: "wE:p1", Dir: api, Focused: true, Handle: "api", TerminalTitle: "fixing the login",
	}})

	if got := handleChain(DefaultMaxLength).ResolvePanes(tab)[0].Name; got != "api › fixing the login" {
		t.Errorf("pane = %q, want %q", got, "api › fixing the login")
	}
}

func TestAHandleIsSanitized(t *testing.T) {
	t.Parallel()

	got := handleChain(DefaultMaxLength).ResolvePanes(handleTab("\x1b[31mdriver\x1b[0m\u202e", ""))[1].Name
	if got != "driver › api" {
		t.Errorf("pane = %q, want %q", got, "driver › api")
	}
}

func TestAHandleWiderThanTheLabelIsCutNotDropped(t *testing.T) {
	t.Parallel()

	// A cut handle still tells two panes apart; a dropped one does not.
	const maxLength = 12

	got := handleChain(maxLength).ResolvePanes(handleTab(strings.Repeat("reviewer", 3), ""))[1].Name
	if width := uniseg.StringWidth(got); width > maxLength {
		t.Errorf("pane %q is %d columns wide, want at most %d", got, width, maxLength)
	}

	if !strings.HasPrefix(got, "reviewer") {
		t.Errorf("pane = %q, want it to start with the handle", got)
	}
}

func TestAHandleFollowsThePaneID(t *testing.T) {
	t.Parallel()

	got := NewPaneIDs(
		handleChain(DefaultMaxLength),
		DefaultMaxLength,
	).ResolvePanes(handleTab("driver", ""))
	if got[1].Name != "[wE:p2] driver › api" {
		t.Errorf("pane = %q, want %q", got[1].Name, "[wE:p2] driver › api")
	}
}
