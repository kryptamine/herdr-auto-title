package desktop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func fixtureJSON(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func connectedRemoteClient(t *testing.T) *Client {
	t.Helper()

	machine := "box"
	ls := fixtureJSON(t, world{Spaces: []space{
		{ID: "s1", Name: "project", Tabs: []tab{{
			ID: "tb1", Name: "1", Active: true,
			Terminals: []paneEntry{{Terminal: "t1", Pane: "p1"}},
		}}},
		{ID: "box/s1", Name: "project", Machine: &machine, Tabs: []tab{{
			ID: "box/tb1", Name: "1", Active: true,
			Terminals: []paneEntry{{
				Terminal: "box/t1", Pane: "box/p1", App: "codex", Status: "working",
			}},
		}}},
	}})
	app := serve(t, func(request map[string]any) string {
		if request["ls"] != nil {
			return ls
		}

		return "{\"focused_pane\":\"box/p1\"}"
	})
	reply := fixtureJSON(t, map[string]any{"result": map[string]any{
		"terminals": []terminal{
			{
				Address: "t1", PID: 100, CWD: "/local", Title: "local editor",
				Foreground: herdr.PaneProcessInfoProcess{PID: 101, Name: "nvim", CWD: "/local"},
			},
			{
				Address: "box/t1", PID: 100, CWD: "/remote", Title: "remote session | project",
				Foreground: herdr.PaneProcessInfoProcess{PID: 101, Name: "codex", CWD: "/remote"},
			},
		},
	}})
	core := serve(t, func(map[string]any) string { return reply })

	return &Client{app: app.path, core: core.path, sessions: newSessionFinder(nil)}
}

func TestSnapshotIncludesConnectedRemoteMachines(t *testing.T) {
	t.Parallel()

	client := connectedRemoteClient(t)

	snapshot := readSnapshot(t, client)
	if len(snapshot.Tabs) != 2 || len(snapshot.Workspaces) != 2 {
		t.Fatalf(
			"snapshot has %d tabs and %d spaces, want local and remote",
			len(snapshot.Tabs),
			len(snapshot.Workspaces),
		)
	}

	remote := paneByID(t, snapshot, "box/p1")
	if remote.TabID != "box/tb1" || remote.CWD != "/remote" ||
		remote.TerminalTitleStripped != "remote session | project" || !remote.Focused {
		t.Errorf("remote pane = %+v", remote)
	}

	processes, err := client.PaneProcesses(t.Context(), "box/p1")
	if err != nil || len(processes) != 1 || processes[0].Name != "codex" {
		t.Errorf("remote processes = %+v, %v", processes, err)
	}

	local := paneByID(t, snapshot, "p1")
	if local.CWD != "/local" || local.Focused {
		t.Errorf("remote focus or directory leaked into local pane: %+v", local)
	}

	if client.PaneMachine(remote.PaneID) != "box" || client.PaneMachine(local.PaneID) != "" {
		t.Error("panes were not assigned to their own filesystem and PID namespace")
	}
}

func TestARemoteAgentNeverUsesTheLocalSessionFinder(t *testing.T) {
	t.Parallel()

	machine := "box"
	ls := fixtureJSON(t, world{Spaces: []space{{
		ID: "box/s", Machine: &machine, Tabs: []tab{{
			ID: "box/tb", Name: "1", Terminals: []paneEntry{{
				Terminal: "box/t", Pane: "box/p", App: "claude-code",
			}},
		}},
	}}})
	app := serve(t, func(request map[string]any) string {
		if request["ls"] != nil {
			return ls
		}

		return "{}"
	})
	reply := fixtureJSON(t, map[string]any{"result": map[string]any{
		"terminals": []terminal{{Address: "box/t", PID: 100}},
	}})
	core := serve(t, func(map[string]any) string { return reply })
	client := &Client{app: app.path, core: core.path, sessions: newSessionFinder(nil)}
	client.sessions.processes = func(_ context.Context) (map[int][]int, error) {
		t.Error("a remote PID reached the local process table")
		return map[int][]int{}, nil
	}

	pane := paneByID(t, readSnapshot(t, client), "box/p")
	if pane.AgentSession != nil {
		t.Errorf("remote pane got a local session: %+v", pane.AgentSession)
	}
}

func TestRemoteJoinNeverFallsBackToALocalAddress(t *testing.T) {
	t.Parallel()

	machine := "box"
	world := world{Spaces: []space{{Machine: &machine, Tabs: []tab{{
		Terminals: []paneEntry{
			{Terminal: "t1", Pane: "box/p1"},
			{Terminal: "other/t1", Pane: "box/p2"},
			{Terminal: "box/t1", Pane: "box/p3"},
		},
	}}}}}

	joined := world.join([]terminal{{Address: "t1"}, {Address: "other/t1"}, {Address: "box/t1"}})

	panes := joined[0].Tabs[0].panes
	if len(panes) != 1 || panes[0].entry.Pane != "box/p3" {
		t.Fatalf("remote panes used another machine's terminal: %+v", panes)
	}

	joined = world.join([]terminal{{Address: "t1"}})
	if len(joined[0].Tabs[0].panes) != 0 {
		t.Error("a second join kept a remote terminal that is no longer listed")
	}
}
