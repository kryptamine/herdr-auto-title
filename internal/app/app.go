// Package app polls the Herdr session and keeps every tab's title in step with
// what that tab is doing, each pane's label too unless the configuration turns
// that off, and reports what each workspace's active tab is doing as its topic.
package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/labels"
	"github.com/kryptamine/herdr-auto-title/internal/reads"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// PollTimeout bounds one poll: a snapshot and the renames it decides on.
const PollTimeout = 5 * time.Second

// LeaveTimeout is how long a displaced instance gets to leave: a poll past its
// deadline, then the wait for its next one, which is its own interval — up to
// five seconds of it, a slower instance being left with a warning.
const LeaveTimeout = PollTimeout + 5*time.Second

// Instance is this process's claim on the session: a newer claim ends the run,
// and the first snapshot read is reported to whoever waits for it — see
// docs/architecture/poll-loop.md.
type Instance interface {
	Taken() bool
	Ready()
}

// App is one run of the Auto Title loop.
type App struct {
	// pollEvery is how often the session is read, which is all the loop itself
	// decides anything by.
	pollEvery time.Duration
	log       *slog.Logger
	titles    resolver.TitleResolver
	// panes names each pane of a tab as well as the tab itself, and is nil
	// when the user turned that off.
	panes resolver.PaneResolver
	// preferAgent names a tab after its agent pane rather than its focused one.
	preferAgent bool
	// topics says what each workspace's active tab is doing, and is nil when the
	// user turned that off.
	topics   *resolver.Topics
	reported *topicReports
	changes  *state.Changes
	labels   *labels.Labels
	reads    *reads.Reader
	// failures is the run of polls that have failed in a row, which decides
	// how loudly the next one is reported.
	failures failureLog
	// server identifies the Herdr this instance answers to, learned from the
	// first poll that could read it. Another server on the socket has started
	// an instance of its own, and this one leaves rather than double it.
	server   string
	instance Instance
}

// New builds the application. The client belongs to Run rather than to the
// App, so one App can be driven by any connection. A nil panes leaves every
// pane's label alone.
func New(
	cfg Config,
	log *slog.Logger,
	titles resolver.TitleResolver,
	panes resolver.PaneResolver,
	topics *resolver.Topics,
	instance Instance,
) *App {
	return &App{
		pollEvery:   cfg.Poll,
		log:         log,
		titles:      titles,
		panes:       panes,
		topics:      topics,
		reported:    newTopicReports(),
		preferAgent: cfg.PreferAgentPane,
		changes:     state.NewChanges(),
		labels:      labels.Load(cfg.ManualPath, log),
		reads: reads.New(reads.Options{
			ClaudeDirs:      cfg.ClaudeDirs,
			BranchMax:       cfg.BranchMax,
			ReadTranscripts: cfg.ReadTranscripts,
		}, log),
		instance: instance,
	}
}

// Resolvers builds what the configuration asks titles to be resolved by: the
// shipped chain, its position in front when asked, panes named unless that is
// turned off and their IDs in front when asked, and topics reported unless that
// is turned off.
func Resolvers(
	cfg Config,
) (resolver.TitleResolver, resolver.PaneResolver, *resolver.Topics) {
	chain := resolver.Default(resolver.Options{
		MaxLength:     cfg.MaxLength,
		BranchMax:     cfg.BranchMax,
		HideAgentName: !cfg.ShowAgentName,
		Home:          cfg.Home,
	})

	var titles resolver.TitleResolver = chain
	if cfg.ShowPosition {
		titles = resolver.NewNumbered(chain, cfg.MaxLength)
	}

	topics := topicsFor(cfg)

	if !cfg.RenamePanes {
		return titles, nil, topics
	}

	var panes resolver.PaneResolver = chain
	if cfg.ShowPaneID {
		panes = resolver.NewPaneIDs(chain, cfg.MaxLength)
	}

	return titles, panes, topics
}

// topicsFor is what a workspace's topic is read by, or nil when the user turned
// topics off. Not the tabs' chain: a topic that followed the foreground process
// would change at every prompt.
func topicsFor(cfg Config) *resolver.Topics {
	if !cfg.ReportWorkspaces {
		return nil
	}

	return resolver.NewTopics(resolver.Options{
		BranchMax:     cfg.BranchMax,
		HideAgentName: !cfg.ShowAgentName,
		Home:          cfg.Home,
	}, cfg.WorkspaceMaxLength)
}

// Run polls the session until the context is cancelled. Herdr's event stream is
// deliberately not used, and the measurements that settled that are in
// docs/architecture/poll-loop.md.
func (a *App) Run(ctx context.Context, client herdr.Client) {
	// Name what already exists before waiting for the first tick.
	if !a.poll(ctx, client) {
		return
	}

	ticker := time.NewTicker(a.pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("shutting down")
			return
		case <-ticker.C:
			if !a.poll(ctx, client) {
				return
			}
		}
	}
}

// poll is one turn of the loop, and reports whether there should be another.
// No failure is fatal — Herdr's socket can lag the process it just launched —
// but a successor ends the run, on the socket or on the claim: docs/architecture/poll-loop.md.
func (a *App) poll(ctx context.Context, client herdr.Client) bool {
	if reason := a.successor(client); reason != "" {
		a.log.Info(reason + ", leaving")
		return false
	}

	err := a.readAndRename(ctx, client)
	if ctx.Err() != nil {
		return true
	}

	if err != nil {
		if run := a.failures.failed(); run > 0 {
			a.log.Warn("poll failed", "error", err, "in a row", run)
		}

		return true
	}

	a.instance.Ready()

	if run := a.failures.recovered(); run > 0 {
		a.log.Info("the session is answering again", "polls missed", run)
	}

	return true
}

// successor says why this instance should leave, or "" when it should go on.
// The claim answers for a new server too; the socket is what is left to read
// when there was nowhere to claim the session — docs/architecture/poll-loop.md.
func (a *App) successor(client herdr.Client) string {
	if a.instance.Taken() {
		return "a newer auto title has claimed the session"
	}

	if a.superseded(client) {
		return "another server holds the socket and starts an auto title of its own"
	}

	return ""
}

// superseded reports whether the socket has passed to a server other than the
// one this instance first saw. While no server can be read nothing is decided:
// Herdr comes and goes, and only a successor is a reason to leave.
func (a *App) superseded(client herdr.Client) bool {
	current := client.Server()

	switch {
	case current == "":
		return false
	case a.server == "":
		a.server = current
		return false
	default:
		return current != a.server
	}
}

func (a *App) readAndRename(ctx context.Context, client herdr.Client) error {
	ctx, cancel := context.WithTimeout(ctx, PollTimeout)
	defer cancel()

	snapshot, err := herdr.SessionSnapshot(ctx, client)
	if err != nil {
		return err
	}

	drew := a.changes.Observe(snapshot.Panes)
	pass := a.labels.Pass(client, snapshot)

	tabs := a.tabsIn(snapshot)
	poll := a.reads.Poll(client, snapshot.Panes, drew)

	for _, tab := range tabs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		a.nameTab(ctx, pass, poll, tab)

		if a.panes != nil {
			a.namePanes(ctx, pass, poll, tab)
		}
	}

	if a.topics != nil {
		// Last, so a poll cut short gives up a topic rather than a tab.
		a.reportTopics(ctx, client, poll, snapshot, tabs)
	}

	// Reached only when every tab was seen, so never deferred.
	pass.Settle()

	return nil
}

// reportTopics tells Herdr what each workspace's active tab is doing, when that
// changed or is due for a refresh. The label is never touched, and a report
// that fails never cuts the poll short.
func (a *App) reportTopics(
	ctx context.Context,
	client herdr.Client,
	poll *reads.Poll,
	snapshot herdr.Snapshot,
	tabs []state.TabState,
) {
	first := firstTabs(snapshot.Tabs)

	contexts := make(map[string]*state.PaneState, len(tabs))
	for _, tab := range tabs {
		contexts[tab.ID] = tab.Context
	}

	a.reported.observe(snapshot.Workspaces)

	for _, workspace := range snapshot.Workspaces {
		if ctx.Err() != nil {
			return
		}

		// An older Herdr may not say which tab is active, and the one it names
		// may have closed since.
		active := workspace.ActiveTabID
		if _, live := contexts[active]; !live {
			active = first[workspace.WorkspaceID]
		}

		if active == "" {
			continue
		}

		pane := contexts[active]
		poll.Fill(ctx, pane)

		topic := a.topics.Topic(pane)
		if a.reported.due(workspace, topic) {
			a.report(ctx, client, workspace.WorkspaceID, topic)
		}
	}
}

// report sends one workspace's topic, or clears it when topic is empty.
func (a *App) report(ctx context.Context, client herdr.Client, id, topic string) {
	err := herdr.ReportWorkspaceTopic(ctx, client, id, topicSource, topic, topicTTL)

	switch {
	case err == nil:
		a.reported.sent(id, topic)
		a.log.Debug("workspace topic reported", "workspace_id", id, "topic", topic)
	case herdr.ErrorCode(err) == herdr.CodeWorkspaceNotFound:
		a.reported.forget(id)
		a.log.Debug("workspace closed before its topic could be reported", "workspace_id", id)
	case errors.Is(err, herdr.ErrUnanswered):
		// Herdr may still apply it, and the next refresh repairs one that lands
		// after a newer topic: docs/architecture/poll-loop.md.
		a.reported.sent(id, topic)
		a.log.Warn(
			"workspace topic report failed",
			"workspace_id",
			id,
			"topic",
			topic,
			"error",
			err,
		)
	case herdr.ErrorCode(err) != "":
		// Herdr refuses the same value the same way every time.
		a.reported.rejected(id, topic)
		a.log.Warn(
			"herdr refused a workspace topic",
			"workspace_id",
			id,
			"topic",
			topic,
			"error",
			err,
		)
	default:
		a.log.Warn(
			"workspace topic report failed",
			"workspace_id",
			id,
			"topic",
			topic,
			"error",
			err,
		)
	}
}

// firstTabs names the first tab of each workspace, in snapshot order.
func firstTabs(tabs []herdr.TabInfo) map[string]string {
	first := make(map[string]string, len(tabs))

	for _, tab := range tabs {
		if _, seen := first[tab.WorkspaceID]; !seen {
			first[tab.WorkspaceID] = tab.TabID
		}
	}

	return first
}

// nameTab keeps one tab's label in step with what the tab is doing.
func (a *App) nameTab(
	ctx context.Context,
	pass *labels.Pass,
	poll *reads.Poll,
	tab state.TabState,
) {
	if pass.TabLocked(tab.ID) {
		return
	}

	// Read here rather than during assembly: the reads are what a poll
	// spends, and only a tab that will be renamed is worth them.
	poll.Fill(ctx, tab.Context)

	pass.Tab(ctx, tab, a.titles.Resolve(tab))
}

// namePanes labels every pane of a tab, which is what Herdr's goto panel lists
// a pane by. It reads each pane rather than the one its tab speaks through, so
// it costs a read per pane — see docs/architecture/poll-loop.md.
func (a *App) namePanes(
	ctx context.Context,
	pass *labels.Pass,
	poll *reads.Poll,
	tab state.TabState,
) {
	// Every pane is named against the tab's own pane, which is read even when
	// the tab is claimed; a poll never spends the same read twice.
	poll.Fill(ctx, tab.Context)

	for _, pane := range tab.Panes {
		if !pass.PaneLocked(pane.ID) {
			poll.Fill(ctx, pane)
		}
	}

	for i, decision := range a.panes.ResolvePanes(tab) {
		if ctx.Err() != nil {
			return
		}

		pass.Pane(ctx, tab.Panes[i], decision)
	}
}

// workspaceLabelsIn indexes the session's workspaces by id, which is what a tab
// is told its workspace is called. Herdr always answers a label here: one
// nobody renamed carries the basename of its pane's directory.
func workspaceLabelsIn(workspaces []herdr.WorkspaceInfo) map[string]string {
	labels := make(map[string]string, len(workspaces))
	for _, workspace := range workspaces {
		labels[workspace.WorkspaceID] = workspace.Label
	}

	return labels
}
