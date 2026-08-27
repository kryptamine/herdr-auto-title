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
)

const testPoll = 10 * time.Millisecond

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
func testResolver(t *testing.T) resolver.TitleResolver {
	t.Helper()
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	return resolver.Default(resolver.DefaultMaxLength, resolver.DefaultBranchMaxLength)
}

// harness runs an App against a stubbed Herdr session.
type harness struct {
	t      *testing.T
	client *herdrtest.Client
	done   chan struct{}
	cancel context.CancelFunc

	stopped bool
}

func start(t *testing.T, tabs []herdr.TabInfo, panes []herdr.PaneInfo) *harness {
	t.Helper()
	return startWith(t, herdrtest.New(tabs, panes))
}

// startWith runs an App against a stub the test has already prepared, which is
// how a test arranges for the very first poll to fail.
func startWith(t *testing.T, client *herdrtest.Client) *harness {
	t.Helper()
	return startConfigured(t, client, testConfig())
}

// startConfigured runs an App whose configuration the test has changed.
func startConfigured(t *testing.T, client *herdrtest.Client, cfg Config) *harness {
	t.Helper()

	app := New(cfg, discardLogger(), testResolver(t))

	ctx, cancel := context.WithCancel(context.Background())

	h := &harness{t: t, client: client, done: make(chan struct{}), cancel: cancel}
	go func() { app.Run(ctx, client); close(h.done) }()

	t.Cleanup(func() { h.stop() })

	return h
}

// stop cancels the run and waits for it, failing the test if it does not
// return. Safe to call twice, so a test can stop early and leave the cleanup.
func (h *harness) stop() {
	if h.stopped {
		return
	}

	h.stopped = true

	h.cancel()

	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		h.t.Fatal("Run did not return after its context was cancelled")
	}
}

// awaitRenames blocks until at least n renames have been issued.
func (h *harness) awaitRenames(n int) []herdrtest.RenameCall {
	h.t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if renames := h.client.Renames(); len(renames) >= n {
			return renames
		}

		time.Sleep(time.Millisecond)
	}

	h.t.Fatalf("timed out waiting for %d renames, saw %v", n, h.client.Renames())

	return nil
}

// awaitPolls blocks until at least n polls have happened, which is how a test
// waits for "the loop has had its chance and did nothing".
func (h *harness) awaitPolls(n int) {
	h.t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.client.Polls() >= n {
			return
		}

		time.Sleep(time.Millisecond)
	}

	h.t.Fatalf("timed out waiting for %d polls, saw %d", n, h.client.Polls())
}

func TestTabsAreNamedFromTheFirstPoll(t *testing.T) {
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)

	renames := h.awaitRenames(1)
	if renames[0] != (herdrtest.RenameCall{TabID: "wE:t1", Label: "dashboard"}) {
		t.Errorf("rename = %+v, want {wE:t1 dashboard}", renames[0])
	}
}

func TestATabAppearingLaterIsNamed(t *testing.T) {
	// Nothing announces it; the next poll simply finds it.
	h := start(t, nil, nil)
	h.awaitPolls(1)

	// A tab Herdr has just made carries its position and nothing else.
	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "1"})
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
		CWD:                   "/Users/dev/work/dashboard",
		TerminalTitleStripped: "Fix OAuth redirect",
	})

	renames := h.awaitRenames(1)
	if want := "dashboard › Fix OAuth redirect"; renames[0].Label != want {
		t.Errorf("rename = %q, want %q", renames[0].Label, want)
	}
}

func TestChangedContextRetitlesTheTab(t *testing.T) {
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: "/Users/dev/work/api",
	})

	renames := h.awaitRenames(2)
	if renames[1].Label != "api" {
		t.Errorf("rename = %q, want api", renames[1].Label)
	}
}

func TestAnUnchangedSessionIsRenamedOnce(t *testing.T) {
	// Polling would be unusable if every tick renamed. Deduplication against
	// the label the snapshot reports is what keeps the loop quiet.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)
	h.awaitPolls(10)

	if renames := h.client.Renames(); len(renames) != 1 {
		t.Errorf("issued %v, want exactly one rename", renames)
	}
}

func TestATabAlreadyCorrectlyNamedIsLeftAlone(t *testing.T) {
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "dashboard"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitPolls(5)

	if renames := h.client.Renames(); len(renames) != 0 {
		t.Errorf("issued %v, want no rename", renames)
	}
}

func TestATabWithNoContextGetsTheFallback(t *testing.T) {
	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", Focused: true}},
	)

	if got := h.awaitRenames(1)[0].Label; got != resolver.GenericFallback {
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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t2", CWD: "/Users/dev/work/api", Focused: true},
		},
	)
	h.awaitRenames(2)

	h.client.CloseTab("wE:t1")
	h.client.ClosePane("wE:p1")
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t2", Focused: true, Revision: 2,
		CWD: "/Users/dev/work/billing",
	})

	renames := h.awaitRenames(3)
	if renames[2].Label != "billing" {
		t.Errorf("rename = %q, want billing", renames[2].Label)
	}
}

func TestFailedRenameIsRetriedOnTheNextPoll(t *testing.T) {
	// Armed before the run starts: the loop names what it finds without waiting
	// for a tick, so a stub armed afterwards races the very first poll.
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	client.SetRenameError(errors.New("herdr is busy"))
	h := startWith(t, client)

	h.awaitPolls(3)

	if renames := h.client.Renames(); len(renames) != 0 {
		t.Fatalf("issued %v while renaming was failing", renames)
	}

	h.client.SetRenameError(nil)

	if got := h.awaitRenames(1)[0].Label; got != "dashboard" {
		t.Errorf("rename = %q, want dashboard", got)
	}
}

func TestAFailedPollDoesNotStopTheLoop(t *testing.T) {
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetCallError(errors.New("socket hiccup"))
	// Waited out rather than slept through: several ticks have to have found
	// the socket shut before clearing the error proves anything.
	h.awaitPolls(h.client.Polls() + 5)
	h.client.SetCallError(nil)

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: "/Users/dev/work/api",
	})

	if got := h.awaitRenames(2)[1].Label; got != "api" {
		t.Errorf("rename = %q, want api", got)
	}
}

func TestAFailingFirstPollDoesNotStopTheRun(t *testing.T) {
	// Herdr's socket can be a moment behind the plugin it launched, and a
	// plugin that gives up stays dead: the startup hook is a one-shot launch,
	// not a supervised daemon. So the first poll is treated like every other.
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	client.SetCallError(errors.New("no such socket"))
	h := startWith(t, client)

	// Several ticks have to have found the socket still shut.
	h.awaitPolls(5)
	client.SetCallError(nil)

	if got := h.awaitRenames(1)[0].Label; got != "dashboard" {
		t.Errorf("rename = %q, want dashboard once the session answered", got)
	}
}

func TestARunOfFailuresIsLoggedOnABackoff(t *testing.T) {
	// Polls run twice a second, so an hour of Herdr being down is seven
	// thousand identical warnings unless the run is allowed to double.
	var failures failureLog

	var logged []int

	for range 2000 {
		if run := failures.failed(); run > 0 {
			logged = append(logged, run)
		}
	}

	want := []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}
	if len(logged) != len(want) {
		t.Fatalf("logged %v, want %v", logged, want)
	}

	for i, run := range want {
		if logged[i] != run {
			t.Fatalf("logged %v, want %v", logged, want)
		}
	}

	if run := failures.recovered(); run != 2000 {
		t.Errorf("recovery reported %d missed polls, want 2000", run)
	}

	if run := failures.recovered(); run != 0 {
		t.Errorf("recovery reported %d after nothing went wrong, want 0", run)
	}
}

func TestRunStopsCleanlyOnCancellation(t *testing.T) {
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	// stop fails the test if Run does not return; there is no outcome besides
	// having returned, because Run cannot fail.
	h.stop()
}

func TestTheMostRecentlyChangedPaneNamesTheTab(t *testing.T) {
	// Neither pane is focused, so the tab is named after whichever moved last.
	// Revisions are how a poll tells that apart.
	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", Revision: 1, CWD: "/Users/dev/work/dashboard"},
			{PaneID: "wE:p2", TabID: "wE:t1", Revision: 1, CWD: "/Users/dev/work/api"},
		},
	)
	h.awaitRenames(1)

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t1", Revision: 2, CWD: "/Users/dev/work/api",
	})

	if got := h.awaitRenames(2)[1].Label; got != "api" {
		t.Errorf("rename = %q, want api", got)
	}
}

func TestAgentContextNamesTheTab(t *testing.T) {
	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
			CWD:         "/Users/dev/work/dashboard",
			Agent:       "claude",
			AgentStatus: herdr.AgentStatusWorking,
			Title:       "Implement OAuth scopes",
		}},
	)

	if want := "dashboard › claude › Implement OAuth scopes"; h.awaitRenames(1)[0].Label != want {
		t.Errorf("rename = %q, want %q", h.awaitRenames(1)[0].Label, want)
	}
}

func TestARemoteSessionIsNamedAfterItsHost(t *testing.T) {
	// What is running in a pane is not in the snapshot, so this exercises the
	// extra read the poll makes for the pane that names the tab.
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

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
		CWD: "/Users/dev/work/dashboard",
	})

	renames := h.awaitRenames(2)
	if want := "ssh › prod-01"; renames[1].Label != want {
		t.Errorf("rename = %q, want %q", renames[1].Label, want)
	}
}

func TestAPaneWhoseProcessesCannotBeReadIsStillNamed(t *testing.T) {
	// The pane closed between the snapshot listing it and the read of what it
	// is running; the snapshot's own context still names the tab.
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)

	app := New(testConfig(), discardLogger(), testResolver(t))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() { app.Run(ctx, client); close(done) }()

	deadline := time.Now().Add(2 * time.Second)

	for {
		if renames := client.Renames(); len(renames) > 0 {
			if renames[0].Label != "dashboard" {
				t.Errorf("rename = %q, want dashboard", renames[0].Label)
			}

			break
		}

		if time.Now().After(deadline) {
			t.Fatal("the tab was never named")
		}

		time.Sleep(time.Millisecond)
	}

	cancel()
	<-done
}

func TestAWorkspaceNameIsNotRepeatedInItsTabs(t *testing.T) {
	// The workspace has to be in the stub before the loop starts: Run polls
	// once immediately, and a first poll that has not seen the workspace yet
	// has nothing to drop, so it renames the tab `dashboard › …`.
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
			CWD:                   "/Users/dev/work/dashboard",
			TerminalTitleStripped: "Fix OAuth redirect",
		}},
	)
	client.SetWorkspaces(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: "dashboard"})
	h := startWith(t, client)

	renames := h.awaitRenames(1)
	if got := renames[len(renames)-1].Label; got != "Fix OAuth redirect" {
		t.Errorf("rename = %q, want %q", got, "Fix OAuth redirect")
	}
}

func TestARenameByTheUserTurnsAutomaticNamingOff(t *testing.T) {
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.awaitPolls(h.client.Polls() + 3)

	// The context moves on; the tab does not.
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: "/Users/dev/work/api",
	})
	h.awaitPolls(h.client.Polls() + 3)

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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.awaitPolls(h.client.Polls() + 3)

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: ""})

	if got := h.awaitRenames(2)[1].Label; got != "dashboard" {
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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.awaitPolls(h.client.Polls() + 3)

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "1"})

	if got := h.awaitRenames(2)[1].Label; got != "dashboard" {
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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	for i, dir := range []string{"api", "billing", "dashboard"} {
		h.client.SetPane(herdr.PaneInfo{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: uint64(i + 2),
			CWD: "/Users/dev/work/" + dir,
		})

		renames := h.awaitRenames(i + 2)
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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t2", CWD: "/Users/dev/work/api", Focused: true},
		},
	)

	renames := h.awaitRenames(2)

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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t9", Label: "My thing"})
	h.client.SetPane(
		herdr.PaneInfo{PaneID: "wE:p9", TabID: "wE:t9", CWD: "/Users/dev/work/api", Focused: true},
	)

	h.awaitPolls(h.client.Polls() + 4)

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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t9", Label: "2"})
	h.client.SetPane(
		herdr.PaneInfo{PaneID: "wE:p9", TabID: "wE:t9", CWD: "/Users/dev/work/api", Focused: true},
	)

	renames := h.awaitRenames(2)
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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)
	h.awaitPolls(10)

	if reads := h.client.ProcessReads(); reads != 1 {
		t.Errorf("read what the pane runs %d times over ten polls, want 1", reads)
	}
}

func TestAPaneThatMovedIsAskedAboutAgain(t *testing.T) {
	h := start(
		t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "nvim"})
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: "/Users/dev/work/dashboard",
	})

	if got := h.awaitRenames(2)[1].Label; got != "dashboard › nvim" {
		t.Errorf("rename = %q, want %q", got, "dashboard › nvim")
	}
}

func TestAPaneThatCannotBeReadIsAskedAgain(t *testing.T) {
	// A failed read is not an answer, so it must not be remembered as one.
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	client.SetProcessError(errors.New("herdr is busy"))
	h := startWith(t, client)
	h.awaitPolls(3)

	// The tab is already named from the snapshot alone; the second rename is
	// the one that could only come from a process read that happened again.
	// The processes go in before the error is cleared: between the two calls a
	// poll can read an empty pane successfully, and that answer is reused for
	// processRefresh, which outlasts what this test is willing to wait.
	client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "nvim"})
	client.SetProcessError(nil)

	if got := h.awaitRenames(2)[1].Label; got != "dashboard › nvim" {
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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard"},
			{PaneID: "wE:p3", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard"},
		},
	)
	h.awaitRenames(1)
	h.awaitPolls(10)

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
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: "/Users/dev/work/dashboard", Focused: true},
		},
	)
	h.awaitRenames(1)

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.awaitPolls(h.client.Polls() + 3)

	before := h.client.ProcessReads()

	// The pane keeps drawing, which is what makes a poll ask again.
	for i := range 4 {
		h.client.SetPane(herdr.PaneInfo{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: uint64(i + 2),
			CWD: "/Users/dev/work/dashboard",
		})
		h.awaitPolls(h.client.Polls() + 2)
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
	root := t.TempDir()
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

func TestAPollNamesATabAfterItsBranch(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true}},
	)

	renames := h.awaitRenames(1)
	if want := filepath.Base(repo) + " › feat/oauth"; renames[0].Label != want {
		t.Errorf("rename = %q, want %q", renames[0].Label, want)
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
	h.awaitRenames(1)

	head := filepath.Join(repo, ".git", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/feat/oauth\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	renames := h.awaitRenames(2)
	if want := filepath.Base(repo) + " › feat/oauth"; renames[1].Label != want {
		t.Errorf("rename = %q, want %q", renames[1].Label, want)
	}
}

func TestBranchesSwitchedOffAreNotRead(t *testing.T) {
	// Zero is how a user turns branches off, and a read whose answer is
	// discarded still costs a walk up the tree on every pane, every poll.
	repo := repoAt(t, "feat/oauth")

	cfg := testConfig()
	cfg.BranchMax = 0
	app := New(cfg, discardLogger(), testResolver(t))

	if checkout := app.checkoutIn(context.Background(), repo); checkout != (git.Checkout{}) {
		t.Errorf("checkout = %+v, want nothing read", checkout)
	}
}

// The session an agent pane is holding in the tests below.
const (
	testSession = "8852bfe0-8b24-4a23-a35e-7521d04da061"
	testDir     = "/Users/dev/work/dashboard"
)

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
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, CWD: testDir,
		TerminalTitleStripped: "Claude Code",
		Agent:                 "claude",
		AgentStatus:           herdr.AgentStatusIdle,
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

	renames := h.awaitRenames(1)
	if want := "dashboard › claude › Poll loop rework"; renames[0].Label != want {
		t.Errorf("rename = %q, want %q", renames[0].Label, want)
	}
}

func TestTranscriptsAreLeftUnreadWhenTurnedOff(t *testing.T) {
	transcript(t, `{"type":"ai-title","aiTitle":"Poll loop rework","sessionId":"`+testSession+`"}`)

	h := start(t,
		[]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		[]herdr.PaneInfo{agentPane()},
	)

	renames := h.awaitRenames(1)
	if want := "dashboard › claude"; renames[0].Label != want {
		t.Errorf("rename = %q, want %q", renames[0].Label, want)
	}
}

func TestAPollPastItsDeadlineStopsReadingTheFilesystem(t *testing.T) {
	// git.Read and the transcript reader take no context: they are file reads,
	// and a pane sitting on a hung mount blocks the whole loop for as long as
	// the mount does. A poll the tab loop will throw away makes none of them.
	repo := repoAt(t, "feat/oauth")
	app := New(testConfig(), discardLogger(), testResolver(t))
	client := herdrtest.New(nil, nil)

	snapshot := herdr.Snapshot{
		Tabs:  []herdr.TabInfo{{TabID: "wE:t1", Label: "1"}},
		Panes: []herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo}},
	}

	// The live read first, so a checkout that never resolves cannot make the
	// spent one below look like the guard working.
	live := app.tabsIn(snapshot)
	app.readInto(context.Background(), client, live[0].Panes[0])

	if got := live[0].Panes[0].Git.Branch; got != "feat/oauth" {
		t.Fatalf("branch = %q with time left, so this test proves nothing", got)
	}

	spent, cancel := context.WithCancel(context.Background())
	cancel()

	tabs := app.tabsIn(snapshot)
	app.readInto(spent, client, tabs[0].Panes[0])

	if got := tabs[0].Panes[0].Git; got != (git.Checkout{}) {
		t.Errorf("checkout = %+v, want a poll past its deadline to read nothing", got)
	}
}
