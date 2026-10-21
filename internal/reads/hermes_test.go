package reads_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/reads"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

const hermesSession = "20261021_133849_f560a2"

func hermesDB(t *testing.T, home, title string) *sql.DB {
	t.Helper()

	if err := os.MkdirAll(home, 0o750); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	if _, err := db.Exec(
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, title TEXT, title_source TEXT, cwd TEXT)`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(
		`INSERT INTO sessions (id,title) VALUES (?,?)`,
		hermesSession,
		title,
	); err != nil {
		t.Fatal(err)
	}

	return db
}

func hermesPane() *state.PaneState {
	pane := paneAt("wE:p1", "")
	pane.Agent = "hermes"
	pane.AgentSession = &herdr.AgentSessionInfo{Agent: "hermes", Kind: "id", Value: hermesSession}

	return pane
}

func TestHermesNativeSessionTitle(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	hermesDB(t, filepath.Join(home, ".hermes"), "Native desktop title")

	pane := hermesPane()
	readOne(reads.Options{Home: home, ReadSessions: true}, pane)

	if pane.AgentTopic != "Native desktop title" {
		t.Fatalf("topic = %q, want native Hermes title", pane.AgentTopic)
	}
}

func TestHermesConfiguredHomes(t *testing.T) {
	t.Parallel()
	custom := t.TempDir()
	hermesDB(t, filepath.Join(custom, "profiles", "work"), "Custom profile title")

	pane := hermesPane()
	readOne(reads.Options{HermesHomes: []string{custom}, ReadSessions: true}, pane)

	if pane.AgentTopic != "Custom profile title" {
		t.Fatalf("topic = %q", pane.AgentTopic)
	}
}

func TestHermesProfilesAndAmbiguity(t *testing.T) {
	t.Parallel()

	for _, duplicate := range []bool{false, true} {
		t.Run(strconv.FormatBool(duplicate), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			hermesDB(t, filepath.Join(home, ".hermes", "profiles", "work"), "Work title")

			want := "Work title"

			if duplicate {
				hermesDB(t, filepath.Join(home, ".hermes"), "Default title")

				want = ""
			}

			pane := hermesPane()
			readOne(reads.Options{Home: home, ReadSessions: true}, pane)

			if pane.AgentTopic != want {
				t.Fatalf("topic = %q, want %q", pane.AgentTopic, want)
			}
		})
	}
}

func TestHermesTitleRefreshAndSessionSwitch(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	db := hermesDB(t, filepath.Join(home, ".hermes"), "Original title")
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}

	reader := newReader(reads.Options{Home: home, ReadSessions: true})
	client := herdrtest.New(nil, nil)
	pane := hermesPane()
	fill := func() { reader.Poll(client, nil, nil).Fill(context.Background(), pane) }
	fill()

	if pane.AgentTopic != "Original title" {
		t.Fatal(pane.AgentTopic)
	}

	if _, err := db.Exec(`UPDATE sessions SET title = 'Refreshed title'`); err != nil {
		t.Fatal(err)
	}

	fill()

	if pane.AgentTopic != "Refreshed title" {
		t.Fatalf("stale topic = %q", pane.AgentTopic)
	}

	pane.AgentSession.Value = "20261021_133850_123abc"

	fill()

	if pane.AgentTopic != "" {
		t.Fatalf("new session retained old title: %q", pane.AgentTopic)
	}

	pane.AgentSession.Value = hermesSession

	fill()

	if pane.AgentTopic != "Refreshed title" {
		t.Fatalf("resumed topic = %q", pane.AgentTopic)
	}
}

func TestHermesSessionGating(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, agent, refAgent, kind, id string }{
		{"wrong pane agent", "claude", "hermes", "id", hermesSession},
		{"wrong reference agent", "hermes", "pi", "id", hermesSession},
		{"wrong kind", "hermes", "hermes", "path", hermesSession},
		{"invalid id", "hermes", "hermes", "id", "../unsafe' OR 1=1 --"},
		{"empty id", "hermes", "hermes", "id", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()

			db := hermesDB(t, filepath.Join(home, ".hermes"), "Should not read")
			if tc.id != hermesSession {
				if _, err := db.Exec(
					`INSERT INTO sessions (id,title) VALUES (?,?)`,
					tc.id,
					"Should not read",
				); err != nil {
					t.Fatal(err)
				}
			}

			pane := hermesPane()
			pane.Agent = tc.agent
			pane.AgentSession = &herdr.AgentSessionInfo{
				Agent: tc.refAgent,
				Kind:  tc.kind,
				Value: tc.id,
			}
			readOne(reads.Options{Home: home, ReadSessions: true}, pane)

			if pane.AgentTopic != "" {
				t.Fatalf("topic = %q, want empty for invalid reference", pane.AgentTopic)
			}
		})
	}
}

func TestHermesMissingDataFallsBack(t *testing.T) {
	t.Parallel()

	for _, tc := range []string{"missing database", "corrupt database", "missing session", "null title", "empty title", "disabled", "missing reference"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := filepath.Join(home, ".hermes")
			pane := hermesPane()
			opts := reads.Options{Home: home, ReadSessions: true}

			switch tc {
			case "missing database":
			case "corrupt database":
				if err := os.MkdirAll(root, 0o750); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(
					filepath.Join(root, "state.db"),
					[]byte("not SQLite"),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			default:
				db := hermesDB(t, root, "A title")

				switch tc {
				case "missing session":
					pane.AgentSession.Value = "20261021_133850_123abc"
				case "null title":
					if _, err := db.Exec(`UPDATE sessions SET title = NULL`); err != nil {
						t.Fatal(err)
					}
				case "empty title":
					if _, err := db.Exec(`UPDATE sessions SET title = ''`); err != nil {
						t.Fatal(err)
					}
				case "disabled":
					opts.ReadSessions = false
				case "missing reference":
					pane.AgentSession = nil
				}
			}

			pane.TerminalTitle = "Terminal fallback"
			readOne(opts, pane)

			if pane.AgentTopic != "" {
				t.Fatalf("topic = %q, want no session title", pane.AgentTopic)
			}

			parts, ok := resolver.NewTerminalTitle().Resolve(pane)
			if !ok || parts.Activity != "Terminal fallback" {
				t.Fatalf("fallback = %+v, %v", parts, ok)
			}

			if tc == "missing database" {
				if _, err := os.Stat(filepath.Join(root, "state.db")); !os.IsNotExist(err) {
					t.Fatalf("missing database was created: %v", err)
				}
			}
		})
	}
}
