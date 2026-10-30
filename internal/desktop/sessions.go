package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec" //nolint:depguard // ps and lsof, given numbers and no shell
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/antigravity"
	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/grok"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// sessionRefresh is how long a pane's session read is reused. ps and lsof take
// tens of milliseconds and /clear moves nothing a poll can see; see
// docs/architecture/desktop-api.md before lowering it.
const sessionRefresh = 5 * time.Second

const sessionID = `[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}`

var (
	grokTranscriptRe = regexp.MustCompile(`/sessions/[^/]+/(` + sessionID + `)/events\.jsonl$`)
	// Any path under a brain directory counts: what is held there varies.
	antigravityBrainRe = regexp.MustCompile(
		`^(.*/antigravity-cli)/brain/(` + sessionID + `)(?:/|$)`,
	)
)

// agent is what is known of one the app reports: the name the resolver knows it
// by, and the route to the session it holds. Any other is taken as reported.
type agent struct {
	name    string
	session func(*sessionFinder, context.Context, []int) (string, error)
}

// agents is keyed by what each reports itself as.
var agents = map[string]agent{
	"claude-code": {name: claude.Agent, session: (*sessionFinder).claudeSession},
	"grok-build":  {name: grok.Agent, session: (*sessionFinder).grokSession},
	"antigravity": {name: antigravity.Agent, session: (*sessionFinder).antigravitySession},
}

// sessionFinder says which session each agent holds, which nothing reports:
// docs/architecture/desktop-api.md has the route each agent takes.
type sessionFinder struct {
	claudeDirs []string
	processes  func(context.Context) (map[int][]int, error)
	openFiles  func(context.Context, []int) ([]string, error)
	now        func() time.Time
	reads      map[string]sessionRead
}

// sessionRead is one pane's answer, valid for the process it was read from.
type sessionRead struct {
	key     sessionKey
	at      time.Time
	session *herdr.AgentSessionInfo
}

type sessionKey struct {
	app             string
	pid, foreground int
	turn            uint64
}

func newSessionFinder(claudeDirs []string) *sessionFinder {
	return &sessionFinder{
		claudeDirs: claudeDirs,
		processes:  processTable,
		openFiles:  openFiles,
		now:        time.Now,
	}
}

// read says which session each pane's agent holds, forgetting every pane it is
// not given. A poll shares one read of the process table, made only if needed.
func (f *sessionFinder) read(
	ctx context.Context,
	panes []localPane,
) map[string]*herdr.AgentSessionInfo {
	table := sync.OnceValues(func() (map[int][]int, error) { return f.processes(ctx) })
	reads := make(map[string]sessionRead)
	sessions := make(map[string]*herdr.AgentSessionInfo)

	for _, pane := range panes {
		if _, routed := agents[pane.entry.App]; routed && pane.terminal.PID != 0 {
			read := f.readPane(ctx, pane, table)
			reads[pane.entry.Pane] = read
			sessions[pane.entry.Pane] = read.session
		}
	}

	f.reads = reads

	return sessions
}

func (f *sessionFinder) readPane(
	ctx context.Context,
	pane localPane,
	table func() (map[int][]int, error),
) sessionRead {
	key := sessionKey{
		app:        pane.entry.App,
		pid:        pane.terminal.PID,
		foreground: pane.terminal.Foreground.PID,
		turn:       pane.terminal.Turn,
	}
	now := f.now()

	last, known := f.reads[pane.entry.Pane]
	if known && last.key == key && now.Sub(last.at) < sessionRefresh {
		return last
	}

	session, err := f.find(ctx, key.app, key.pid, table)
	// A failure says nothing of this process's session, but the last read only
	// speaks for the process it came from.
	if err != nil && last.key.app == key.app && last.key.pid == key.pid {
		session = last.session
	}

	return sessionRead{key: key, at: now, session: session}
}

func (f *sessionFinder) find(
	ctx context.Context,
	app string,
	root int,
	table func() (map[int][]int, error),
) (*herdr.AgentSessionInfo, error) {
	children, err := table()
	if err != nil {
		return nil, err
	}

	id, err := agents[app].session(f, ctx, treeOf(children, root))
	if err != nil {
		return nil, err
	}

	if id == "" {
		return nil, nil //nolint:nilnil // an agent with no session yet is no failure
	}

	return &herdr.AgentSessionInfo{
		Agent: agents[app].name,
		Kind:  herdr.SessionRefID,
		Value: id,
	}, nil
}

// treeOf is root and its descendants, shallowest first.
func treeOf(children map[int][]int, root int) []int {
	pids := []int{root}
	for i := 0; i < len(pids); i++ {
		pids = append(pids, children[pids[i]]...)
	}

	return pids
}

// claudeSession reads the file Claude Code keeps per process. The shallowest
// wins, because an agent can start another below it with a file of its own.
func (f *sessionFinder) claudeSession(_ context.Context, pids []int) (string, error) {
	for _, pid := range pids {
		for _, dir := range f.claudeDirs {
			//nolint:gosec // the directory is configured and the name is a number
			raw, err := os.ReadFile(filepath.Join(dir, "sessions", strconv.Itoa(pid)+".json"))
			if err != nil {
				continue
			}

			var file struct {
				SessionID string `json:"sessionId"`
			}

			if json.Unmarshal(raw, &file) == nil && file.SessionID != "" {
				return file.SessionID, nil
			}
		}
	}

	return "", nil
}

// grokSession takes the greatest id held open: /new leaves the old transcript
// open too, and a Grok id is ordered by time.
func (f *sessionFinder) grokSession(ctx context.Context, pids []int) (string, error) {
	open, err := f.openFiles(ctx, pids)
	if err != nil {
		return "", err
	}

	newest := ""

	for _, path := range open {
		if match := grokTranscriptRe.FindStringSubmatch(path); match != nil && match[1] > newest {
			newest = match[1]
		}
	}

	return newest, nil
}

// antigravitySession takes the held session with the newest conversation file:
// /new leaves the old one held too, and an Antigravity id is random.
func (f *sessionFinder) antigravitySession(ctx context.Context, pids []int) (string, error) {
	open, err := f.openFiles(ctx, pids)
	if err != nil {
		return "", err
	}

	newest, newestAt := "", time.Time{}

	for _, path := range open {
		match := antigravityBrainRe.FindStringSubmatch(path)
		if match == nil || match[2] == newest {
			continue
		}

		// A session before its first prompt has no conversation file yet, and
		// is the newest there is: every older one has had its prompt.
		at := f.now()
		if info, err := os.Stat(
			filepath.Join(match[1], "conversations", match[2]+".db"),
		); err == nil {
			at = info.ModTime()
		}

		if newest == "" || at.After(newestAt) {
			newest, newestAt = match[2], at
		}
	}

	return newest, nil
}

// processTable maps each process to its children.
func processTable(ctx context.Context) (map[int][]int, error) {
	//nolint:forbidigo // no terminal-derived value and no shell
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}

	children := make(map[int][]int)

	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}

		pid, pidErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])

		if pidErr == nil && parentErr == nil {
			children[parent] = append(children[parent], pid)
		}
	}

	return children, nil
}

// openFiles lists every path the processes hold open. Linux has no need of an
// lsof that may not be installed.
func openFiles(ctx context.Context, pids []int) ([]string, error) {
	if runtime.GOOS == "linux" {
		return procFiles(pids), nil
	}

	list := make([]string, 0, len(pids))
	for _, pid := range pids {
		list = append(list, strconv.Itoa(pid))
	}

	//nolint:forbidigo,gosec // nothing but numbers reaches it, and no shell
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-Fn", "-p", strings.Join(list, ",")).
		Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("lsof: %w", ctx.Err())
	}

	// lsof exits 1 when any one pid has gone, and still lists the others.
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("lsof: %w", err)
	}

	var paths []string

	for line := range bytes.Lines(out) {
		if name, isName := bytes.CutPrefix(bytes.TrimRight(line, "\n"), []byte("n")); isName {
			paths = append(paths, string(name))
		}
	}

	return paths, nil
}

// procFiles reads /proc, where a process that has gone simply lists nothing.
func procFiles(pids []int) []string {
	var paths []string

	for _, pid := range pids {
		dir := filepath.Join("/proc", strconv.Itoa(pid), "fd")

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if path, err := os.Readlink(filepath.Join(dir, entry.Name())); err == nil {
				paths = append(paths, path)
			}
		}
	}

	return paths
}
