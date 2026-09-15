package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// tabsIn assembles the snapshot's tabs with their panes, and reads nothing:
// what a pane is running, has checked out and is talking about each costs a
// request or a file, and paneReads spends that on the panes that earn it.
func (a *App) tabsIn(snapshot herdr.Snapshot) []state.TabState {
	workspaces := make(map[string]string, len(snapshot.Workspaces))

	for _, workspace := range snapshot.Workspaces {
		workspaces[workspace.WorkspaceID] = workspace.Label
	}

	byTab := make(map[string][]*state.PaneState, len(snapshot.Tabs))

	for _, pane := range snapshot.Panes {
		byTab[pane.TabID] = append(
			byTab[pane.TabID],
			state.PaneFrom(pane, a.changes.ChangedAt(pane.PaneID)),
		)
	}

	// An unnamed tab carries its place in the workspace, and the snapshot lists
	// tabs in display order, so counting them gives that label.
	positions := make(map[string]int, len(snapshot.Workspaces))
	tabs := make([]state.TabState, 0, len(snapshot.Tabs))

	for _, info := range snapshot.Tabs {
		positions[info.WorkspaceID]++

		tabs = append(
			tabs,
			state.TabFrom(
				info,
				workspaces[info.WorkspaceID],
				positions[info.WorkspaceID],
				byTab[info.TabID],
				a.preferAgent,
			),
		)
	}

	return tabs
}

// paneReader reads what a snapshot cannot say about a pane. It outlives a poll
// because what it remembers does: what each pane was last seen running, and how
// far each agent's transcript has been read.
type paneReader struct {
	changes *state.Changes
	topics  *claude.Reader
	log     *slog.Logger
	// branchMax and readTranscripts are the settings that turn a read off
	// rather than shape it, so they are tested before one is started.
	branchMax       int
	readTranscripts bool
}

func newPaneReader(cfg Config, log *slog.Logger, changes *state.Changes) *paneReader {
	return &paneReader{
		changes:         changes,
		topics:          claude.NewReader(),
		log:             log,
		branchMax:       cfg.BranchMax,
		readTranscripts: cfg.ReadTranscripts,
	}
}

// forPoll opens the reads of one poll, forgetting the agent sessions the
// snapshot no longer holds. The panes come from the snapshot rather than from
// the tabs, because a session is gone once no pane holds it.
func (r *paneReader) forPoll(panes []herdr.PaneInfo) *paneReads {
	r.topics.Retain(sessionsIn(panes))

	return &paneReads{reader: r, checkouts: make(checkoutMemo)}
}

// paneReads are one poll's reads. It is a thing of its own so that what it
// memoizes cannot outlast the poll that filled it.
type paneReads struct {
	reader    *paneReader
	checkouts checkoutMemo
}

// fill supplies what the snapshot could not say about a pane. A pane nobody
// fills keeps what the snapshot said, which is most of them.
func (p *paneReads) fill(ctx context.Context, client herdr.Client, pane *state.PaneState) {
	if pane == nil {
		return
	}

	processes := p.processes(ctx, client, pane.ID)
	dir := state.PaneDir(processes, pane.Dir)

	pane.Processes = state.ProcessesFrom(processes)
	pane.Dir = dir

	// The transcript is read before the checkout because it says where the
	// agent is working, and that is where the branch is read from.
	topic := p.topic(ctx, pane, dir)
	pane.AgentTopic = topic.Text()
	pane.Git = p.checkout(ctx, dir, topic.Dir)
}

// processes reports what a pane is running, reusing the last read while the
// pane's revision holds and that read is recent. Neither test is exact, which
// is why there are two — see docs/architecture/poll-loop.md.
func (p *paneReads) processes(
	ctx context.Context,
	client herdr.Client,
	paneID string,
) []herdr.PaneProcessInfoProcess {
	if processes, read := p.reader.changes.Processes(paneID); read {
		return processes
	}

	processes, err := herdr.PaneProcesses(ctx, client, paneID)
	if err != nil {
		if herdr.ErrorCode(err) != herdr.CodePaneNotFound && ctx.Err() == nil {
			p.reader.log.Debug(
				"could not read what a pane is running",
				"pane_id", paneID,
				"error", err,
			)
		}

		return nil
	}

	p.reader.changes.Ran(paneID, processes)

	return processes
}

// checkout reports what the pane has checked out: the repository holding dir,
// except that agentDir speaks for the branch when the agent is working in
// another view of the same repository. See docs/architecture/title-resolution.md.
func (p *paneReads) checkout(ctx context.Context, dir, agentDir string) git.Checkout {
	// A branch width of zero is how branches are turned off, and a read whose
	// answer is thrown away is still a read on every pane twice a second.
	if p.reader.branchMax <= 0 {
		return git.Checkout{}
	}

	if spentPoll(ctx) {
		return git.Checkout{}
	}

	checkout := p.checkouts.read(dir)
	if agentDir == "" || agentDir == dir || !isDir(agentDir) {
		return checkout
	}

	if agent := p.checkouts.read(agentDir); overridesBranch(checkout, agent) {
		return agent
	}

	return checkout
}

// isDir reports whether dir is still there to be read. A worktree the agent
// left behind is removed while the transcript still names it, and the walk up
// would answer with the ancestor repository instead.
func isDir(dir string) bool {
	info, err := os.Stat(dir)

	return err == nil && info.IsDir()
}

// overridesBranch reports that the agent's checkout speaks for the branch: it
// is another view of the same repository, and it has something to name. Why the
// common directories are not resolved is in docs/architecture/title-resolution.md.
func overridesBranch(own, agent git.Checkout) bool {
	// A pane outside a repository has no common directory to match, which is
	// the gate's boundary rather than a case to widen.
	if agent == (git.Checkout{}) || agent.CommonDir != own.CommonDir {
		return false
	}

	// A detached HEAD always has something to say. A trunk never does, and nor
	// does a branch in a repository recording no default, which cannot be told
	// from that repository's trunk — so the pane's own answer stands instead.
	if agent.Branch == "" {
		return true
	}

	return agent.Branch != agent.Default && (agent.Default != "" || own.Branch == "")
}

// checkoutMemo holds the checkouts one poll has read. The tabs of a project
// share a directory, and the memo dies with the poll, so collapsing their
// reads costs no staleness — see docs/architecture/title-resolution.md.
type checkoutMemo map[string]git.Checkout

// read reports what the repository holding dir has checked out, going to the
// filesystem at most once. A directory that holds no repository is remembered
// too: finding that out costs the same walk as finding one.
func (m checkoutMemo) read(dir string) git.Checkout {
	if checkout, known := m[dir]; known {
		return checkout
	}

	checkout := git.Read(dir)
	m[dir] = checkout

	return checkout
}

// topic reports what the session the pane's agent is holding says it is about,
// and where that agent is working. Only Claude Code's transcripts are
// understood, and only Herdr's integration hook says which session a pane holds.
func (p *paneReads) topic(
	ctx context.Context,
	pane *state.PaneState,
	dir string,
) claude.Topic {
	if !p.reader.readTranscripts || spentPoll(ctx) {
		return claude.Topic{}
	}

	sessionID, ok := pane.AgentSession.IDFor(claude.Agent)
	if !ok {
		return claude.Topic{}
	}

	return p.reader.topics.Topic(sessionID, dir)
}

// spentPoll reports that this poll is past its deadline. The reads it guards
// go to the filesystem, which takes no context, so the only way to bound them
// is not to start them.
func spentPoll(ctx context.Context) bool {
	return ctx.Err() != nil
}

func sessionsIn(panes []herdr.PaneInfo) []string {
	sessions := make([]string, 0, len(panes))
	for _, pane := range panes {
		if pane.AgentSession != nil {
			sessions = append(sessions, pane.AgentSession.Value)
		}
	}

	return sessions
}
