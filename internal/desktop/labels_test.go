package desktop

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/labels"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

type renameRecorder struct {
	herdr.Client

	names []string
}

func (c *renameRecorder) RenameTab(_ context.Context, _, name string) error {
	c.names = append(c.names, name)
	return nil
}

func TestManualLauncherNamesAreProtected(t *testing.T) {
	t.Parallel()

	for _, manual := range []string{"My work", "Codex", "Claude Code"} {
		t.Run(manual, func(t *testing.T) {
			t.Parallel()

			client := &renameRecorder{}
			adapter := &Client{}
			path := filepath.Join(t.TempDir(), "manual.json")
			owner := labels.Load(path, nil, slog.New(slog.DiscardHandler), false)
			poll := func(name string) {
				item := tab{ID: "tab1", Name: name, Terminals: []paneEntry{{App: "codex"}}}
				snapshot, _ := adapter.build([]space{{ID: "space1", Tabs: []tab{item}}}, "", nil)
				pass := owner.Pass(client, snapshot)
				pass.Tab(
					t.Context(),
					state.TabFrom(snapshot.Tabs[0], "", 1, nil, false),
					resolver.Decision{Name: "1 - project"},
				)
				pass.Settle()
			}
			poll("Codex")
			poll("1 - project")

			client.names = nil

			poll(manual)
			poll(manual)

			adapter = &Client{}
			owner = labels.Load(path, nil, slog.New(slog.DiscardHandler), false)

			poll(manual)

			if len(client.names) != 0 {
				t.Fatalf("manual name %q overwritten with %q", manual, client.names)
			}
		})
	}
}

func TestLauncherHistoryEndsWhenTheTabCloses(t *testing.T) {
	t.Parallel()

	client := &Client{}

	read := func(name string) herdr.TabInfo {
		snapshot, _ := client.build([]space{{Tabs: []tab{{
			ID: "tab", Name: name, Terminals: []paneEntry{{App: "codex"}},
		}}}}, "", nil)

		return snapshot.Tabs[0]
	}
	for range 2 {
		if tab := read("Codex"); !tab.Unnamed || tab.Label != "Codex" {
			t.Fatalf("unchanged launcher name: %+v", tab)
		}
	}

	read("topic")

	if tab := read("Codex"); tab.Unnamed {
		t.Fatal("manual name treated as default")
	}

	client.build(nil, "", nil)

	if tab := read("Codex"); !tab.Unnamed {
		t.Fatal("closed tab history retained")
	}
}

// A tab the `+` menu starts is seen before its agent reports itself, and would
// be locked as the user's on that first poll, never to be titled.
func TestLauncherTabIsTitledBeforeItsAgentReports(t *testing.T) {
	t.Parallel()

	wrapper := []string{
		"/opt/homebrew/bin/fish",
		"-lic",
		"command $argv[2..]; exec $argv[1] -li",
		"/opt/homebrew/bin/fish",
		"codex",
	}

	for _, tc := range []struct {
		name   string
		argv   []string
		titled bool
	}{
		{"started by the launcher", wrapper, true},
		{"a plain shell", []string{"/opt/homebrew/bin/fish", "-l"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := &renameRecorder{}
			adapter := &Client{}
			owner := labels.Load(
				filepath.Join(t.TempDir(), "manual.json"),
				nil,
				slog.New(slog.DiscardHandler),
				false,
			)
			poll := func(app string) {
				world := world{Spaces: []space{{ID: "space1", Tabs: []tab{
					{
						ID:        "tab1",
						Name:      "Codex",
						Terminals: []paneEntry{{Terminal: "t1", Pane: "p1", App: app}},
					},
				}}}}
				local := world.local([]terminal{{Address: "t1", Argv: tc.argv}})
				snapshot, _ := adapter.build(local, "", nil)
				pass := owner.Pass(client, snapshot)
				pass.Tab(
					t.Context(),
					state.TabFrom(snapshot.Tabs[0], "", 1, nil, false),
					resolver.Decision{Name: "codex - topic"},
				)
				pass.Settle()
			}
			// The first poll of a process claims nothing, and this tab opens later.
			snapshot, _ := adapter.build(nil, "", nil)
			owner.Pass(client, snapshot).Settle()
			poll("")
			poll("codex")

			if titled := len(client.names) != 0; titled != tc.titled {
				t.Fatalf("renamed to %q, want titled %v", client.names, tc.titled)
			}
		})
	}
}
