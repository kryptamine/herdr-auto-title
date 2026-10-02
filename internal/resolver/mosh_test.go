package resolver

import (
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// moshPane builds a pane running the mosh-client the `mosh` wrapper execs for
// the given wrapper arguments, with argv shaped as the wrapper passes it.
func moshPane(wrapperArgs string) *state.PaneState {
	return &state.PaneState{
		Dir: dashboard,
		Processes: []state.Process{
			{Name: "zsh", Args: []string{"-zsh"}},
			{Name: "mosh-client", Args: []string{
				"/opt/homebrew/bin/mosh-client",
				"-# " + wrapperArgs + " |",
				"203.0.113.5",
				"60001",
			}},
		},
	}
}

func TestTheMoshHostBecomesTheContext(t *testing.T) {
	t.Parallel()

	// What the wrapper received, which it hands on joined after `-#`.
	cases := map[string]string{
		"devbox":                                     "devbox",
		"user@devbox":                                "devbox",
		"-p 60001 devbox":                            "devbox",
		"--port=60001 devbox":                        "devbox",
		"--port 60000:60010 devbox":                  "devbox",
		"-a -n -4 -6 -o devbox":                      "devbox",
		"--predict always --no-init devbox":          "devbox",
		"--server=/usr/local/bin/mosh-server devbox": "devbox",
		"--ssh=ssh -p 2222 deploy@prod-01":           "prod-01",
		"--ssh=ssh -i ~/.ssh/id_ed25519 prod-01":     "prod-01",
		"--ssh ssh -o ProxyJump=bastion prod-01":     "prod-01",
		"--ssh=ssh --family=inet prod-01":            "prod-01",
		"-- prod-01":                                 "prod-01",

		// Getopt::Long takes a long name after one dash, and any unique prefix.
		"-port 60001 devbox":             "devbox",
		"-predict always devbox":         "devbox",
		"--p 60001 devbox":               "devbox",
		"--por 60001 devbox":             "devbox",
		"--serv /opt/mosh-server devbox": "devbox",
		"-ssh ssh -p 2222 prod-01":       "prod-01",
		"--noinit devbox":                "devbox",

		// A remote command follows the host and must not replace it.
		"devbox tmux attach":               "devbox",
		"devbox -- tmux new -A -s main":    "devbox",
		"devbox -- sh -c uptime | tail -1": "devbox",

		// Address forms.
		"root@[2001:db8::1]": "2001:db8::1",
		"192.0.2.10":         "192.0.2.10",
	}

	for wrapperArgs, want := range cases {
		pane := moshPane(wrapperArgs)
		process, _ := pane.Foreground()

		if got := moshHost(process.Args); got != want {
			t.Errorf("mosh %s → %q, want %q", wrapperArgs, got, want)
		}
	}
}

func TestTheTabIsNamedAfterTheMoshHost(t *testing.T) {
	t.Parallel()

	got := defaultChain().Resolve(tabWithPane(moshPane("user@devbox")))
	if want := "mosh › devbox"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "remote" {
		t.Errorf("reason = %q, want remote", got.Reason)
	}

	if got.Confidence != ConfidenceRemote {
		t.Errorf("confidence = %d, want %d", got.Confidence, ConfidenceRemote)
	}
}

func TestTheMoshTitlePrefixIsNotRepeated(t *testing.T) {
	t.Parallel()

	// mosh-client marks the remote title `[mosh]`, which the context already
	// says; the tab read `mosh-client › [mosh] user@devbox - byobu`.
	pane := moshPane("devbox")
	pane.TerminalTitle = "[mosh] user@devbox (192.0.2.10) - byobu"

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "mosh › devbox › user@devbox (192.0.2.10) - byobu"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestTheMoshPrefixAloneIsNoActivity(t *testing.T) {
	t.Parallel()

	for _, title := range []string{"[mosh]", "[mosh] ", "mosh devbox", "mosh -p 60001 user@dev"} {
		pane := moshPane("devbox")
		pane.TerminalTitle = title

		got := defaultChain().Resolve(tabWithPane(pane))
		if want := "mosh › devbox"; got.Name != want {
			t.Errorf("title %q → %q, want %q", title, got.Name, want)
		}
	}
}

func TestAnUnreadableMoshHostStillMarksTheTabRemote(t *testing.T) {
	t.Parallel()

	// mosh-client run by hand, or argv Herdr could not read.
	for _, argv := range [][]string{
		nil,
		{"mosh-client"},
		{"mosh-client", "203.0.113.5", "60001"},
		{"mosh-client", "-# --port=60001 |", "203.0.113.5", "60001"},
	} {
		pane := &state.PaneState{
			Dir:       dashboard,
			Processes: []state.Process{{Name: "mosh-client", Args: argv}},
		}

		if got := defaultChain().Resolve(tabWithPane(pane)); got.Name != "mosh" {
			t.Errorf("argv %v → %q, want mosh", argv, got.Name)
		}
	}
}

func TestAMoshPaneShowsNoLocalBranch(t *testing.T) {
	t.Parallel()

	// The branch belongs to the directory mosh was started in, not the host.
	pane := repoPane("feat/oauth", "main")
	pane.Processes = moshPane("devbox").Processes

	if got := resolveRepoPane(pane); got != "mosh › devbox" {
		t.Errorf("title %q, want no branch beside the host", got)
	}
}

func TestAMoshSessionsHostLeadsTheTopic(t *testing.T) {
	t.Parallel()

	pane := moshPane("devbox")
	pane.TerminalTitle = "[mosh] Restart the queue workers"

	if got, want := topics().Topic(pane), "mosh › devbox › Restart the queue workers"; got != want {
		t.Errorf("topic = %q, want %q", got, want)
	}
}

func TestTheMoshTitlePrefixIsKeptWithoutMosh(t *testing.T) {
	t.Parallel()

	// Only mosh-client's own prefix is noise; a local title is left as it is.
	pane := &state.PaneState{Dir: dashboard, TerminalTitle: "[mosh] notes"}

	if got := defaultChain().Resolve(tabWithPane(pane)); !strings.HasSuffix(
		got.Name,
		"[mosh] notes",
	) {
		t.Errorf("name = %q, want the title unchanged", got.Name)
	}
}
