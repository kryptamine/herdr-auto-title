package labels_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/labels"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func TestOwnedTabsAndPanesAreSavedAndReloadedAcrossModes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "names.json")
	info := herdr.PaneInfo{PaneID: "wE:p1", TabID: theTab}
	client := herdrtest.New([]herdr.TabInfo{{TabID: theTab, Label: "1"}}, []herdr.PaneInfo{info})
	l := load(path)
	pass, tab := open(t, l, client)
	pass.Tab(context.Background(), tab, resolver.Decision{Name: "Owned tab"})
	pass.Pane(
		context.Background(),
		state.PaneFrom(info, "", time.Time{}),
		resolver.Decision{Name: "Owned pane"},
	)
	pass.Settle()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var saved struct {
		Tabs  map[string]string `json:"owned_tabs"`
		Panes map[string]string `json:"owned_panes"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}

	if saved.Tabs[theTab] != "Owned tab" || saved.Panes[info.PaneID] != "Owned pane" {
		t.Fatalf("missing persisted names: %s", raw)
	}

	reloaded := labels.Load(path, nil, slog.New(slog.DiscardHandler), true)
	pass, tab = open(t, reloaded, client)
	pass.Tab(context.Background(), tab, resolver.Decision{Name: "Updated tab"})

	info.Label = "Owned pane"
	pass.Pane(
		context.Background(),
		state.PaneFrom(info, "", time.Time{}),
		resolver.Decision{Name: "Updated pane"},
	)

	if pass.TabLocked(theTab) || pass.PaneLocked(info.PaneID) {
		t.Fatal("owned names became manual")
	}

	wantRenames(t, client, "Owned tab", "Updated tab")

	if got := client.PaneRenames(); len(got) != 2 || got[1].Label != "Updated pane" {
		t.Fatalf("pane renames = %v", got)
	}
}
