package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/reads/readstest"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

const testPoll = 10 * time.Millisecond

// The directories the fixtures sit in, absolute on whichever platform the
// tests run on: a relative directory names no tab, and Windows has no /Users.
var (
	dashboard = herdrtest.Dir("work", "dashboard")
	api       = herdrtest.Dir("work", "api")
	billing   = herdrtest.Dir("work", "billing")
)

// testConfig reads nothing from the environment, so no title depends on the
// developer's own home or Claude sessions, and tests can run in parallel.
func testConfig() Config {
	return Config{
		Poll:      testPoll,
		MaxLength: resolver.DefaultMaxLength,
		BranchMax: resolver.DefaultBranchMaxLength,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// testResolver builds the shipped chain against a home directory of the test's
// own, because CWD declines a pane sitting in the user's and the fixtures below
// must not depend on whose machine they run on.
func testResolver(t *testing.T, cfg Config) *resolver.Deterministic {
	t.Helper()

	return resolver.Default(resolver.Options{
		MaxLength: resolver.DefaultMaxLength,
		BranchMax: resolver.DefaultBranchMaxLength,
		// A tab reads the row above it as parts only when this wrote that row,
		// so the chain under test has to know what the configuration does.
		NamesWorkspaces: cfg.namesWorkspaces(),
		Home:            testHome(t),
	})
}

// testHome is a home directory of the test's own, never the developer's.
func testHome(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "home")
}

// fakeInstance is a claim on the session that a test can have taken over, and
// that says whether the loop has reported the session read.
type fakeInstance struct {
	taken atomic.Bool
	ready atomic.Bool
}

func (f *fakeInstance) Taken() bool { return f.taken.Load() }
func (f *fakeInstance) Ready()      { f.ready.Store(true) }

// newTestApp builds an App on the shipped chain, naming panes only when the
// configuration asks for it, holding a claim nobody takes over.
func newTestApp(t *testing.T, cfg Config) *App {
	t.Helper()
	return newTestAppOn(t, cfg, &fakeInstance{})
}

func newTestAppOn(t *testing.T, cfg Config, instance Instance) *App {
	t.Helper()

	chain := testResolver(t, cfg)

	var panes resolver.PaneResolver
	if cfg.RenamePanes {
		panes = chain
	}

	return New(cfg, discardLogger(), chain, panes, WorkspaceResolver(cfg), instance)
}

// harness drives an App against a stubbed Herdr session one poll at a time, so
// a test arranges the session and then says when it is read.
type harness struct {
	t        *testing.T
	app      *App
	client   *herdrtest.Client
	instance *fakeInstance
}

func start(t *testing.T, tabs []herdr.TabInfo, panes []herdr.PaneInfo) *harness {
	t.Helper()
	return startConfigured(t, herdrtest.New(tabs, panes), testConfig())
}

// startConfigured builds an App whose configuration the test has changed.
func startConfigured(t *testing.T, client *herdrtest.Client, cfg Config) *harness {
	t.Helper()

	instance := &fakeInstance{}

	return &harness{
		t:        t,
		app:      newTestAppOn(t, cfg, instance),
		client:   client,
		instance: instance,
	}
}

// poll runs the step the ticker runs, its failure handling included, so a test
// exercises what the loop does rather than a shortcut past it. It reports what
// the step reports: whether the loop would go on.
func (h *harness) poll() bool {
	h.t.Helper()

	return h.app.poll(context.Background(), h.client)
}

func (h *harness) polls(n int) {
	h.t.Helper()

	for range n {
		h.poll()
	}
}

// awaitClock returns once the wall clock has moved on. Which pane changed last
// is told by the clock, and on Windows it ticks too coarsely to tell two polls
// apart unless one waits for it.
func awaitClock() {
	start := time.Now()
	for !time.Now().After(start) {
		time.Sleep(time.Millisecond)
	}
}

func TestTabsAreNamedFromTheFirstPoll(t *testing.T) {
	t.Parallel()

	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	renames := h.client.Renames()
	if len(renames) != 1 {
		t.Fatalf("issued %v, want one rename", renames)
	}

	if renames[0] != (herdrtest.RenameCall{TabID: "wE:t1", Label: "dashboard"}) {
		t.Errorf("rename = %+v, want {wE:t1 dashboard}", renames[0])
	}
}

func TestATabAppearingLaterIsNamed(t *testing.T) {
	t.Parallel()

	// Nothing announces it; the next poll simply finds it.
	h := start(t, nil, nil)
	h.poll()

	// A tab Herdr has just made carries its position and nothing else.
	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "1"})
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
		CWD:                   dashboard,
		TerminalTitleStripped: "Fix OAuth redirect",
	})
	h.poll()

	renames := h.client.Renames()
	if want := "dashboard › Fix OAuth redirect"; renames[0].Label != want {
		t.Errorf("rename = %q, want %q", renames[0].Label, want)
	}
}

func TestChangedContextRetitlesTheTab(t *testing.T) {
	t.Parallel()

	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: api,
	})
	h.poll()

	if got := h.client.Renames()[1].Label; got != "api" {
		t.Errorf("rename = %q, want api", got)
	}
}

func TestARenameLandingAfterItsCallFailedIsRenamedOver(t *testing.T) {
	t.Parallel()

	// A stalled Herdr applies a rename after the call timed out, and the tab
	// has moved on by then. Read as the user's, it kept a stale number forever.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetRenameError(herdr.ErrUnanswered)
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: api,
	})
	h.poll()

	h.client.SetRenameError(nil)
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 3,
		CWD: billing,
	})
	h.poll()

	renames := h.client.Renames()
	if got := renames[len(renames)-1].Label; got != "billing" {
		t.Errorf("last rename = %q, want billing", got)
	}
}

func TestARenameThatCannotHaveLandedIsNotTakenForItsOwn(t *testing.T) {
	t.Parallel()

	// Neither a rename Herdr refused nor one that never reached it can land, so
	// a tab later found wearing that label was named by the user, and stays so.
	for name, err := range map[string]error{
		"refused":    &herdr.APIError{Code: "invalid_params", Message: "refused"},
		"never sent": errors.New("connect to herdr socket: i/o timeout"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := start(
				t,
				[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
				[]herdr.PaneInfo{
					{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
				},
			)
			h.poll()

			h.client.SetRenameError(err)
			h.client.SetPane(herdr.PaneInfo{
				PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
				CWD: api,
			})
			h.poll()

			h.client.SetRenameError(nil)
			h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "api"})
			h.client.SetPane(herdr.PaneInfo{
				PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 3,
				CWD: billing,
			})
			h.poll()

			if renames := h.client.Renames(); len(renames) != 1 {
				t.Errorf("issued %v, want the tab left as the user named it", renames)
			}
		})
	}
}

func TestAnUnchangedSessionIsRenamedOnce(t *testing.T) {
	t.Parallel()

	// Polling would be unusable if every tick renamed. Deduplication against
	// the label the snapshot reports is what keeps the loop quiet.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.polls(10)

	if renames := h.client.Renames(); len(renames) != 1 {
		t.Errorf("issued %v, want exactly one rename", renames)
	}
}

func TestATabAlreadyCorrectlyNamedIsLeftAlone(t *testing.T) {
	t.Parallel()

	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "dashboard"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.polls(5)

	if renames := h.client.Renames(); len(renames) != 0 {
		t.Errorf("issued %v, want no rename", renames)
	}
}

func TestATabWithNoContextGetsTheFallback(t *testing.T) {
	t.Parallel()

	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", Focused: true}},
	)
	h.poll()

	if got := h.client.Renames()[0].Label; got != resolver.GenericFallback {
		t.Errorf("rename = %q, want %q", got, resolver.GenericFallback)
	}
}

func TestATabClosingMidPollIsNotFatal(t *testing.T) {
	t.Parallel()

	h := start(t,
		[]herdr.TabInfo{
			{TabID: "wE:t1", Label: "1"},
			{TabID: "wE:t2", Label: "2"},
		},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t2", CWD: api, Focused: true},
		},
	)
	h.poll()

	h.client.CloseTab("wE:t1")
	h.client.ClosePane("wE:p1")
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t2", Focused: true, Revision: 2,
		CWD: billing,
	})
	h.poll()

	if got := h.client.Renames()[2].Label; got != "billing" {
		t.Errorf("rename = %q, want billing", got)
	}
}

func TestFailedRenameIsRetriedOnTheNextPoll(t *testing.T) {
	t.Parallel()

	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.client.SetRenameError(errors.New("herdr is busy"))
	h.polls(3)

	if renames := h.client.Renames(); len(renames) != 0 {
		t.Fatalf("issued %v while renaming was failing", renames)
	}

	h.client.SetRenameError(nil)
	h.poll()

	if got := h.client.Renames()[0].Label; got != "dashboard" {
		t.Errorf("rename = %q, want dashboard", got)
	}
}

func TestAFailedPollIsFollowedByAWorkingOne(t *testing.T) {
	t.Parallel()

	// A poll that could not read the session says nothing about it, and the
	// next one decides again from state it has read again.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetCallError(errors.New("socket hiccup"))
	h.polls(5)
	h.client.SetCallError(nil)

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: api,
	})
	h.poll()

	if got := h.client.Renames()[1].Label; got != "api" {
		t.Errorf("rename = %q, want api", got)
	}
}

func TestAnotherServerOnTheSocketEndsTheRun(t *testing.T) {
	t.Parallel()

	// Herdr neither stops a startup process when it stops nor looks for one
	// when it starts, so the instance an earlier server started would double
	// the new one's, and lock every tab the two named differently.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	if !h.poll() {
		t.Fatal("the first poll ended the run")
	}

	h.client.SetServer("")

	if !h.poll() {
		t.Fatal("a poll with no server on the socket ended the run")
	}

	h.client.SetServer("herdrtest")

	if !h.poll() {
		t.Fatal("the same server back on the socket ended the run")
	}

	h.client.SetServer("successor")

	if h.poll() {
		t.Error("a successor on the socket did not end the run")
	}
}

func TestTheServerIsLearnedFromTheFirstPollThatSeesOne(t *testing.T) {
	t.Parallel()

	// A startup hook can outrun the socket, so the first poll may find no
	// server; the one that then appears is this instance's own, not a successor.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.client.SetServer("")
	h.polls(3)

	h.client.SetServer("herdrtest")

	if !h.poll() {
		t.Fatal("the first server seen ended the run")
	}

	h.client.SetServer("successor")

	if h.poll() {
		t.Error("a successor on the socket did not end the run")
	}
}

func TestRunReturnsWhenAnotherServerTakesTheSocket(t *testing.T) {
	t.Parallel()

	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)

	cfg := testConfig()
	cfg.Poll = time.Millisecond
	app := newTestApp(t, cfg)

	done := make(chan struct{})

	go func() { app.Run(t.Context(), client); close(done) }()

	// The first poll has to learn the server before a successor can be one.
	deadline := time.Now().Add(2 * time.Second)
	for len(client.Renames()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("nothing was named in two seconds")
		}

		time.Sleep(time.Millisecond)
	}

	client.SetServer("successor")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within two seconds of another server taking the socket")
	}
}

func TestANewerInstanceClaimingTheSessionEndsTheRun(t *testing.T) {
	t.Parallel()

	// A restart is a newer instance claiming the session, and it waits for this
	// one to leave before it names anything: two would lock every tab they
	// named differently.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	if !h.poll() {
		t.Fatal("the first poll ended the run")
	}

	h.instance.taken.Store(true)
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: api,
	})

	if h.poll() {
		t.Error("a poll after the takeover did not end the run")
	}

	if renames := h.client.Renames(); len(renames) != 1 {
		t.Errorf("issued %v, want nothing named after the takeover", renames)
	}
}

func TestTheSessionIsReportedReadOnceASnapshotCameBack(t *testing.T) {
	t.Parallel()

	// What a restart waits for is the new instance polling, and a poll that
	// could not reach Herdr is not that.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.client.SetCallError(errors.New("no such socket"))
	h.polls(3)

	if h.instance.ready.Load() {
		t.Fatal("reported ready while the session could not be read")
	}

	h.client.SetCallError(nil)
	h.poll()

	if !h.instance.ready.Load() {
		t.Error("not reported ready after a snapshot came back")
	}
}

func TestRunReturnsWhenANewerInstanceClaimsTheSession(t *testing.T) {
	t.Parallel()

	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)

	cfg := testConfig()
	cfg.Poll = time.Millisecond
	instance := &fakeInstance{}
	app := newTestAppOn(t, cfg, instance)

	done := make(chan struct{})

	go func() { app.Run(t.Context(), client); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for len(client.Renames()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("nothing was named in two seconds")
		}

		time.Sleep(time.Millisecond)
	}

	instance.taken.Store(true)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within two seconds of a newer instance claiming the session")
	}
}

func TestAFailingFirstPollIsTreatedLikeAnyOther(t *testing.T) {
	t.Parallel()

	// Herdr's socket can be a moment behind the plugin it launched, and a
	// plugin that gives up stays dead: the startup hook is a one-shot launch,
	// not a supervised daemon.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.client.SetCallError(errors.New("no such socket"))
	h.polls(5)

	h.client.SetCallError(nil)
	h.poll()

	if got := h.client.Renames()[0].Label; got != "dashboard" {
		t.Errorf("rename = %q, want dashboard once the session answered", got)
	}
}

func TestRunStopsCleanlyOnCancellation(t *testing.T) {
	t.Parallel()

	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	app := newTestApp(t, testConfig())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { app.Run(ctx, client); close(done) }()

	cancel()

	// There is no outcome besides having returned, because Run cannot fail.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

func TestTheMostRecentlyChangedPaneNamesTheTab(t *testing.T) {
	t.Parallel()

	// Neither pane is focused, so the tab is named after whichever moved last.
	// Revisions are how a poll tells that apart.
	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", Revision: 1, CWD: dashboard},
			{PaneID: "wE:p2", TabID: "wE:t1", Revision: 1, CWD: api},
		},
	)
	h.poll()

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t1", Revision: 2, CWD: api,
	})
	awaitClock()
	h.poll()

	if got := h.client.Renames()[1].Label; got != "api" {
		t.Errorf("rename = %q, want api", got)
	}
}

func TestAgentContextNamesTheTab(t *testing.T) {
	t.Parallel()

	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
			CWD:         dashboard,
			Agent:       "claude",
			AgentStatus: herdr.AgentStatusWorking,
			Title:       "Implement OAuth scopes",
		}},
	)
	h.poll()

	got := h.client.Renames()[0].Label
	if want := "dashboard › claude › Implement OAuth scopes"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestAnAgentPaneIsNamedAfterTheAgentsOwnDirectory(t *testing.T) {
	t.Parallel()

	// Both directories the snapshot carries are a descendant's: the agent moved
	// on to another project, and its MCP server sits in a third place.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
			CWD:           dashboard,
			ForegroundCWD: herdrtest.Dir("tmp"),
			Agent:         "claude",
			AgentStatus:   herdr.AgentStatusWorking,
			Title:         "Implement OAuth scopes",
		}},
	)
	h.client.SetProcesses(
		"wE:p1",
		herdr.PaneProcessInfoProcess{Name: "fff-mcp", CWD: herdrtest.Dir("tmp")},
		herdr.PaneProcessInfoProcess{
			Name: "claude",
			CWD:  herdrtest.Dir("work", "self-care-portal"),
		},
	)
	h.poll()

	want := "self-care-portal › claude › Implement OAuth scopes"
	if got := h.client.Renames()[0].Label; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestAPreferredAgentPaneIsTheOneRead(t *testing.T) {
	t.Parallel()

	// The pane a tab is named after is the one whose processes are read, or its
	// directory is the snapshot's guess rather than where the agent runs.
	cfg := testConfig()
	cfg.PreferAgentPane = true

	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t1", CWD: api, Agent: "claude"},
		},
	)
	client.SetProcesses("wE:p2", herdr.PaneProcessInfoProcess{Name: "claude", CWD: billing})

	h := startConfigured(t, client, cfg)
	h.poll()

	if got := h.client.Renames(); len(got) != 1 || got[0].Label != "billing › claude" {
		t.Errorf("renames = %v, want the agent pane's read directory", got)
	}
}

func TestARemoteSessionIsNamedAfterItsHost(t *testing.T) {
	t.Parallel()

	// What is running in a pane is not in the snapshot, so this exercises the
	// extra read the poll makes for the pane that names the tab.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	// Typing the command draws in the pane, so a revision moves with it and the
	// next poll knows to ask what is running now.
	h.client.SetProcesses(
		"wE:p1",
		herdr.PaneProcessInfoProcess{Name: "fish", Argv: []string{"-fish"}},
		herdr.PaneProcessInfoProcess{
			Name: "ssh",
			Argv: []string{"ssh", "-p", "2222", "deploy@prod-01"},
		},
	)
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: dashboard,
	})
	h.poll()

	if got := h.client.Renames()[1].Label; got != "ssh › prod-01" {
		t.Errorf("rename = %q, want %q", got, "ssh › prod-01")
	}
}

func TestAPaneWhoseProcessesCannotBeReadIsStillNamed(t *testing.T) {
	t.Parallel()

	// The pane closed between the snapshot listing it and the read of what it
	// is running; the snapshot's own context still names the tab.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.client.SetProcessError(&herdr.APIError{
		Code:    herdr.CodePaneNotFound,
		Message: "pane wE:p1 not found",
	})
	h.poll()

	renames := h.client.Renames()
	if len(renames) != 1 {
		t.Fatalf("issued %v, want the tab named from the snapshot alone", renames)
	}

	if renames[0].Label != "dashboard" {
		t.Errorf("rename = %q, want dashboard", renames[0].Label)
	}
}

func TestAWorkspaceNameIsNotRepeatedInItsTabs(t *testing.T) {
	t.Parallel()

	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
			CWD:                   dashboard,
			TerminalTitleStripped: "Fix OAuth redirect",
		}},
	)
	h.client.SetWorkspaces(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: "dashboard"})
	h.poll()

	if got := h.client.Renames()[0].Label; got != "Fix OAuth redirect" {
		t.Errorf("rename = %q, want %q", got, "Fix OAuth redirect")
	}
}

func TestARenameByTheUserTurnsAutomaticNamingOff(t *testing.T) {
	t.Parallel()

	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.poll()

	// The context moves on; the tab does not.
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: api,
	})
	h.poll()

	if renames := h.client.Renames(); len(renames) != 1 {
		t.Errorf("issued %v, want only the one before the user took the tab", renames)
	}
}

func TestClearingTheNameHandsTheTabBack(t *testing.T) {
	t.Parallel()

	// The way out of a lock, and the one a user reaches for: clear the name and
	// the tab is nobody's again. Herdr stores that as an empty label.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: ""})
	h.poll()

	if got := h.client.Renames()[1].Label; got != "dashboard" {
		t.Errorf("rename = %q, want the tab named again", got)
	}
}

func TestATabPutBackOnItsPositionIsHandedBack(t *testing.T) {
	t.Parallel()

	// The same way out, spelled the other way Herdr says a tab is unnamed: the
	// position it carries while nobody has named it.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "1"})
	h.poll()

	if got := h.client.Renames()[1].Label; got != "dashboard" {
		t.Errorf("rename = %q, want the tab named again", got)
	}
}

func TestThePluginsOwnRenamesDoNotLockTheTab(t *testing.T) {
	t.Parallel()

	// Every rename changes a label the plugin then sees again. Reading its own
	// work as the user's would stop it naming anything after the first time.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	for i, dir := range []string{"api", "billing", "dashboard"} {
		h.client.SetPane(herdr.PaneInfo{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: uint64(i + 2),
			CWD: herdrtest.Dir("work", dir),
		})
		h.poll()

		renames := h.client.Renames()
		if got := renames[len(renames)-1].Label; got != dir {
			t.Fatalf("rename = %q, want %q", got, dir)
		}
	}
}

func TestNoTabIsLockedOnTheFirstPoll(t *testing.T) {
	t.Parallel()

	// Every tab starts out carrying a label that is not what the resolver
	// would produce. Locking on that would claim the session at startup.
	h := start(t,
		[]herdr.TabInfo{
			{TabID: "wE:t1", Label: "1"},
			{TabID: "wE:t2", Label: "2"},
		},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t2", CWD: api, Focused: true},
		},
	)
	h.poll()

	renames := h.client.Renames()
	if len(renames) != 2 {
		t.Fatalf("issued %v, want both tabs named", renames)
	}

	labels := map[string]bool{renames[0].Label: true, renames[1].Label: true}
	if !labels["dashboard"] || !labels["api"] {
		t.Errorf("renames = %v, want both tabs named", renames)
	}
}

func TestATabCreatedAndNamedBeforeTheNextPollIsLeftAlone(t *testing.T) {
	t.Parallel()

	// The reported failure: a tab made and named in the half-second before the
	// poll that would first see it. Auto Title never saw it carrying its
	// number, so the name on it is not Auto Title's.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t9", Label: "My thing"})
	h.client.SetPane(
		herdr.PaneInfo{PaneID: "wE:p9", TabID: "wE:t9", CWD: api, Focused: true},
	)
	h.polls(2)

	for _, rename := range h.client.Renames() {
		if rename.TabID == "wE:t9" {
			t.Fatalf("renamed a tab the user had already named: %+v", rename)
		}
	}
}

func TestATabCreatedWithoutANameIsNamed(t *testing.T) {
	t.Parallel()

	// Herdr names a new tab after its place in the workspace, which is nobody's
	// choice. The second tab is "2" — not TabInfo.number, which counts every
	// tab the workspace has ever held.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t9", Label: "2"})
	h.client.SetPane(
		herdr.PaneInfo{PaneID: "wE:p9", TabID: "wE:t9", CWD: api, Focused: true},
	)
	h.poll()

	renames := h.client.Renames()
	if got := renames[len(renames)-1]; got.TabID != "wE:t9" || got.Label != "api" {
		t.Errorf("rename = %+v, want {wE:t9 api}", got)
	}
}

func TestAPaneHoldingStillIsAskedAboutOnce(t *testing.T) {
	t.Parallel()

	// pane.process_info is a request per pane, and at two polls a second an
	// unchanging session would spend all day repeating it.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.polls(10)

	if reads := h.client.ProcessReads(); reads != 1 {
		t.Errorf("read what the pane runs %d times over ten polls, want 1", reads)
	}
}

func TestAPaneThatMovedIsAskedAboutAgain(t *testing.T) {
	t.Parallel()

	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "nvim"})
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: dashboard,
	})
	h.poll()

	if got := h.client.Renames()[1].Label; got != "dashboard › nvim" {
		t.Errorf("rename = %q, want %q", got, "dashboard › nvim")
	}
}

func TestAPaneThatCannotBeReadIsAskedAgain(t *testing.T) {
	t.Parallel()

	// A failed read is not an answer, so it must not be remembered as one.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.client.SetProcessError(errors.New("herdr is busy"))
	h.poll()

	// The tab is already named from the snapshot alone; the second rename can
	// only come from a process read that happened again. The processes go in
	// before the error clears: an empty read between the two would be reused.
	h.client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "nvim"})
	h.client.SetProcessError(nil)
	h.poll()

	if got := h.client.Renames()[1].Label; got != "dashboard › nvim" {
		t.Errorf("rename = %q, want %q", got, "dashboard › nvim")
	}
}

func TestAPaneThatDoesNotNameItsTabIsNotRead(t *testing.T) {
	t.Parallel()

	// A tab is named from one pane, so asking what the others are running is a
	// request each whose answer nothing would look at.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t1", CWD: dashboard},
			{PaneID: "wE:p3", TabID: "wE:t1", CWD: dashboard},
		},
	)
	h.polls(10)

	if reads := h.client.ProcessReads(); reads != 1 {
		t.Errorf("read %d panes, want only the one the tab is named from", reads)
	}
}

func TestALockedTabIsNotReadEither(t *testing.T) {
	t.Parallel()

	// A tab the user has claimed is never renamed, so everything a rename
	// would have been decided from is a read nobody asked for.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.poll()

	before := h.client.ProcessReads()

	// The pane keeps drawing, which is what makes a poll ask again.
	for i := range 4 {
		h.client.SetPane(herdr.PaneInfo{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: uint64(i + 2),
			CWD: dashboard,
		})
		h.poll()
	}

	if reads := h.client.ProcessReads() - before; reads != 0 {
		t.Errorf("asked what a locked tab's pane runs %d times, want never", reads)
	}
}

func TestAPollNamesATabAfterItsBranch(t *testing.T) {
	t.Parallel()

	repo := readstest.Repo(t, "feat/oauth")

	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true}},
	)
	h.poll()

	got := h.client.Renames()[0].Label
	if want := filepath.Base(repo) + " › feat/oauth"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestCheckingOutABranchRetitlesTheTab(t *testing.T) {
	t.Parallel()

	// Nothing in the session announces a checkout, and the pane's revision does
	// not have to move for one — the next poll simply reads HEAD again.
	repo := readstest.Repo(t, "main")

	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true}},
	)
	h.poll()

	head := filepath.Join(repo, ".git", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/feat/oauth\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h.poll()

	got := h.client.Renames()[1].Label
	if want := filepath.Base(repo) + " › feat/oauth"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

// The session an agent pane is holding in the tests below.
const testSession = "8852bfe0-8b24-4a23-a35e-7521d04da061"

// The second session, for the tests that give one repository two agents.
const otherSession = "0c3a1d94-77b1-4f2e-8a6d-5e91b2c4d803"

// transcript lays down a Claude Code session transcript and points the plugin
// at the state directory holding it. Which project directory it lands in is
// the transcript reader's business, and its own tests cover that.
func transcript(t *testing.T, lines ...string) string {
	t.Helper()

	home := stateDir(t)
	readstest.Transcript(t, home, testSession, lines...)

	return home
}

// agentPane is a pane holding a Claude Code session that never titled its
// terminal, which is the only pane shape these tests care about.
func agentPane() herdr.PaneInfo {
	return agentPaneInfo("wE:p1", testSession, dashboard, true)
}

func TestATabIsNamedFromTheAgentsOwnSession(t *testing.T) {
	t.Parallel()

	// The agent never titled its terminal, so the transcript Herdr pointed at
	// is the only thing that says what the session is about.
	home := transcript(
		t,
		`{"type":"user","origin":{"kind":"human"},"message":{"role":"user","content":"rework the poll loop"}}`,
		`{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`,
	)

	cfg := transcriptConfig(home)
	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{agentPane()},
	), cfg)
	h.poll()

	got := h.client.Renames()[0].Label
	if want := "dashboard › claude › Poll loop rework"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestTranscriptsAreLeftUnreadWhenTurnedOff(t *testing.T) {
	t.Parallel()

	home := transcript(
		t,
		`{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`,
	)

	cfg := testConfig()
	cfg.ClaudeDirs = []string{home}

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{agentPane()},
	), cfg)
	h.poll()

	if got := h.client.Renames()[0].Label; got != "dashboard › claude" {
		t.Errorf("rename = %q, want %q", got, "dashboard › claude")
	}
}

func TestRunNamesWhatExistsBeforeTheFirstTick(t *testing.T) {
	t.Parallel()

	// A tab is named as the plugin starts, not a poll interval later. The
	// interval here is long enough that a rename arriving at all can only have
	// come from the poll Run makes before it waits.
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		},
	)

	cfg := testConfig()
	cfg.Poll = time.Minute
	app := newTestApp(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() { app.Run(ctx, client); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for len(client.Renames()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("nothing was named in the two seconds before the first tick was due")
		}

		time.Sleep(time.Millisecond)
	}

	if got := client.Renames()[0].Label; got != "dashboard" {
		t.Errorf("rename = %q, want dashboard", got)
	}

	cancel()
	<-done
}

func TestAWindowsShellPaneIsNamedAfterItsDirectory(t *testing.T) {
	t.Parallel()

	// What Herdr reports for an idle pane on Windows: the shell with its
	// extension, its directory with a trailing separator, and a title of
	// Herdr's own making that names both. None of it is what the pane is doing.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
			CWD:                   dashboard,
			TerminalTitleStripped: "pwsh in dashboard",
		}},
	)
	h.client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{
		Name: "pwsh.exe",
		CWD:  dashboard + string(filepath.Separator),
	})
	h.poll()

	if got := h.client.Renames()[0].Label; got != "dashboard" {
		t.Errorf("rename = %q, want dashboard", got)
	}
}

// stateDir points the plugin at a Claude Code state directory of the test's
// own, so several sessions can be laid down in one.
func stateDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func transcriptConfig(homes ...string) Config {
	cfg := testConfig()
	cfg.ReadTranscripts = true
	cfg.ClaudeDirs = homes

	return cfg
}

func TestATabIsNamedAfterTheBranchTheAgentIsWorkingOn(t *testing.T) {
	t.Parallel()

	// The pane sits at the repository root on the trunk while its agent works
	// in a worktree, so the branch the user cares about is only the agent's.
	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "wt", "feat/oauth")

	home := transcript(
		t,
		readstest.AgentIn(worktree),
		`{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`,
	)

	pane := agentPane()
	pane.CWD = repo

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig(home))
	h.poll()

	got := h.client.Renames()[0].Label
	if want := filepath.Base(repo) + " › feat/oauth › claude › Poll loop rework"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

// The homes travel from configuration to the transcript reader as a value. A
// session in one of them is named, which is what says the wiring is connected:
// nothing reads the setting out of the environment on the reader's behalf.
func TestASessionInAnExtraConfigHomeIsNamed(t *testing.T) {
	t.Parallel()

	first := stateDir(t)

	other := t.TempDir()
	readstest.Transcript(
		t,
		other,
		testSession,
		`{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`,
	)

	cfg := transcriptConfig(first, other)

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{agentPane()},
	), cfg)
	h.poll()

	if got := h.client.Renames()[0].Label; !strings.Contains(got, "Poll loop rework") {
		t.Errorf("rename = %q, want the extra home's title in it", got)
	}
}

func TestAnAgentsBranchIsNamedWhereNoTrunkIsRecorded(t *testing.T) {
	t.Parallel()

	// A repository with no origin records no trunk, and the pane standing on a
	// branch of its own must still be named after the worktree its agent is in.
	repo := readstest.RepoWithNoTrunk(t, "main")
	worktree := readstest.Worktree(t, repo, "wt", "feat/oauth")

	home := transcript(t, readstest.AgentIn(worktree))

	pane := agentPane()
	pane.CWD = repo

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig(home))
	h.poll()

	got := h.client.Renames()[0].Label
	if want := filepath.Base(repo) + " › feat/oauth › claude"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestAnAgentOnATrunkNobodyRecordedLeavesThePanesBranch(t *testing.T) {
	t.Parallel()

	// A name only a trunk carries is taken to be one even where no trunk is
	// recorded, so the pane keeps the worktree branch it is standing on.
	repo := readstest.RepoWithNoTrunk(t, "main")
	worktree := readstest.Worktree(t, repo, "wt", "feat/oauth")

	home := transcript(t, readstest.AgentIn(repo))

	pane := agentPane()
	pane.CWD = worktree

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig(home))
	h.poll()

	got := h.client.Renames()[0].Label
	if want := "wt › feat/oauth › claude"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestALongTopicAndAWorktreeBranchFitTheDefaultBounds(t *testing.T) {
	t.Parallel()

	// The branch takes width the topic used to have, so both bounds are pinned
	// here: a later change to either must not reduce the topic to a fragment.
	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "wt", "feat/oauth")

	home := transcript(
		t,
		readstest.AgentIn(worktree),
		`{"type":"ai-title","aiTitle":"Make the branch follow the agent worktree",`+
			`"sessionId":"`+testSession+`"}`,
	)

	pane := agentPane()
	pane.CWD = repo

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig(home))
	h.poll()

	got := h.client.Renames()[0].Label
	want := filepath.Base(repo) + " › feat/oauth › claude › Make the branch follow"

	if got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestAHumanPaneOnAWorktreeStillSaysItsBranchOnce(t *testing.T) {
	t.Parallel()

	// A worktree is named after the branch checked out in it, so a pane sitting
	// in one reads both facts from the same directory and is worth the width of
	// only one of them.
	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "oauth", "oauth")

	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", CWD: worktree, Focused: true,
			TerminalTitleStripped: "Fix OAuth redirect",
		}},
	)
	h.poll()

	if got := h.client.Renames()[0].Label; got != "oauth › Fix OAuth redirect" {
		t.Errorf("rename = %q, want the branch its directory already says dropped", got)
	}
}

func TestAnAgentsWorktreeBranchStandsBesideTheProject(t *testing.T) {
	t.Parallel()

	// The same worktree, read through the agent instead: the context names the
	// project the pane sits in, so the branch repeats nothing and both segments
	// are worth their width.
	repo := readstest.Repo(t, "main")
	worktree := readstest.Worktree(t, repo, "oauth", "oauth")

	home := transcript(t, readstest.AgentIn(worktree))

	pane := agentPane()
	pane.CWD = repo

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig(home))
	h.poll()

	got := h.client.Renames()[0].Label
	if want := filepath.Base(repo) + " › oauth › claude"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestAPaneHoldingSeveralRepositoriesFollowsItsAgentsWorktree(t *testing.T) {
	t.Parallel()

	// The pane sits in a parent directory holding several projects, so it has
	// no checkout of its own and the only branch anyone could name is the one
	// its agent is working on, inside one of them.
	parent := t.TempDir()
	repo := readstest.RepoIn(t, filepath.Join(parent, "dashboard"), "main")
	readstest.RepoIn(t, filepath.Join(parent, "billing"), "main")
	worktree := readstest.Worktree(t, repo, "node", "chore/node-24.21.0")

	home := transcript(t, readstest.AgentIn(worktree))

	pane := agentPane()
	pane.CWD = parent

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig(home))
	h.poll()

	got := h.client.Renames()[0].Label
	if want := filepath.Base(parent) + " › NODE-24 › claude"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

// Resolvers hands the home directory to both chains it builds: without it a
// pane sitting in the home names the tab and the workspace row after the account.
func TestResolversKeepAPaneInTheHomeFromNamingAnything(t *testing.T) {
	t.Parallel()

	cfg := workspaceConfig(t)
	cfg.Home = testHome(t)

	titles, _, workspaces := Resolvers(cfg)

	pane := &state.PaneState{ID: "w1:p1", Dir: cfg.Home, Focused: true}
	tab := state.TabFrom(
		herdr.TabInfo{TabID: "w1:t1", WorkspaceID: "w1", Label: "1"},
		"",
		1,
		[]*state.PaneState{pane},
		false,
	)

	if got := titles.Resolve(tab).Name; got != resolver.GenericFallback {
		t.Errorf("tab named %q, want %q", got, resolver.GenericFallback)
	}

	if got := workspaces.ResolveWorkspace(state.WorkspaceState{Context: pane}).Name; got != "" {
		t.Errorf("workspace named %q, want the row left alone", got)
	}
}
