// Package reads supplies what a session snapshot cannot say about a pane: what
// it is running and the directory that puts it in, what its agent's session is
// about, and what the repositories under both have checked out.
package reads

import (
	"context"
	"log/slog"

	"github.com/kryptamine/herdr-auto-title/internal/antigravity"
	"github.com/kryptamine/herdr-auto-title/internal/claude"
	"github.com/kryptamine/herdr-auto-title/internal/codex"
	"github.com/kryptamine/herdr-auto-title/internal/files"
	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/grok"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/hermes"
	"github.com/kryptamine/herdr-auto-title/internal/pi"
	"github.com/kryptamine/herdr-auto-title/internal/state"
	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

// Options are the settings that decide whether a read is made at all, rather
// than how its answer is shown.
type Options struct {
	// HermesHomes are the Hermes homes sessions are searched in, each with its
	// named profiles.
	HermesHomes []string
	// ClaudeDirs are the Claude Code configuration homes transcripts are found in.
	ClaudeDirs []string
	// Home is the user's home directory, where Antigravity CLI keeps its state.
	Home string
	// CodexHome is the Codex home rollouts and the session index are found in.
	CodexHome string
	// GrokHome is the Grok Build home session summaries are found in.
	GrokHome string
	// BranchMax of zero or less turns branches off, and then no checkout is read.
	BranchMax    int
	ReadSessions bool
	SessionOnly  bool
}

// Reader outlives a poll because what it remembers does: what each pane was
// last seen running, how far each agent's transcript has been read, and what
// each connected machine last said of its files.
type Reader struct {
	hermesHomes       []string
	remotes           map[string]*remote
	processes         *processCache
	claudeTopics      *claude.Reader
	antigravityTitles *antigravity.Reader
	grokTitles        *grok.Reader
	piTopics          *pi.Reader
	codexTopics       *codex.Reader
	log               *slog.Logger
	readBranches      bool
	readSessions      bool
	sessionOnly       bool
}

func New(opts Options, log *slog.Logger) *Reader {
	return &Reader{
		hermesHomes:       opts.HermesHomes,
		remotes:           make(map[string]*remote),
		processes:         newProcessCache(),
		claudeTopics:      claude.NewReader(opts.ClaudeDirs...),
		antigravityTitles: antigravity.NewReader(opts.Home),
		grokTitles:        grok.NewReader(opts.GrokHome),
		piTopics:          pi.NewReader(),
		codexTopics:       codex.NewReader(files.Local, opts.CodexHome),
		log:               log,
		readBranches:      opts.BranchMax > 0,
		readSessions:      opts.ReadSessions,
		sessionOnly:       opts.SessionOnly,
	}
}

// Poll opens the reads of one poll over the snapshot's panes, marking what was
// read of the panes that drew as stale and forgetting the panes and agent
// sessions the snapshot no longer holds.
func (r *Reader) Poll(client herdr.Client, panes []herdr.PaneInfo, drew map[string]bool) *Poll {
	r.processes.observe(panes, drew)

	// Each reader keeps only the references it knows, so all are handed every
	// session the snapshot holds without being told which is whose.
	held := sessionsIn(panes)
	r.claudeTopics.Retain(held)
	r.grokTitles.Retain(held)
	r.piTopics.Retain(held)
	r.codexTopics.Retain(held)
	r.retainRemotes(panes)

	return &Poll{
		reader:    r,
		client:    client,
		checkouts: make(map[checkoutKey]git.Checkout),
		filled:    make(map[*state.PaneState]bool),
		fetched:   make(map[string]bool),
	}
}

// Poll is one poll's reads. It is a thing of its own so that what it memoizes
// cannot outlast the poll that filled it.
type Poll struct {
	reader    *Reader
	client    herdr.Client
	checkouts map[checkoutKey]git.Checkout
	filled    map[*state.PaneState]bool
	fetched   map[string]bool
}

// checkoutKey is a directory on a machine, "" being this computer.
type checkoutKey struct{ machine, dir string }

// Fill supplies what the snapshot could not say about a pane, once per poll
// however often it is asked, and reports whether what it runs is known. A pane
// nobody fills keeps what the snapshot said.
func (p *Poll) Fill(ctx context.Context, pane *state.PaneState) bool {
	if pane == nil {
		return false
	}

	if read, done := p.filled[pane]; done {
		return read
	}

	processes, read := p.reader.processesOf(ctx, p.client, pane.ID)
	dir := state.PaneDir(processes, pane.Dir)

	pane.Processes = state.ProcessesFrom(processes)
	pane.Dir = dir

	if pane.Machine == "" {
		// The agent's session is read before the checkout because it can say
		// where the agent is working, and that is where the branch is read from.
		topic := p.reader.topic(ctx, pane, pane.Dir, processes)
		p.fillFiles(ctx, pane, files.Local, topic)
	} else {
		read = p.fillRemote(ctx, pane, processes) && read
	}

	// A read that failed with time left is asked again by the next Fill, so a
	// second reader of the pane in this poll is not left with the snapshot's guess.
	if read || spent(ctx) {
		p.filled[pane] = read
	}

	return read
}

// fillFiles puts on a pane what its agent's session and its checkouts say.
func (p *Poll) fillFiles(
	ctx context.Context,
	pane *state.PaneState,
	fsys files.FS,
	topic sessionTopic,
) {
	pane.AgentTopic = topic.text
	pane.Git = p.checkout(ctx, fsys, pane.Machine, pane.Dir)
	pane.AgentGit = p.checkout(ctx, fsys, pane.Machine, topic.dir)
	pane.AgentDir = topic.dir
}

// fillRemote reads a pane on a connected machine from that machine's files.
// A read that missed some keeps the pane's last whole one, and with none the
// pane is not yet read, so its tab is named once rather than twice.
func (p *Poll) fillRemote(
	ctx context.Context,
	pane *state.PaneState,
	processes []herdr.PaneProcessInfoProcess,
) bool {
	machine := p.remoteSource(ctx, pane.Machine)
	if machine == nil {
		return true
	}

	before := machine.files.Misses()
	p.fillFiles(ctx, pane, machine.files, p.reader.remoteTopic(ctx, machine, pane, processes))

	if machine.files.Misses() == before {
		machine.panes[pane.ID] = remoteRead{
			topic: pane.AgentTopic, git: pane.Git, agentGit: pane.AgentGit, agentDir: pane.AgentDir,
		}

		return true
	}

	last, known := machine.panes[pane.ID]
	if known {
		pane.AgentTopic, pane.Git, pane.AgentGit, pane.AgentDir = last.topic, last.git, last.agentGit, last.agentDir
	}

	return known
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

	processes, err := client.PaneProcesses(ctx, paneID)
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

		// What it ran last is closer to the truth than nothing: a mirror pane
		// read as empty would look local and be named.
		return r.processes.last(paneID)
	}

	r.processes.record(paneID, processes)

	return processes, true
}

// checkout reports what the repository holding dir has checked out, going to
// the filesystem at most once per directory. A directory holding no repository
// is remembered too: finding that out costs the same walk as finding one.
func (p *Poll) checkout(ctx context.Context, fsys files.FS, machine, dir string) git.Checkout {
	// A read whose answer is thrown away is still a read on every pane twice a
	// second.
	if !p.reader.readBranches || spent(ctx) {
		return git.Checkout{}
	}

	key := checkoutKey{machine: machine, dir: dir}
	if checkout, known := p.checkouts[key]; known {
		return checkout
	}

	misses := p.misses(machine)
	checkout := git.Read(fsys, dir)
	// The same path on two machines is two repositories.
	if machine != "" && checkout.CommonDir != "" {
		checkout.CommonDir = machine + "/" + checkout.CommonDir
	}

	// A walk a machine has not answered whole is walked again by the next pane
	// in it, so that pane too knows its read is not whole.
	if p.misses(machine) == misses {
		p.checkouts[key] = checkout
	}

	return checkout
}

// sessionTopic is what an agent's own session says it is about, and where the
// agent works when the session says so.
type sessionTopic struct {
	text string
	dir  string
}

// topic reports what the session the pane's agent is holding says it is about.
// Only the snapshot says which session a pane holds.
func (r *Reader) topic(
	ctx context.Context,
	pane *state.PaneState,
	dir string,
	processes []herdr.PaneProcessInfoProcess,
) sessionTopic {
	if !r.readSessions || spent(ctx) {
		return sessionTopic{}
	}

	if pane.AgentSession == nil {
		return sessionTopic{}
	}

	if pane.AgentSession.Agent == hermes.Agent {
		return r.hermesTopic(ctx, pane, processes)
	}
	// Pi's session arrives as a path; every other agent's as an id.
	if pane.AgentSession.Agent == pi.Agent {
		path, ok := pane.AgentSession.Path()
		if !ok {
			return sessionTopic{}
		}

		return r.namedTopic(r.piTopics.Topic(path))
	}

	sessionID, ok := pane.AgentSession.ID()
	if !ok {
		return sessionTopic{}
	}

	switch pane.AgentSession.Agent {
	case claude.Agent:
		topic := r.claudeTopics.Topic(sessionID, dir)
		if r.sessionOnly {
			topic.Opening = ""
		}

		return sessionTopic{text: topic.Text(), dir: topic.Dir}
	case antigravity.Agent:
		// An annotation says nothing of where the agent works, so the pane's own
		// directory keeps the branch.
		return sessionTopic{text: r.antigravityTitles.Title(sessionID)}
	case grok.Agent:
		return sessionTopic{text: r.grokTitles.Title(sessionID, dir)}
	case codex.Agent:
		topic := r.codexTopics.Topic(sessionID)
		if !r.codexTopics.Shows(pane.TerminalTitle, topic) {
			return sessionTopic{}
		}

		return r.namedTopic(topic)
	}

	return sessionTopic{}
}

func (r *Reader) namedTopic(topic transcript.Topic) sessionTopic {
	if r.sessionOnly {
		return sessionTopic{text: topic.Name, dir: topic.Dir}
	}

	return sessionTopic{text: topic.Text(), dir: topic.Dir}
}

func (r *Reader) hermesTopic(
	ctx context.Context,
	pane *state.PaneState,
	processes []herdr.PaneProcessInfoProcess,
) sessionTopic {
	if pane.Agent != hermes.Agent || pane.AgentSession.Kind != herdr.SessionRefID {
		return sessionTopic{}
	}

	home := ""
	if last := len(processes) - 1; last >= 0 {
		home = hermes.ProcessHome(processes[last].PID)
	}

	return sessionTopic{
		text: hermes.Title(ctx, r.hermesHomes, pane.AgentSession.Value, home, r.sessionOnly),
	}
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
