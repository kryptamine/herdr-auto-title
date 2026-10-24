// Package readstest lays down on disk what a pane read goes to: repositories,
// their worktrees, and the agent session files that say what an agent is doing.
package readstest

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Repo builds a repository on disk. Its trunk is always `main`, so passing that
// as the branch is how a pane on the trunk is written.
func Repo(t *testing.T, branch string) string {
	t.Helper()
	return RepoIn(t, t.TempDir(), branch)
}

// RepoIn builds one in a directory that may already exist, so a test can watch
// a repository appear under a pane that was read before it did.
func RepoIn(t *testing.T, root, branch string) string {
	t.Helper()

	gitDir := filepath.Join(root, ".git")
	remote := filepath.Join(gitDir, "refs", "remotes", "origin")

	mkdir(t, remote)
	write(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/"+branch+"\n")
	write(t, filepath.Join(remote, "HEAD"), "ref: refs/remotes/origin/main\n")

	return root
}

// RepoWithNoTrunk builds a repository recording no default branch, which is
// what one that was never cloned looks like: every branch in it is worth naming.
func RepoWithNoTrunk(t *testing.T, branch string) string {
	t.Helper()

	root := Repo(t, branch)

	if err := os.Remove(
		filepath.Join(root, ".git", "refs", "remotes", "origin", "HEAD"),
	); err != nil {
		t.Fatal(err)
	}

	return root
}

// Worktree builds a worktree of the repository at root out of files, the way
// git records one: the branch in the worktree's own HEAD, and the refs it
// shares with the repository a commondir away.
func Worktree(t *testing.T, root, name, branch string) string {
	t.Helper()

	gitDir := filepath.Join(root, ".git", "worktrees", name)
	tree := filepath.Join(root, ".claude", "worktrees", name)

	mkdir(t, gitDir)
	mkdir(t, tree)
	write(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/"+branch+"\n")
	write(t, filepath.Join(gitDir, "commondir"), "../..\n")
	write(t, filepath.Join(tree, ".git"), "gitdir: "+gitDir+"\n")

	return tree
}

// Transcript lays down one session's transcript under a Claude Code
// configuration home. Which project directory it lands in is the transcript
// reader's business, and its own tests cover that.
func Transcript(t *testing.T, home, sessionID string, lines ...string) {
	t.Helper()

	path := filepath.Join(home, "projects", "any-project", sessionID+".jsonl")

	mkdir(t, filepath.Dir(path))
	write(t, path, strings.Join(lines, "\n")+"\n")
}

// Annotation lays down the title Antigravity CLI keeps for one session under a
// home directory.
func Annotation(t *testing.T, home, sessionID, content string) {
	t.Helper()

	path := filepath.Join(home, ".gemini", "antigravity-cli", "annotations", sessionID+".pbtxt")

	mkdir(t, filepath.Dir(path))
	write(t, path, content)
}

// AgentIn is a transcript line saying where the agent is working. Only the
// directory is read out of it, and a Windows path is full of escapes.
func AgentIn(dir string) string {
	return `{"type":"assistant","cwd":` + strconv.Quote(dir) + `}`
}

// PiSession lays down one Pi session file in a directory of its own and
// reports its path, which is what Herdr hands the plugin for a Pi pane.
func PiSession(t *testing.T, lines ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "2026-10-12T04-22-57-889Z_01a12e59.jsonl")
	write(t, path, strings.Join(lines, "\n")+"\n")

	return path
}

// PiStartedIn is the header line of a Pi session, which is where the file says
// what directory the session was opened in.
func PiStartedIn(dir string) string {
	return `{"type":"session","version":3,"id":"01a12e59","cwd":` + strconv.Quote(dir) + `}`
}

// PiPrompt is a prompt the user typed into a Pi session.
func PiPrompt(text string) string {
	return `{"type":"message","id":"a1b2c3d4","message":{"role":"user","content":` + strconv.Quote(
		text,
	) + `}}`
}

// CodexRollout lays down one Codex session's rollout under a Codex home, filed
// under a day the way Codex files it.
func CodexRollout(t *testing.T, home, sessionID string, lines ...string) {
	t.Helper()

	path := filepath.Join(
		home, "sessions", "2026", "10", "19",
		"rollout-2026-10-19T09-17-14-"+sessionID+".jsonl",
	)

	mkdir(t, filepath.Dir(path))
	write(t, path, strings.Join(lines, "\n")+"\n")
}

// CodexName lays down the session index under a Codex home, naming one
// session.
func CodexName(t *testing.T, home, sessionID, name string) {
	t.Helper()

	write(t, filepath.Join(home, "session_index.jsonl"),
		`{"id":"`+sessionID+`","thread_name":`+strconv.Quote(name)+
			`,"updated_at":"2026-10-19T09:20:00Z"}`+"\n")
}

// CodexStartedIn is the header line of a Codex rollout, which is where the
// file says what directory the session was opened in.
func CodexStartedIn(dir string) string {
	return `{"type":"session_meta","payload":{"id":"01a070db","cwd":` + strconv.Quote(dir) + `}}`
}

// CodexPrompt is a prompt the user typed into a Codex session.
func CodexPrompt(text string) string {
	return `{"type":"event_msg","payload":{"type":"user_message","message":` +
		strconv.Quote(text) + `}}`
}

func mkdir(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
