package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
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

func testConfig() Config {
	// The homes come from the environment the way LoadConfig would read them,
	// so a fixture that points CLAUDE_CONFIG_DIR at its own directory reaches
	// the transcript reader without also naming the home twice.
	homes, _ := configHomes()

	return Config{
		Poll:       testPoll,
		MaxLength:  resolver.DefaultMaxLength,
		BranchMax:  resolver.DefaultBranchMaxLength,
		ClaudeDirs: homes,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// setHome points os.UserHomeDir at dir. Unix reads HOME and Windows reads
// USERPROFILE, and setting both spares every fixture from knowing which.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// testResolver builds the shipped chain against a home directory of the test's
// own, because CWD declines a pane sitting in the user's and the fixtures below
// must not depend on whose machine they run on.
func testResolver(t *testing.T, cfg Config) *resolver.Deterministic {
	t.Helper()
	setHome(t, filepath.Join(t.TempDir(), "home"))

	return resolver.Default(resolver.Options{
		MaxLength: resolver.DefaultMaxLength,
		BranchMax: resolver.DefaultBranchMaxLength,
		// A tab reads the row above it as parts only when this wrote that row,
		// so the chain under test has to know what the configuration does.
		NamesWorkspaces: cfg.namesWorkspaces(),
	})
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
	// Neither a rename Herdr refused nor one that never reached it can land, so
	// a tab later found wearing that label was named by the user, and stays so.
	for name, err := range map[string]error{
		"refused":    &herdr.APIError{Code: "invalid_params", Message: "refused"},
		"never sent": errors.New("connect to herdr socket: i/o timeout"),
	} {
		t.Run(name, func(t *testing.T) {
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

// repoAt builds a repository on disk, since the branch is the one thing a poll
// reads from the filesystem rather than from the session. Its trunk is always
// `main`, so passing that as the branch is how a tab on the trunk is written.
func repoAt(t *testing.T, branch string) string {
	t.Helper()
	return repoIn(t, t.TempDir(), branch)
}

// repoIn builds one in a directory that already exists, so a test can watch a
// repository appear under a pane that was read before it did.
func repoIn(t *testing.T, root, branch string) string {
	t.Helper()

	gitDir := filepath.Join(root, ".git")

	remote := filepath.Join(gitDir, "refs", "remotes", "origin")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(gitDir, "HEAD"), "ref: refs/heads/"+branch+"\n")
	write(filepath.Join(remote, "HEAD"), "ref: refs/remotes/origin/main\n")

	return root
}

// repoWithNoTrunkAt builds a repository recording no default branch, which is
// what one that was never cloned looks like: it has no origin to read a trunk
// from, so every branch in it is worth naming.
func repoWithNoTrunkAt(t *testing.T, branch string) string {
	t.Helper()

	root := repoAt(t, branch)

	origin := filepath.Join(root, ".git", "refs", "remotes", "origin", "HEAD")
	if err := os.Remove(origin); err != nil {
		t.Fatal(err)
	}

	return root
}

// paneAt is a pane sitting in dir and running nothing Herdr will answer for,
// so that a read of it finds only what the directory holds.
func paneAt(paneID, dir string) *state.PaneState {
	return state.PaneFrom(herdr.PaneInfo{PaneID: paneID, TabID: "wE:t1", CWD: dir}, time.Time{})
}

func TestAPollNamesATabAfterItsBranch(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

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
	// Nothing in the session announces a checkout, and the pane's revision does
	// not have to move for one — the next poll simply reads HEAD again.
	repo := repoAt(t, "main")

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

func TestARepositoryIsWalkedOncePerPoll(t *testing.T) {
	// Every tab of a project reads the same directory, and the walk up to it is
	// the read. Rewriting HEAD between two panes of one poll is how the test
	// sees that the second one never reached the disk.
	repo := repoAt(t, "feat/oauth")
	app := newTestApp(t, testConfig())
	ctx, client := context.Background(), herdrtest.New(nil, nil)

	reads := app.reads.forPoll(nil)

	first := paneAt("wE:p1", repo)
	reads.fill(ctx, client, first)

	if first.Git.Branch != "feat/oauth" {
		t.Fatalf("branch = %q, want feat/oauth", first.Git.Branch)
	}

	head := filepath.Join(repo, ".git", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/fix/token\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := paneAt("wE:p2", repo)
	reads.fill(ctx, client, second)

	if second.Git.Branch != "feat/oauth" {
		t.Errorf("branch = %q, want the answer this poll already had", second.Git.Branch)
	}

	next := paneAt("wE:p3", repo)
	app.reads.forPoll(nil).fill(ctx, client, next)

	if next.Git.Branch != "fix/token" {
		t.Errorf("branch = %q, want the next poll to read HEAD again", next.Git.Branch)
	}
}

func TestADirectoryHoldingNoRepositoryIsRememberedToo(t *testing.T) {
	// Finding out that there is no repository costs the same walk to the root
	// as finding one, so a pane outside a checkout must not repeat it per tab.
	dir := t.TempDir()
	app := newTestApp(t, testConfig())
	ctx, client := context.Background(), herdrtest.New(nil, nil)

	reads := app.reads.forPoll(nil)

	first := paneAt("wE:p1", dir)
	reads.fill(ctx, client, first)

	if first.Git != (git.Checkout{}) {
		t.Fatalf("checkout = %+v, want nothing found", first.Git)
	}

	// A repository under the same directory is what a second walk would find,
	// and a poll that remembered the miss makes none.
	repoIn(t, dir, "feat/oauth")

	second := paneAt("wE:p2", dir)
	reads.fill(ctx, client, second)

	if second.Git != (git.Checkout{}) {
		t.Errorf("checkout = %+v, want the miss this poll already had", second.Git)
	}
}

func TestBranchesSwitchedOffAreNotRead(t *testing.T) {
	// Zero is how a user turns branches off, and a read whose answer is
	// discarded still costs a walk up the tree on every pane, every poll.
	repo := repoAt(t, "feat/oauth")

	cfg := testConfig()
	cfg.BranchMax = 0
	app := newTestApp(t, cfg)

	pane := paneAt("wE:p1", repo)
	app.reads.forPoll(nil).fill(context.Background(), herdrtest.New(nil, nil), pane)

	if pane.Git != (git.Checkout{}) {
		t.Errorf("checkout = %+v, want nothing read", pane.Git)
	}
}

// The session an agent pane is holding in the tests below.
const testSession = "8852bfe0-8b24-4a23-a35e-7521d04da061"

// transcript lays down a Claude Code session transcript and points the plugin
// at the state directory holding it. Which project directory it lands in is
// the transcript reader's business, and its own tests cover that.
func transcript(t *testing.T, lines ...string) {
	t.Helper()
	writeTranscript(t, stateDir(t), testSession, lines...)
}

// writeTranscript lays down one session's transcript in a state directory the
// plugin is already pointed at.
func writeTranscript(t *testing.T, root, sessionID string, lines ...string) {
	t.Helper()

	path := filepath.Join(root, "projects", "any-project", sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// agentPane is a pane holding a Claude Code session that never titled its
// terminal, which is the only pane shape these tests care about.
func agentPane() herdr.PaneInfo {
	return agentPaneInfo("wE:p1", testSession, dashboard, true)
}

func TestATabIsNamedFromTheAgentsOwnSession(t *testing.T) {
	// The agent never titled its terminal, so the transcript Herdr pointed at
	// is the only thing that says what the session is about.
	transcript(
		t,
		`{"type":"user","origin":{"kind":"human"},"message":{"role":"user","content":"rework the poll loop"}}`,
		`{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`,
	)

	cfg := testConfig()
	cfg.ReadTranscripts = true
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
	transcript(t, `{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`)

	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{agentPane()},
	)
	h.poll()

	if got := h.client.Renames()[0].Label; got != "dashboard › claude" {
		t.Errorf("rename = %q, want %q", got, "dashboard › claude")
	}
}

func TestAPollPastItsDeadlineStopsReadingTheFilesystem(t *testing.T) {
	// git.Read and the transcript reader take no context: they are file reads,
	// and a pane sitting on a hung mount blocks the whole loop for as long as
	// the mount does. A poll the tab loop will throw away makes none of them.
	repo := repoAt(t, "feat/oauth")
	app := newTestApp(t, testConfig())
	client := herdrtest.New(nil, nil)

	snapshot := herdr.Snapshot{
		Tabs:  []herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		Panes: []herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo}},
	}

	// The live read first, so a checkout that never resolves cannot make the
	// spent one below look like the guard working.
	live := app.tabsIn(snapshot)
	app.reads.forPoll(nil).fill(context.Background(), client, live[0].Panes[0])

	if got := live[0].Panes[0].Git.Branch; got != "feat/oauth" {
		t.Fatalf("branch = %q with time left, so this test proves nothing", got)
	}

	spent, cancel := context.WithCancel(context.Background())
	cancel()

	tabs := app.tabsIn(snapshot)
	app.reads.forPoll(nil).fill(spent, client, tabs[0].Panes[0])

	if got := tabs[0].Panes[0].Git; got != (git.Checkout{}) {
		t.Errorf("checkout = %+v, want a poll past its deadline to read nothing", got)
	}
}

func TestAPaneIsReadFromItsForegroundProcessesDirectory(t *testing.T) {
	// Both directories the snapshot carries point at a server the agent spawned
	// elsewhere, so a checkout read from either finds no repository at all.
	repo := repoAt(t, "feat/oauth")
	elsewhere := t.TempDir()

	pane := herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Agent: "claude",
		CWD: elsewhere, ForegroundCWD: elsewhere,
	}

	app := newTestApp(t, testConfig())
	client := herdrtest.New([]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}}, []herdr.PaneInfo{pane})
	client.SetProcesses(
		"wE:p1",
		herdr.PaneProcessInfoProcess{Name: "gimp-mcp", CWD: elsewhere},
		herdr.PaneProcessInfoProcess{Name: "claude", CWD: repo},
	)

	tabs := app.tabsIn(herdr.Snapshot{
		Tabs:  []herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		Panes: []herdr.PaneInfo{pane},
	})

	read := tabs[0].Panes[0]
	app.reads.forPoll(nil).fill(context.Background(), client, read)

	if read.Dir != repo {
		t.Errorf("dir = %q, want the agent's own %q", read.Dir, repo)
	}

	if got := read.Git.Branch; got != "feat/oauth" {
		t.Errorf("branch = %q, want the checkout of the directory the pane is in", got)
	}
}

func TestRunNamesWhatExistsBeforeTheFirstTick(t *testing.T) {
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

// worktreeIn builds a worktree of the repository at root out of files, the way
// git records one: the branch in the worktree's own HEAD, and the refs it
// shares with the repository a commondir away.
func worktreeIn(t *testing.T, root, name, branch string) string {
	t.Helper()

	gitDir := filepath.Join(root, ".git", "worktrees", name)
	tree := filepath.Join(root, ".claude", "worktrees", name)

	for _, dir := range []string{gitDir, tree} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(gitDir, "HEAD"), "ref: refs/heads/"+branch+"\n")
	write(filepath.Join(gitDir, "commondir"), "../..\n")
	write(filepath.Join(tree, ".git"), "gitdir: "+gitDir+"\n")

	return tree
}

// stateDir points the plugin at a Claude Code state directory of the test's
// own, so several sessions can be laid down in one.
func stateDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv(EnvClaudeConfigDir, root)

	return root
}

// agentIn is a transcript line saying where the agent is working. Only the
// directory is read out of it, and a Windows path is full of escapes.
func agentIn(dir string) string {
	return `{"type":"assistant","cwd":` + strconv.Quote(dir) + `}`
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

func transcriptConfig() Config {
	cfg := testConfig()
	cfg.ReadTranscripts = true

	return cfg
}

// readOne fills one pane the way a poll does, on a configuration of its own.
func readOne(t *testing.T, cfg Config, pane *state.PaneState) {
	t.Helper()

	app := newTestApp(t, cfg)
	app.reads.forPoll(nil).fill(context.Background(), herdrtest.New(nil, nil), pane)
}

// branchFor is the branch a filled pane would put in its tab's title, which is
// where the pane's checkout and its agent's are chosen between.
func branchFor(pane *state.PaneState) string {
	parts, _ := resolver.NewGit(resolver.DefaultBranchMaxLength).Resolve(pane)

	return parts.Branch
}

func TestATabIsNamedAfterTheBranchTheAgentIsWorkingOn(t *testing.T) {
	// The pane sits at the repository root on the trunk while its agent works
	// in a worktree, so the branch the user cares about is only the agent's.
	repo := repoAt(t, "main")
	worktree := worktreeIn(t, repo, "wt", "feat/oauth")

	transcript(
		t,
		agentIn(worktree),
		`{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`,
	)

	pane := agentPane()
	pane.CWD = repo

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig())
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
	stateDir(t)

	other := t.TempDir()
	writeTranscript(
		t,
		other,
		testSession,
		`{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`,
	)

	cfg := transcriptConfig()
	cfg.ClaudeDirs = []string{other}

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
	// A repository with no origin records no trunk, and the pane standing on a
	// branch of its own must still be named after the worktree its agent is in.
	repo := repoWithNoTrunkAt(t, "main")
	worktree := worktreeIn(t, repo, "wt", "feat/oauth")

	transcript(t, agentIn(worktree))

	pane := agentPane()
	pane.CWD = repo

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig())
	h.poll()

	got := h.client.Renames()[0].Label
	if want := filepath.Base(repo) + " › feat/oauth › claude"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestAnAgentOnATrunkNobodyRecordedLeavesThePanesBranch(t *testing.T) {
	// A name only a trunk carries is taken to be one even where no trunk is
	// recorded, so the pane keeps the worktree branch it is standing on.
	repo := repoWithNoTrunkAt(t, "main")
	worktree := worktreeIn(t, repo, "wt", "feat/oauth")

	transcript(t, agentIn(repo))

	pane := agentPane()
	pane.CWD = worktree

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig())
	h.poll()

	got := h.client.Renames()[0].Label
	if want := "wt › feat/oauth › claude"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

// The second session, for the tests that give one repository two agents.
const otherSession = "0c3a1d94-77b1-4f2e-8a6d-5e91b2c4d803"

func TestTwoAgentsOnTwoWorktreesOfOneRepositoryShowDifferentBranches(t *testing.T) {
	// The panes share a project and are told apart only by the worktree each
	// agent was sent into, which is the whole point of reading it.
	repo := repoAt(t, "main")
	oauth := worktreeIn(t, repo, "oauth", "feat/oauth")
	token := worktreeIn(t, repo, "token", "fix/token")

	root := stateDir(t)
	writeTranscript(t, root, testSession, agentIn(oauth))
	writeTranscript(t, root, otherSession, agentIn(token))

	app := newTestApp(t, transcriptConfig())
	reads := app.reads.forPoll(nil)

	first := agentPaneAt("wE:p1", testSession, repo)
	second := agentPaneAt("wE:p2", otherSession, repo)

	ctx, client := context.Background(), herdrtest.New(nil, nil)
	reads.fill(ctx, client, first)
	reads.fill(ctx, client, second)

	if got := branchFor(first); got != "feat/oauth" {
		t.Errorf("first branch = %q, want feat/oauth", got)
	}

	if got := branchFor(second); got != "fix/token" {
		t.Errorf("second branch = %q, want fix/token", got)
	}
}

func TestAnAgentInAnotherRepositoryIsIgnored(t *testing.T) {
	// A branch from somewhere else beside this project's name is a wrong
	// label, and worse than the pane having none.
	repo := repoAt(t, "feat/oauth")
	unrelated := repoAt(t, "fix/token")

	transcript(t, agentIn(unrelated))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestAnAgentInNoRepositoryKeepsThePanesBranch(t *testing.T) {
	// An agent that ended up in a home directory or a scratch directory must
	// not cost the pane the branch it already had.
	repo := repoAt(t, "feat/oauth")

	transcript(t, agentIn(t.TempDir()))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestAnAgentInASubdirectoryReadsTheCheckoutAboveIt(t *testing.T) {
	// A directory below a checkout is still that checkout, so an agent working
	// in one names its branch — here a worktree's, where the pane's own
	// directory is the trunk and says nothing.
	repo := repoAt(t, "main")
	worktree := worktreeIn(t, repo, "wt", "feat/oauth")

	nested := filepath.Join(worktree, "internal", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	transcript(t, agentIn(nested))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the worktree the subdirectory sits in", got)
	}
}

func TestAWorktreeTakenOffDiskLeavesThePaneItsOwnBranch(t *testing.T) {
	// The directory the transcript names is gone, so the agent has nothing to
	// say and the branch the pane sits on is the one the tab keeps.
	repo := repoAt(t, "main")
	worktree := worktreeIn(t, repo, "wt", "feat/oauth")

	transcript(t, agentIn(filepath.Join(repo, ".claude", "worktrees", "gone")))

	pane := agentPaneAt("wE:p1", testSession, worktree)
	readOne(t, transcriptConfig(), pane)

	if pane.AgentGit != (git.Checkout{}) {
		t.Errorf("agent checkout = %+v, want nothing read", pane.AgentGit)
	}

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the branch the pane sits on", got)
	}
}

func TestAWorktreeTakenOffDiskIsNotReadAtAll(t *testing.T) {
	// The walk up from a removed worktree answers with the repository above it,
	// whose branch is not the agent's, so the directory is refused for being
	// gone rather than for what it would have said.
	repo := repoAt(t, "main")
	above := worktreeIn(t, repo, "above", "feat/oauth")
	gone := filepath.Join(above, ".claude", "worktrees", "gone")

	transcript(t, agentIn(gone))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(t, transcriptConfig(), pane)

	if pane.AgentGit != (git.Checkout{}) {
		t.Errorf("agent checkout = %+v, want the gone directory refused", pane.AgentGit)
	}

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none — feat/oauth is the worktree above the gone one", got)
	}
}

func TestAPaneOnAWorktreeKeepsItsBranchWhenItsAgentWalkedUp(t *testing.T) {
	// The agent's directory is the repository root, whose trunk the branch
	// source then suppresses, so taking it would delete a segment the pane
	// shows today.
	repo := repoAt(t, "main")
	worktree := worktreeIn(t, repo, "wt", "feat/oauth")

	transcript(t, agentIn(repo))

	pane := agentPaneAt("wE:p1", testSession, worktree)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own worktree branch", got)
	}
}

func TestAnAgentOnTheTrunkKeepsThePanesBranch(t *testing.T) {
	repo := repoAt(t, "feat/oauth")
	worktree := worktreeIn(t, repo, "wt", "main")

	transcript(t, agentIn(worktree))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestADetachedAgentWorktreeShowsItsShortHash(t *testing.T) {
	// A detached HEAD is where commits get lost, so it is worth the segment
	// even in a repository that records no trunk to compare it against.
	for _, remote := range []bool{true, false} {
		repo := repoAt(t, "main")
		worktree := worktreeIn(t, repo, "wt", "side")

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

		transcript(t, agentIn(worktree))

		pane := agentPaneAt("wE:p1", testSession, repo)
		readOne(t, transcriptConfig(), pane)

		if got := branchFor(pane); got != "aaf1fd8" {
			t.Errorf("remote %v: branch = %q, want aaf1fd8", remote, got)
		}
	}
}

func TestTranscriptsSwitchedOffLeaveTheBranchOnThePanesDirectory(t *testing.T) {
	// The transcript is what says where the agent is, so a user who turned it
	// off is named exactly as before the branch ever followed one.
	repo := repoAt(t, "feat/oauth")
	worktreeIn(t, repo, "wt", "fix/token")

	transcript(t, agentIn(filepath.Join(repo, ".claude", "worktrees", "wt")))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(t, testConfig(), pane)

	if pane.AgentGit != (git.Checkout{}) {
		t.Errorf("agent checkout = %+v, want nothing read", pane.AgentGit)
	}

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestBranchesSwitchedOffReadNeitherDirectory(t *testing.T) {
	repo := repoAt(t, "main")
	worktree := worktreeIn(t, repo, "wt", "feat/oauth")

	transcript(t, agentIn(worktree))

	cfg := transcriptConfig()
	cfg.BranchMax = 0

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(t, cfg, pane)

	if pane.Git != (git.Checkout{}) || pane.AgentGit != (git.Checkout{}) {
		t.Errorf(
			"checkouts = %+v and %+v, want neither directory read",
			pane.Git, pane.AgentGit,
		)
	}
}

func TestAPaneWhoseAgentHoldsNoSessionKeepsItsBranch(t *testing.T) {
	// Herdr reports no session until that agent's integration hook is
	// installed, and a pane with no agent at all never had one.
	repo := repoAt(t, "feat/oauth")

	transcript(t, agentIn(worktreeIn(t, repo, "wt", "fix/token")))

	cfg := transcriptConfig()

	for name, pane := range map[string]*state.PaneState{
		"no agent": paneAt("wE:p1", repo),
		"no session": state.PaneFrom(
			herdr.PaneInfo{PaneID: "wE:p2", TabID: "wE:t1", CWD: repo, Agent: "claude"},
			time.Time{},
		),
	} {
		readOne(t, cfg, pane)

		if pane.AgentGit != (git.Checkout{}) {
			t.Errorf("%s: agent checkout = %+v, want none read", name, pane.AgentGit)
		}

		if got := branchFor(pane); got != "feat/oauth" {
			t.Errorf("%s: branch = %q, want the pane's own", name, got)
		}
	}
}

func TestALongTopicAndAWorktreeBranchFitTheDefaultBounds(t *testing.T) {
	// The branch takes width the topic used to have, so both bounds are pinned
	// here: a later change to either must not reduce the topic to a fragment.
	repo := repoAt(t, "main")
	worktree := worktreeIn(t, repo, "wt", "feat/oauth")

	transcript(
		t,
		agentIn(worktree),
		`{"type":"ai-title","aiTitle":"Make the branch follow the agent worktree",`+
			`"sessionId":"`+testSession+`"}`,
	)

	pane := agentPane()
	pane.CWD = repo

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig())
	h.poll()

	got := h.client.Renames()[0].Label
	want := filepath.Base(repo) + " › feat/oauth › claude › Make the branch follow"

	if got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestOneRepositorySpelledTwoWaysDoesNotFollowTheAgent(t *testing.T) {
	// Both directories are the same repository, but the pane reaches it through
	// a symlink and the worktree records the real path, so the two common
	// directories do not match and the pane keeps its own branch.
	repo := repoAt(t, "feat/oauth")
	worktree := worktreeIn(t, repo, "wt", "fix/token")

	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}

	transcript(t, agentIn(worktree))

	pane := agentPaneAt("wE:p1", testSession, alias)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestAPaneOutsideARepositoryFollowsNoAgent(t *testing.T) {
	// An agent's directory follows every cd it makes, and one outside the tree
	// the pane sits in could only be labelling someone else's project.
	repo := repoAt(t, "main")

	transcript(t, agentIn(worktreeIn(t, repo, "wt", "feat/oauth")))

	pane := agentPaneAt("wE:p1", testSession, t.TempDir())
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none", got)
	}
}

func TestAHumanPaneOnAWorktreeStillSaysItsBranchOnce(t *testing.T) {
	// A worktree is named after the branch checked out in it, so a pane sitting
	// in one reads both facts from the same directory and is worth the width of
	// only one of them.
	repo := repoAt(t, "main")
	worktree := worktreeIn(t, repo, "oauth", "oauth")

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
	// The same worktree, read through the agent instead: the context names the
	// project the pane sits in, so the branch repeats nothing and both segments
	// are worth their width.
	repo := repoAt(t, "main")
	worktree := worktreeIn(t, repo, "oauth", "oauth")

	transcript(t, agentIn(worktree))

	pane := agentPane()
	pane.CWD = repo

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig())
	h.poll()

	got := h.client.Renames()[0].Label
	if want := filepath.Base(repo) + " › oauth › claude"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestAPaneHoldingSeveralRepositoriesFollowsItsAgentsWorktree(t *testing.T) {
	// The pane sits in a parent directory holding several projects, so it has
	// no checkout of its own and the only branch anyone could name is the one
	// its agent is working on, inside one of them.
	parent := t.TempDir()
	repo := repoIn(t, filepath.Join(parent, "dashboard"), "main")
	repoIn(t, filepath.Join(parent, "billing"), "main")
	worktree := worktreeIn(t, repo, "node", "chore/node-24.21.0")

	transcript(t, agentIn(worktree))

	pane := agentPane()
	pane.CWD = parent

	h := startConfigured(t, herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{pane},
	), transcriptConfig())
	h.poll()

	got := h.client.Renames()[0].Label
	if want := filepath.Base(parent) + " › NODE-24 › claude"; got != want {
		t.Errorf("rename = %q, want %q", got, want)
	}
}

func TestAnAgentOnATrunkNamesNoBranchForAPaneOutsideARepository(t *testing.T) {
	// A trunk says nothing wherever it is read, and the pane has no branch of
	// its own for it to replace either.
	parent := t.TempDir()
	repo := repoIn(t, filepath.Join(parent, "dashboard"), "feat/oauth")

	transcript(t, agentIn(worktreeIn(t, repo, "wt", "main")))

	pane := agentPaneAt("wE:p1", testSession, parent)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none", got)
	}
}

func TestAPaneInARepositoryRefusesAnAgentInOneNestedUnderIt(t *testing.T) {
	// Containment is what lets a pane with no checkout follow its agent, and a
	// pane that has one must not gain a nested clone's branch through it.
	repo := repoAt(t, "feat/oauth")
	nested := repoIn(t, filepath.Join(repo, "vendor", "other"), "fix/token")

	transcript(t, agentIn(nested))

	pane := agentPaneAt("wE:p1", testSession, repo)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "feat/oauth" {
		t.Errorf("branch = %q, want the pane's own", got)
	}
}

func TestAPaneOutsideARepositoryRefusesAnAgentOutsideOneToo(t *testing.T) {
	// Neither directory holds a repository, so there is no branch anywhere to
	// name and the walk up must not answer with one from above the pane.
	parent := t.TempDir()

	scratch := filepath.Join(parent, "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}

	transcript(t, agentIn(scratch))

	pane := agentPaneAt("wE:p1", testSession, parent)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none", got)
	}
}

func TestADirectoryNamedLikeThePanesIsNotInsideIt(t *testing.T) {
	// A sibling whose name begins with the pane's own would pass a prefix test
	// on the spelling, and its branch belongs to a tree the pane does not hold.
	parent := t.TempDir()

	own := filepath.Join(parent, "code")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}

	sibling := repoIn(t, filepath.Join(parent, "code-review"), "feat/oauth")

	transcript(t, agentIn(sibling))

	pane := agentPaneAt("wE:p1", testSession, own)
	readOne(t, transcriptConfig(), pane)

	if got := branchFor(pane); got != "" {
		t.Errorf("branch = %q, want none", got)
	}
}
