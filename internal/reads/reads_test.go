package reads_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/reads"
	"github.com/kryptamine/herdr-auto-title/internal/reads/readstest"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// testSession and otherSession are what the agent panes below are holding.
const (
	testSession  = "8852bfe0-8b24-4a23-a35e-7521d04da061"
	otherSession = "0c3a1d94-77b1-4f2e-8a6d-5e91b2c4d803"
)

func newReader(opts reads.Options) *reads.Reader {
	return reads.New(opts, slog.New(slog.DiscardHandler))
}

func branchOptions() reads.Options {
	return reads.Options{BranchMax: resolver.DefaultBranchMaxLength}
}

func transcriptOptions(homes ...string) reads.Options {
	opts := branchOptions()
	opts.ReadTranscripts = true
	opts.ClaudeDirs = homes

	return opts
}

// transcript lays down the test session's transcript in a home of its own.
func transcript(t *testing.T, lines ...string) string {
	t.Helper()

	home := t.TempDir()
	readstest.Transcript(t, home, testSession, lines...)

	return home
}

// paneAt is a pane sitting in dir and running nothing Herdr will answer for,
// so that a read of it finds only what the directory holds.
func paneAt(paneID, dir string) *state.PaneState {
	return state.PaneFrom(herdr.PaneInfo{PaneID: paneID, TabID: "wE:t1", CWD: dir}, time.Time{})
}

// agentPaneAt is a pane sitting in dir and holding a Claude Code session, so a
// read of it goes to the transcript as well as to the directory.
func agentPaneAt(paneID, sessionID, dir string) *state.PaneState {
	return state.PaneFrom(herdr.PaneInfo{
		PaneID: paneID, TabID: "wE:t1", CWD: dir, Agent: "claude",
		AgentSession: &herdr.AgentSessionInfo{
			Agent: "claude", Kind: herdr.SessionRefID, Value: sessionID,
		},
	}, time.Time{})
}

// readOne fills one pane in a poll of its own, on options of its own.
func readOne(opts reads.Options, pane *state.PaneState) {
	newReader(opts).Poll(herdrtest.New(nil, nil), nil).Fill(context.Background(), pane)
}

// branchFor is the branch a filled pane would put in its tab's title, which is
// where the pane's checkout and its agent's are chosen between.
func branchFor(pane *state.PaneState) string {
	parts, _ := resolver.NewGit(resolver.DefaultBranchMaxLength).Resolve(pane)

	return parts.Branch
}

func TestAPaneFilledTwiceInOnePollIsReadOnce(t *testing.T) {
	t.Parallel()

	// A tab's own pane is filled for the tab, for its panes and for its row.
	// A read that failed is left failed until the next poll, which asks again.
	info := herdr.PaneInfo{PaneID: "wE:p1", TabID: "wE:t1", CWD: t.TempDir()}
	client := herdrtest.New([]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}}, []herdr.PaneInfo{info})

	poll := newReader(branchOptions()).Poll(client, []herdr.PaneInfo{info})
	pane := state.PaneFrom(info, time.Time{})

	client.SetProcessError(errors.New("herdr is busy"))
	poll.Fill(context.Background(), pane)

	client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "nvim"})
	client.SetProcessError(nil)
	poll.Fill(context.Background(), pane)

	if reads := client.ProcessReads(); reads != 0 {
		t.Errorf("asked what the pane runs %d more times in the same poll, want 0", reads)
	}
}

func TestAPaneThatMovedIsAskedAboutAgain(t *testing.T) {
	t.Parallel()

	info := herdr.PaneInfo{PaneID: "wE:p1", TabID: "wE:t1", CWD: t.TempDir()}
	client := herdrtest.New([]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}}, []herdr.PaneInfo{info})
	reader := newReader(branchOptions())

	reader.Poll(client, []herdr.PaneInfo{info}).
		Fill(context.Background(), state.PaneFrom(info, time.Time{}))

	client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "nvim"})

	info.Revision = 2

	pane := state.PaneFrom(info, time.Time{})
	reader.Poll(client, []herdr.PaneInfo{info}).Fill(context.Background(), pane)

	if foreground, ok := pane.Foreground(); !ok || foreground.Name != "nvim" {
		t.Errorf("foreground = %+v, want nvim read again after the pane drew", foreground)
	}
}

func TestAPaneThatCannotBeReadIsAskedAgain(t *testing.T) {
	t.Parallel()

	// A failed read is not an answer, so it must not be remembered as one.
	info := herdr.PaneInfo{PaneID: "wE:p1", TabID: "wE:t1", CWD: t.TempDir()}
	client := herdrtest.New([]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}}, []herdr.PaneInfo{info})
	reader := newReader(branchOptions())

	client.SetProcessError(errors.New("herdr is busy"))
	reader.Poll(client, []herdr.PaneInfo{info}).
		Fill(context.Background(), state.PaneFrom(info, time.Time{}))

	client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "nvim"})
	client.SetProcessError(nil)

	pane := state.PaneFrom(info, time.Time{})
	reader.Poll(client, []herdr.PaneInfo{info}).Fill(context.Background(), pane)

	if foreground, ok := pane.Foreground(); !ok || foreground.Name != "nvim" {
		t.Errorf("foreground = %+v, want the pane asked again after a failed read", foreground)
	}
}

func TestARepositoryIsWalkedOncePerPoll(t *testing.T) {
	t.Parallel()

	// Every tab of a project reads the same directory, and the walk up to it is
	// the read. Rewriting HEAD between two panes of one poll is how the test
	// sees that the second one never reached the disk.
	repo := readstest.Repo(t, "feat/oauth")
	reader := newReader(branchOptions())
	ctx, client := context.Background(), herdrtest.New(nil, nil)

	poll := reader.Poll(client, nil)

	first := paneAt("wE:p1", repo)
	poll.Fill(ctx, first)

	if first.Git.Branch != "feat/oauth" {
		t.Fatalf("branch = %q, want feat/oauth", first.Git.Branch)
	}

	head := filepath.Join(repo, ".git", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/fix/token\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := paneAt("wE:p2", repo)
	poll.Fill(ctx, second)

	if second.Git.Branch != "feat/oauth" {
		t.Errorf("branch = %q, want the answer this poll already had", second.Git.Branch)
	}

	next := paneAt("wE:p3", repo)
	reader.Poll(client, nil).Fill(ctx, next)

	if next.Git.Branch != "fix/token" {
		t.Errorf("branch = %q, want the next poll to read HEAD again", next.Git.Branch)
	}
}

func TestADirectoryHoldingNoRepositoryIsRememberedToo(t *testing.T) {
	t.Parallel()

	// Finding out that there is no repository costs the same walk to the root
	// as finding one, so a pane outside a checkout must not repeat it per tab.
	dir := t.TempDir()
	poll := newReader(branchOptions()).Poll(herdrtest.New(nil, nil), nil)

	first := paneAt("wE:p1", dir)
	poll.Fill(context.Background(), first)

	if first.Git != (git.Checkout{}) {
		t.Fatalf("checkout = %+v, want nothing found", first.Git)
	}

	// A repository under the same directory is what a second walk would find,
	// and a poll that remembered the miss makes none.
	readstest.RepoIn(t, dir, "feat/oauth")

	second := paneAt("wE:p2", dir)
	poll.Fill(context.Background(), second)

	if second.Git != (git.Checkout{}) {
		t.Errorf("checkout = %+v, want the miss this poll already had", second.Git)
	}
}

func TestBranchesSwitchedOffAreNotRead(t *testing.T) {
	t.Parallel()

	// Zero is how a user turns branches off, and a read whose answer is
	// discarded still costs a walk up the tree on every pane, every poll.
	pane := paneAt("wE:p1", readstest.Repo(t, "feat/oauth"))
	readOne(reads.Options{}, pane)

	if pane.Git != (git.Checkout{}) {
		t.Errorf("checkout = %+v, want nothing read", pane.Git)
	}
}

func TestAPollPastItsDeadlineStopsReadingTheFilesystem(t *testing.T) {
	t.Parallel()

	// git.Read and the transcript reader take no context: they are file reads,
	// and a pane sitting on a hung mount blocks the whole loop for as long as
	// the mount does. A poll the tab loop will throw away makes none of them.
	repo := readstest.Repo(t, "feat/oauth")
	reader := newReader(branchOptions())
	client := herdrtest.New(nil, nil)

	// The live read first, so a checkout that never resolves cannot make the
	// spent one below look like the guard working.
	live := paneAt("wE:p1", repo)
	reader.Poll(client, nil).Fill(context.Background(), live)

	if got := live.Git.Branch; got != "feat/oauth" {
		t.Fatalf("branch = %q with time left, so this test proves nothing", got)
	}

	spent, cancel := context.WithCancel(context.Background())
	cancel()

	late := paneAt("wE:p1", repo)
	reader.Poll(client, nil).Fill(spent, late)

	if got := late.Git; got != (git.Checkout{}) {
		t.Errorf("checkout = %+v, want a poll past its deadline to read nothing", got)
	}
}

func TestAPaneIsReadFromItsForegroundProcessesDirectory(t *testing.T) {
	t.Parallel()

	// Both directories the snapshot carries point at a server the agent spawned
	// elsewhere, so a checkout read from either finds no repository at all.
	repo := readstest.Repo(t, "feat/oauth")
	elsewhere := t.TempDir()

	info := herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Agent: "claude",
		CWD: elsewhere, ForegroundCWD: elsewhere,
	}

	client := herdrtest.New([]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}}, []herdr.PaneInfo{info})
	client.SetProcesses(
		"wE:p1",
		herdr.PaneProcessInfoProcess{Name: "gimp-mcp", CWD: elsewhere},
		herdr.PaneProcessInfoProcess{Name: "claude", CWD: repo},
	)

	pane := state.PaneFrom(info, time.Time{})
	newReader(branchOptions()).Poll(client, []herdr.PaneInfo{info}).Fill(context.Background(), pane)

	if pane.Dir != repo {
		t.Errorf("dir = %q, want the agent's own %q", pane.Dir, repo)
	}

	if got := pane.Git.Branch; got != "feat/oauth" {
		t.Errorf("branch = %q, want the checkout of the directory the pane is in", got)
	}
}

func TestTwoAgentsOnTwoWorktreesOfOneRepositoryShowDifferentBranches(t *testing.T) {
	t.Parallel()

	// The panes share a project and are told apart only by the worktree each
	// agent was sent into, which is the whole point of reading it.
	repo := readstest.Repo(t, "main")
	oauth := readstest.Worktree(t, repo, "oauth", "feat/oauth")
	token := readstest.Worktree(t, repo, "token", "fix/token")

	home := t.TempDir()
	readstest.Transcript(t, home, testSession, readstest.AgentIn(oauth))
	readstest.Transcript(t, home, otherSession, readstest.AgentIn(token))

	poll := newReader(transcriptOptions(home)).Poll(herdrtest.New(nil, nil), nil)

	first := agentPaneAt("wE:p1", testSession, repo)
	second := agentPaneAt("wE:p2", otherSession, repo)

	poll.Fill(context.Background(), first)
	poll.Fill(context.Background(), second)

	if got := branchFor(first); got != "feat/oauth" {
		t.Errorf("first branch = %q, want feat/oauth", got)
	}

	if got := branchFor(second); got != "fix/token" {
		t.Errorf("second branch = %q, want fix/token", got)
	}
}

func TestAnAgentInAnotherRepositoryIsIgnored(t *testing.T) {
	t.Parallel()

	// A branch from somewhere else beside this project's name is a wrong
	// label, and worse than the pane having none.
	repo := readstest.Repo(t, "feat/oauth")
	unrelated := readstest.Repo(t, "fix/token")

	home := transcript(t, readstest.AgentIn(unrelated))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestAnAgentInNoRepositoryKeepsThePanesBranch(t *testing.T) {
	t.Parallel()

	// An agent that ended up in a home directory or a scratch directory must
	// not cost the pane the branch it already had.
	repo := readstest.Repo(t, "feat/oauth")

	home := transcript(t, readstest.AgentIn(t.TempDir()))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestAnAgentInASubdirectoryReadsTheCheckoutAboveIt(t *testing.T) {
	t.Parallel()

	// A directory below a checkout is still that checkout, so an agent working
	// in one names its branch — here a worktree's, where the pane's own
	// directory is the trunk and says nothing.
	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "wt", "feat/oauth")

	nested := filepath.Join(worktree, "internal", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	home := transcript(t, readstest.AgentIn(nested))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the worktree the subdirectory sits in", got)
	}
}

func TestAWorktreeTakenOffDiskLeavesThePaneItsOwnBranch(t *testing.T) {
	t.Parallel()

	// The directory the transcript names is gone, so the agent has nothing to
	// say and the branch the pane sits on is the one the tab keeps.
	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "wt", "feat/oauth")

	home := transcript(t, readstest.AgentIn(filepath.Join(repo, ".claude", "worktrees", "gone")))

	pane := agentPaneAt("wE:p1", testSession, worktree)
	readOne(transcriptOptions(home), pane)

	if pane.AgentGit != (git.Checkout{}) {
		t.Errorf("agent checkout = %+v, want nothing read", pane.AgentGit)
	}

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the branch the pane sits on", got)
	}
}

func TestAWorktreeTakenOffDiskIsNotReadAtAll(t *testing.T) {
	t.Parallel()

	// The walk up from a removed worktree answers with the repository above it,
	// whose branch is not the agent's, so the directory is refused for being
	// gone rather than for what it would have said.
	repo := readstest.Repo(t, "main")
	above := readstest.Worktree(t, repo, "above", "feat/oauth")
	gone := filepath.Join(above, ".claude", "worktrees", "gone")

	home := transcript(t, readstest.AgentIn(gone))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(transcriptOptions(home), pane)

	if pane.AgentGit != (git.Checkout{}) {
		t.Errorf("agent checkout = %+v, want the gone directory refused", pane.AgentGit)
	}

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none — feat/oauth is the worktree above the gone one", got)
	}
}

func TestAPaneOnAWorktreeKeepsItsBranchWhenItsAgentWalkedUp(t *testing.T) {
	t.Parallel()

	// The agent's directory is the repository root, whose trunk the branch
	// source then suppresses, so taking it would delete a segment the pane
	// shows today.
	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "wt", "feat/oauth")

	home := transcript(t, readstest.AgentIn(repo))

	pane := agentPaneAt("wE:p1", testSession, worktree)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own worktree branch", got)
	}
}

func TestAnAgentOnTheTrunkKeepsThePanesBranch(t *testing.T) {
	t.Parallel()

	repo := readstest.Repo(t, "feat/oauth")
	worktree := readstest.Worktree(t, repo, "wt", "main")

	home := transcript(t, readstest.AgentIn(worktree))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestADetachedAgentWorktreeShowsItsShortHash(t *testing.T) {
	t.Parallel()

	// A detached HEAD is where commits get lost, so it is worth the segment
	// even in a repository that records no trunk to compare it against.
	for _, remote := range []bool{true, false} {
		repo := readstest.Repo(t, "main")
		worktree := readstest.Worktree(t, repo, "wt", "side")

		head := filepath.Join(repo, ".git", "worktrees", "wt", "HEAD")
		if err := os.WriteFile(
			head,
			[]byte("aaf1fd85f68047764760489dbfc3ecb5ab9d0cb8\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}

		if !remote {
			origin := filepath.Join(repo, ".git", "refs", "remotes", "origin", "HEAD")
			if err := os.Remove(origin); err != nil {
				t.Fatal(err)
			}
		}

		home := transcript(t, readstest.AgentIn(worktree))

		pane := agentPaneAt("wE:p1", testSession, repo)
		readOne(transcriptOptions(home), pane)

		if got := branchFor(pane); got != "aaf1fd8" {
			t.Errorf("remote %v: branch = %q, want aaf1fd8", remote, got)
		}
	}
}

func TestTranscriptsSwitchedOffLeaveTheBranchOnThePanesDirectory(t *testing.T) {
	t.Parallel()

	// The transcript is what says where the agent is, so a user who turned it
	// off is named exactly as before the branch ever followed one.
	repo := readstest.Repo(t, "feat/oauth")
	readstest.Worktree(t, repo, "wt", "fix/token")

	home := transcript(t, readstest.AgentIn(filepath.Join(repo, ".claude", "worktrees", "wt")))

	opts := branchOptions()
	opts.ClaudeDirs = []string{home}

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(opts, pane)

	if pane.AgentGit != (git.Checkout{}) {
		t.Errorf("agent checkout = %+v, want nothing read", pane.AgentGit)
	}

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestBranchesSwitchedOffReadNeitherDirectory(t *testing.T) {
	t.Parallel()

	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "wt", "feat/oauth")

	home := transcript(t, readstest.AgentIn(worktree))

	opts := transcriptOptions(home)
	opts.BranchMax = 0

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(opts, pane)

	if pane.Git != (git.Checkout{}) || pane.AgentGit != (git.Checkout{}) {
		t.Errorf(
			"checkouts = %+v and %+v, want neither directory read",
			pane.Git, pane.AgentGit,
		)
	}
}

func TestAPaneWhoseAgentHoldsNoSessionKeepsItsBranch(t *testing.T) {
	t.Parallel()

	// Herdr reports no session until that agent's integration hook is
	// installed, and a pane with no agent at all never had one.
	repo := readstest.Repo(t, "feat/oauth")

	home := transcript(t, readstest.AgentIn(readstest.Worktree(t, repo, "wt", "fix/token")))

	opts := transcriptOptions(home)

	for name, pane := range map[string]*state.PaneState{
		"no agent": paneAt("wE:p1", repo),
		"no session": state.PaneFrom(
			herdr.PaneInfo{PaneID: "wE:p2", TabID: "wE:t1", CWD: repo, Agent: "claude"},
			time.Time{},
		),
	} {
		readOne(opts, pane)

		if pane.AgentGit != (git.Checkout{}) {
			t.Errorf("%s: agent checkout = %+v, want none read", name, pane.AgentGit)
		}

		if got := branchFor(pane); got != "feat/oauth" {
			t.Errorf("%s: branch = %q, want the pane's own", name, got)
		}
	}
}

func TestOneRepositorySpelledTwoWaysDoesNotFollowTheAgent(t *testing.T) {
	t.Parallel()

	// Both directories are the same repository, but the pane reaches it through
	// a symlink and the worktree records the real path, so the two common
	// directories do not match and the pane keeps its own branch.
	repo := readstest.Repo(t, "feat/oauth")
	worktree := readstest.Worktree(t, repo, "wt", "fix/token")

	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}

	home := transcript(t, readstest.AgentIn(worktree))

	pane := agentPaneAt("wE:p1", testSession, alias)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestAPaneOutsideARepositoryFollowsNoAgent(t *testing.T) {
	t.Parallel()

	// An agent's directory follows every cd it makes, and one outside the tree
	// the pane sits in could only be labelling someone else's project.
	repo := readstest.Repo(t, "main")

	home := transcript(t, readstest.AgentIn(readstest.Worktree(t, repo, "wt", "feat/oauth")))

	pane := agentPaneAt("wE:p1", testSession, t.TempDir())
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none", got)
	}
}

func TestAnAgentOnATrunkNamesNoBranchForAPaneOutsideARepository(t *testing.T) {
	t.Parallel()

	// A trunk says nothing wherever it is read, and the pane has no branch of
	// its own for it to replace either.
	parent := t.TempDir()
	repo := readstest.RepoIn(t, filepath.Join(parent, "dashboard"), "feat/oauth")

	home := transcript(t, readstest.AgentIn(readstest.Worktree(t, repo, "wt", "main")))

	pane := agentPaneAt("wE:p1", testSession, parent)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none", got)
	}
}

func TestAPaneInARepositoryRefusesAnAgentInOneNestedUnderIt(t *testing.T) {
	t.Parallel()

	// Containment is what lets a pane with no checkout follow its agent, and a
	// pane that has one must not gain a nested clone's branch through it.
	repo := readstest.Repo(t, "feat/oauth")
	nested := readstest.RepoIn(t, filepath.Join(repo, "vendor", "other"), "fix/token")

	home := transcript(t, readstest.AgentIn(nested))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestAPaneOutsideARepositoryRefusesAnAgentOutsideOneToo(t *testing.T) {
	t.Parallel()

	// Neither directory holds a repository, so there is no branch anywhere to
	// name and the walk up must not answer with one from above the pane.
	parent := t.TempDir()

	scratch := filepath.Join(parent, "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}

	home := transcript(t, readstest.AgentIn(scratch))

	pane := agentPaneAt("wE:p1", testSession, parent)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none", got)
	}
}

func TestADirectoryNamedLikeThePanesIsNotInsideIt(t *testing.T) {
	t.Parallel()

	// A sibling whose name begins with the pane's own would pass a prefix test
	// on the spelling, and its branch belongs to a tree the pane does not hold.
	parent := t.TempDir()

	own := filepath.Join(parent, "code")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}

	sibling := readstest.RepoIn(t, filepath.Join(parent, "code-review"), "feat/oauth")

	home := transcript(t, readstest.AgentIn(sibling))

	pane := agentPaneAt("wE:p1", testSession, own)
	readOne(transcriptOptions(home), pane)

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none", got)
	}
}
