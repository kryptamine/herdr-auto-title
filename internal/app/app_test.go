package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	return Config{
		Poll:      testPoll,
		MaxLength: resolver.DefaultMaxLength,
		BranchMax: resolver.DefaultBranchMaxLength,
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
func testResolver(t *testing.T) *resolver.Deterministic {
	t.Helper()
	setHome(t, filepath.Join(t.TempDir(), "home"))

	return resolver.Default(resolver.Options{
		MaxLength: resolver.DefaultMaxLength,
		BranchMax: resolver.DefaultBranchMaxLength,
	})
}

// newTestApp builds an App on the shipped chain, which names a pane as well as
// a tab. Whether it does is the configuration's to say, not the chain's.
func newTestApp(t *testing.T, cfg Config) *App {
	t.Helper()

	chain := testResolver(t)

	return New(cfg, discardLogger(), chain, chain)
}

// harness drives an App against a stubbed Herdr session one poll at a time, so
// a test arranges the session and then says when it is read.
type harness struct {
	t      *testing.T
	app    *App
	client *herdrtest.Client
}

func start(t *testing.T, tabs []herdr.TabInfo, panes []herdr.PaneInfo) *harness {
	t.Helper()
	return startConfigured(t, herdrtest.New(tabs, panes), testConfig())
}

// startConfigured builds an App whose configuration the test has changed.
func startConfigured(t *testing.T, client *herdrtest.Client, cfg Config) *harness {
	t.Helper()

	return &harness{t: t, app: newTestApp(t, cfg), client: client}
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
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)

	path := filepath.Join(root, "projects", "any-project", testSession+".jsonl")
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
	return herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, CWD: dashboard,
		TerminalTitleStripped: "Claude Code",
		Agent:                 "claude",
		AgentStatus:           "idle",
		AgentSession: &herdr.AgentSessionInfo{
			Agent: "claude", Kind: herdr.SessionRefID, Value: testSession,
		},
	}
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
