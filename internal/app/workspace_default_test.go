package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
)

// The tab is locked from the lock file and panes are off, so nothing has read
// the pane before the row is judged; the snapshot's foreground_cwd is a
// descendant's directory, and the pane's own is only known once read.
func TestARowIsJudgedByItsPaneDirectoryNotTheSnapshotGuess(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	cfg := workspaceConfig(t)

	cfg.ManualPath = filepath.Join(t.TempDir(), "manual-names.json")
	if err := os.WriteFile(
		cfg.ManualPath,
		[]byte(`{"locked_tabs":{"wE:t1":"mine"}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wE:t1", WorkspaceID: "wE", Label: "mine"}},
		[]herdr.PaneInfo{{
			PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
			CWD:           repo,
			ForegroundCWD: herdrtest.Dir("work", "gimp-mcp"),
		}},
	)
	client.SetWorkspaces(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: filepath.Base(repo)})
	client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "node", CWD: repo})

	h := startConfigured(t, client, cfg)
	h.polls(3)

	if h.app.manual.Workspaces.Locked("wE") {
		t.Errorf("claimed a row still wearing its directory's basename %q", filepath.Base(repo))
	}

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")
}

// The claim pass reads through the first tab whatever the count, and that tab
// can be locked from the file too. Judged on the guess, a two-tab row would be
// claimed on the first poll and never named once it shrinks to one tab.
func TestAMultiTabRowIsJudgedByItsPaneDirectoryNotTheSnapshotGuess(t *testing.T) {
	repo := repoAt(t, "feat/oauth")

	cfg := workspaceConfig(t)

	cfg.ManualPath = filepath.Join(t.TempDir(), "manual-names.json")
	if err := os.WriteFile(
		cfg.ManualPath,
		[]byte(`{"locked_tabs":{"wE:t1":"mine"}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	client := herdrtest.New(
		[]herdr.TabInfo{
			{TabID: "wE:t1", WorkspaceID: "wE", Label: "mine"},
			{TabID: "wE:t2", WorkspaceID: "wE", Label: "2"},
		},
		[]herdr.PaneInfo{
			{
				PaneID: "wE:p1", TabID: "wE:t1", Focused: true,
				CWD:           repo,
				ForegroundCWD: herdrtest.Dir("work", "gimp-mcp"),
			},
			{PaneID: "wE:p2", TabID: "wE:t2", Focused: true, CWD: repo},
		},
	)
	client.SetWorkspaces(herdr.WorkspaceInfo{WorkspaceID: "wE", Label: filepath.Base(repo)})
	client.SetProcesses("wE:p1", herdr.PaneProcessInfoProcess{Name: "node", CWD: repo})

	h := startConfigured(t, client, cfg)
	h.poll()

	if h.app.manual.Workspaces.Locked("wE") {
		t.Errorf(
			"claimed a two-tab row still wearing its directory's basename %q",
			filepath.Base(repo),
		)
	}

	h.client.CloseTab("wE:t1")
	h.polls(3)

	wantRow(t, h, filepath.Base(repo)+" › feat/oauth")
}
