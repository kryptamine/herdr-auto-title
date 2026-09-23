package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// workspaceConfig is testConfig with the row turned on, which is the only way
// it is ever named, and a file for the labels it writes, without which it is not.
func workspaceConfig(t *testing.T) Config {
	t.Helper()

	cfg := testConfig()
	cfg.RenameWorkspaces = true
	cfg.WorkspaceMaxLength = resolver.DefaultWorkspaceMaxLength
	cfg.ManualPath = filepath.Join(t.TempDir(), "manual-names.json")

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

// oneTab is the shape the feature is for: a workspace holding a single tab,
// still carrying the label Herdr derived from its directory.
func soleTabWorkspace(t *testing.T, cfg Config, dir string, pane herdr.PaneInfo) *harness {
	t.Helper()

	pane.PaneID, pane.TabID, pane.Focused = "wE:p1", "wE:t1", true
	if pane.CWD == "" {
		pane.CWD = dir
	}

	return startWorkspaces(t, cfg,
		[]herdr.WorkspaceInfo{{WorkspaceID: "wE", Label: filepath.Base(dir)}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"}},
		[]herdr.PaneInfo{pane},
	)
}

// theWorkspace is the id every fixture here builds its single workspace under.
const theWorkspace = "wE"

// wantRow asserts that the workspace, and only it, was named, and that it ended
// on want.
func wantRow(t *testing.T, h *harness, want string) {
	t.Helper()

	id := theWorkspace

	renames := h.client.WorkspaceRenames()
	if len(renames) == 0 {
		t.Fatalf("no row was named, want %s = %q", id, want)
	}

	last := ""

	for _, rename := range renames {
		if rename.WorkspaceID != id {
			t.Errorf("named %s as well: %q", rename.WorkspaceID, rename.Label)
			continue
		}

		last = rename.Label
	}

	if last != want {
		t.Errorf("%s named %q, want %q", id, last, want)
	}
}

func wantRowUntouched(t *testing.T, h *harness, why string) {
	t.Helper()

	if renames := h.client.WorkspaceRenames(); len(renames) != 0 {
		t.Errorf("%s, but the row was renamed: %v", why, renames)
	}
}

func TestAWorkspaceWithOneTabIsNamedAfterIt(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := soleTabWorkspace(t, workspaceConfig(t), repo, herdr.PaneInfo{})
	h.poll()

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")
}

// The rule is exactly one tab: there is no cross-tab answer to give, so a
// workspace that has grown a second tab is left where it is.
func TestAWorkspaceWithSeveralTabsIsLeftAlone(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: "wE", Label: filepath.Base(repo)}},
		[]herdr.TabInfo{
			{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"},
			{TabID: "wE:t2", WorkspaceID: "wE", Label: "2"},
		},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t2", CWD: repo},
		},
	)
	h.polls(2)

	wantRowUntouched(t, h, "the workspace holds two tabs")
}

// Herdr labels an unnamed workspace after its directory, so any other label on
// the first poll is one its owner wrote, and this never takes it back.
func TestALabelThatIsNotTheDirectoryIsTreatedAsTheUsers(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: "wE", Label: "the migration"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true}},
	)
	h.polls(3)

	wantRowUntouched(t, h, "the row carries a name its owner chose")
}

// The way out of a workspace lock, the same gesture as for a tab: put the
// directory's own name back on the row and it is nobody's again. The claim is
// released by Retain, which sees the label the lock was taken under move on.
func TestClearingAClaimedRowHandsItBack(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: "wE", Label: "the migration"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true}},
	)
	h.polls(2)
	wantRowUntouched(t, h, "the row carries a name its owner chose")

	h.client.SetWorkspaces(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: filepath.Base(repo)})
	h.polls(2)

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")
}

// A row the plugin named and the user then took is handed back the same way.
func TestARowTakenAfterItWasNamedIsHandedBackWhenCleared(t *testing.T) {
	repo := repoAt(t, "feat/oauth")
	want := filepath.Base(repo) + " › feat/oauth"

	h := soleTabWorkspace(t, workspaceConfig(t), repo, herdr.PaneInfo{})
	h.polls(2)
	wantRow(t, h, want)

	h.client.SetWorkspaces(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: "the migration"})
	h.polls(2)

	if n := len(h.client.WorkspaceRenames()); n != 1 {
		t.Fatalf("issued %d renames, want only the one before the user took the row", n)
	}

	h.client.SetWorkspaces(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: filepath.Base(repo)})
	h.polls(2)

	if n := len(h.client.WorkspaceRenames()); n != 2 {
		t.Errorf("issued %d renames, want the row named again once it was handed back", n)
	}
}

// Two workspaces open in one directory would resolve alike, and replacing both
// rows would leave the sidebar unable to tell them apart. Only the one still
// wearing the directory's own name is taken.
func TestOnlyTheRowStillWearingItsDirectoryIsTaken(t *testing.T) {
	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{
			{WorkspaceID: "wE", Label: "reviewing"},
			{WorkspaceID: "wF", Label: filepath.Base(dashboard)},
		},
		[]herdr.TabInfo{
			{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"},
			{TabID: "wF:t1", WorkspaceID: "wF", Label: "1"},
		},
		[]herdr.PaneInfo{
			{
				PaneID:  "wE:p1",
				TabID:   "wE:t1",
				CWD:     dashboard,
				Focused: true,
				Agent:   "claude",
				Title:   "Review the API",
			},
			{
				PaneID:  "wF:p1",
				TabID:   "wF:t1",
				CWD:     dashboard,
				Focused: true,
				Agent:   "claude",
				Title:   "Ship the API",
			},
		},
	)
	h.polls(2)

	taken := false

	for _, rename := range h.client.WorkspaceRenames() {
		switch rename.WorkspaceID {
		case "wE":
			t.Errorf("took a row its owner named: %q", rename.Label)
		case "wF":
			taken = true
		}
	}

	if !taken {
		t.Error("the row still wearing its directory was not taken either")
	}
}

// A workspace opened after startup carries its directory's name, so it is taken
// like any other rather than counted as something the user wrote.
func TestAWorkspaceOpenedAfterStartupIsStillTaken(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t), nil, nil, nil)
	h.poll()

	h.client.SetWorkspaces(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: filepath.Base(repo)})
	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"})
	h.client.SetPane(herdr.PaneInfo{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true})
	h.polls(2)

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")
}

// A workspace outlives every command typed in it, so the foreground process is
// the one source the row's chain leaves out. The tab beside it still takes the
// process, which is what keeps the row steady while the tab moves.
func TestTheRowDoesNotFollowTheForegroundProcess(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := soleTabWorkspace(t, workspaceConfig(t), repo, herdr.PaneInfo{})
	h.client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{
		Name: "npm",
		Argv: []string{"npm", "run", "build"},
		CWD:  repo,
	})
	h.polls(2)

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")

	for _, rename := range h.client.WorkspaceRenames() {
		if strings.Contains(rename.Label, "npm") {
			t.Errorf("the row followed the foreground process: %q", rename.Label)
		}
	}

	if tabs := h.client.Renames(); len(tabs) == 0 ||
		!strings.Contains(tabs[len(tabs)-1].Label, "npm") {
		t.Errorf("the tab should still take the process, got %v", tabs)
	}
}

// The row's chain leaves the process source out, and the terminal title must
// not bring the process back as a kind: a row is not rewritten from
// `nvim › auth.ts` to `less › auth.ts` when only the foreground process moves.
func TestTheRowDoesNotFollowTheForegroundProcessBehindATerminalTitle(t *testing.T) {
	h := soleTabWorkspace(t, workspaceConfig(t), dashboard,
		herdr.PaneInfo{TerminalTitleStripped: "auth.ts"})
	h.client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "nvim", CWD: dashboard})
	h.polls(2)

	h.client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "less", CWD: dashboard})
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: dashboard, TerminalTitleStripped: "auth.ts",
	})
	h.polls(2)

	wantRow(t, h, "dashboard › auth.ts")

	for _, rename := range h.client.WorkspaceRenames() {
		if strings.Contains(rename.Label, "nvim") || strings.Contains(rename.Label, "less") {
			t.Errorf("the row followed the foreground process: %q", rename.Label)
		}
	}

	if tabs := h.client.Renames(); len(tabs) == 0 ||
		!strings.Contains(tabs[len(tabs)-1].Label, "less") {
		t.Errorf("the tab should still take the process, got %v", tabs)
	}
}

func TestTheRowIsLeftAloneUnlessItIsAskedFor(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := soleTabWorkspace(t, testConfig(), repo, herdr.PaneInfo{})
	h.polls(2)

	wantRowUntouched(t, h, "the setting is off")
}

func TestAWorkspaceWhoseTabHasNoPaneIsLeftAlone(t *testing.T) {
	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: "wE", Label: "dashboard"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: "wE", Label: "1"}},
		nil,
	)
	h.polls(2)

	wantRowUntouched(t, h, "the tab holds no pane")
}

// A claimed tab is not read for its own sake, but the row above it is still
// named, so the row costs that read where pane naming is off, as it is in this
// package's tests (see poll-loop.md). The work moves after the tab is claimed.
func TestARowAboveAClaimedTabIsStillNamed(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := soleTabWorkspace(t, workspaceConfig(t), repo, herdr.PaneInfo{})
	h.poll()

	// The user names the tab, which takes it out of naming from here on.
	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", WorkspaceID: "wE", Label: "mine"})
	h.polls(2)

	// The work moves. Only a poll that still reads the claimed tab's pane can
	// see it, and only that keeps the row current.
	repoIn(t, repo, "feat/billing")
	h.polls(2)

	wantRow(t, h, filepath.Base(repo)+" › feat/billing")

	for _, rename := range h.client.Renames() {
		if rename.TabID == "wE:t1" && rename.Label != "mine" {
			if rename.Label == filepath.Base(repo)+" › feat/billing" {
				t.Errorf("renamed a tab the user claimed: %q", rename.Label)
			}
		}
	}
}

// claimedTabReads counts pane.process_info requests spent on a sole-tab
// workspace after the user has claimed the tab, while its pane keeps drawing.
func claimedTabReads(t *testing.T, cfg Config) int {
	t.Helper()

	repo := repoAt(t, "feat/oauth")
	h := soleTabWorkspace(t, cfg, repo, herdr.PaneInfo{})
	h.poll()

	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", WorkspaceID: "wE", Label: "mine"})
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
// its panes, so the row above it is named from a read the poll has paid for.
func TestARowAboveAClaimedTabCostsNoReadWhilePanesAreNamed(t *testing.T) {
	both := paneConfig()
	both.RenameWorkspaces = true
	both.WorkspaceMaxLength = resolver.DefaultWorkspaceMaxLength

	without := claimedTabReads(t, paneConfig())
	with := claimedTabReads(t, both)

	if without == 0 {
		t.Fatalf("the claimed tab's pane was never read for its panes, so nothing is compared")
	}

	if with != without {
		t.Errorf(
			"naming the row cost %d reads on top of the %d pane naming spent, want 0",
			with-without,
			without,
		)
	}
}

// With pane naming off, that pane is read for the row alone.
func TestARowAboveAClaimedTabCostsAReadWhilePanesAreNotNamed(t *testing.T) {
	without := claimedTabReads(t, testConfig())
	with := claimedTabReads(t, workspaceConfig(t))

	if without != 0 || with == 0 {
		t.Errorf("reads: %d without the row, %d with; want 0 and >0", without, with)
	}
}

// The row is what a tab deduplicates against, and Herdr stores it as one
// string. A tab handed the whole label would match neither half of a
// two-part row and repeat both.
func TestATabDoesNotRepeatTheRowAboveIt(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := soleTabWorkspace(t, workspaceConfig(t), repo,
		herdr.PaneInfo{TerminalTitleStripped: "auth.ts"})
	h.polls(3)

	// The row is over twenty columns with its context, so the context is the
	// part it loses.
	wantRow(t, h, "feat/oauth › auth.ts")

	tabs := h.client.Renames()
	if len(tabs) == 0 {
		t.Fatal("the tab was never named")
	}
	// The branch goes, because the row carries it. The context stays, because
	// the row lost it: a tab repeats nothing it can still read above itself.
	// Without the fix the tab reads "001 › feat/oauth › auth.ts".
	if got := tabs[len(tabs)-1].Label; got != filepath.Base(repo)+" › auth.ts" {
		t.Errorf("tab named %q, want %q", got, filepath.Base(repo)+" › auth.ts")
	}
}

// The first poll is the only one that can tell a workspace's owner apart from
// Herdr, so it judges every workspace -- including the ones it cannot name. A
// workspace that holds two tabs today may hold one tomorrow.
func TestAMultiTabWorkspaceKeepsItsNameWhenATabCloses(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: "release"}},
		[]herdr.TabInfo{
			{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"},
			{TabID: "wE:t2", WorkspaceID: theWorkspace, Label: "2"},
		},
		[]herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true},
			{PaneID: "wE:p2", TabID: "wE:t2", CWD: repo},
		},
	)
	h.poll()

	h.client.CloseTab("wE:t2")
	h.polls(3)

	wantRowUntouched(t, h, "the row was named before it held one tab")
}

// The row's chain is the shipped one under the shipped settings, so a user who
// turned the agent's name off does not get it back in the sidebar. The title
// is short so the bound, which drops parts from the front, cannot be what
// leaves the agent's name out.
func TestTheRowObeysTheAgentNameSetting(t *testing.T) {
	cfg := workspaceConfig(t)
	cfg.ShowAgentName = false

	h := soleTabWorkspace(t, cfg, dashboard, herdr.PaneInfo{
		Agent: "claude",
		Title: "Fix",
	})
	h.polls(2)

	wantRow(t, h, "dashboard › Fix")

	for _, rename := range h.client.WorkspaceRenames() {
		if strings.Contains(rename.Label, "claude") {
			t.Errorf("the row carries the agent's name: %q", rename.Label)
		}
	}
}

// A tab reads the row above it as parts only where this wrote that row. With
// the feature off the label is whatever its owner typed, separator and all, and
// a tab must name itself exactly as it did before this branch.
func TestTheFeatureOffLeavesTabNamingWhereItWas(t *testing.T) {
	h := startWorkspaces(t, testConfig(),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: "group › dashboard"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", CWD: dashboard, Focused: true,
			TerminalTitleStripped: "nvim",
		}},
	)
	h.polls(2)

	tabs := h.client.Renames()
	if len(tabs) == 0 {
		t.Fatal("the tab was never named")
	}
	// The whole label matches nothing, so the context stays -- which is what
	// the unsplit comparison on main does.
	if got := tabs[len(tabs)-1].Label; got != filepath.Base(dashboard)+" › nvim" {
		t.Errorf("tab named %q, want %q", got, filepath.Base(dashboard)+" › nvim")
	}
}

// With the row left alone its label is also matched against the context alone,
// as it was before this branch. A branch that happens to equal the label is a
// branch the tab still has to say.
func TestTheFeatureOffMatchesTheLabelAgainstTheContextAlone(t *testing.T) {
	repo := repoAt(t, "release")

	h := startWorkspaces(t, testConfig(),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: "release"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true,
			TerminalTitleStripped: "nvim",
		}},
	)
	h.polls(2)

	tabs := h.client.Renames()
	if len(tabs) == 0 {
		t.Fatal("the tab was never named")
	}

	if want := filepath.Base(repo) + " › release › nvim"; tabs[len(tabs)-1].Label != want {
		t.Errorf("tab named %q, want %q", tabs[len(tabs)-1].Label, want)
	}
}

// A tab can exist before its pane does. The row is not named on that poll, but
// its label is still judged, because that poll is the only one that can tell
// its owner's name from Herdr's own.
func TestAWorkspaceWhosePaneArrivesLateKeepsItsName(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: "the migration"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"}},
		nil,
	)
	h.poll()

	h.client.SetPane(herdr.PaneInfo{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true})
	h.polls(3)

	wantRowUntouched(t, h, "the row was named before its pane existed")
}

// A tab can exist before its pane does, and a row still wearing the basename
// Herdr gave it is nobody's name. Without a pane there is no directory to
// compare against, which must read as "cannot tell yet", not as a claim.
func TestAWorkspaceStillWearingItsBasenameIsNotClaimedBeforeItsPaneExists(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: filepath.Base(repo)}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"}},
		nil,
	)
	h.poll()

	if h.app.manual.Workspaces.Locked(theWorkspace) {
		t.Errorf("claimed a row still wearing its directory's basename, before any pane existed")
	}

	h.client.SetPane(herdr.PaneInfo{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true})
	h.polls(3)

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")
}

// A workspace opened after the first poll with a tab but no pane shows no
// default to judge by either, so it is neither claimed nor named until its
// pane arrives, and then flows through the general path like any other.
func TestAPanelessWorkspaceOpenedLaterIsNamedOnceItsPaneArrives(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t), nil, nil, nil)
	h.poll()

	h.client.SetWorkspaces(
		herdr.WorkspaceInfo{WorkspaceID: theWorkspace, Label: filepath.Base(repo)},
	)
	h.client.SetTab(herdr.TabInfo{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"})
	h.polls(2)

	wantRowUntouched(t, h, "the tab holds no pane yet")

	h.client.SetPane(herdr.PaneInfo{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true})
	h.polls(2)

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")
}

// The claim pass reads the directory through the first tab, whatever the
// count. A two-tab workspace whose first tab has no pane yet shows no default
// either, and is judged once it does rather than claimed under the basename.
func TestATwoTabWorkspaceWhoseFirstTabHasNoPaneIsNotClaimed(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: filepath.Base(repo)}},
		[]herdr.TabInfo{
			{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"},
			{TabID: "wE:t2", WorkspaceID: theWorkspace, Label: "2"},
		},
		[]herdr.PaneInfo{{PaneID: "wE:p2", TabID: "wE:t2", CWD: repo, Focused: true}},
	)
	h.poll()

	if h.app.manual.Workspaces.Locked(theWorkspace) {
		t.Errorf("claimed a two-tab row still wearing its directory's basename")
	}

	h.client.CloseTab("wE:t1")
	h.polls(3)

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")
}

// A lock read back from disk has no sighting behind it and a locked row is not
// named, so no poll recorded the label the lock was taken with. The user's next
// rename must still read as a label that moved, not as a workspace never seen.
func TestAReloadedWorkspaceLockSurvivesASecondRename(t *testing.T) {
	repo := repoAt(t, "feat/oauth")
	cfg := workspaceConfig(t)
	cfg.ManualPath = filepath.Join(t.TempDir(), "manual-names.json")

	h := startWorkspaces(t, cfg,
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: "the migration"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wE:p1", TabID: "wE:t1", CWD: repo, Focused: true}},
	)
	h.polls(2)
	wantRowUntouched(t, h, "the user named the row before the first poll")

	// The plugin restarts against the same session and the same lock file.
	restarted := startConfigured(t, h.client, cfg)
	restarted.polls(2)
	wantRowUntouched(t, restarted, "the lock was read back")

	restarted.client.SetWorkspaces(
		herdr.WorkspaceInfo{WorkspaceID: theWorkspace, Label: "the migration, again"},
	)
	restarted.polls(2)
	wantRowUntouched(t, restarted, "the user renamed the row a second time")

	if !restarted.app.manual.Workspaces.Locked(theWorkspace) {
		t.Error("the row renamed twice by the user is not locked")
	}
}

// The common shape: a sole tab with a context and a branch and nothing else, so
// the row named after it says everything the tab does. On the polls after the
// row is named the tab must keep its title rather than fall back to "Shell".
func TestATabSaidWhollyByTheRowKeepsItsTitle(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	h := soleTabWorkspace(t, workspaceConfig(t), repo, herdr.PaneInfo{})
	h.polls(3)

	want := filepath.Base(repo) + " › feat/oauth"
	wantRow(t, h, want)

	tabs := h.client.Renames()
	if len(tabs) == 0 {
		t.Fatal("the tab was never named")
	}

	if got := tabs[len(tabs)-1].Label; got != want {
		t.Errorf("tab named %q, want %q", got, want)
	}
}

// A pane rooted at the filesystem has no basename, so the first poll cannot
// tell "mine" from Herdr's own label; the row must wait, not be renamed over.
func TestAWorkspaceWithoutABasenameIsNotRenamedBeforeItIsJudged(t *testing.T) {
	h := startWorkspaces(t, workspaceConfig(t),
		[]herdr.WorkspaceInfo{{WorkspaceID: theWorkspace, Label: "mine"}},
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", CWD: string(filepath.Separator), Focused: true,
			TerminalTitleStripped: "auth.ts",
		}},
	)
	h.poll()

	wantRowUntouched(t, h, "the first poll has no basename to compare against")

	if h.app.manual.Workspaces.Locked(theWorkspace) {
		t.Fatal("claimed a row before any basename could be compared against")
	}

	// The first poll that shows a basename judges it, and "mine" is not one.
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", CWD: t.TempDir(), Focused: true,
		TerminalTitleStripped: "auth.ts",
	})
	h.polls(3)

	if !h.app.manual.Workspaces.Locked(theWorkspace) {
		t.Error(
			"a label that is not the basename was not claimed on the first poll that showed one",
		)
	}

	wantRowUntouched(t, h, "the row wears its owner's name")
}

func TestARowRenameLandingAfterItsCallFailedIsRenamedOver(t *testing.T) {
	// A stalled Herdr applies a row rename after the call timed out, and the
	// row has moved on by then. Read as the user's, it would be locked for good.
	repo := repoAt(t, "feat/oauth")
	h := soleTabWorkspace(t, workspaceConfig(t), repo, herdr.PaneInfo{})
	h.poll()

	h.client.SetRenameError(herdr.ErrUnanswered)
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2,
		CWD: repo, TerminalTitleStripped: "auth.ts",
	})
	h.poll()

	h.client.SetRenameError(nil)
	h.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 3,
		CWD: repo, TerminalTitleStripped: "billing.ts",
	})
	h.poll()

	wantRow(t, h, "billing.ts")

	if h.app.manual.Workspaces.Locked(theWorkspace) {
		t.Error("a row wearing a rename of this plugin's own was claimed for the user")
	}
}

// Herdr runs the startup hook again at every start and live handoff, against
// the same session and the same lock file. A row this plugin wrote is then
// neither the directory's basename nor empty, and must not read as the owner's.
func TestARowAutoTitleNamedIsNotClaimedAfterARestart(t *testing.T) {
	repo := repoAt(t, "feat/oauth")
	cfg := workspaceConfig(t)
	cfg.ManualPath = filepath.Join(t.TempDir(), "manual-names.json")

	h := soleTabWorkspace(t, cfg, repo, herdr.PaneInfo{})
	h.polls(2)

	named := filepath.Base(repo) + " › feat/oauth"
	wantRow(t, h, named)

	restarted := startConfigured(t, h.client, cfg)
	restarted.polls(2)

	if restarted.app.manual.Workspaces.Locked(theWorkspace) {
		t.Errorf("the row Auto Title wrote (%q) was claimed as the user's", named)
	}

	head := filepath.Join(repo, ".git", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/feat/billing\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	restarted.client.SetPane(herdr.PaneInfo{
		PaneID: "wE:p1", TabID: "wE:t1", Focused: true, Revision: 2, CWD: repo,
	})
	restarted.polls(3)

	wantRow(t, restarted, filepath.Base(repo)+" › feat/billing")
}

// With locks kept in memory only, a restart would find the row this named
// wearing a label it cannot recognise and claim it for the user; rather than
// name it once and freeze it, the row is left alone.
func TestTheRowIsLeftAloneWhenLocksHaveNowhereToLive(t *testing.T) {
	repo := repoAt(t, "feat/oauth")
	cfg := workspaceConfig(t)
	cfg.ManualPath = ""

	h := soleTabWorkspace(t, cfg, repo, herdr.PaneInfo{})
	h.polls(2)

	wantRowUntouched(t, h, "the locks live in memory only")
}

// The tab chain must not read the row as parts either: with the row never
// written, a label is the user's opaque text and is matched whole, as with the
// feature off. Resolvers is what production builds from, so it is asked directly.
func TestTheTabChainMatchesTheRowWholeWhenLocksHaveNowhereToLive(t *testing.T) {
	cfg := workspaceConfig(t)
	cfg.ManualPath = ""

	cfg.Home = testHome(t)

	titles, _, workspaces := Resolvers(cfg)
	if workspaces != nil {
		t.Fatal("a workspace resolver was built with nowhere to keep what it writes")
	}

	tab := state.TabFrom(
		herdr.TabInfo{TabID: "wE:t1", WorkspaceID: theWorkspace, Label: "1"},
		"group › dashboard",
		1,
		[]*state.PaneState{
			{
				ID:            "wE:p1",
				Dir:           herdrtest.Dir("work", "dashboard"),
				TerminalTitle: "nvim",
				Focused:       true,
			},
		},
		false,
	)

	if got := titles.Resolve(tab).Name; got != "dashboard › nvim" {
		t.Errorf("tab named %q, want \"dashboard › nvim\": the row read as parts", got)
	}
}
