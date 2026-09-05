package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// running builds a pane whose processes Herdr reports by name.
func running(names ...string) []state.Process {
	processes := make([]state.Process, 0, len(names))
	for _, name := range names {
		processes = append(processes, state.Process{Name: name, Args: []string{name}})
	}

	return processes
}

func TestTheKindQualifiesTheTitle(t *testing.T) {
	// The editor names itself in its own title; under the kind that is noise.
	pane := &state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "auth.provider.ts - Nvim",
		Processes:     running("nvim"),
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "dashboard › nvim › auth.provider.ts"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAKindWithNothingToAddStandsAlone(t *testing.T) {
	// Neovim showing a file manager titles the window after itself alone.
	pane := &state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Nvim",
		Processes:     running("nvim"),
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "dashboard › nvim"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAKindWithNoTitleAtAllStillNamesThePane(t *testing.T) {
	pane := &state.PaneState{Dir: "/Users/dev/work/dashboard", Processes: running("htop")}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "dashboard › htop"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "process" {
		t.Errorf("reason = %q, want process", got.Reason)
	}

	if got.Confidence != ConfidenceProcess {
		t.Errorf("confidence = %d, want %d", got.Confidence, ConfidenceProcess)
	}
}

func TestOnlyALoneProcessNamesAPane(t *testing.T) {
	// Every process list here was taken from a live session.
	cases := []struct {
		names []string
		want  string
	}{
		// A shell is not what a pane is for.
		{[]string{"fish"}, ""},
		{[]string{"nvim"}, "nvim"},
		{[]string{"nvim", "fish"}, "nvim"},
		{[]string{"esbuild"}, "esbuild"},
		// yarn dev: a build tool and its workers. Picking one is guesswork.
		{[]string{"esbuild", "node", "node", "node", "node", "node"}, ""},
		// An agent and its helpers.
		{[]string{"caffeinate", "node", "node", "fff-mcp", "2.1.241"}, ""},
		// A bare runtime names the language, not the work.
		{[]string{"node"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := paneKind(&state.PaneState{Processes: running(c.names...)}); got != c.want {
			t.Errorf("paneKind(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}

func TestARemoteSessionIsNotNamedTwice(t *testing.T) {
	// ssh is marked on the host, where the mark cannot be outranked. Repeating
	// it in the activity would read `ssh › prod-01 › ssh`.
	pane := &state.PaneState{
		Dir:       "/Users/dev/work/dashboard",
		Processes: []state.Process{{Name: "ssh", Args: []string{"ssh", "prod-01"}}},
	}

	if got := paneKind(pane); got != "" {
		t.Errorf("paneKind = %q, want empty", got)
	}

	if got := defaultChain().Resolve(tabWithPane(pane)); got.Name != "ssh › prod-01" {
		t.Errorf("name = %q, want ssh › prod-01", got.Name)
	}
}

func TestStripKind(t *testing.T) {
	cases := []struct{ detail, kind, want string }{
		{"auth.provider.ts - Nvim", "nvim", "auth.provider.ts"},
		{"Nvim", "nvim", ""},
		{"nvim auth.provider.ts", "nvim", "auth.provider.ts"},
		{"lazygit - Nvim", "nvim", "lazygit"},
		{"Fix OAuth redirect", "nvim", "Fix OAuth redirect"},
		{"", "nvim", ""},
		{"anything", "", "anything"},
	}
	for _, c := range cases {
		if got := stripKind(c.detail, c.kind); got != c.want {
			t.Errorf("stripKind(%q, %q) = %q, want %q", c.detail, c.kind, got, c.want)
		}
	}
}

func TestAProjectNeverTakesAColon(t *testing.T) {
	// The rule the format rests on: a colon binds a kind to its detail, and a
	// project is a place rather than a kind.
	pane := &state.PaneState{
		Dir:           "/Users/dev/work/self-care-portal",
		TerminalTitle: "yarn dev",
		Processes:     running("esbuild", "node", "node"),
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "self-care-portal › yarn dev"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAnAgentIsItsOwnKind(t *testing.T) {
	// Herdr recognizes the agent directly. Its process list does not: a coding
	// agent shows up as a caffeinate, several nodes and an MCP helper, with its
	// own name nowhere among them.
	pane := &state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Git email configuration",
		Agent:         "claude",
		Processes:     running("caffeinate", "node", "node", "fff-mcp"),
	}

	if got := paneKind(pane); got != "claude" {
		t.Errorf("paneKind = %q, want claude", got)
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "dashboard › claude › Git email configuration"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAStartingAgentIsNamedByItsKindAlone(t *testing.T) {
	// Claude Code titles its window after itself until the conversation has a
	// subject. That is generic as an activity, and exactly right as a kind.
	pane := &state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Claude Code",
		Agent:         "claude",
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "dashboard › claude"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAPaneWithoutAnAgentIsNotNamedAfterOne(t *testing.T) {
	pane := &state.PaneState{Dir: "/Users/dev/work/dashboard", TerminalTitle: "Claude Code"}

	if got := defaultChain().Resolve(tabWithPane(pane)); got.Name != "dashboard" {
		t.Errorf("name = %q, want dashboard", got.Name)
	}
}
