package app

import (
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
)

func TestHermesHomesConfiguration(t *testing.T) {
	isolate(t)
	custom, extra := t.TempDir(), t.TempDir()
	t.Setenv("HERMES_HOME", custom)
	t.Setenv("HERDR_AUTO_TITLE_HERMES_HOMES", extra+string(os.PathListSeparator)+custom)

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Fatal(warnings)
	}

	if !slices.Equal(cfg.HermesHomes, []string{custom, extra, custom}) {
		t.Fatalf("Hermes homes = %v", cfg.HermesHomes)
	}
}

func TestHermesTitlesFlowThroughPollAndRespectManualNames(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	const session = "20261021_133849_f560a2"
	if _, err := db.Exec(
		`CREATE TABLE sessions(id TEXT PRIMARY KEY,title TEXT); INSERT INTO sessions VALUES(?,?)`,
		session,
		"\x1b[31mNative\nDesktop\x1b[0m",
	); err != nil {
		t.Fatal(err)
	}

	info := herdr.PaneInfo{
		PaneID:       "wE:p1",
		TabID:        "wE:t1",
		CWD:          dashboard,
		Agent:        "hermes",
		Focused:      true,
		AgentSession: &herdr.AgentSessionInfo{Agent: "hermes", Kind: "id", Value: session},
	}
	client := herdrtest.New([]herdr.TabInfo{{TabID: "wE:t1", Label: "1"}}, []herdr.PaneInfo{info})
	cfg := testConfig()
	cfg.ReadSessions = true
	cfg.HermesHomes = []string{home}
	h := startConfigured(t, client, cfg)
	h.poll()

	renames := client.Renames()
	if len(renames) != 1 || !strings.HasSuffix(renames[0].Label, "Native Desktop") {
		t.Fatalf("native title not sanitized into tab: %v", renames)
	}

	if _, err := db.Exec(`UPDATE sessions SET title = 'Updated title'`); err != nil {
		t.Fatal(err)
	}

	h.poll()

	renames = client.Renames()
	if len(renames) != 2 || !strings.HasSuffix(renames[1].Label, "Updated title") {
		t.Fatalf("title did not refresh: %v", renames)
	}

	info.AgentSession.Value = "20261021_133850_123abc"
	client.SetPane(info)
	h.poll()

	renames = client.Renames()
	if len(renames) != 3 || strings.Contains(renames[2].Label, "Updated title") {
		t.Fatalf("new session retained old title: %v", renames)
	}

	info.AgentSession.Value = session
	client.SetPane(info)
	h.poll()

	renames = client.Renames()
	if len(renames) != 4 || !strings.HasSuffix(renames[3].Label, "Updated title") {
		t.Fatalf("resume lost title: %v", renames)
	}

	client.SetTab(herdr.TabInfo{TabID: "wE:t1", Label: "My manual tab"})
	h.poll()

	if _, err := db.Exec(`UPDATE sessions SET title = 'Must not rename manual tab'`); err != nil {
		t.Fatal(err)
	}

	h.poll()

	if len(client.Renames()) != len(renames) {
		t.Fatal("Hermes title overwrote manual tab ownership")
	}
}
