package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/files/filestest"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
)

const (
	remoteFirst  = "01a070db-95a3-70b3-bd9d-a9ee3186a65b"
	remoteSecond = "01a070db-93d3-7b51-830c-889411bdc194"
)

// remoteTestClient answers machine.exec from this computer's files, as the
// helper would on a Linux machine whose /proc the test supplies.
type remoteTestClient struct {
	*herdrtest.Client

	machine *filestest.Machine
}

// Snapshot puts back the machine the stub's wire round trip drops, as the
// desktop client sets it from the space a pane is in.
func (c *remoteTestClient) Snapshot(ctx context.Context) (herdr.Snapshot, error) {
	snapshot, err := c.Client.Snapshot(ctx)
	for i, pane := range snapshot.Panes {
		if machine, _, remote := strings.Cut(pane.PaneID, "/"); remote {
			snapshot.Panes[i].Machine = machine
		}
	}

	return snapshot, err
}

func (c *remoteTestClient) ExecMachine(
	ctx context.Context, _ string, argv []string, input []byte,
) ([]byte, error) {
	return c.machine.Exec(ctx, argv, input)
}

// holdSession makes the remote Codex at pid hold sessionID's rollout alone.
func (c *remoteTestClient) holdSession(t *testing.T, home string, pid int, sessionID string) {
	t.Helper()

	rollouts, _ := filepath.Glob(
		filepath.Join(home, "sessions", "*", "*", "*", "rollout-*"+sessionID+".jsonl"),
	)
	for i, rollout := range rollouts {
		rollouts[i] = filestest.Remote(rollout)
	}

	c.machine.SetProc(pid, filestest.Proc{Argv: []string{"codex"}, Open: rollouts})
	c.SetProcesses("box/p1", herdr.PaneProcessInfoProcess{
		PID: pid, Name: "codex", Argv: []string{"codex"}, CWD: dashboard,
	})
}

func remoteCodexHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()

	var index strings.Builder

	for i, session := range []struct{ id, name string }{
		{remoteFirst, "Remote first"}, {remoteSecond, "Resumed conversation"},
	} {
		dir := filepath.Join(home, "sessions", "2026", "10", "19")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		rollout := filepath.Join(
			dir,
			"rollout-2026-10-19T09-1"+strconv.Itoa(i)+"-00-"+session.id+".jsonl",
		)
		if err := os.WriteFile(
			rollout,
			[]byte(`{"type":"session_meta","payload":{}}`+"\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}

		index.WriteString(
			`{"id":"` + session.id + `","thread_name":"` + session.name + `"}` + "\n",
		)
	}

	if err := os.WriteFile(
		filepath.Join(home, "session_index.jsonl"),
		[]byte(index.String()),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	return home
}

// pollUntil polls until the remote tab's label holds want: a remote read takes a few
// polls to fetch the files it walks.
func pollUntil(t *testing.T, application *App, client herdr.Client, want string) string {
	t.Helper()

	label := ""

	for range 10 {
		application.poll(t.Context(), client)

		if label = remoteLabel(t, client, "box/0"); strings.Contains(label, want) {
			return label
		}
	}

	return label
}

func remoteLoop(t *testing.T) (*App, *remoteTestClient, Config, herdr.PaneInfo, string) {
	t.Helper()

	cfg := testConfig()
	cfg.ReadSessions = true
	cfg.ManualPath = filepath.Join(t.TempDir(), "manual.json")
	local := herdr.PaneInfo{
		PaneID: "p1", TabID: "0", CWD: dashboard, Agent: "codex",
		TerminalTitleStripped: "Local first | project", Revision: 1,
	}
	remote := herdr.PaneInfo{
		PaneID:   "box/p1",
		TabID:    "box/0",
		CWD:      dashboard,
		Agent:    "codex",
		Revision: 1,
	}

	client := &remoteTestClient{
		Client: herdrtest.New(
			[]herdr.TabInfo{
				{TabID: "0", WorkspaceID: "s1", Label: "1"},
				{TabID: "box/0", WorkspaceID: "box/s1", Label: "1"},
			},
			[]herdr.PaneInfo{local, remote},
		),
		machine: filestest.New(),
	}
	client.SetProcesses("p1", herdr.PaneProcessInfoProcess{
		PID: 123, Name: "codex", Argv: []string{"codex"}, CWD: dashboard,
	})

	home := remoteCodexHome(t)
	client.holdSession(t, home, 123, remoteFirst)

	return newTestApp(t, cfg), client, cfg, remote, home
}

func remoteLabel(t *testing.T, client herdr.Client, tabID string) string {
	t.Helper()

	snapshot, err := client.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	for _, tab := range snapshot.Tabs {
		if tab.TabID == tabID {
			return tab.Label
		}
	}

	t.Fatalf("missing tab %s", tabID)

	return ""
}

func TestRemoteNativeNamesUpdateWithoutAnOSCTitle(t *testing.T) {
	t.Parallel()

	application, client, _, remote, home := remoteLoop(t)

	if label := pollUntil(
		t,
		application,
		client,
		"Remote first",
	); !strings.Contains(
		label,
		"Remote first",
	) {
		t.Fatalf("first remote title = %q", label)
	}

	// A new Codex process holding another session is what /resume leaves, and
	// the terminal redraws for it.
	client.holdSession(t, home, 456, remoteSecond)

	remote.Revision++
	client.SetPane(remote)

	if label := pollUntil(
		t,
		application,
		client,
		"Resumed",
	); !strings.Contains(
		label,
		"Resumed conversation",
	) {
		t.Errorf("resumed title = %q", label)
	}
}

// A restart finds remote tabs already carrying the last instance's names, and a
// read that takes a few polls to fetch must not turn them into the user's.
func TestRemoteTabsNamedBeforeAStartAreNotClaimed(t *testing.T) {
	t.Parallel()

	application, client, _, _, _ := remoteLoop(t)
	client.SetTab(herdr.TabInfo{TabID: "box/0", WorkspaceID: "box/s1", Label: "dashboard › codex"})

	if label := pollUntil(
		t,
		application,
		client,
		"Remote first",
	); !strings.Contains(
		label,
		"Remote first",
	) {
		t.Errorf("a remote tab named before the start was claimed: %q", label)
	}
}

func TestRemoteManualNamesSurviveDisconnectAndRestart(t *testing.T) {
	t.Parallel()

	application, client, cfg, remote, home := remoteLoop(t)
	pollUntil(t, application, client, "Remote first")
	client.SetTab(herdr.TabInfo{TabID: "box/0", WorkspaceID: "box/s1", Label: "Manual remote"})
	application.poll(t.Context(), client)
	client.ClosePane("box/p1")
	client.SetPane(herdr.PaneInfo{
		PaneID: "p1", TabID: "0", CWD: dashboard, Agent: "codex",
		TerminalTitleStripped: "Local second | project", Revision: 2,
	})

	application = newTestApp(t, cfg)
	application.poll(t.Context(), client)

	if label := remoteLabel(t, client, "0"); !strings.Contains(label, "Local second") {
		t.Errorf("a disconnected remote blocked the local update: %q", label)
	}

	client.SetPane(remote)
	client.holdSession(t, home, 456, remoteSecond)

	for range 5 {
		application.poll(t.Context(), client)
	}

	if label := remoteLabel(t, client, "box/0"); label != "Manual remote" {
		t.Errorf("a disconnect lost the manual lock: %q", label)
	}

	client.SetTab(herdr.TabInfo{TabID: "box/0", WorkspaceID: "box/s1", Label: ""})

	if label := pollUntil(
		t,
		application,
		client,
		"Resumed",
	); !strings.Contains(
		label,
		"Resumed conversation",
	) {
		t.Errorf("clearing a remote name did not resume automatic titles: %q", label)
	}
}

func TestAStalledRemoteReadStillCompletesTheLocalPoll(t *testing.T) {
	t.Parallel()

	application, client, _, _, _ := remoteLoop(t)

	// The first poll only finds out what to ask; the second asks it.
	application.poll(t.Context(), client)
	client.machine.SetStalled(true)

	if !application.poll(t.Context(), client) || client.machine.Calls() == 0 {
		t.Error("a remote helper exhausted the whole poll deadline")
	}

	if label := remoteLabel(t, client, "0"); !strings.Contains(label, "Local first") {
		t.Errorf("local title = %q", label)
	}
}
