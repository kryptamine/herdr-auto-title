package app

import (
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
)

// paneConfig is the configuration with pane naming asked for, which nothing
// else in these tests turns on.
func paneConfig() Config {
	cfg := testConfig()
	cfg.RenamePanes = true

	return cfg
}

// split is a tab of two panes in different directories. The focused one names
// the tab, so `wE:p2` is the pane with something of its own left to say — which
// is the only kind of pane that gets a label at all.
func split() []herdr.PaneInfo {
	return []herdr.PaneInfo{
		{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
		{PaneID: "wE:p2", TabID: "wE:t1", CWD: api},
	}
}

func oneTab() []herdr.TabInfo {
	return []herdr.TabInfo{{TabID: "wE:t1", Label: "1"}}
}

func startPanes(t *testing.T, tabs []herdr.TabInfo, panes []herdr.PaneInfo) *harness {
	t.Helper()

	return startConfigured(t, herdrtest.New(tabs, panes), paneConfig())
}

// labelsOf is the labels one pane was given, in order, so a test says what
// happened to the pane it is about rather than to the whole split.
func labelsOf(h *harness, paneID string) []string {
	var labels []string

	for _, call := range h.client.PaneRenames() {
		if call.PaneID == paneID {
			labels = append(labels, call.Label)
		}
	}

	return labels
}

func TestPanesAreLeftAloneUnlessAskedFor(t *testing.T) {
	// The setting exists because naming panes costs a read per pane rather
	// than per tab, and because it changes what an existing user sees.
	h := start(t, oneTab(), split())
	h.poll()

	if renames := h.client.PaneRenames(); len(renames) != 0 {
		t.Errorf("issued %v, want no pane renamed while the setting is off", renames)
	}

	if len(h.client.Renames()) != 1 {
		t.Error("the tab was not named")
	}
}

func TestAPaneIsNamedForWhatTellsItFromItsTab(t *testing.T) {
	// The goto panel lists a pane under its tab, so the pane that differs is
	// named by the difference alone rather than by the whole of its context.
	h := startPanes(t, oneTab(), split())
	h.poll()

	if tabs := h.client.Renames(); len(tabs) != 1 || tabs[0].Label != "dashboard" {
		t.Fatalf("tab = %v, want the focused pane's directory", tabs)
	}

	if got := labelsOf(h, "wE:p2"); len(got) != 1 || got[0] != "api" {
		t.Errorf("pane = %v, want the directory its tab does not carry", got)
	}
}

func TestEveryPaneIsNamedEvenWhenItEchoesItsTab(t *testing.T) {
	// The whole point of the feature: Herdr lists an unlabelled pane by its
	// agent, so every pane of a session reads `claude`. A label that repeats
	// the tab is redundant; `claude` on every row says nothing at all.
	h := startPanes(
		t,
		oneTab(),
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true, Agent: "claude"},
		},
	)
	h.poll()

	if got := labelsOf(h, "wE:p1"); len(got) != 1 || got[0] != "dashboard › claude" {
		t.Errorf("pane = %v, want the lone pane named for what it is about", got)
	}
}

func TestAPaneIsNotRenamedToWhatItAlreadyCarries(t *testing.T) {
	h := startPanes(t, oneTab(), split())
	h.polls(3)

	if got := labelsOf(h, "wE:p2"); len(got) != 1 {
		t.Errorf("pane = %v, want it named once and then left alone", got)
	}
}

func TestAPaneTheUserRenamedIsLeftAlone(t *testing.T) {
	// The reason this feature cannot ship without the protection: a name the
	// user set by hand is theirs, and overwriting it twice a second is data
	// loss rather than a cosmetic bug.
	h := startPanes(t, oneTab(), split())
	h.poll()

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t1", Revision: 2, CWD: api,
		Label: "Important work",
	})
	h.poll()

	// The context moves on; the pane does not.
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t1", Revision: 3, CWD: billing,
		Label: "Important work",
	})
	h.poll()

	if got := labelsOf(h, "wE:p2"); len(got) != 1 {
		t.Errorf("pane = %v, want only the one before the user took the pane", got)
	}
}

func TestClearingAPaneLabelHandsThePaneBack(t *testing.T) {
	// The way out of a pane lock. Herdr has one spelling for it: pane.rename
	// clears an empty label rather than storing it, so a released pane carries
	// no label at all.
	h := startPanes(t, oneTab(), split())
	h.poll()

	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t1", CWD: api, Label: "Important work",
	})
	h.poll()

	h.client.SetPane(herdr.PaneInfo{PaneID: "wE:p2", TabID: "wE:t1", CWD: api})
	h.poll()

	if got := labelsOf(h, "wE:p2"); len(got) != 2 || got[1] != "api" {
		t.Errorf("pane = %v, want it named again once it was handed back", got)
	}
}

func TestThePluginsOwnPaneRenamesDoNotLockThePane(t *testing.T) {
	// Every rename changes a label the plugin then sees again. Reading its own
	// work as the user's would stop it naming anything after the first time.
	h := startPanes(t, oneTab(), split())
	h.poll()

	awaitClock()
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t1", Revision: 2, CWD: billing, Label: "api",
	})
	h.poll()

	if got := labelsOf(h, "wE:p2"); len(got) != 2 || got[1] != "billing" {
		t.Errorf("pane = %v, want it renamed as its context moved", got)
	}
}

func TestATabTheUserClaimedStillHasItsPanesNamed(t *testing.T) {
	// The two locks are independent. A tab named by hand says nothing about
	// what the panes inside it should be listed as.
	h := startPanes(t, oneTab(), split())
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p2", TabID: "wE:t1", Revision: 2, CWD: billing,
	})
	h.poll()

	if renames := h.client.Renames(); len(renames) != 1 {
		t.Errorf("issued %v, want the claimed tab left alone", renames)
	}

	if got := labelsOf(h, "wE:p2"); len(got) != 2 || got[1] != "billing" {
		t.Errorf("pane = %v, want the pane inside it still named", got)
	}
}

func TestAPaneClosingBeforeItsRenameIsNotFatal(t *testing.T) {
	// A pane can close between the snapshot that listed it and the rename that
	// follows, and the poll it happens in must still finish.
	h := startPanes(t, oneTab(), split())
	h.client.ClosePane("wE:p2")

	if !h.poll() {
		t.Fatal("the loop stopped over a pane that closed")
	}
}

func TestNamingPanesCostsOneProcessReadPerPane(t *testing.T) {
	// The measured cost the setting exists for, stated as a number a change
	// would move: each pane is read once, and the tab's is not read twice.
	h := startPanes(
		t,
		oneTab(),
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t1", CWD: api},
			{PaneID: "wE:p3", TabID: "wE:t1", CWD: billing},
		},
	)
	h.poll()

	if got := h.client.ProcessReads(); got != 3 {
		t.Errorf("process reads = %d, want one per pane and no second for the tab's", got)
	}
}

// appFromConfig builds the App from the resolvers the configuration asks for,
// which is what decides what reaches the pane path and what does not.
func appFromConfig(t *testing.T, cfg Config) *App {
	t.Helper()
	setHome(t, filepath.Join(t.TempDir(), "home"))

	titles, panes := Resolvers(cfg)

	return New(cfg, discardLogger(), titles, panes)
}

func TestTheSettingsThatShapeATitleShapeAPaneLabel(t *testing.T) {
	// A pane is named by the chain that names tabs, so everything the user has
	// tuned about a title holds for a pane too. The position is the exception,
	// and the last case states which way round that goes.
	tests := []struct {
		name     string
		tune     func(*Config)
		wantPane string
		wantTab  string
	}{
		{
			"a pane carries the agent its tab does not",
			func(*Config) {},
			"billing › claude",
			"dashboard",
		},
		{
			"the agent name can be left out",
			func(c *Config) { c.ShowAgentName = false },
			"billing",
			"dashboard",
		},
		{
			"the length limit binds a pane too",
			func(c *Config) { c.MaxLength = 6 },
			"billin",
			"dashbo",
		},
		{
			"the position leads the tab and not the pane",
			func(c *Config) { c.ShowPosition = true },
			"billing › claude",
			"1 · dashboard",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := paneConfig()
			cfg.ShowAgentName = true
			tc.tune(&cfg)

			client := herdrtest.New(oneTab(), []herdr.PaneInfo{
				{PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true},
				{PaneID: "wE:p2", TabID: "wE:t1", CWD: billing, Agent: "claude"},
			})
			appFromConfig(t, cfg).poll(t.Context(), client)

			var got string

			for _, call := range client.PaneRenames() {
				if call.PaneID == "wE:p2" {
					got = call.Label
				}
			}

			if got != tc.wantPane {
				t.Errorf("pane = %q, want %q", got, tc.wantPane)
			}

			if tabs := client.Renames(); len(tabs) != 1 || tabs[0].Label != tc.wantTab {
				t.Errorf("tab = %v, want %q", tabs, tc.wantTab)
			}
		})
	}
}

func TestAPaneKeepsItsNameWhenItsTabIsClaimed(t *testing.T) {
	// A pane is named against its tab's own pane, which a claimed tab does not
	// read for itself. Unread, it has no branch, so a pane ordered before it
	// would take the branch back the moment the user named the tab.
	repo := repoAt(t, "feat/oauth")
	h := startPanes(t, oneTab(), []herdr.PaneInfo{
		{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Agent: "claude"},
		{PaneID: "wE:p2", TabID: "wE:t1", CWD: repo, Focused: true},
	})
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "Important work"})
	h.polls(2)

	if got := labelsOf(h, "wE:p1"); len(got) != 1 || got[0] != "claude" {
		t.Errorf("pane = %v, want it named once and kept when its tab was claimed", got)
	}
}
