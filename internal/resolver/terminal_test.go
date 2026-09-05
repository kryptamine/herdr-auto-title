package resolver

import (
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func tabWithPane(pane *state.PaneState) state.TabState {
	pane.ID = "wE:p1"
	_ = pane
	pane.Focused = true

	return state.TabState{
		ID:    "wE:t1",
		Panes: []*state.PaneState{pane},
	}
}

func TestTerminalTitleBeatsTheWorkingDirectory(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Fix OAuth redirect",
	}))

	if got.Name != "dashboard › Fix OAuth redirect" {
		t.Errorf("name = %q, want %q", got.Name, "dashboard › Fix OAuth redirect")
	}

	if got.Reason != "terminal_title" {
		t.Errorf("reason = %q, want terminal_title", got.Reason)
	}

	if got.Confidence != ConfidenceTerminalTitle {
		t.Errorf("confidence = %d, want %d", got.Confidence, ConfidenceTerminalTitle)
	}
}

func TestGenericTerminalTitleFallsThrough(t *testing.T) {
	// Every one of these was observed on a live Herdr session.
	for _, title := range []string{"zsh", "Claude Code", "node", "~", "~/W/dashboard", ""} {
		t.Run(title, func(t *testing.T) {
			got := defaultChain().Resolve(tabWithPane(&state.PaneState{
				Dir:           "/Users/dev/work/dashboard",
				TerminalTitle: title,
			}))

			if got.Name != "dashboard" {
				t.Errorf("name = %q, want %q", got.Name, "dashboard")
			}

			if got.Reason != "cwd" {
				t.Errorf("reason = %q, want cwd", got.Reason)
			}
		})
	}
}

func TestTerminalTitleFallsBackToTheRawField(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:              "/Users/dev/work/dashboard",
		TerminalTitleRaw: "\x1b[32m✳ Fix OAuth redirect\x1b[0m",
	}))

	// The raw field carries escapes Herdr would normally have stripped.
	if got.Name != "dashboard › ✳ Fix OAuth redirect" {
		t.Errorf("name = %q, want %q", got.Name, "dashboard › ✳ Fix OAuth redirect")
	}
}

func TestStrippedTerminalTitleWinsOverTheRawOne(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:              "/Users/dev/work/dashboard",
		TerminalTitle:    "Fix OAuth redirect",
		TerminalTitleRaw: "◐ Fix OAuth redirect",
	}))

	if got.Name != "dashboard › Fix OAuth redirect" {
		t.Errorf("name = %q, want %q", got.Name, "dashboard › Fix OAuth redirect")
	}
}

func TestTerminalTitleIsSanitized(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "\x1b[31mFix OAuth\nredirect\x1b[0m\t",
	}))

	if got.Name != "dashboard › Fix OAuth redirect" {
		t.Errorf("name = %q, want %q", got.Name, "dashboard › Fix OAuth redirect")
	}
}

func TestLongTerminalTitleIsTruncatedAsAWhole(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: strings.Repeat("long ", 40),
	}))

	if runes := len([]rune(got.Name)); runes > DefaultMaxLength {
		t.Errorf("name is %d runes, want at most %d: %q", runes, DefaultMaxLength, got.Name)
	}

	if !strings.HasPrefix(got.Name, "dashboard › long") {
		t.Errorf("name = %q, want it to start with the context", got.Name)
	}
}

func TestTerminalTitleWithoutAWorkingDirectory(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		TerminalTitle: "Fix OAuth redirect",
	}))

	// No context to pair it with, so no dangling separator.
	if got.Name != "Fix OAuth redirect" {
		t.Errorf("name = %q, want %q", got.Name, "Fix OAuth redirect")
	}
}

func TestTerminalTitleRepeatingTheContextIsDropped(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Dashboard",
	}))

	if got.Name != "dashboard" {
		t.Errorf("name = %q, want %q", got.Name, "dashboard")
	}
}

func TestTerminalTitleWithNothingElse(t *testing.T) {
	got := defaultChain().Resolve(tabWithPane(&state.PaneState{
		TerminalTitle: "zsh",
	}))

	if got.Name != GenericFallback {
		t.Errorf("name = %q, want %q", got.Name, GenericFallback)
	}
}

func TestEditorTitleKeepsTheFileAndDropsThePath(t *testing.T) {
	// All three were observed on a live session.
	tests := []struct {
		title string
		want  string
	}{
		{
			"Makefile (~/Work/herdr-auto-title) - Nvim",
			"herdr-auto-title › Makefile - Nvim",
		},
		{
			"03-terminal-title-source.md (~/Work/herdr-auto-title/docs/issues) - Nvim",
			"herdr-auto-title › 03-terminal-title-source.md - Nvim",
		},
		{
			// A file browser buffer names no file, so only the editor is left.
			"- (oil:///Users/dev/Work/herdr-auto-title) - Nvim",
			"herdr-auto-title › Nvim",
		},
	}

	// The cap is not what this tests, and the longest of the three outgrows it.
	const wide = 80

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			got := Default(
				Options{MaxLength: wide, BranchMax: DefaultBranchMaxLength},
			).Resolve(tabWithPane(&state.PaneState{
				Dir:           "/Users/dev/Work/herdr-auto-title",
				TerminalTitle: tc.title,
			}))

			if got.Name != tc.want {
				t.Errorf("name = %q, want %q", got.Name, tc.want)
			}
		})
	}
}
