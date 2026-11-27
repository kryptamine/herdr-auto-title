package app

import (
	"errors"
	"slices"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
)

// twinSession is two workspaces each named after its directory, whose agents
// gave their tabs the same title, so each tab drops its directory.
func twinSession() *herdrtest.Client {
	client := herdrtest.New(
		[]herdr.TabInfo{
			{TabID: "w1:t1", WorkspaceID: "w1", Label: "1"},
			{TabID: "w2:t1", WorkspaceID: "w2", Label: "1"},
		},
		[]herdr.PaneInfo{
			{
				PaneID: "w1:p1", TabID: "w1:t1", Focused: true,
				CWD:                   dashboard,
				TerminalTitleStripped: "Fix OAuth redirect",
			},
			{
				PaneID: "w2:p1", TabID: "w2:t1", Focused: true,
				CWD:                   api,
				TerminalTitleStripped: "Fix OAuth redirect",
			},
		},
	)
	client.SetWorkspaces(
		herdr.WorkspaceInfo{WorkspaceID: "w1", Label: "dashboard"},
		herdr.WorkspaceInfo{WorkspaceID: "w2", Label: "api"},
	)

	return client
}

func TestTwinTitlesAreLeftAlikeByDefault(t *testing.T) {
	t.Parallel()

	// Off by default, a tab's title stays a matter of that tab alone.
	h := startConfigured(t, twinSession(), testConfig())
	h.poll()

	want := []string{"Fix OAuth redirect", "Fix OAuth redirect"}
	if got := tabLabels(h); !slices.Equal(got, want) {
		t.Errorf("renames = %q, want %q", got, want)
	}
}

func TestTwinTitlesSayWhereEachTabIs(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.DistinctTitles = true

	h := startConfigured(t, twinSession(), cfg)
	h.poll()

	want := []string{"dashboard › Fix OAuth redirect", "api › Fix OAuth redirect"}
	if got := tabLabels(h); !slices.Equal(got, want) {
		t.Errorf("renames = %q, want %q", got, want)
	}
}

func TestATwinThatGoesTakesTheQualifierWithIt(t *testing.T) {
	t.Parallel()

	// The qualifier is decided from each poll's tabs, so a tab left on its own
	// reads as it would have without one.
	cfg := testConfig()
	cfg.DistinctTitles = true

	h := startConfigured(t, twinSession(), cfg)
	h.poll()

	h.client.CloseTab("w2:t1")
	h.client.ClosePane("w2:p1")
	h.poll()

	renames := h.client.Renames()
	if got := renames[len(renames)-1].Label; got != "Fix OAuth redirect" {
		t.Errorf("rename = %q, want %q", got, "Fix OAuth redirect")
	}
}

// distinctTwins starts twinSession with twins looked for and names both once.
func distinctTwins(t *testing.T) *harness {
	t.Helper()

	cfg := testConfig()
	cfg.DistinctTitles = true

	h := startConfigured(t, twinSession(), cfg)
	h.poll()

	return h
}

// lastLabel is the label the tab was last renamed to.
func lastLabel(h *harness, tabID string) string {
	label := ""

	for _, call := range h.client.Renames() {
		if call.TabID == tabID {
			label = call.Label
		}
	}

	return label
}

func TestATwinTheUserRenamedNoLongerQualifiesTheOther(t *testing.T) {
	t.Parallel()

	// The user's label is not the title that was shared, so the other tab has
	// no twin left to be told from.
	h := distinctTwins(t)

	h.client.SetTab(herdr.TabInfo{TabID: "w2:t1", WorkspaceID: "w2", Label: "Mine"})
	h.polls(2)

	if got := lastLabel(h, "w1:t1"); got != "Fix OAuth redirect" {
		t.Errorf("label = %q, want %q", got, "Fix OAuth redirect")
	}
}

func TestATwinAnotherProgramLabelsNoLongerQualifiesTheOther(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.DistinctTitles = true
	cfg.ForeignProcesses = []string{"labeller"}

	h := startConfigured(t, twinSession(), cfg)
	h.poll()

	// A split beside the agent now runs a program that writes the tab's label,
	// so that tab is no longer Auto Title's to name, nor a twin of anything.
	h.client.SetPane(herdr.PaneInfo{PaneID: "w2:p2", TabID: "w2:t1", CWD: api})
	h.client.SetProcesses("w2:p2", herdr.PaneProcessInfoProcess{
		Name: "labeller", Argv: []string{"labeller"}, CWD: api,
	})
	h.poll()

	if got := lastLabel(h, "w1:t1"); got != "Fix OAuth redirect" {
		t.Errorf("label = %q, want %q", got, "Fix OAuth redirect")
	}
}

func TestATabWithNoPaneIsNoTwin(t *testing.T) {
	t.Parallel()

	// A tab with no pane says nothing, so the fallback it would resolve to
	// must not make a twin of the one tab that really reads that way.
	cfg := testConfig()
	cfg.DistinctTitles = true

	client := herdrtest.New(
		[]herdr.TabInfo{
			{TabID: "w1:t1", WorkspaceID: "w1", Label: "1"},
			{TabID: "w2:t1", WorkspaceID: "w2", Label: "1"},
		},
		[]herdr.PaneInfo{{
			PaneID: "w2:p1", TabID: "w2:t1", Focused: true,
			AgentSession: &herdr.AgentSessionInfo{
				Agent: "claude", Kind: herdr.SessionRefID,
				Value: "3fa9c1d2-0000-4000-8000-000000000001",
			},
		}},
	)

	h := startConfigured(t, client, cfg)
	h.poll()

	if got := lastLabel(h, "w2:t1"); got != resolver.GenericFallback {
		t.Errorf("label = %q, want %q", got, resolver.GenericFallback)
	}
}

func TestAFailedReadKeepsTheTwinsAsTheyWere(t *testing.T) {
	t.Parallel()

	// A read that fails after one succeeded answers with the last, so one bad
	// poll is not a twin gone, which would rename the other tab and back.
	h := distinctTwins(t)

	for _, pane := range []herdr.PaneInfo{
		{PaneID: "w1:p1", TabID: "w1:t1", CWD: dashboard},
		{PaneID: "w2:p1", TabID: "w2:t1", CWD: api},
	} {
		pane.Focused, pane.Revision = true, 2
		pane.TerminalTitleStripped = "Fix OAuth redirect"
		h.client.SetPane(pane)
	}

	h.client.FailNextProcessRead(errors.New("herdr is busy"))
	h.poll()

	want := []string{"dashboard › Fix OAuth redirect", "api › Fix OAuth redirect"}
	if got := tabLabels(h); !slices.Equal(got, want) {
		t.Errorf("renames = %q, want only %q", got, want)
	}
}

func TestSessionOnlyTwinsWithNoSessionTitleAreLeftAlone(t *testing.T) {
	t.Parallel()

	// With no session title yet, SESSION_ONLY leaves a tab alone; a twin of
	// another such tab is no reason to name it after its directory.
	cfg := testConfig()
	cfg.SessionOnly, cfg.DistinctTitles = true, true

	client := herdrtest.New(
		[]herdr.TabInfo{
			{TabID: "w1:t1", WorkspaceID: "w1", Label: "1"},
			{TabID: "w2:t1", WorkspaceID: "w2", Label: "1"},
		},
		[]herdr.PaneInfo{
			{
				PaneID: "w1:p1", TabID: "w1:t1", Focused: true, CWD: dashboard,
				AgentSession: &herdr.AgentSessionInfo{
					Agent: "claude", Kind: herdr.SessionRefID,
					Value: "3fa9c1d2-0000-4000-8000-000000000001",
				},
			},
			{
				PaneID: "w2:p1", TabID: "w2:t1", Focused: true, CWD: api,
				AgentSession: &herdr.AgentSessionInfo{
					Agent: "claude", Kind: herdr.SessionRefID,
					Value: "77aa1100-0000-4000-8000-000000000002",
				},
			},
		},
	)

	h := startSessionConfigured(t, client, cfg)
	h.poll()

	if got := tabLabels(h); len(got) != 0 {
		t.Errorf("renames = %q, want none", got)
	}
}

func TestATabWhoseFirstReadFailedIsNoTwin(t *testing.T) {
	t.Parallel()

	// A tab that could not be read may run a program that writes its label,
	// so it must not make the other tab read as its twin.
	cfg := testConfig()
	cfg.DistinctTitles = true

	client := twinSession()
	client.FailNextProcessRead(errors.New("herdr is busy"))

	h := startConfigured(t, client, cfg)
	h.poll()

	if got := lastLabel(h, "w2:t1"); got != "Fix OAuth redirect" {
		t.Errorf("label = %q, want %q", got, "Fix OAuth redirect")
	}
}
