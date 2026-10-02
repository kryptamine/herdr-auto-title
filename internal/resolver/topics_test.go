package resolver

import (
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func topics() *Topics {
	return NewTopics(Options{BranchMax: DefaultBranchMaxLength}, 0)
}

// The agent's name follows the setting the tab bar obeys, so a user who hid it
// there does not find it on the row.
func TestTheTopicIsWhatTheAgentIsDoing(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{Dir: api, Agent: "claude", AgentTitle: "Fix login"}

	if got, want := topics().Topic(pane), "claude › Fix login"; got != want {
		t.Errorf("topic = %q, want %q", got, want)
	}

	hidden := NewTopics(Options{BranchMax: DefaultBranchMaxLength, HideAgentName: true}, 0)
	if got, want := hidden.Topic(pane), "Fix login"; got != want {
		t.Errorf("with the name hidden: topic = %q, want %q", got, want)
	}
}

// An empty topic clears the row's token, so anything a tab would fall back to
// must come out empty here rather than as `Shell` or a bare agent's name.
func TestATopicSaysNothingRatherThanShell(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		pane *state.PaneState
	}{
		{name: "no pane", pane: nil},
		{name: "a pane with nothing to read", pane: &state.PaneState{}},
		{
			name: "a bare shell",
			pane: &state.PaneState{Dir: api, Processes: []state.Process{{Name: "zsh", Args: []string{"zsh"}}}},
		},
		{name: "a silent agent", pane: &state.PaneState{Dir: api, Agent: "claude"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := topics().Topic(test.pane); got != "" {
				t.Errorf("topic = %q, want empty", got)
			}
		})
	}
}

// The label names the project and Herdr draws the branch beside it, so a topic
// that said either would say it twice.
func TestATopicCarriesNeitherTheDirectoryNorTheBranch(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{
		Dir:           dashboard,
		Git:           git.Checkout{Branch: "feat/oauth", Default: "main"},
		TerminalTitle: "Tests",
	}

	if got, want := topics().Topic(pane), "Tests"; got != want {
		t.Errorf("topic = %q, want %q", got, want)
	}
}

func TestAnSSHSessionsHostLeadsTheTopic(t *testing.T) {
	t.Parallel()

	got := topics().Topic(sshPane("ssh", "root@prod-01"))
	if want := "ssh › prod-01"; got != want {
		t.Errorf("topic = %q, want %q", got, want)
	}
}

// Unset, the bound leaves the fitting to Herdr; set, it cuts in columns and
// leaves no separator dangling where it cut.
func TestATopicIsBoundOnlyWhenABoundIsSet(t *testing.T) {
	t.Parallel()

	activity := "Reconcile the ledger against the bank statement for every account"
	pane := &state.PaneState{Dir: api, Agent: "claude", AgentTitle: activity}

	for _, test := range []struct {
		maxLen int
		want   string
	}{
		{maxLen: 0, want: "claude › " + activity},
		{maxLen: 20, want: "claude › Reconcile t"},
		{maxLen: 8, want: "claude"},
	} {
		bounded := NewTopics(Options{BranchMax: DefaultBranchMaxLength}, test.maxLen)
		if got := bounded.Topic(pane); got != test.want {
			t.Errorf("bound %d: topic = %q, want %q", test.maxLen, got, test.want)
		}
	}
}

// A topic's chain is the shipped one minus one source. Each of the rest is checked by
// giving a pane only what that source reads, and the process is checked by
// giving it one and finding it unused.
func TestTheTopicChainKeepsEverySourceButTheProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pane *state.PaneState
		want string
	}{
		{
			name: "cwd",
			pane: &state.PaneState{Dir: api},
			want: "cwd",
		},
		{
			name: "git",
			pane: &state.PaneState{
				Dir: api,
				Git: git.Checkout{Branch: "feat/oauth", Default: "main"},
			},
			want: "git",
		},
		{
			name: "terminal_title",
			pane: &state.PaneState{Dir: api, TerminalTitle: "Fix invoice totals"},
			want: "terminal_title",
		},
		{
			name: "transcript",
			pane: &state.PaneState{Dir: api, Agent: "claude", AgentTopic: "Fix invoice totals"},
			want: "transcript",
		},
		{
			name: "agent",
			pane: &state.PaneState{Dir: api, Agent: "claude", AgentTitle: "Fix invoice totals"},
			want: "agent",
		},
		{
			name: "remote",
			pane: &state.PaneState{
				Dir:       api,
				Processes: []state.Process{{Name: "ssh", Args: []string{"ssh", "prod-01"}}},
			},
			want: "remote",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := topicChain(
				Options{BranchMax: DefaultBranchMaxLength},
			).Resolve(tabWithPane(test.pane))
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
				Dir:       api,
				Processes: []state.Process{{Name: "npm", Args: []string{"npm", "run", "build"}}},
			},
		},
		{
			name: "not the process behind a terminal title",
			pane: &state.PaneState{
				Dir:           api,
				TerminalTitle: "auth.ts",
				Processes:     []state.Process{{Name: "npm", Args: []string{"npm", "run", "build"}}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := topics().Topic(test.pane); strings.Contains(got, "npm") {
				t.Errorf("the topic took the foreground process: %q", got)
			}
		})
	}
}

// A terminal title binds the pane's kind to a tab's title, `nvim › auth.ts`.
// The topic reads the same title without the kind: a row that followed the
// process would rewrite itself at every prompt.
func TestTheTopicDoesNotCarryTheForegroundProcessThroughTheTerminalTitle(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{
		Dir:           herdrtest.Dir("work", "dashboard"),
		TerminalTitle: "auth.ts",
		Processes:     []state.Process{{Name: "nvim", Args: []string{"nvim", "auth.ts"}}},
	}

	first := topics().Topic(pane)
	if first != "auth.ts" {
		t.Errorf("topic = %q, want auth.ts", first)
	}

	pane.Processes = []state.Process{{Name: "less", Args: []string{"less", "auth.ts"}}}

	if second := topics().Topic(pane); first != second {
		t.Errorf("the topic followed the process: %q then %q", first, second)
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
