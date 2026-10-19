// Package reads supplies what a session snapshot cannot say about a pane: what
// it is running and the directory that puts it in, what its agent's session is
// about, and what the repositories under both have checked out.
package reads

import (
	"context"
	"log/slog"

	"github.com/kryptamine/herdr-auto-title/internal/antigravity"
	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/grok"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/pi"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// Options are the settings that decide whether a read is made at all, rather
// than how its answer is shown.
type Options struct {
	// ClaudeDirs are the Claude Code configuration homes transcripts are found in.
	ClaudeDirs []string
	// Home is the user's home directory, where Antigravity CLI keeps its state.
	Home string
	// GrokHome is the Grok Build home session summaries are found in.
	GrokHome string
	// BranchMax of zero or less turns branches off, and then no checkout is read.
	BranchMax    int
	ReadSessions bool
}

// Reader outlives a poll because what it remembers does: what each pane was
// last seen running, and how far each agent's transcript has been read.
type Reader struct {
	processes         *processCache
	claudeTopics      *claude.Reader
	antigravityTitles *antigravity.Reader
	grokTitles        *grok.Reader
	piTopics          *pi.Reader
	log               *slog.Logger
	readBranches      bool
	readSessions      bool
}

func New(opts Options, log *slog.Logger) *Reader {
	return &Reader{
		processes:         newProcessCache(),
		claudeTopics:      claude.NewReader(opts.ClaudeDirs...),
		antigravityTitles: antigravity.NewReader(opts.Home),
		grokTitles:        grok.NewReader(opts.GrokHome),
		piTopics:          pi.NewReader(),
		log:               log,
		readBranches:      opts.BranchMax > 0,
		readSessions:      opts.ReadSessions,
	}
}

// Poll opens the reads of one poll over the snapshot's panes, forgetting what
// was read of the panes that drew and of the panes and agent sessions the
// snapshot no longer holds.
func (r *Reader) Poll(client herdr.Client, panes []herdr.PaneInfo, drew map[string]bool) *Poll {
	r.processes.observe(panes, drew)

	// Each reader keeps only the references it knows, so all are handed every
	// session the snapshot holds without being told which is whose.
	held := sessionsIn(panes)
	r.claudeTopics.Retain(held)
	r.grokTitles.Retain(held)
	r.piTopics.Retain(held)

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

	// A read that failed with time left is asked again by the next Fill, so a
	// second reader of the pane in this poll is not left with the snapshot's guess.
	processes, read := p.reader.processesOf(ctx, p.client, pane.ID)
	if read || spent(ctx) {
		p.filled[pane] = struct{}{}
	}

	dir := state.PaneDir(processes, pane.Dir)

	pane.Processes = state.ProcessesFrom(processes)
	pane.Dir = dir

	// The agent's session is read before the checkout because it can say where
	// the agent is working, and that is where the branch is read from.
	topic := p.reader.topic(ctx, pane, dir)
	pane.AgentTopic = topic.text
	pane.Git = p.checkout(ctx, dir)
	pane.AgentGit = p.checkout(ctx, topic.dir)
	pane.AgentDir = topic.dir
}

// processesOf reports what a pane is running, or false when Herdr gave no
// answer. A read is reused while the pane has not drawn and the read is recent;
// neither test is exact alone — see docs/architecture/poll-loop.md.
func (r *Reader) processesOf(
	ctx context.Context,
	client herdr.Client,
	paneID string,
) ([]herdr.PaneProcessInfoProcess, bool) {
	if processes, read := r.processes.lookup(paneID); read {
		return processes, true
	}

	processes, err := herdr.PaneProcesses(ctx, client, paneID)
	if err != nil {
		// A pane that closed is an answer, and asking again cannot change it.
		if herdr.ErrorCode(err) == herdr.CodePaneNotFound {
			return nil, true
		}

		if ctx.Err() == nil {
			r.log.Debug(
				"could not read what a pane is running",
				"pane_id", paneID,
				"error", err,
			)
		}

		return nil, false
	}

	r.processes.record(paneID, processes)

	return processes, true
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

// sessionTopic is what an agent's own session says it is about, and where the
// agent works when the session says so.
type sessionTopic struct {
	text string
	dir  string
}

// topic reports what the session the pane's agent is holding says it is about.
// Only Herdr's integration hook says which session a pane holds.
func (r *Reader) topic(ctx context.Context, pane *state.PaneState, dir string) sessionTopic {
	if !r.readSessions || spent(ctx) {
		return sessionTopic{}
	}

	if pane.AgentSession == nil {
		return sessionTopic{}
	}

	switch pane.AgentSession.Agent {
	case claude.Agent:
		sessionID, ok := pane.AgentSession.ID()
		if !ok {
			return sessionTopic{}
		}

		topic := r.claudeTopics.Topic(sessionID, dir)

		return sessionTopic{text: topic.Text(), dir: topic.Dir}
	case antigravity.Agent:
		sessionID, ok := pane.AgentSession.ID()
		if !ok {
			return sessionTopic{}
		}

		// An annotation says nothing of where the agent works, so the pane's own
		// directory keeps the branch.
		return sessionTopic{text: r.antigravityTitles.Title(sessionID)}
	case grok.Agent:
		sessionID, ok := pane.AgentSession.ID()
		if !ok {
			return sessionTopic{}
		}

		return sessionTopic{text: r.grokTitles.Title(sessionID, dir)}
	case pi.Agent:
		path, ok := pane.AgentSession.Path()
		if !ok {
			return sessionTopic{}
		}

		topic := r.piTopics.Topic(path)

		return sessionTopic{text: topic.Text(), dir: topic.Dir}
	}

	return sessionTopic{}
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
