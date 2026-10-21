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
		client,
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
		client,
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

func writeOwned(t *testing.T, path, label string) {
	t.Helper()

	raw, err := json.Marshal(map[string]map[string]string{"owned_tabs": {theTab: label}})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A reloaded name is proof a label is ours, never that one is the user's:
// default mode must not lock a tab renamed while the plugin was down.
func TestDefaultModeDoesNotLockATabRenamedWhileDown(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "names.json")
	writeOwned(t, path, "1 · old")

	client := session("1 · changed while down")

	l := load(path)
	name(t, l, client, "Wanted")

	if pass, _ := open(t, l, client); pass.TabLocked(theTab) {
		t.Fatal("default mode locked a tab on its first poll")
	}

	if pass, _ := open(t, load(path), client); pass.TabLocked(theTab) {
		t.Fatal("the lock reached the file")
	}

	wantRenames(t, client, "Wanted")
}

// Session-only mode still claims a name that differs from the one saved.
func TestSessionOnlyClaimsATabRenamedWhileDown(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "names.json")
	writeOwned(t, path, "Ours")

	client := session("Mine")

	pass, tab := open(t, labels.Load(path, nil, slog.New(slog.DiscardHandler), true), client)
	pass.Tab(context.Background(), tab, resolver.Decision{Name: "Wanted"})

	if !pass.TabLocked(theTab) {
		t.Fatal("session-only mode renamed over a name changed while down")
	}

	wantRenames(t, client)
}

// A default-mode run that renamed nothing must still record its labels, or
// switching to session-only would lock every one of them.
func TestStableDefaultRunRecordsItsLabelsForSessionOnly(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "names.json")
	client := session("Ours")

	l := load(path)
	name(t, l, client, "Ours")
	name(t, l, client, "Ours")

	pass, tab := open(t, labels.Load(path, nil, slog.New(slog.DiscardHandler), true), client)
	pass.Tab(context.Background(), tab, resolver.Decision{Name: "Session name"})

	if pass.TabLocked(theTab) {
		t.Fatal("session-only mode locked a label the default run left in place")
	}

	wantRenames(t, client, "Session name")
}
