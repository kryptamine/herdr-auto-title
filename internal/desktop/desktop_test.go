//go:build !windows

package desktop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// fakeSocket answers each line it reads with what reply returns for it.
type fakeSocket struct {
	path string

	mu   sync.Mutex
	seen []map[string]any
}

func serve(t *testing.T, reply func(map[string]any) string) *fakeSocket {
	t.Helper()

	//nolint:usetesting // t.TempDir() overruns the 104 bytes of a sun_path
	dir, err := os.MkdirTemp("", "dt")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	socket := &fakeSocket{path: filepath.Join(dir, "s.sock")}

	var config net.ListenConfig

	listener, err := config.Listen(context.Background(), "unix", socket.path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			line, _ := bufio.NewReader(conn).ReadBytes('\n')

			var request map[string]any

			_ = json.Unmarshal(line, &request)

			socket.mu.Lock()
			socket.seen = append(socket.seen, request)
			socket.mu.Unlock()

			// One line each way: the fixtures below are spread out to be read.
			var answer bytes.Buffer
			if err := json.Compact(&answer, []byte(reply(request))); err != nil {
				t.Errorf("fixture: %v", err)
			}

			answer.WriteByte('\n')

			_, _ = conn.Write(answer.Bytes())
			_ = conn.Close()
		}
	}()

	return socket
}

func (s *fakeSocket) last() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.seen[len(s.seen)-1]
}

const fakeWorld = `{"spaces":[
 {"id":"s1","name":"eris","machine":null,"tabs":[
  {"id":"tb1","name":"1","active":false,"terminals":[
   {"terminal":"t1","pane":"p1","status":null,"app":null}]},
  {"id":"tb2","name":"Claude Code","active":true,"terminals":[
   {"terminal":"t2","pane":"p2","status":"working","app":"claude-code"},
   {"terminal":"t3","pane":"p3","status":"blocked","app":"grok-build"}]},
  {"id":"tb3","name":"Codex","active":false,"terminals":[
   {"terminal":"t4","pane":"p4","status":null,"app":null}]}]},
 {"id":"s2","name":"box","machine":"box","tabs":[
  {"id":"box/tb9","name":"1","active":true,"terminals":[
   {"terminal":"box/t9","pane":"box/p9","status":null,"app":null}]}]}]}`

const fakeTerminals = `{"jsonrpc":"2.0","id":1,"result":{"terminals":[
 {"address":"t1","cwd":"/home","title":"~/home","turn":0,
  "foreground":{"pid":10,"name":"fish","argv":["fish"],"cwd":"/home/x"}},
 {"address":"t2","cwd":"/repo","title":"⠇ ⠇ | fix login","turn":1,"foreground":null},
 {"address":"t3","cwd":"/repo","title":"","turn":0,"foreground":null},
 {"address":"t4","cwd":"/repo","title":"","turn":0,"foreground":null}]}}`

func fakeClient(t *testing.T) (*Client, *fakeSocket) {
	t.Helper()

	app := serve(t, func(request map[string]any) string {
		switch {
		case request["ls"] != nil:
			return fakeWorld
		case request["status"] != nil:
			return `{"focused_pane":"p2"}`
		default:
			return `{"ok":true}`
		}
	})
	core := serve(t, func(map[string]any) string { return fakeTerminals })

	return &Client{app: app.path, core: core.path, sessions: newSessionFinder(nil)}, app
}

func readSnapshot(t *testing.T, client *Client) herdr.Snapshot {
	t.Helper()

	snapshot, err := client.Snapshot(t.Context())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	return snapshot
}

func paneByID(t *testing.T, snapshot herdr.Snapshot, id string) herdr.PaneInfo {
	t.Helper()

	for _, pane := range snapshot.Panes {
		if pane.PaneID == id {
			return pane
		}
	}

	t.Fatalf("no pane %s in %+v", id, snapshot.Panes)

	return herdr.PaneInfo{}
}

// Protects the join: `ls` names no title or directory, the core names no tab.
func TestSnapshotJoinsTheAppToTheCore(t *testing.T) {
	t.Parallel()

	client, _ := fakeClient(t)
	snapshot := readSnapshot(t, client)

	if got := paneByID(t, snapshot, "p1"); got.ForegroundCWD != "/home/x" || got.CWD != "/home" ||
		got.TerminalTitle != "~/home" || got.Focused {
		t.Errorf("p1 = %+v", got)
	}

	if got := paneByID(t, snapshot, "p2"); !got.Focused || got.TabID != "tb2" ||
		got.TerminalTitleStripped != "fix login" || got.Agent != "claude" {
		t.Errorf("p2 = %+v", got)
	}

	if len(snapshot.Workspaces) != 1 || snapshot.Workspaces[0].ActiveTabID != "tb2" {
		t.Errorf("workspaces = %+v", snapshot.Workspaces)
	}
}

// A remote machine's files are out of reach, so its tabs must stay untouched.
func TestSnapshotLeavesOtherMachinesOut(t *testing.T) {
	t.Parallel()

	client, _ := fakeClient(t)

	for _, tab := range readSnapshot(t, client).Tabs {
		if tab.TabID == "box/tb9" {
			t.Errorf("a remote tab was listed: %+v", tab)
		}
	}
}

// Manual rename protection would otherwise lock every tab the `+` menu starts,
// while a name no launcher uses, or one on a tab with no agent, stays the user's.
func TestLauncherNameCountsAsUnnamed(t *testing.T) {
	t.Parallel()

	client, _ := fakeClient(t)
	tabs := readSnapshot(t, client).Tabs

	if tabs[1].Label != "2" {
		t.Errorf("launcher tab label = %q, want its position", tabs[1].Label)
	}

	if tabs[2].Label != "Codex" {
		t.Errorf("agentless tab label = %q, want it kept", tabs[2].Label)
	}
}

func TestTabReportsItsMostUrgentStatus(t *testing.T) {
	t.Parallel()

	client, _ := fakeClient(t)

	if got := readSnapshot(t, client).Tabs[1].AgentStatus; got != herdr.AgentStatusBlocked {
		t.Errorf("status = %q", got)
	}
}

// The resolver reads these shapes, and a frame must not read as a new title.
func TestWithoutSpinner(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"⠋ herdr-auto-title":               "herdr-auto-title",
		"⠇ ⠇ | herdr-auto-title":           "herdr-auto-title",
		"⠹ | herdr-auto-title":             "herdr-auto-title",
		"◐ build":                          "build",
		"Reply with ok | herdr-auto-title": "Reply with ok | herdr-auto-title",
		"⠹ - Waiting for response… - grok": "- Waiting for response… - grok",
		"π - herdr-auto-title":             "π - herdr-auto-title",
		"":                                 "",
	}

	for title, want := range cases {
		if got := withoutSpinner(title); got != want {
			t.Errorf("withoutSpinner(%q) = %q, want %q", title, got, want)
		}
	}
}

// Claude Code's process calls itself by its version, which is no name for a tab.
func TestForegroundNamedByAVersionTakesItsCommand(t *testing.T) {
	t.Parallel()

	got := foregroundOf(terminal{Foreground: herdr.PaneProcessInfoProcess{
		PID: 1, Name: "2.1.314", Argv: []string{"/usr/local/bin/claude"},
	}})
	if got.Name != "claude" {
		t.Errorf("name = %q", got.Name)
	}

	kept := foregroundOf(terminal{Foreground: herdr.PaneProcessInfoProcess{
		PID: 1, Name: "7z", Argv: []string{"7zz"},
	}})
	if kept.Name != "7z" {
		t.Errorf("a name that is no version was replaced: %q", kept.Name)
	}
}

// Spinner frames must not count as a change, or a working agent's pane would
// outrank every other pane on recency alone. What it reports must count.
func TestRevisionIgnoresSpinnerFrames(t *testing.T) {
	t.Parallel()

	entry := paneEntry{Terminal: "t", Pane: "p", Status: "working"}
	first := paneFrom("tb", localPane{entry, terminal{Title: "⠋ name"}}, "")
	second := paneFrom("tb", localPane{entry, terminal{Title: "⠙ name"}}, "")

	if first.Revision != second.Revision {
		t.Error("a spinner frame moved the revision")
	}

	entry.Status = "done"
	if third := paneFrom(
		"tb",
		localPane{entry, terminal{Title: "⠙ name"}},
		"",
	); third.Revision == second.Revision {
		t.Error("a new status did not move the revision")
	}
}

// A numeric name must arrive as a string: the app rejects any other type.
func TestRenameSendsTheNameAsAString(t *testing.T) {
	t.Parallel()

	client, app := fakeClient(t)

	if err := client.RenameTab(t.Context(), "tb1", "123"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got := app.last()
	args, _ := got["args"].(map[string]any)

	if got["do"] != "tab.rename" || args["id"] != "tb1" || args["name"] != "123" {
		t.Errorf("sent %v", got)
	}
}

// labels forgets a tab that is gone, and keeps the claim on a rename that may
// still land, so both must arrive in the form it recognizes.
func TestRenameErrorsKeepTheirMeaning(t *testing.T) {
	t.Parallel()

	var reply atomic.Value

	app := serve(t, func(map[string]any) string {
		answer, _ := reply.Load().(string)
		return answer
	})
	client := &Client{app: app.path}

	reply.Store(`{"error":"no tab \"tb1\""}`)

	if err := client.RenameTab(t.Context(), "tb1",
		"x",
	); herdr.ErrorCode(
		err,
	) != herdr.CodeTabNotFound {
		t.Errorf("gone tab: %v", err)
	}

	reply.Store(`{"error":"the window did not answer"}`)

	if err := client.RenameTab(t.Context(), "tb1",
		"x",
	); !errors.Is(
		err,
		herdr.ErrUnanswered,
	) {
		t.Errorf("slow window: %v", err)
	}

	reply.Store(`{"error":"layout unavailable"}`)

	err := client.RenameTab(t.Context(), "tb1", "x")
	if err == nil || errors.Is(err, herdr.ErrUnanswered) || herdr.ErrorCode(err) != "" {
		t.Errorf("other failure: %v", err)
	}
}

// A closed window leaves its socket file behind and must read as nothing sent.
func TestClosedWindowIsNotUnanswered(t *testing.T) {
	t.Parallel()

	client := &Client{app: filepath.Join(t.TempDir(), "gone.sock")}

	err := client.RenameTab(t.Context(), "tb1", "x")
	if err == nil || errors.Is(err, herdr.ErrUnanswered) {
		t.Errorf("err = %v", err)
	}
}

func TestProcessesComeFromTheLastSnapshot(t *testing.T) {
	t.Parallel()

	client, _ := fakeClient(t)
	readSnapshot(t, client)

	processes, err := client.PaneProcesses(t.Context(), "p1")
	if err != nil || len(processes) != 1 || processes[0].Name != "fish" {
		t.Errorf("p1 = %+v, %v", processes, err)
	}

	// A terminal's foreground is null for its first moments.
	processes, err = client.PaneProcesses(t.Context(), "p2")
	if err != nil || len(processes) != 0 {
		t.Errorf("p2 = %+v, %v", processes, err)
	}

	_, err = client.PaneProcesses(t.Context(), "gone")
	if herdr.ErrorCode(err) != herdr.CodePaneNotFound {
		t.Errorf("gone = %v", err)
	}
}

func TestMethodsWithoutADesktopEquivalentFail(t *testing.T) {
	t.Parallel()

	client := &Client{}
	if err := client.RenamePane(t.Context(), "p1", "x"); err == nil {
		t.Error("RenamePane succeeded")
	}

	if err := client.ReportWorkspaceTopic(t.Context(), "w", "s", "t", time.Second); err == nil {
		t.Error("ReportWorkspaceTopic succeeded")
	}
}

// A pane in `ls` that the core has not listed yet has no title or directory, so
// as a blank pane it could speak for its tab over a sibling that has both. Its
// tab's label is still judged by all of `ls`, or a launcher's tab would read as
// named for that one poll and lock.
func TestAPaneTheCoreDoesNotListYetIsLeftOut(t *testing.T) {
	t.Parallel()

	world := world{Spaces: []space{{ID: "s", Tabs: []tab{
		{ID: "split", Name: "1", Terminals: []paneEntry{
			{Terminal: "t1", Pane: "listed"}, {Terminal: "new", Pane: "blank"},
		}},
		{ID: "launched", Name: "Codex", Terminals: []paneEntry{
			{Terminal: "new2", Pane: "blank2", App: "codex"},
		}},
	}}}}

	snapshot, foregrounds := build(world.local([]terminal{{Address: "t1"}}), "", nil)

	if len(snapshot.Panes) != 1 || snapshot.Panes[0].PaneID != "listed" {
		t.Errorf("panes = %+v, want only the listed one", snapshot.Panes)
	}

	if _, blank := foregrounds["blank"]; blank {
		t.Error("an unlisted pane answers as a pane that runs nothing")
	}

	if got := snapshot.Tabs[1].Label; got != "2" {
		t.Errorf("launcher's tab label = %q, want its position", got)
	}
}

// One read of `ls` must not leave a remote space's panes behind.
func TestPanesOfOtherMachinesAreNotLocal(t *testing.T) {
	t.Parallel()

	machine := "box"
	world := world{Spaces: []space{{ID: "s", Machine: &machine, Tabs: []tab{
		{ID: "t", Terminals: []paneEntry{{Terminal: "t1", Pane: "p"}}},
	}}}}

	if got := panesIn(world.local([]terminal{{Address: "t1"}})); len(got) != 0 {
		t.Errorf("panes = %+v", got)
	}
}
