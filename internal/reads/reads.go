// Package reads supplies what a session snapshot cannot say about a pane: what
// it is running and the directory that puts it in, what its agent's session is
// about, and what the repositories under both have checked out.
package reads

import (
	"context"
	"log/slog"

	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// Options are the settings that decide whether a read is made at all, rather
// than how its answer is shown.
type Options struct {
	// ClaudeDirs are the Claude Code configuration homes transcripts are found in.
	ClaudeDirs []string
	// BranchMax of zero or less turns branches off, and then no checkout is read.
	BranchMax       int
	ReadTranscripts bool
}

// Reader outlives a poll because what it remembers does: what each pane was
// last seen running, and how far each agent's transcript has been read.
type Reader struct {
	processes       *processCache
	topics          *claude.Reader
	log             *slog.Logger
	readBranches    bool
	readTranscripts bool
}

func New(opts Options, log *slog.Logger) *Reader {
	return &Reader{
		processes:       newProcessCache(),
		topics:          claude.NewReader(opts.ClaudeDirs...),
		log:             log,
		readBranches:    opts.BranchMax > 0,
		readTranscripts: opts.ReadTranscripts,
	}
}

// Poll opens the reads of one poll over the snapshot's panes, forgetting what
// was read of a pane whose revision moved and of the panes and agent sessions
// the snapshot no longer holds.
func (r *Reader) Poll(client herdr.Client, panes []herdr.PaneInfo) *Poll {
	r.processes.observe(panes)
	r.topics.Retain(sessionsIn(panes))

	return &Poll{
		reader:    r,
		client:    client,
		checkouts: make(map[string]git.Checkout),
		filled:    make(map[*state.PaneState]struct{}),
	}
}

// Poll is one poll's reads. It is a thing of its own so that what it memoizes
// cannot outlast the poll that filled it.
type Poll struct {
	reader    *Reader
	client    herdr.Client
	checkouts map[string]git.Checkout
	filled    map[*state.PaneState]struct{}
}

// Fill supplies what the snapshot could not say about a pane, once per poll
// however often it is asked. A pane nobody fills keeps what the snapshot said.
func (p *Poll) Fill(ctx context.Context, pane *state.PaneState) {
	if pane == nil {
		return
	}

	if _, done := p.filled[pane]; done {
		return
	}

	p.filled[pane] = struct{}{}

	processes := p.reader.processesOf(ctx, p.client, pane.ID)
	dir := state.PaneDir(processes, pane.Dir)

	pane.Processes = state.ProcessesFrom(processes)
	pane.Dir = dir

	// The transcript is read before the checkout because it says where the
	// agent is working, and that is where the branch is read from.
	topic := p.reader.topic(ctx, pane, dir)
	pane.AgentTopic = topic.Text()
	pane.Git = p.checkout(ctx, dir)
	pane.AgentGit = p.checkout(ctx, topic.Dir)
	pane.AgentDir = topic.Dir
}

// processesOf reports what a pane is running, reusing the last read while the
// pane's revision holds and that read is recent. Neither test is exact, which
// is why there are two — see docs/architecture/poll-loop.md.
func (r *Reader) processesOf(
	ctx context.Context,
	client herdr.Client,
	paneID string,
) []herdr.PaneProcessInfoProcess {
	if processes, read := r.processes.lookup(paneID); read {
		return processes
	}

	processes, err := herdr.PaneProcesses(ctx, client, paneID)
	if err != nil {
		if herdr.ErrorCode(err) != herdr.CodePaneNotFound && ctx.Err() == nil {
			r.log.Debug(
				"could not read what a pane is running",
				"pane_id", paneID,
				"error", err,
			)
		}

		return nil
	}

	r.processes.record(paneID, processes)

	return processes
}

// checkout reports what the repository holding dir has checked out, going to
// the filesystem at most once per directory. A directory holding no repository
// is remembered too: finding that out costs the same walk as finding one.
func (p *Poll) checkout(ctx context.Context, dir string) git.Checkout {
	// A read whose answer is thrown away is still a read on every pane twice a
	// second.
	if !p.reader.readBranches || spent(ctx) {
		return git.Checkout{}
	}

	if checkout, known := p.checkouts[dir]; known {
		return checkout
	}

	checkout := git.Read(dir)
	p.checkouts[dir] = checkout

	return checkout
}

// topic reports what the session the pane's agent is holding says it is about,
// and where that agent is working. Only Claude Code's transcripts are
// understood, and only Herdr's integration hook says which session a pane holds.
func (r *Reader) topic(ctx context.Context, pane *state.PaneState, dir string) claude.Topic {
	if !r.readTranscripts || spent(ctx) {
		return claude.Topic{}
	}

	sessionID, ok := pane.AgentSession.IDFor(claude.Agent)
	if !ok {
		return claude.Topic{}
	}

	return r.topics.Topic(sessionID, dir)
}

// spent reports that the poll is past its deadline. The reads it guards go to
// the filesystem, which takes no context, so the only way to bound them is not
// to start them.
func spent(ctx context.Context) bool {
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
