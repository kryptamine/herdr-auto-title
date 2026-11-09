package resolver

import (
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func ompPane(title string) *state.PaneState {
	return &state.PaneState{
		Dir:           dashboard,
		TerminalTitle: title,
		Agent:         "omp",
		AgentStatus:   herdr.AgentStatusWorking,
	}
}

// omp redraws the state every 80 ms while it works; a frame read as part of
// the title renamed the tab on every one.
func TestEveryOmpStateReadsAsOneTitle(t *testing.T) {
	t.Parallel()

	const want = "dashboard › omp › Rework the poll loop"

	for _, title := range []string{
		"π > Rework the poll loop",
		"π ⠋ Rework the poll loop", "π ⠙ Rework the poll loop",
		"π ○ Rework the poll loop", "π ◔ Rework the poll loop",
		`π \ Rework the poll loop`, "π - Rework the poll loop",
		"π ! Rework the poll loop", "π : Rework the poll loop",
		"π: Rework the poll loop",
	} {
		if got := defaultChain().Resolve(tabWithPane(ompPane(title))).Name; got != want {
			t.Errorf("%q: name = %q, want %q", title, got, want)
		}
	}
}

// Until a session has a name omp frames its directory, which the context
// already says, or nothing at all.
func TestAnUnnamedOmpSessionDeclinesItsTitle(t *testing.T) {
	t.Parallel()

	for _, title := range []string{"π ⠋ dashboard", "π > dashboard", "π ⠋", "π >", "π:", "π"} {
		if got := defaultChain().Resolve(tabWithPane(ompPane(title))).
			Name; got != "dashboard › omp" {
			t.Errorf("%q: name = %q, want %q", title, got, "dashboard › omp")
		}
	}
}

// In the home directory the context is empty, so only the title's own check
// keeps the directory omp frames from becoming the activity.
func TestAnUnnamedOmpSessionAtHomeDeclinesItsTitle(t *testing.T) {
	t.Parallel()

	home := herdrtest.Dir()
	pane := ompPane("π > " + filepath.Base(home))
	pane.Dir = home

	chain := Default(
		Options{MaxLength: DefaultMaxLength, BranchMax: DefaultBranchMaxLength, Home: home},
	)
	if got := chain.Resolve(tabWithPane(pane)).Name; got != "omp" {
		t.Errorf("name = %q, want %q", got, "omp")
	}
}

func TestATitleOmpDidNotFrameIsReadAsItIs(t *testing.T) {
	t.Parallel()

	for _, title := range []string{"Deploying staging", "π-calculus notes", "πr² ⠋ area", "π is irrational"} {
		if got, want := defaultChain().Resolve(tabWithPane(ompPane(title))).
			Name, "dashboard › omp › "+title; got != want {
			t.Errorf("name = %q, want %q", got, want)
		}
	}
}
