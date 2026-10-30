//go:build !windows

package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

const (
	sessionA = "01a18ca2-b264-7e03-9a9e-a5cbd02dd669"
	sessionB = "01a18cb2-f7f6-7870-99e9-0f48e88a077f"
)

// agentPane is one local pane, its tab and the terminal behind it.
type agentPane struct {
	id, app         string
	pid, foreground int
	turn            int
}

func encoded(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Errorf("fixture: %v", err)
	}

	return string(raw)
}

func worldOf(t *testing.T, panes []agentPane) string {
	t.Helper()

	tabs := make([]map[string]any, 0, len(panes))

	for _, pane := range panes {
		tabs = append(tabs, map[string]any{
			"id": "tb-" + pane.id, "name": "1", "terminals": []map[string]any{{
				"terminal": "t-" + pane.id, "pane": pane.id, "app": pane.app,
			}},
		})
	}

	return encoded(t, map[string]any{"spaces": []map[string]any{{"id": "s1", "tabs": tabs}}})
}

func terminalsOf(t *testing.T, panes []agentPane) string {
	t.Helper()

	terminals := make([]map[string]any, 0, len(panes))

	for _, pane := range panes {
		terminals = append(terminals, map[string]any{
			"address": "t-" + pane.id, "pid": pane.pid, "turn": pane.turn,
			"foreground": map[string]any{"pid": pane.foreground, "name": "fish"},
		})
	}

	return encoded(t, map[string]any{"result": map[string]any{"terminals": terminals}})
}

// sessionClient answers from the panes it is given, and from whatever panes the
// returned function replaces them with.
func sessionClient(
	t *testing.T,
	sessions *sessionFinder,
	panes ...agentPane,
) (*Client, func(...agentPane)) {
	t.Helper()

	var current atomic.Pointer[[]agentPane]

	current.Store(&panes)

	app := serve(t, func(request map[string]any) string {
		if request["ls"] != nil {
			return worldOf(t, *current.Load())
		}

		return `{}`
	})
	core := serve(t, func(map[string]any) string { return terminalsOf(t, *current.Load()) })

	return &Client{app: app.path, core: core.path, sessions: sessions},
		func(next ...agentPane) { current.Store(&next) }
}

func processTree(tree map[int][]int) func(context.Context) (map[int][]int, error) {
	return func(context.Context) (map[int][]int, error) { return tree, nil }
}

func writeClaudeSession(t *testing.T, dir string, pid int, sessionID string) {
	t.Helper()

	sessions := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"pid":%d,"sessionId":%q}`, pid, sessionID)
	if err := os.WriteFile(
		filepath.Join(sessions, fmt.Sprintf("%d.json", pid)),
		[]byte(body),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func onlySession(t *testing.T, client *Client) *herdr.AgentSessionInfo {
	t.Helper()

	return paneByID(t, readSnapshot(t, client), "p1").AgentSession
}

func wantSession(t *testing.T, got *herdr.AgentSessionInfo, agent, id string) {
	t.Helper()

	if got == nil || got.Agent != agent || got.Kind != herdr.SessionRefID || got.Value != id {
		t.Errorf("session = %+v, want %s %s", got, agent, id)
	}
}

// In a tab started with argv the agent is the wrapping shell's child, and two
// sessions in one directory stay apart because each has its own pid.
func TestClaudeSessionIsFoundUnderTheTerminal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeClaudeSession(t, dir, 11, sessionA)
	writeClaudeSession(t, dir, 21, sessionB)

	finder := newSessionFinder([]string{dir})
	finder.processes = processTree(map[int][]int{10: {11}, 20: {21}})

	client, _ := sessionClient(t, finder,
		agentPane{id: "p1", app: "claude-code", pid: 10, foreground: 10},
		agentPane{id: "p2", app: "claude-code", pid: 20, foreground: 20},
	)
	snapshot := readSnapshot(t, client)

	wantSession(t, paneByID(t, snapshot, "p1").AgentSession, "claude", sessionA)
	wantSession(t, paneByID(t, snapshot, "p2").AgentSession, "claude", sessionB)
}

func filesOpen(files map[int][]string) func(context.Context, []int) ([]string, error) {
	return func(_ context.Context, pids []int) ([]string, error) {
		var open []string
		for _, pid := range pids {
			open = append(open, files[pid]...)
		}

		return open, nil
	}
}

// /new leaves the old transcript open too, and a Grok id is ordered by time.
func TestGrokSessionIsTheNewestItHoldsOpen(t *testing.T) {
	t.Parallel()

	finder := newSessionFinder(nil)
	finder.processes = processTree(map[int][]int{10: {11}})
	finder.openFiles = filesOpen(map[int][]string{11: {
		"/home/x/.grok/sessions/%2Frepo/" + sessionB + "/events.jsonl",
		"/home/x/.grok/sessions/%2Frepo/" + sessionA + "/events.jsonl",
		"/home/x/.grok/sessions/%2Frepo/prompt_history.jsonl",
		"/usr/lib/libc.dylib",
	}})

	client, _ := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "grok-build", pid: 10, foreground: 10},
	)

	wantSession(t, onlySession(t, client), "grok", sessionB)
}

// An Antigravity id is random, so only its conversation file says which is newer.
func TestAntigravitySessionIsTheNewestConversation(t *testing.T) {
	t.Parallel()

	const older, newer = "805a32b5-c7c5-4974-9009-d86e07b4899c", "0da7e9e5-a9a3-4362-8353-796c94a6b721"

	root := filepath.Join(t.TempDir(), "antigravity-cli")
	conversations := filepath.Join(root, "conversations")

	if err := os.MkdirAll(conversations, 0o700); err != nil {
		t.Fatal(err)
	}

	base := time.Now()

	for id, age := range map[string]time.Duration{older: time.Hour, newer: 0} {
		path := filepath.Join(conversations, id+".db")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		if err := os.Chtimes(path, base.Add(-age), base.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	finder := newSessionFinder(nil)
	finder.processes = processTree(map[int][]int{10: {11}})
	finder.openFiles = filesOpen(map[int][]string{11: {
		filepath.Join(root, "brain", older),
		filepath.Join(root, "brain", newer, ".system_generated", "log"),
		filepath.Join(root, "conversation_summaries.db"),
	}})

	client, _ := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "antigravity", pid: 10, foreground: 10},
	)

	wantSession(t, onlySession(t, client), "agy", newer)
}

// A session just started has no conversation file yet and is still the session.
func TestAntigravitySessionWithNoConversationYet(t *testing.T) {
	t.Parallel()

	finder := newSessionFinder(nil)
	finder.processes = processTree(map[int][]int{10: nil})
	finder.openFiles = filesOpen(map[int][]string{10: {
		"/home/x/.gemini/antigravity-cli/brain/" + sessionA,
	}})

	client, _ := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "antigravity", pid: 10, foreground: 10},
	)

	wantSession(t, onlySession(t, client), "agy", sessionA)
}

// /new holds both before its first prompt, and the old file must not keep the tab.
func TestAntigravitySessionWithNoConversationBeatsAnOlderOne(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "antigravity-cli")
	conversations := filepath.Join(root, "conversations")

	if err := os.MkdirAll(conversations, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(conversations, sessionA+".db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	finder := newSessionFinder(nil)
	finder.processes = processTree(map[int][]int{10: nil})
	finder.openFiles = filesOpen(map[int][]string{10: {
		filepath.Join(root, "brain", sessionA),
		filepath.Join(root, "brain", sessionB),
	}})

	client, _ := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "antigravity", pid: 10, foreground: 10},
	)

	wantSession(t, onlySession(t, client), "agy", sessionB)
}

// Codex and Pi have no route: their panes keep their title and cost nothing.
func TestAgentsWithNoRouteReadNothing(t *testing.T) {
	t.Parallel()

	finder := newSessionFinder(nil)
	finder.processes = func(context.Context) (map[int][]int, error) {
		t.Error("the process table was read")
		return map[int][]int{}, nil
	}

	client, _ := sessionClient(t, finder,
		agentPane{id: "p1", app: "codex", pid: 10, foreground: 10},
		agentPane{id: "p2", pid: 20, foreground: 20},
	)
	snapshot := readSnapshot(t, client)

	if got := paneByID(t, snapshot, "p1").AgentSession; got != nil {
		t.Errorf("codex = %+v", got)
	}

	if got := paneByID(t, snapshot, "p2").AgentSession; got != nil {
		t.Errorf("shell = %+v", got)
	}
}

// ps and lsof cost tens of milliseconds, far too much for every poll, but /clear
// moves nothing a poll can see, so the read must still expire on its own.
func TestSessionReadIsReusedUntilItExpires(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeClaudeSession(t, dir, 11, sessionA)

	reads := 0
	now := time.Now()

	finder := newSessionFinder([]string{dir})
	finder.now = func() time.Time { return now }
	finder.processes = func(context.Context) (map[int][]int, error) {
		reads++
		return map[int][]int{10: {11}}, nil
	}

	client, _ := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "claude-code", pid: 10, foreground: 10},
	)

	wantSession(t, onlySession(t, client), "claude", sessionA)

	writeClaudeSession(t, dir, 11, sessionB)
	wantSession(t, onlySession(t, client), "claude", sessionA)

	if reads != 1 {
		t.Errorf("process table read %d times within the refresh", reads)
	}

	now = now.Add(sessionRefresh)

	wantSession(t, onlySession(t, client), "claude", sessionB)
}

// A new foreground process or turn is a likely new session: no waiting for it.
func TestSessionReadEndsWhenThePaneMoves(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeClaudeSession(t, dir, 11, sessionA)

	finder := newSessionFinder([]string{dir})
	finder.processes = processTree(map[int][]int{10: {11}})

	client, replace := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "claude-code", pid: 10, foreground: 10},
	)
	wantSession(t, onlySession(t, client), "claude", sessionA)

	writeClaudeSession(t, dir, 11, sessionB)
	replace(agentPane{id: "p1", app: "claude-code", pid: 10, foreground: 11})
	wantSession(t, onlySession(t, client), "claude", sessionB)

	writeClaudeSession(t, dir, 11, sessionA)
	replace(agentPane{id: "p1", app: "claude-code", pid: 10, foreground: 11, turn: 1})
	wantSession(t, onlySession(t, client), "claude", sessionA)
}

// A failed ps says nothing about the session, so the topic must not flicker off.
func TestFailedReadKeepsTheSessionItHad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeClaudeSession(t, dir, 11, sessionA)

	now := time.Now()

	finder := newSessionFinder([]string{dir})
	finder.now = func() time.Time { return now }
	finder.processes = processTree(map[int][]int{10: {11}})

	client, _ := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "claude-code", pid: 10, foreground: 10},
	)
	wantSession(t, onlySession(t, client), "claude", sessionA)

	finder.processes = func(context.Context) (map[int][]int, error) {
		return nil, errors.New("ps failed")
	}
	now = now.Add(sessionRefresh)

	wantSession(t, onlySession(t, client), "claude", sessionA)
}

// The session it had belonged to another process, and must not name this agent.
func TestFailedReadDropsTheSessionOfAnotherAgent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeClaudeSession(t, dir, 11, sessionA)

	finder := newSessionFinder([]string{dir})
	finder.processes = processTree(map[int][]int{10: {11}})

	client, replace := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "claude-code", pid: 10, foreground: 10},
	)
	wantSession(t, onlySession(t, client), "claude", sessionA)

	finder.processes = func(context.Context) (map[int][]int, error) {
		return nil, errors.New("ps failed")
	}

	replace(agentPane{id: "p1", app: "grok-build", pid: 10, foreground: 10})

	if got := onlySession(t, client); got != nil {
		t.Errorf("session = %+v", got)
	}
}

// A pane that closes takes its read along, or the map grows for good.
func TestClosedPaneIsForgotten(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeClaudeSession(t, dir, 11, sessionA)

	finder := newSessionFinder([]string{dir})
	finder.processes = processTree(map[int][]int{10: {11}})

	client, replace := sessionClient(
		t,
		finder,
		agentPane{id: "p1", app: "claude-code", pid: 10, foreground: 10},
	)
	readSnapshot(t, client)

	replace(agentPane{id: "p2", pid: 20, foreground: 20})
	readSnapshot(t, client)

	if len(finder.reads) != 0 {
		t.Errorf("reads = %+v", finder.reads)
	}
}

// ps and lsof are mocked everywhere else, so only this keeps their output readable.
func TestOpenFilesListsWhatAProcessHolds(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = file.Close() })

	// A pid that has gone must not cost the answer for the rest of the tree.
	open, err := openFiles(t.Context(), []int{os.Getpid(), 1 << 30})
	if err != nil {
		t.Fatalf("openFiles: %v", err)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(open, path) && !slices.Contains(open, resolved) {
		t.Errorf("%s not among %d open files", path, len(open))
	}
}

// An lsof that answered nothing failed, which must not read as no session held.
func TestOpenFilesReportsAReadWithNoAnswer(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux reads /proc, where a process that has gone simply lists nothing")
	}

	t.Parallel()

	if open, err := openFiles(t.Context(), []int{1 << 30}); err == nil {
		t.Errorf("open = %v, want an error", open)
	}
}
