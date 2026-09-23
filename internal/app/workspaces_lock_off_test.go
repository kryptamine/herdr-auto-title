package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// A workspace lock outlives the setting that took it, and is pruned on every
// poll the way a pane's is, whether or not the row is being named.
func TestAStaleWorkspaceLockIsReleasedWhenTheRowIsNotNamed(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.ManualPath = filepath.Join(t.TempDir(), "manual-names.json")

	// Written by an earlier run with the row turned on.
	stored := `{"locked_tabs":{},"locked_panes":{},"locked_workspaces":{"wE":"the migration"}}`
	if err := os.WriteFile(cfg.ManualPath, []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}

	// The session no longer holds wE at all.
	h := startWorkspaces(t, cfg,
		[]herdr.WorkspaceInfo{{WorkspaceID: "wF", Label: "dashboard"}},
		[]herdr.TabInfo{{TabID: "wF:t1", WorkspaceID: "wF", Label: "1"}},
		[]herdr.PaneInfo{{PaneID: "wF:p1", TabID: "wF:t1", CWD: dashboard, Focused: true}},
	)
	h.polls(2)

	if h.app.manual.Workspaces.Locked("wE") {
		t.Errorf("wE is still locked after two polls of a session that does not hold it")
	}

	raw, err := os.ReadFile(cfg.ManualPath)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(raw), "the migration") {
		t.Errorf("the lock file still carries wE:\n%s", raw)
	}
}
