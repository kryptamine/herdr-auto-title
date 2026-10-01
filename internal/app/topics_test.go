package app

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/reads/readstest"
)

// topicConfig is testConfig with topics reported, as they ship, and the agent's
// name shown so a topic's agent half can be asserted.
func topicConfig() Config {
	cfg := testConfig()
	cfg.ReportWorkspaces = true
	cfg.ShowAgentName = true

	return cfg
}

func startWorkspaces(
	t *testing.T,
	cfg Config,
	workspaces []herdr.WorkspaceInfo,
	tabs []herdr.TabInfo,
	panes []herdr.PaneInfo,
) *harness {
	t.Helper()

	client := herdrtest.New(tabs, panes)
	client.SetWorkspaces(workspaces...)

	return startConfigured(t, client, cfg)
}

// theWorkspace is the id every fixture here builds its workspace under.
const theWorkspace = "wE"

// oneTabWorkspace is a workspace carrying label, whose one tab shows pane.
func oneTabWorkspace(t *testing.T, cfg Config, label string, pane herdr.PaneInfo) *harness {
	t.Helper()

	pane.PaneID, pane.TabID, pane.Focused = "wE:p1", "wE:t1", true
	if pane.CWD == "" {
		pane.CWD = dashboard
	}

	return startWorkspaces(t, cfg,
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: label, ActiveTabID: "wE:t1"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"}},
		[]herdr.PaneInfo{pane},
	)
}

// twoTabWorkspace is a workspace showing active, whose tabs' panes carry the
// terminal titles given.
func twoTabWorkspace(t *testing.T, active, first, second string) *harness {
	t.Helper()

	return startWorkspaces(t, topicConfig(),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: "api", ActiveTabID: active}},
		[]herdr.TabInfo{
			{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"},
			{TabID: "wE:t2", WorkspaceID: theWorkspace, Label: "2"},
		},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, TerminalTitleStripped: first},
			{PaneID: "wE:p2", TabID: "wE:t2", CWD: dashboard, TerminalTitleStripped: second},
		},
	)
}

// cleared stands for a report whose topic is null.
const cleared = "(cleared)"

// topicsSent is every topic reported so far, in order, after checking that each
// report is the one shape this ever sends.
func topicsSent(t *testing.T, h *harness) []string {
	t.Helper()

	topics := []string{}

	for _, report := range h.client.WorkspaceReports() {
		if report.WorkspaceID != theWorkspace || report.Source != topicSource ||
			report.TTLMs != topicTTL.Milliseconds() || len(report.Tokens) != 1 {
			t.Errorf("unexpected report: %+v", report)
		}

		value, carried := report.Tokens[herdr.TopicToken]
		switch {
		case !carried:
			t.Errorf("a report without the topic: %+v", report)
		case value == nil:
			topics = append(topics, cleared)
		default:
			topics = append(topics, *value)
		}
	}

	return topics
}

func wantTopics(t *testing.T, h *harness, want ...string) {
	t.Helper()

	if want == nil {
		want = []string{}
	}

	if got := topicsSent(t, h); !slices.Equal(got, want) {
		t.Errorf("topics sent = %q, want %q", got, want)
	}
}

// fakeClock drives the reports' clock, so a refresh is a step of the test
// rather than a wait.
func fakeClock(h *harness) func(time.Duration) {
	now := time.Now()
	h.app.reported.now = func() time.Time { return now }

	return func(d time.Duration) { now = now.Add(d) }
}

// Whatever the label says -- Herdr's directory basename, a name the user gave,
// or a row an earlier version wrote -- it is left alone and the topic reported.
func TestATopicIsReportedWhateverTheLabel(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		label string
		pane  herdr.PaneInfo
		want  string
	}{
		{
			label: "rails",
			pane: herdr.PaneInfo{
				Agent: "claude", TerminalTitleStripped: "Tokyo dev workspace thread",
			},
			want: "claude › Tokyo dev workspace thread",
		},
		{
			label: "KK - Improvement project",
			pane:  herdr.PaneInfo{TerminalTitleStripped: "What is next"},
			want:  "What is next",
		},
		{
			label: "kids-study › worktree › Pair Pau's iPad",
			pane:  herdr.PaneInfo{TerminalTitleStripped: "Pair Pau's iPad"},
			want:  "Pair Pau's iPad",
		},
	} {
		t.Run(test.label, func(t *testing.T) {
			t.Parallel()

			h := oneTabWorkspace(t, topicConfig(), test.label, test.pane)
			h.polls(2)

			wantTopics(t, h, test.want)

			snapshot, err := herdr.SessionSnapshot(t.Context(), h.client)
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}

			if got := snapshot.Workspaces[0].Label; got != test.label {
				t.Errorf("label = %q, want it left at %q", got, test.label)
			}
		})
	}
}

// The row describes the tab the workspace opens to, so switching tabs moves
// the topic with it.
func TestTheActiveTabIsTheOneReported(t *testing.T) {
	t.Parallel()

	h := twoTabWorkspace(t, "wE:t1", "Redis performance", "Search params")
	h.poll()

	h.client.SetWorkspaces(
		herdr.WorkspaceInfo{WorkspaceID: theWorkspace, Label: "api", ActiveTabID: "wE:t2"},
	)
	h.polls(2)

	wantTopics(t, h, "Redis performance", "Search params")
}

// A snapshot that names no active tab, or one that has closed since, is read
// through the first tab rather than said nothing about.
func TestTheFirstTabStandsInForAnActiveTabTheSnapshotCannotName(t *testing.T) {
	t.Parallel()

	for _, active := range []string{"", "wE:t9"} {
		t.Run(fmt.Sprintf("active %q", active), func(t *testing.T) {
			t.Parallel()

			h := twoTabWorkspace(t, active, "Redis performance", "Search params")
			h.poll()

			wantTopics(t, h, "Redis performance")
		})
	}
}

// A shell has nothing to report, and nothing is sent for it: no topic showing
// needs no clear.
func TestNothingIsSentWhereThereIsNothingToSay(t *testing.T) {
	t.Parallel()

	h := oneTabWorkspace(t, topicConfig(), "dashboard", herdr.PaneInfo{})
	h.polls(3)

	wantTopics(t, h)

	empty := startWorkspaces(t, topicConfig(),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: "dashboard"}}, nil, nil)
	empty.polls(2)

	wantTopics(t, empty)
}

// A topic that is no longer true is cleared at once rather than left to the
// TTL, and a clear is sent once.
func TestATopicThatGoesIsClearedOnce(t *testing.T) {
	t.Parallel()

	h := oneTabWorkspace(t, topicConfig(), "dashboard",
		herdr.PaneInfo{TerminalTitleStripped: "Fix login"})
	advance := fakeClock(h)
	h.poll()

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2, CWD: dashboard,
	})
	h.polls(2)

	advance(topicTTL)
	h.poll()

	wantTopics(t, h, "Fix login", cleared)
}

// A topic lives topicTTL once sent, so an unchanged one is sent again before it
// can expire -- and not on every poll before that.
func TestAnUnchangedTopicIsRefreshedBeforeItExpires(t *testing.T) {
	t.Parallel()

	h := oneTabWorkspace(t, topicConfig(), "dashboard",
		herdr.PaneInfo{TerminalTitleStripped: "Fix login"})
	advance := fakeClock(h)
	h.poll()

	advance(topicRefresh - time.Second)
	h.poll()
	wantTopics(t, h, "Fix login")

	advance(time.Second)
	h.poll()
	wantTopics(t, h, "Fix login", "Fix login")
}

// Herdr keeps a topic for topicTTL after its reporter is gone, so a restarted
// instance clears what its predecessor left, and only once.
func TestARestartClearsATopicItsPredecessorLeft(t *testing.T) {
	t.Parallel()

	h := startWorkspaces(t, topicConfig(),
		[]herdr.WorkspaceInfo{{
			WorkspaceID: theWorkspace, Label: "dashboard", ActiveTabID: "wE:t1",
			Tokens: map[string]string{herdr.TopicToken: "old task"},
		}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true}},
	)
	h.polls(2)

	wantTopics(t, h, cleared)
}

func TestTopicsAreNotReportedWhenTurnedOff(t *testing.T) {
	t.Parallel()

	h := oneTabWorkspace(t, testConfig(), "dashboard",
		herdr.PaneInfo{TerminalTitleStripped: "Fix login"})
	h.polls(3)

	wantTopics(t, h)

	if len(h.client.Renames()) == 0 {
		t.Error("the tab was not named either")
	}
}

// A report that never reached Herdr is not remembered at all, so the next poll
// sends it again rather than waiting out a refresh or a change of topic.
func TestAReportThatNeverReachedHerdrIsSentAgain(t *testing.T) {
	t.Parallel()

	h := oneTabWorkspace(t, topicConfig(), "dashboard",
		herdr.PaneInfo{TerminalTitleStripped: "Fix login"})
	fakeClock(h)
	h.client.SetReportError(errors.New("connect: connection refused"))
	h.poll()

	h.client.SetReportError(nil)
	h.poll()

	wantTopics(t, h, "Fix login")
}

// A workspace that closed between the snapshot and the report is forgotten, so
// one Herdr hands back under the same id is reported afresh.
func TestAReportToAClosedWorkspaceIsNotRemembered(t *testing.T) {
	t.Parallel()

	h := oneTabWorkspace(t, topicConfig(), "dashboard",
		herdr.PaneInfo{TerminalTitleStripped: "Fix login"})
	h.client.SetReportError(&herdr.APIError{Code: herdr.CodeWorkspaceNotFound})
	h.poll()

	h.client.SetReportError(nil)
	h.poll()

	wantTopics(t, h, "Fix login")
}

// Herdr may still apply a report it did not answer, so it counts as sent: a
// slow Herdr is not asked again every poll, and the refresh repairs a late one.
func TestAnUnansweredReportCountsAsSent(t *testing.T) {
	t.Parallel()

	h := oneTabWorkspace(t, topicConfig(), "dashboard",
		herdr.PaneInfo{TerminalTitleStripped: "Fix login"})
	advance := fakeClock(h)
	h.client.SetReportError(fmt.Errorf("%w: read timed out", herdr.ErrUnanswered))
	h.polls(2)

	wantTopics(t, h, "Fix login")

	advance(topicRefresh)
	h.poll()

	wantTopics(t, h, "Fix login", "Fix login")
}

// Herdr refuses the same value the same way every time, so a refused topic is
// not sent again, refresh or not, until it changes.
func TestARefusedTopicWaitsForTheNextOne(t *testing.T) {
	t.Parallel()

	h := oneTabWorkspace(t, topicConfig(), "dashboard",
		herdr.PaneInfo{TerminalTitleStripped: "Fix login"})
	advance := fakeClock(h)
	h.client.SetReportError(&herdr.APIError{Code: "invalid_metadata_token"})
	h.poll()

	h.client.SetReportError(nil)
	advance(topicRefresh)
	h.polls(2)

	wantTopics(t, h)

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2, CWD: dashboard,
		TerminalTitleStripped: "Fix logout",
	})
	h.poll()

	wantTopics(t, h, "Fix logout")
}

// Unset, the width is Herdr's to fit; set, the topic arrives already cut.
func TestATopicIsBoundOnlyWhenAWidthIsSet(t *testing.T) {
	t.Parallel()

	title := "Reconcile the ledger against the bank statement"

	for _, test := range []struct {
		width int
		want  string
	}{
		{width: 0, want: title},
		{width: 10, want: "Reconcile"},
	} {
		t.Run(strconv.Itoa(test.width), func(t *testing.T) {
			t.Parallel()

			cfg := topicConfig()
			cfg.WorkspaceMaxLength = test.width

			h := oneTabWorkspace(t, cfg, "dashboard", herdr.PaneInfo{TerminalTitleStripped: title})
			h.poll()

			wantTopics(t, h, test.want)
		})
	}
}

func TestTheTopicObeysTheAgentNameSetting(t *testing.T) {
	t.Parallel()

	cfg := topicConfig()
	cfg.ShowAgentName = false

	h := oneTabWorkspace(t, cfg, "dashboard", herdr.PaneInfo{Agent: "claude", Title: "Fix"})
	h.poll()

	wantTopics(t, h, "Fix")
}

// A topic that followed the foreground process would change at every prompt,
// so a build says nothing on the row while the tab still names it.
func TestTheTopicDoesNotFollowTheForegroundProcess(t *testing.T) {
	t.Parallel()

	repo := readstest.Repo(t, "feat/oauth")

	h := oneTabWorkspace(t, topicConfig(), "dashboard", herdr.PaneInfo{CWD: repo})
	h.client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{
		Name: "npm",
		Argv: []string{"npm", "run", "build"},
		CWD:  repo,
	})
	h.polls(2)

	wantTopics(t, h)

	if tabs := h.client.Renames(); len(tabs) == 0 ||
		!strings.Contains(tabs[len(tabs)-1].Label, "npm") {
		t.Errorf("the tab should still take the process, got %v", tabs)
	}
}

// claimedTabReads counts pane.process_info requests spent on a one-tab
// workspace after the user has claimed the tab, while its pane keeps drawing.
func claimedTabReads(t *testing.T, cfg Config) int {
	t.Helper()

	repo := readstest.Repo(t, "feat/oauth")
	h := oneTabWorkspace(t, cfg, "dashboard", herdr.PaneInfo{CWD: repo})
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "mine"})
	h.poll()

	before := h.client.ProcessReads()

	for i := range 4 {
		h.client.SetPane(herdr.PaneInfo{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: uint64(i + 2),
			CWD: repo,
		})
		h.poll()
	}

	return h.client.ProcessReads() - before
}

// With pane naming on, as it ships, the claimed tab's pane is already read for
// its panes, so its topic comes from a read the poll has paid for.
func TestATopicAboveAClaimedTabCostsNoReadWhilePanesAreNamed(t *testing.T) {
	t.Parallel()

	both := paneConfig()
	both.ReportWorkspaces = true

	without := claimedTabReads(t, paneConfig())
	with := claimedTabReads(t, both)

	if without == 0 {
		t.Fatalf("the claimed tab's pane was never read for its panes, so nothing is compared")
	}

	if with != without {
		t.Errorf("topics cost %d reads on top of the %d pane naming spent, want 0",
			with-without, without)
	}
}

// With pane naming off, that pane is read for the topic alone.
func TestATopicAboveAClaimedTabCostsAReadWhilePanesAreNotNamed(t *testing.T) {
	t.Parallel()

	without := claimedTabReads(t, testConfig())
	with := claimedTabReads(t, topicConfig())

	if without != 0 || with == 0 {
		t.Errorf("reads: %d without topics, %d with; want 0 and >0", without, with)
	}
}
