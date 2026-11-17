package reads_test

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/files/filestest"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/reads"
	"github.com/kryptamine/herdr-auto-title/internal/reads/readstest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

const remotePID = 4242

// remoteClient routes each machine's batches to a machine whose files are
// this computer's, so a test lays them down as it does local ones.
type remoteClient struct {
	*herdrtest.Client

	machines map[string]*filestest.Machine
}

func (c *remoteClient) ExecMachine(
	ctx context.Context, machine string, argv []string, input []byte,
) ([]byte, error) {
	return c.machines[machine].Exec(ctx, argv, input)
}

// remoteInfo is a pane on machine sitting in the local directory dir, which
// the machine names its own way.
func remoteInfo(paneID, machine, dir string) herdr.PaneInfo {
	return herdr.PaneInfo{
		PaneID:  paneID,
		TabID:   machine + "/tb",
		CWD:     filestest.Remote(dir),
		Agent:   "codex",
		Machine: machine,
	}
}

func newRemoteClient(infos ...herdr.PaneInfo) *remoteClient {
	client := &remoteClient{
		Client:   herdrtest.New(nil, infos),
		machines: make(map[string]*filestest.Machine),
	}

	for _, info := range infos {
		client.SetProcesses(info.PaneID, herdr.PaneProcessInfoProcess{
			PID: remotePID, Name: "codex", Argv: []string{"codex"}, CWD: info.CWD,
		})

		if client.machines[info.Machine] == nil {
			client.machines[info.Machine] = filestest.New()
		}
	}

	return client
}

// fillUntilRead polls until the remote pane's read sees every file it asks
// for, which takes a batch per step of the walk: a Codex session's takes about ten.
func fillUntilRead(
	t *testing.T,
	reader *reads.Reader,
	client herdr.Client,
	info herdr.PaneInfo,
) *state.PaneState {
	t.Helper()

	for range 20 {
		pane := state.PaneFrom(info, "", time.Time{})
		if reader.Poll(client, []herdr.PaneInfo{info}, nil).Fill(t.Context(), pane) {
			return pane
		}
	}

	t.Fatal("the remote read never saw its files")

	return nil
}

func TestARemoteBranchIsReadByTheLocalGitReader(t *testing.T) {
	t.Parallel()

	main := readstest.Repo(t, "main")
	tree := readstest.Worktree(t, main, "wt", "feature/remote")

	// The machine reads the gitdir line as it would its own path.
	gitDir := filepath.Join(main, ".git", "worktrees", "wt")
	if err := os.WriteFile(
		filepath.Join(tree, ".git"),
		[]byte("gitdir: "+filestest.Remote(gitDir)+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	info := remoteInfo("box/p", "box", tree)
	client := newRemoteClient(info)

	pane := fillUntilRead(t, newReader(branchOptions()), client, info)

	if pane.Git.Branch != "feature/remote" || pane.Git.Default != "main" {
		t.Errorf("remote checkout = %+v, want the worktree's branch and trunk", pane.Git)
	}
}

func TestARemotePaneIsNotReadUntilItsFilesAre(t *testing.T) {
	t.Parallel()

	// Naming the tab from a half-read checkout would rename it again a poll
	// later, once the rest arrived.
	info := remoteInfo("box/p", "box", readstest.Repo(t, "main"))
	client := newRemoteClient(info)
	pane := state.PaneFrom(info, "", time.Time{})

	if newReader(
		branchOptions(),
	).Poll(client, []herdr.PaneInfo{info}, nil).
		Fill(t.Context(), pane) {
		t.Errorf("a pane whose files were never fetched was read: %+v", pane.Git)
	}
}

func TestTwoRemotePanesInOneDirectoryWaitForTheWholeCheckout(t *testing.T) {
	t.Parallel()

	// The second pane's checkout comes from the poll's memo, which must not
	// hold a walk the machine has not answered whole.
	dir := readstest.Repo(t, "main")
	first, second := remoteInfo("box/p1", "box", dir), remoteInfo("box/p2", "box", dir)
	client := newRemoteClient(first, second)
	reader := newReader(branchOptions())

	for range 8 {
		poll := reader.Poll(client, []herdr.PaneInfo{first, second}, nil)
		one, two := state.PaneFrom(first, "", time.Time{}), state.PaneFrom(second, "", time.Time{})
		readOne, readTwo := poll.Fill(t.Context(), one), poll.Fill(t.Context(), two)

		if readOne != readTwo {
			t.Fatalf("panes in one directory disagree on being read: %v and %v", readOne, readTwo)
		}

		if readTwo && two.Git.Branch != "main" {
			t.Fatalf("a pane read whole has branch %q", two.Git.Branch)
		}
	}
}

func TestARemoteReadCostsOneBatchPerRefresh(t *testing.T) {
	t.Parallel()

	info := remoteInfo("box/p", "box", readstest.Repo(t, "main"))
	client := newRemoteClient(info)
	reader := newReader(branchOptions())

	fillUntilRead(t, reader, client, info)
	settled := client.machines["box"].Calls()

	for range 10 {
		pane := state.PaneFrom(info, "", time.Time{})
		reader.Poll(client, []herdr.PaneInfo{info}, nil).Fill(t.Context(), pane)

		if pane.Git.Branch != "main" {
			t.Fatalf("a settled read lost its branch: %+v", pane.Git)
		}
	}

	if calls := client.machines["box"].Calls(); calls != settled {
		t.Errorf("batches after the read settled = %d, want none before the refresh", calls-settled)
	}
}

func TestAFailedRemoteBatchKeepsTheLastRead(t *testing.T) {
	t.Parallel()

	info := remoteInfo("box/p", "box", readstest.Repo(t, "main"))
	client := newRemoteClient(info)
	reader := newReader(branchOptions())

	fillUntilRead(t, reader, client, info)
	client.machines["box"].SetDown(true)

	// A new directory is a question the machine cannot answer now.
	info.CWD = filestest.Remote(readstest.Repo(t, "other"))
	client.SetProcesses(info.PaneID, herdr.PaneProcessInfoProcess{
		PID: remotePID, Name: "codex", Argv: []string{"codex"}, CWD: info.CWD,
	})

	for range 3 {
		pane := state.PaneFrom(info, "", time.Time{})
		if !reader.Poll(client, []herdr.PaneInfo{info}, nil).Fill(t.Context(), pane) ||
			pane.Git.Branch != "main" {
			t.Fatalf("a dropped connection changed the pane: %+v", pane.Git)
		}
	}
}

func TestAMachineWithNoHelperNamesFromItsMetadata(t *testing.T) {
	t.Parallel()

	info := remoteInfo("box/p", "box", readstest.Repo(t, "main"))
	client := newRemoteClient(info)
	client.machines["box"].SetDown(true)

	reader := newReader(branchOptions())

	reader.Poll(client, []herdr.PaneInfo{info}, nil).
		Fill(t.Context(), state.PaneFrom(info, "", time.Time{}))

	pane := state.PaneFrom(info, "", time.Time{})
	if !reader.Poll(client, []herdr.PaneInfo{info}, nil).Fill(t.Context(), pane) {
		t.Error("a machine that cannot answer kept its pane unread")
	}

	if pane.Git.Branch != "" {
		t.Errorf("branch = %q from a machine that never answered", pane.Git.Branch)
	}
}

func TestIdenticalPathsOnTwoMachinesAreTwoRepositories(t *testing.T) {
	t.Parallel()

	dir := readstest.Repo(t, "main")
	box, other := remoteInfo("box/p", "box", dir), remoteInfo("other/p", "other", dir)
	client := newRemoteClient(box, other)
	reader := newReader(branchOptions())

	first := fillUntilRead(t, reader, client, box)
	second := fillUntilRead(t, reader, client, other)

	if first.Git.SameRepository(second.Git) {
		t.Errorf("two machines share a repository: %q", first.Git.CommonDir)
	}
}

func remoteCodex(t *testing.T, name string) (herdr.PaneInfo, *remoteClient, reads.Options, string) {
	t.Helper()

	repo := readstest.Repo(t, "main")
	opts := transcriptOptions()
	home := t.TempDir()
	readstest.CodexRollout(
		t,
		home,
		testSession,
		readstest.CodexStartedIn(
			filestest.Remote(repo),
		),
		readstest.CodexPrompt("rework the poll loop"),
	)

	if name != "" {
		readstest.CodexName(t, home, testSession, name)
	}

	rollouts, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*.jsonl"))
	for i, rollout := range rollouts {
		rollouts[i] = filestest.Remote(rollout)
	}

	info := remoteInfo("box/p", "box", repo)
	client := newRemoteClient(info)
	client.machines["box"].SetProc(remotePID, filestest.Proc{
		Argv: []string{"codex"}, Open: append([]string{"/dev/pts/3"}, rollouts...),
	})

	return info, client, opts, rollouts[0]
}

func TestARemoteCodexSessionIsReadByTheLocalCodexReader(t *testing.T) {
	t.Parallel()

	info, client, opts, _ := remoteCodex(t, "Rework poll loop")
	pane := fillUntilRead(t, newReader(opts), client, info)

	if pane.AgentTopic != "Rework poll loop" {
		t.Errorf("remote topic = %q, want the session's name", pane.AgentTopic)
	}
}

func TestARemoteCodexSessionFallsBackToItsOpeningPrompt(t *testing.T) {
	t.Parallel()

	// The local reader names an unnamed session from how it opened, and a
	// remote one must not be named differently.
	info, client, opts, _ := remoteCodex(t, "")
	pane := fillUntilRead(t, newReader(opts), client, info)

	if pane.AgentTopic != "rework the poll loop" {
		t.Errorf("remote topic = %q, want the opening prompt", pane.AgentTopic)
	}
}

func TestARemoteCodexSessionMustBeHeldByItsOwnProcess(t *testing.T) {
	t.Parallel()

	for name, proc := range map[string]func(rollout string) filestest.Proc{
		"shared daemon": func(rollout string) filestest.Proc {
			return filestest.Proc{Argv: []string{"codex", "app-server"}, Open: []string{rollout}}
		},
		"two rollouts": func(rollout string) filestest.Proc {
			return filestest.Proc{
				Argv: []string{"codex"},
				Open: []string{rollout, path.Join(path.Dir(rollout), "rollout-x-"+otherSession+".jsonl")},
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			info, client, opts, rollout := remoteCodex(t, "Rework poll loop")
			client.machines["box"].SetProc(remotePID, proc(rollout))

			if pane := fillUntilRead(t, newReader(opts), client, info); pane.AgentTopic != "" {
				t.Errorf("topic = %q from a session the pane does not own", pane.AgentTopic)
			}
		})
	}
}

func TestRemoteReadsSwitchedOffAskTheMachineNothing(t *testing.T) {
	t.Parallel()

	info, client, _, _ := remoteCodex(t, "Rework poll loop")
	reader := newReader(reads.Options{})

	for range 3 {
		pane := state.PaneFrom(info, "", time.Time{})
		if !reader.Poll(client, []herdr.PaneInfo{info}, nil).Fill(t.Context(), pane) {
			t.Fatal("a pane with nothing to read was left unread")
		}
	}

	if calls := client.machines["box"].Calls(); calls != 0 {
		t.Errorf("batches with every read switched off = %d", calls)
	}
}

// A connected machine's paths and sessions can match this computer's by chance,
// and reading them here would name a remote tab after a local checkout.
func TestRemoteReadsNeverUseTheLocalCheckoutOrTranscript(t *testing.T) {
	t.Parallel()

	dir := readstest.Repo(t, "local-only")
	home := transcript(
		t,
		"{\"type\":\"ai-title\",\"aiTitle\":\"local-only topic\",\"sessionId\":\""+testSession+"\"}",
	)
	info := herdr.PaneInfo{
		PaneID: "box/p", TabID: "box/tb", CWD: dir, Agent: "claude", Machine: "box",
		AgentSession: &herdr.AgentSessionInfo{
			Agent: "claude", Kind: herdr.SessionRefID, Value: testSession,
		},
	}
	client := herdrtest.New(nil, []herdr.PaneInfo{info})
	client.SetProcesses(info.PaneID, herdr.PaneProcessInfoProcess{
		PID: 123, Name: "claude", Argv: []string{"claude"}, CWD: dir,
	})

	pane := state.PaneFrom(info, "", time.Time{})
	if !newReader(transcriptOptions(home)).Poll(client, []herdr.PaneInfo{info}, nil).
		Fill(t.Context(), pane) {
		t.Fatal("remote foreground was not read")
	}

	if pane.Dir != dir || len(pane.Processes) != 1 {
		t.Errorf("remote terminal metadata was lost: %+v", pane)
	}

	if pane.AgentTopic != "" || pane.Git.Branch != "" || pane.AgentGit.Branch != "" {
		t.Errorf(
			"remote pane read local files: topic %q, branch %q",
			pane.AgentTopic,
			pane.Git.Branch,
		)
	}
}
