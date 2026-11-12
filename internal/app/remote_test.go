package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
)

type remoteTestClient struct {
	*herdrtest.Client

	topics map[string]string
	calls  int
	stall  bool
}

func (*remoteTestClient) PaneMachine(paneID string) string {
	if strings.HasPrefix(paneID, "box/") {
		return "box"
	}

	return ""
}

func (c *remoteTestClient) ExecMachine(
	ctx context.Context, _ string, _ []string, _ []byte,
) ([]byte, error) {
	c.calls++
	if c.stall {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	replies := make(map[string]map[string]string)
	for pane, topic := range c.topics {
		replies[pane] = map[string]string{"topic": topic}
	}

	return json.Marshal(replies)
}

func remoteLoop(t *testing.T) (*App, *remoteTestClient, Config, herdr.PaneInfo) {
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
		topics: map[string]string{"box/p1": "Remote first"},
	}
	for _, pane := range []herdr.PaneInfo{local, remote} {
		client.SetProcesses(pane.PaneID, herdr.PaneProcessInfoProcess{
			PID: 123, Name: "codex", Argv: []string{"codex"}, CWD: dashboard,
		})
	}

	return newTestApp(t, cfg), client, cfg, remote
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

	application, client, _, _ := remoteLoop(t)
	application.poll(t.Context(), client)

	if label := remoteLabel(t, client, "box/0"); !strings.Contains(label, "Remote first") {
		t.Fatalf("first remote title = %q", label)
	}

	client.topics["box/p1"] = "Resumed conversation"
	application.poll(t.Context(), client)

	if label := remoteLabel(t, client, "box/0"); !strings.Contains(label, "Resumed conversation") {
		t.Errorf("resumed title = %q", label)
	}

	if client.calls != 2 {
		t.Errorf("remote batches = %d, want one per poll", client.calls)
	}
}

func TestRemoteManualNamesSurviveDisconnectAndRestart(t *testing.T) {
	t.Parallel()

	application, client, cfg, remote := remoteLoop(t)
	application.poll(t.Context(), client)
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
	client.topics["box/p1"] = "Remote changed"
	application.poll(t.Context(), client)

	if label := remoteLabel(t, client, "box/0"); label != "Manual remote" {
		t.Errorf("a disconnect lost the manual lock: %q", label)
	}

	client.SetTab(herdr.TabInfo{TabID: "box/0", WorkspaceID: "box/s1", Label: ""})
	application.poll(t.Context(), client)

	if label := remoteLabel(t, client, "box/0"); !strings.Contains(label, "Remote changed") {
		t.Errorf("clearing a remote name did not resume automatic titles: %q", label)
	}
}

func TestAStalledRemoteReadStillCompletesTheLocalPoll(t *testing.T) {
	t.Parallel()

	application, client, _, _ := remoteLoop(t)

	client.stall = true
	if !application.poll(t.Context(), client) {
		t.Error("a remote helper exhausted the whole poll deadline")
	}

	if label := remoteLabel(t, client, "0"); !strings.Contains(label, "Local first") {
		t.Errorf("local title = %q", label)
	}
}
