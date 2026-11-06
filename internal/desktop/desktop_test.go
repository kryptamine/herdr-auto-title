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
	"slices"
	"sync"
	"sync/atomic"
	"testing"

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

	socket := &fakeSocket{}
	socket.path = listen(t, func(conn net.Conn) {
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
	})

	return socket
}

// listen hands each connection to a Unix socket to handle, then closes it.
func listen(t *testing.T, handle func(net.Conn)) string {
	t.Helper()

	//nolint:usetesting // t.TempDir() overruns the 104 bytes of a sun_path
	dir, err := os.MkdirTemp("", "dt")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	path := filepath.Join(dir, "s.sock")

	var config net.ListenConfig

	listener, err := config.Listen(context.Background(), "unix", path)
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

			handle(conn)
			_ = conn.Close()
		}
	}()

	return path
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

	return fakeClientAnswering(t, fakeWorld, `{"focused_pane":"p2"}`)
}

func fakeClientAnswering(t *testing.T, ls, status string) (*Client, *fakeSocket) {
	t.Helper()

	app := serve(t, func(request map[string]any) string {
		switch {
		case request["ls"] != nil:
			return ls
		case request["status"] != nil:
			return status
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

	if tabs[1].Label != "Claude Code" || !tabs[1].Unnamed {
		t.Errorf("launcher tab = %+v, want its raw label marked unnamed", tabs[1])
	}

	if tabs[2].Label != "Codex" || tabs[2].Unnamed {
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

// An error reply read as an empty session would drop every lock and rewrite
// the locks file; a slow window must also keep the meaning labels gives it.
func TestAnErrorReplyFailsTheSnapshot(t *testing.T) {
	t.Parallel()

	slow := `{"error":"the window did not answer"}`

	for name, replies := range map[string][2]string{
		"ls":     {slow, `{"focused_pane":"p2"}`},
		"status": {fakeWorld, slow},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client, _ := fakeClientAnswering(t, replies[0], replies[1])

			if _, err := client.Snapshot(t.Context()); !errors.Is(err, herdr.ErrUnanswered) {
				t.Errorf("err = %v", err)
			}
		})
	}

	client, _ := fakeClientAnswering(t, `{"error":"no space"}`, `{"focused_pane":"p2"}`)

	_, err := client.Snapshot(t.Context())
	if err == nil || errors.Is(err, herdr.ErrUnanswered) {
		t.Errorf("other failure: %v", err)
	}
}

// A request the app read may still apply, so a hang-up or a garbled answer
// after sending must keep labels' claim on the name.
func TestABrokenAnswerIsUnanswered(t *testing.T) {
	t.Parallel()

	handlers := map[string]func(net.Conn){
		"hang up": func(conn net.Conn) { _, _ = bufio.NewReader(conn).ReadBytes('\n') },
		"garbage": func(conn net.Conn) {
			_, _ = bufio.NewReader(conn).ReadBytes('\n')
			_, _ = conn.Write([]byte("not json\n"))
		},
	}

	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := &Client{app: listen(t, handle)}

			if err := client.RenameTab(
				t.Context(),
				"tb1",
				"x",
			); !errors.Is(
				err,
				herdr.ErrUnanswered,
			) {
				t.Errorf("err = %v", err)
			}
		})
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

// The app has no panes to name and no workspace tokens, and the poll loop
// leaves both alone only for a client that does not offer them.
func TestTheClientOffersNoPaneNamesOrTopics(t *testing.T) {
	t.Parallel()

	var client herdr.Client = &Client{}
	if _, renames := client.(herdr.PaneRenamer); renames {
		t.Error("the client renames panes")
	}

	if _, reports := client.(herdr.TopicReporter); reports {
		t.Error("the client reports topics")
	}
}

// An unlisted pane must not outrank a known sibling, but its launcher name
// must still count as unnamed until the core catches up.
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

	client := &Client{}
	snapshot, foregrounds := client.build(world.local([]terminal{{Address: "t1"}}), "", nil)

	if len(snapshot.Panes) != 1 || snapshot.Panes[0].PaneID != "listed" {
		t.Errorf("panes = %+v, want only the listed one", snapshot.Panes)
	}

	if _, blank := foregrounds["blank"]; blank {
		t.Error("an unlisted pane answers as a pane that runs nothing")
	}

	if got := snapshot.Tabs[1]; got.Label != "Codex" || !got.Unnamed {
		t.Errorf("launcher's tab = %+v, want its raw label marked unnamed", got)
	}
}

// The app names only the shown tab's focus, so a split tab in the background
// must keep its own focused pane, or it is renamed each time it is left.
func TestABackgroundTabKeepsItsFocusedPane(t *testing.T) {
	t.Parallel()

	split := tab{ID: "split", Name: "1", Terminals: []paneEntry{
		{Terminal: "t1", Pane: "left"}, {Terminal: "t2", Pane: "right"},
	}}
	other := tab{ID: "other", Name: "2", Terminals: []paneEntry{{Terminal: "t3", Pane: "solo"}}}
	terminals := []terminal{{Address: "t1"}, {Address: "t2"}, {Address: "t3"}}

	client := &Client{}
	focus := func(world world, pane string) []herdr.LayoutInfo {
		snapshot, _ := client.build(world.local(terminals), pane, nil)
		return snapshot.Layouts
	}

	both := world{Spaces: []space{{ID: "s", Tabs: []tab{split, other}}}}
	focus(both, "right")

	layouts := focus(both, "solo")
	if !slices.Contains(layouts, herdr.LayoutInfo{TabID: "split", FocusedPaneID: "right"}) {
		t.Errorf("layouts = %+v, want split's focus kept on right", layouts)
	}

	split.Terminals = split.Terminals[:1]
	if layouts := focus(
		world{Spaces: []space{{ID: "s", Tabs: []tab{split}}}},
		"",
	); len(
		layouts,
	) != 0 {
		t.Errorf("layouts = %+v, want a closed pane's focus and a closed tab forgotten", layouts)
	}
}

func TestAppSocketPath(t *testing.T) {
	t.Setenv(appHomeEnv, "")

	if got, want := appSocketPath(
		"/h",
	), filepath.Join(
		"/h",
		".config",
		"herdr-app",
		appSocket,
	); got != want {
		t.Errorf("default = %q, want %q", got, want)
	}

	t.Setenv(appHomeEnv, "/app")

	if got, want := appSocketPath("/h"), filepath.Join("/app", appSocket); got != want {
		t.Errorf("HERDR_APP_HOME = %q, want %q", got, want)
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
