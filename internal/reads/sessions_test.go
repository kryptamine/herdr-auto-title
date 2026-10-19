package reads_test

import (
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/reads"
	"github.com/kryptamine/herdr-auto-title/internal/reads/readstest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func grokOptions(home string) reads.Options {
	opts := branchOptions()
	opts.ReadSessions = true
	opts.GrokHome = home

	return opts
}

func TestAGrokSummaryNamesThePane(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	home := t.TempDir()
	readstest.GrokSummary(t, home, testSession, "Fix the login redirect")

	pane := state.PaneFrom(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", CWD: dir, Agent: "grok",
		AgentSession: &herdr.AgentSessionInfo{
			Agent: "grok", Kind: herdr.SessionRefID, Value: testSession,
		},
	}, "", time.Time{})

	readOne(grokOptions(home), pane)

	if pane.AgentTopic != "Fix the login redirect" {
		t.Errorf("topic = %q, want the summary's title", pane.AgentTopic)
	}
}

// antigravityPaneAt is a pane sitting in dir and holding an Antigravity CLI
// session, which Herdr labels `agy`.
func antigravityPaneAt(paneID, sessionID, dir string) *state.PaneState {
	return state.PaneFrom(herdr.PaneInfo{
		PaneID: paneID, TabID: "wE:t1", CWD: dir, Agent: "agy",
		AgentSession: &herdr.AgentSessionInfo{
			Agent: "agy", Kind: herdr.SessionRefID, Value: sessionID,
		},
	}, "", time.Time{})
}

func TestAnAntigravitySessionIsNamedByItsAnnotation(t *testing.T) {
	t.Parallel()

	// The agent sets no terminal title, so the annotation is the only place
	// its pane says what it is about.
	opts := transcriptOptions()
	opts.Home = t.TempDir()
	readstest.Annotation(t, opts.Home, testSession, `title:"Casual Greeting And Request"`)

	pane := antigravityPaneAt("wE:p1", testSession, t.TempDir())
	readOne(opts, pane)

	if pane.AgentTopic != "Casual Greeting And Request" {
		t.Errorf("topic = %q, want the annotated title", pane.AgentTopic)
	}
}

func TestTranscriptsSwitchedOffLeaveAnAntigravitySessionUnread(t *testing.T) {
	t.Parallel()

	opts := branchOptions()
	opts.Home = t.TempDir()
	readstest.Annotation(t, opts.Home, testSession, `title:"Casual Greeting And Request"`)

	pane := antigravityPaneAt("wE:p1", testSession, t.TempDir())
	readOne(opts, pane)

	if pane.AgentTopic != "" {
		t.Errorf("topic = %q, want nothing read", pane.AgentTopic)
	}
}

// piPaneAt is a pane sitting in dir and holding a Pi session, which Herdr
// reports by the path of its file rather than by an id.
func piPaneAt(paneID, path, dir string) *state.PaneState {
	return state.PaneFrom(herdr.PaneInfo{
		PaneID: paneID, TabID: "wE:t1", CWD: dir, Agent: "pi",
		AgentSession: &herdr.AgentSessionInfo{
			Agent: "pi", Kind: herdr.SessionRefPath, Value: path,
		},
	}, "", time.Time{})
}

func TestAPiPaneIsNamedFromItsSessionFile(t *testing.T) {
	t.Parallel()

	repo := readstest.Repo(t, "main")
	path := readstest.PiSession(
		t,
		readstest.PiStartedIn(repo),
		readstest.PiPrompt("rework the poll loop"),
	)

	pane := piPaneAt("wE:p1", path, repo)
	readOne(transcriptOptions(), pane)

	if pane.AgentTopic != "rework the poll loop" {
		t.Errorf("topic = %q, want the session's first prompt", pane.AgentTopic)
	}

	if pane.AgentDir != repo {
		t.Errorf("agent dir = %q, want where the session started, %q", pane.AgentDir, repo)
	}
}

func TestAPiSessionInAWorktreeNamesItsBranch(t *testing.T) {
	t.Parallel()

	// The pane sits at the repository root on the trunk while its Pi was
	// started in a worktree, so the branch worth showing is the session's.
	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "oauth", "feat/oauth")
	path := readstest.PiSession(t, readstest.PiStartedIn(worktree))

	pane := piPaneAt("wE:p1", path, repo)
	readOne(transcriptOptions(), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want feat/oauth", got)
	}
}

func TestTranscriptsSwitchedOffLeaveAPiSessionUnread(t *testing.T) {
	t.Parallel()

	path := readstest.PiSession(
		t,
		readstest.PiStartedIn(t.TempDir()),
		readstest.PiPrompt("rework the poll loop"),
	)

	pane := piPaneAt("wE:p1", path, t.TempDir())
	readOne(branchOptions(), pane)

	if pane.AgentTopic != "" || pane.AgentDir != "" {
		t.Errorf("topic = %q, dir = %q, want neither read", pane.AgentTopic, pane.AgentDir)
	}
}
