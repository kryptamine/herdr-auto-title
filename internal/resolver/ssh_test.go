package resolver

import (
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// sshPane builds a pane running the given ssh command line, beside the shell
// that started it — which is how Herdr reports a pane's processes.
// paneCWD is the directory every ssh pane in these tests sits in.
const paneCWD = "/Users/dev/work/dashboard"

func sshPane(argv ...string) *state.PaneState {
	return &state.PaneState{
		Dir: paneCWD,
		Processes: []state.Process{
			{Name: "fish", Args: []string{"-fish"}},
			{Name: "ssh", Args: argv},
		},
	}
}

func TestTheHostBecomesTheContext(t *testing.T) {
	// Every form the ticket lists, and the flags that must not be mistaken for
	// a destination.
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
	pane := sshPane("ssh", "root@prod-01")

	got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(pane))
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
	// The local directory of a pane running ssh describes the wrong machine.
	pane := sshPane("ssh", "prod-01")

	if got := Default(
		DefaultMaxLength,
		DefaultBranchMaxLength,
	).Resolve(tabWithPane(pane)); got.Name != "ssh › prod-01" {
		t.Errorf("name = %q, want %q", got.Name, "ssh › prod-01")
	}
}

func TestAnUnreadableDestinationStillMarksTheTabRemote(t *testing.T) {
	// Herdr could not read argv, or ssh was invoked with no destination at all.
	// The mark stands alone rather than letting the working directory claim the
	// context, which would name a remote tab after a local directory.
	for _, argv := range [][]string{nil, {"ssh"}, {"ssh", "-p", "2222"}, {"ssh", "-"}} {
		pane := sshPane(argv...)

		got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(pane))
		if want := "ssh"; got.Name != want {
			t.Errorf("argv %v → %q, want %q", argv, got.Name, want)
		}
	}
}

func TestAnUnreadableDestinationKeepsTheMarkUnderARemoteTitle(t *testing.T) {
	// The case the activity slot lost: with no host to bind the mark to it used
	// to go into the activity, where the remote shell's own title outranked it
	// and the tab read exactly like a local one.
	pane := sshPane()
	pane.TerminalTitle = "Restart the queue workers"

	got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(pane))
	if want := "ssh › Restart the queue workers"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestATunnelDoesNotMarkTheTabRemote(t *testing.T) {
	// `ssh -N` forwards ports and runs no remote shell: an MCP server or a
	// database client behind an agent keeps one open, and the pane's work is
	// still local. Alone, clustered, or beside options that take a value.
	for _, argv := range [][]string{
		{"ssh", "-N", "-L", "5432:db:5432", "bastion"},
		{"ssh", "-N", "-T", "-o", "BatchMode=yes", "-L", "5432:db:5432", "bastion"},
		{"ssh", "-NT", "bastion"},
		{"ssh", "-fN", "bastion"},
		{"ssh", "-p", "2222", "-N", "bastion"},
	} {
		pane := sshPane(argv...)

		got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(pane))
		if strings.Contains(strings.ToLower(got.Name), "ssh") {
			t.Errorf("argv %v → %q, want nothing about ssh", argv, got.Name)
		}
	}
}

func TestAValueSpelledNIsNotTheTunnelSwitch(t *testing.T) {
	// -pN reads N as the port and -o N=... as an option, and past the
	// destination -N is part of the remote command; none is the switch.
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
	pane := &state.PaneState{
		Dir: "/Users/dev/work/dashboard",
		Processes: []state.Process{
			{Name: "fish", Args: []string{"-fish"}},
			{Name: "nvim", Args: []string{"nvim"}},
		},
	}

	got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(pane))
	if strings.Contains(strings.ToLower(got.Name), "ssh") {
		t.Errorf("name = %q, want nothing about ssh", got.Name)
	}
}

func TestSSHIsFoundAmongOtherProcesses(t *testing.T) {
	// Herdr lists the foreground process and its descendants, so ssh can be
	// anywhere in the list.
	pane := &state.PaneState{
		Dir: "/Users/dev/work/dashboard",
		Processes: []state.Process{
			{Name: "fish", Args: []string{"-fish"}},
			{Name: "ssh", Args: []string{"ssh", "prod-01"}},
			{Name: "tail", Args: []string{"tail", "-f", "/var/log/syslog"}},
		},
	}

	if got := Default(
		DefaultMaxLength,
		DefaultBranchMaxLength,
	).Resolve(tabWithPane(pane)); got.Name != "ssh › prod-01" {
		t.Errorf("name = %q, want %q", got.Name, "ssh › prod-01")
	}
}

func TestTheMarkSurvivesARemoteTitle(t *testing.T) {
	// The reason the mark is on the host: a remote shell's title outranks
	// anything this source could put in the activity slot, and a tab must not
	// stop saying it is remote at the moment it has most to say.
	pane := sshPane("ssh", "prod-01")
	pane.TerminalTitle = "Restart the queue workers"

	got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(pane))
	if want := "ssh › prod-01 › Restart the queue workers"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestHostsFromArgvAreSanitized(t *testing.T) {
	// argv is terminal-derived input like any other.
	pane := sshPane("ssh", "root@prod\x1b[31m-01")

	got := Default(DefaultMaxLength, DefaultBranchMaxLength).Resolve(tabWithPane(pane))
	if strings.ContainsRune(got.Name, '\x1b') {
		t.Errorf("name = %q, still carries an escape", got.Name)
	}

	if want := "ssh › prod-01"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}
