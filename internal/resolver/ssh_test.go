package resolver

import (
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// sshPane builds a pane whose foreground process is the given ssh command line.
func sshPane(argv ...string) *state.PaneState {
	return &state.PaneState{
		Dir:       dashboard,
		Processes: []state.Process{{Name: "ssh", Args: argv}},
	}
}

func TestTheHostBecomesTheContext(t *testing.T) {
	t.Parallel()

	// Every destination form ssh accepts, and the flags that must not be
	// mistaken for one.
	cases := map[string]string{
		"ssh prod-01":                             "prod-01",
		"ssh root@prod-01":                        "prod-01",
		"ssh dev@production.example.com":          "production.example.com",
		"ssh -p 2222 deploy@prod-01":              "prod-01",
		"ssh -p2222 prod-01":                      "prod-01",
		"ssh -i ~/.ssh/id_ed25519 prod-01":        "prod-01",
		"ssh -L 8080:localhost:80 prod-01":        "prod-01",
		"ssh -o StrictHostKeyChecking=no prod-01": "prod-01",
		"ssh -J bastion prod-01":                  "prod-01",
		"ssh -4 -q -t prod-01":                    "prod-01",
		"ssh -tt prod-01":                         "prod-01",

		// A remote command follows the destination and must not replace it.
		"ssh root@prod-01 tail -f /var/log/syslog": "prod-01",
		"ssh prod-01 -- systemctl status":          "prod-01",

		// URL and port forms.
		"ssh ssh://deploy@prod-01:2222": "prod-01",
		"ssh ssh://prod-01/":            "prod-01",
		"ssh deploy@prod-01:2222":       "prod-01",

		// IPv6, bracketed and bare.
		"ssh root@[2001:db8::1]:22": "2001:db8::1",
		"ssh 2001:db8::1":           "2001:db8::1",
		"ssh 10.0.0.5":              "10.0.0.5",
	}

	for command, want := range cases {
		got := sshHost(strings.Fields(command))
		if got != want {
			t.Errorf("%s → %q, want %q", command, got, want)
		}
	}
}

func TestTheTabIsNamedAfterTheMarkedHost(t *testing.T) {
	t.Parallel()

	pane := sshPane("ssh", "root@prod-01")

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "ssh › prod-01"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}

	if got.Reason != "ssh" {
		t.Errorf("reason = %q, want ssh", got.Reason)
	}

	if got.Confidence != ConfidenceSSH {
		t.Errorf("confidence = %d, want %d", got.Confidence, ConfidenceSSH)
	}
}

func TestTheHostOutranksTheWorkingDirectory(t *testing.T) {
	t.Parallel()

	// The local directory of a pane running ssh describes the wrong machine.
	pane := sshPane("ssh", "prod-01")

	if got := defaultChain().Resolve(tabWithPane(pane)); got.Name != "ssh › prod-01" {
		t.Errorf("name = %q, want %q", got.Name, "ssh › prod-01")
	}
}

func TestAnUnreadableDestinationStillMarksTheTabRemote(t *testing.T) {
	t.Parallel()

	// Herdr could not read argv, or ssh was invoked with no destination at all.
	// The mark stands alone rather than letting the working directory claim the
	// context, which would name a remote tab after a local directory.
	for _, argv := range [][]string{nil, {"ssh"}, {"ssh", "-p", "2222"}, {"ssh", "-"}} {
		pane := sshPane(argv...)

		got := defaultChain().Resolve(tabWithPane(pane))
		if want := "ssh"; got.Name != want {
			t.Errorf("argv %v → %q, want %q", argv, got.Name, want)
		}
	}
}

func TestAnUnreadableDestinationKeepsTheMarkUnderARemoteTitle(t *testing.T) {
	t.Parallel()

	// The case the activity slot lost: with no host to bind the mark to it used
	// to go into the activity, where the remote shell's own title outranked it
	// and the tab read exactly like a local one.
	pane := sshPane()
	pane.TerminalTitle = "Restart the queue workers"

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "ssh › Restart the queue workers"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestTheLocalShellsCommandTitleIsNotRepeatedWhileConnecting(t *testing.T) {
	t.Parallel()

	// Until the remote shell sets a title, the local one is still showing the
	// command it ran, trimmed by fish to twenty columns, and the tab read
	// `ssh › prod-01 › ssh root@prod-01` for as long as the handshake took.
	for _, title := range []string{
		"ssh root@prod-01 ~/W/dashboard",
		"ssh deploy@productio ~/W/dashboard",
		"ssh -p 2222 prod-01",
		"SSH prod-01",
	} {
		pane := sshPane("ssh", "root@prod-01")
		pane.TerminalTitle = title

		got := defaultChain().Resolve(tabWithPane(pane))
		if want := "ssh › prod-01"; got.Name != want {
			t.Errorf("title %q → %q, want %q", title, got.Name, want)
		}
	}
}

func TestATunnelDoesNotMarkTheTabRemote(t *testing.T) {
	t.Parallel()

	for _, argv := range [][]string{
		{"ssh", "-N", "-L", "5432:db:5432", "bastion"},
		{"ssh", "-N", "-T", "-o", "BatchMode=yes", "-L", "5432:db:5432", "bastion"},
		{"ssh", "-NT", "bastion"},
		{"ssh", "-fN", "bastion"},
		{"ssh", "-p", "2222", "-N", "bastion"},
	} {
		pane := sshPane(argv...)

		got := defaultChain().Resolve(tabWithPane(pane))
		if strings.Contains(strings.ToLower(got.Name), "ssh") {
			t.Errorf("argv %v → %q, want nothing about ssh", argv, got.Name)
		}
	}
}

func TestAValueSpelledNIsNotTheTunnelSwitch(t *testing.T) {
	t.Parallel()

	for _, argv := range [][]string{
		{"ssh", "-pN", "prod-01"},
		{"ssh", "-o", "N=1", "prod-01"},
		{"ssh", "prod-01", "-N"},
		{"ssh", "prod-01", "--", "-N"},
	} {
		if sshIsTunnel(argv) {
			t.Errorf("argv %v read as a tunnel", argv)
		}
	}
}

func TestAPaneWithoutSSHIsUnaffected(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{
		Dir: dashboard,
		Processes: []state.Process{
			{Name: "fish", Args: []string{"-fish"}},
			{Name: "nvim", Args: []string{"nvim"}},
		},
	}

	got := defaultChain().Resolve(tabWithPane(pane))
	if strings.Contains(strings.ToLower(got.Name), "ssh") {
		t.Errorf("name = %q, want nothing about ssh", got.Name)
	}
}

func TestAnSSHStartedByAnotherProgramDoesNotMarkThePaneRemote(t *testing.T) {
	t.Parallel()

	// Herdr lists a pane's foreground process last, after its descendants.
	// Claude Code checks GitHub over ssh and git pushes through it, and either
	// named its tab `ssh › github.com` for as long as the connection lived.
	agent := &state.PaneState{
		Dir:   dashboard,
		Agent: "claude",
		Processes: []state.Process{
			{Name: "ssh", Args: strings.Fields("ssh -T -o BatchMode=yes git@github.com")},
			{Name: "claude", Args: []string{"claude"}},
		},
	}
	push := &state.PaneState{
		Dir: dashboard,
		Processes: []state.Process{
			{Name: "ssh", Args: strings.Fields("ssh git@github.com git-receive-pack")},
			{Name: "git", Args: strings.Fields("git push")},
		},
	}

	for _, pane := range []*state.PaneState{agent, push} {
		got := defaultChain().Resolve(tabWithPane(pane))
		if strings.Contains(got.Name, "github.com") {
			t.Errorf("name = %q, want nothing about github.com", got.Name)
		}
	}
}

func TestAJumpHostIsNotTheDestination(t *testing.T) {
	t.Parallel()

	// ProxyJump runs a second ssh to the bastion as a child of the first.
	pane := &state.PaneState{
		Dir: dashboard,
		Processes: []state.Process{
			{Name: "ssh", Args: strings.Fields("ssh -W [prod-01]:22 bastion")},
			{Name: "ssh", Args: strings.Fields("ssh -J bastion prod-01")},
		},
	}

	if got := defaultChain().Resolve(tabWithPane(pane)); got.Name != "ssh › prod-01" {
		t.Errorf("name = %q, want %q", got.Name, "ssh › prod-01")
	}
}

func TestTheMarkSurvivesARemoteTitle(t *testing.T) {
	t.Parallel()

	// The reason the mark is on the host: a remote shell's title outranks
	// anything this source could put in the activity slot, and a tab must not
	// stop saying it is remote at the moment it has most to say.
	pane := sshPane("ssh", "prod-01")
	pane.TerminalTitle = "Restart the queue workers"

	got := defaultChain().Resolve(tabWithPane(pane))
	if want := "ssh › prod-01 › Restart the queue workers"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestHostsFromArgvAreSanitized(t *testing.T) {
	t.Parallel()

	// argv is terminal-derived input like any other.
	pane := sshPane("ssh", "root@prod\x1b[31m-01")

	got := defaultChain().Resolve(tabWithPane(pane))
	if strings.ContainsRune(got.Name, '\x1b') {
		t.Errorf("name = %q, still carries an escape", got.Name)
	}

	if want := "ssh › prod-01"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}
