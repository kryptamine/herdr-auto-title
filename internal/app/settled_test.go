package app

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func TestSessionOnlyIgnoresTerminalAndAgentPlaceholders(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.SessionOnly = true
	titles, _, _ := Resolvers(cfg)
	pane := &state.PaneState{
		Agent:         "hermes",
		AgentTopic:    "Finished context name",
		AgentTitle:    "raw opening prompt",
		TerminalTitle: "Shell",
	}

	decision := titles.Resolve(state.TabState{Context: pane, Panes: []*state.PaneState{pane}})
	if decision.Name != "Finished context name" {
		t.Fatalf("not native title alone: %q", decision.Name)
	}
}

func TestSessionOnlyOwnNameSurvivesRestart(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	const sid = "20261021_120000_abcdef"
	if _, err := db.Exec(
		`CREATE TABLE sessions(id TEXT PRIMARY KEY,title TEXT,title_source TEXT); INSERT INTO sessions VALUES(?, 'First native name','llm')`,
		sid,
	); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	cfg.SessionOnly = true
	cfg.ReadSessions = true
	cfg.ManualPath = filepath.Join(home, "names.json")
	cfg.HermesHomes = []string{home}
	info := herdr.PaneInfo{
		PaneID:       "wS:p1",
		TabID:        "wS:t1",
		CWD:          home,
		Agent:        "hermes",
		Focused:      true,
		AgentSession: &herdr.AgentSessionInfo{Agent: "hermes", Kind: "id", Value: sid},
	}
	client := herdrtest.New([]herdr.TabInfo{{TabID: "wS:t1", Label: "1"}}, []herdr.PaneInfo{info})
	h := startConfigured(t, client, cfg)
	h.poll()

	if len(client.Renames()) != 1 {
		t.Fatal("initial title missing")
	}

	if _, err := db.Exec(`UPDATE sessions SET title='Updated native name'`); err != nil {
		t.Fatal(err)
	}

	restarted := startConfigured(t, client, cfg)
	restarted.poll()

	if got := client.Renames(); len(got) != 2 ||
		!strings.HasSuffix(got[1].Label, "Updated native name") {
		t.Fatalf("own name became manual on restart: %v", got)
	}
}

func TestSessionOnlyPreservesExistingUserName(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	const sid = "20261021_120000_abcdef"
	if _, err := db.Exec(
		`CREATE TABLE sessions(id TEXT PRIMARY KEY,title TEXT,title_source TEXT); INSERT INTO sessions VALUES(?, 'Generated name','llm')`,
		sid,
	); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	cfg.SessionOnly = true
	cfg.ReadSessions = true
	cfg.HermesHomes = []string{home}
	info := herdr.PaneInfo{
		PaneID:       "wS:p1",
		TabID:        "wS:t1",
		CWD:          home,
		Agent:        "hermes",
		Focused:      true,
		AgentSession: &herdr.AgentSessionInfo{Agent: "hermes", Kind: "id", Value: sid},
	}
	client := herdrtest.New(
		[]herdr.TabInfo{{TabID: "wS:t1", Label: "My chosen name"}},
		[]herdr.PaneInfo{info},
	)
	h := startConfigured(t, client, cfg)
	h.poll()

	if got := client.Renames(); len(got) != 0 {
		t.Fatalf("overwrote pre-existing user name: %v", got)
	}
}

func TestSessionOnlyWaitsForFinishedHermesTitle(t *testing.T) {
	isolate(t)
	t.Setenv("HERDR_AUTO_TITLE_SESSION_ONLY", "true")

	cfg, _ := LoadConfig()
	cfg.Poll = testPoll
	cfg.RenamePanes = false
	cfg.ReportWorkspaces = false
	home := t.TempDir()
	cfg.Home = home
	cfg.HermesHomes = []string{home}
	cfg.ShowPosition = false

	const sid = "20261021_120000_abcdef"

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	if _, err := db.Exec(
		`CREATE TABLE sessions(id TEXT PRIMARY KEY,title TEXT,title_source TEXT); INSERT INTO sessions VALUES(?, '', '')`,
		sid,
	); err != nil {
		t.Fatal(err)
	}

	info := herdr.PaneInfo{PaneID: "wS:p1", TabID: "wS:t1", CWD: home, Focused: true}
	client := herdrtest.New([]herdr.TabInfo{{TabID: "wS:t1", Label: "1"}}, []herdr.PaneInfo{info})
	h := startConfigured(t, client, cfg)
	h.poll()

	if got := client.Renames(); len(got) != 0 {
		t.Fatalf("shell renamed before context: %v", got)
	}

	info.Agent = "hermes"
	info.AgentSession = &herdr.AgentSessionInfo{Agent: "hermes", Kind: "id", Value: sid}
	client.SetPane(info)
	h.poll()

	if got := client.Renames(); len(got) != 0 {
		t.Fatalf("agent renamed before title: %v", got)
	}

	if _, err := db.Exec(
		`UPDATE sessions SET title='the raw opening prompt',title_source='derived'`,
	); err != nil {
		t.Fatal(err)
	}

	h.poll()

	if got := client.Renames(); len(got) != 0 {
		t.Fatalf("provisional title leaked: %v", got)
	}

	if _, err := db.Exec(
		`UPDATE sessions SET title='Useful conversation name',title_source='llm'`,
	); err != nil {
		t.Fatal(err)
	}

	h.poll()

	if got := client.Renames(); len(got) != 1 ||
		!strings.HasSuffix(got[0].Label, "Useful conversation name") {
		t.Fatalf("finished title not applied once: %v", got)
	}

	info.Agent = ""
	info.AgentSession = nil
	client.SetPane(info)
	h.poll()

	if got := client.Renames(); len(got) != 1 {
		t.Fatalf("title regressed to shell: %v", got)
	}

	info.Agent = "hermes"
	info.AgentSession = &herdr.AgentSessionInfo{Agent: "hermes", Kind: "id", Value: sid}
	client.SetPane(info)
	client.SetTab(herdr.TabInfo{TabID: "wS:t1", Label: "My custom title"})

	if _, err := db.Exec(
		`UPDATE sessions SET title='Later native name',title_source='user'`,
	); err != nil {
		t.Fatal(err)
	}

	h.poll()

	if got := client.Renames(); len(got) != 1 {
		t.Fatalf("manual rename overwritten: %v", got)
	}
}
