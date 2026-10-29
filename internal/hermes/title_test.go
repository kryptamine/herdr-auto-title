package hermes_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/hermes"
)

const sessionID = "20261021_133849_f560a2"

func database(t *testing.T, home, title string) {
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
		`CREATE TABLE sessions(id TEXT PRIMARY KEY,title TEXT); INSERT INTO sessions VALUES(?,?)`,
		sessionID,
		title,
	); err != nil {
		t.Fatal(err)
	}
}

func TestTitleReadIsBounded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	database(t, home, strings.Repeat("x", 10000))

	if got := hermes.Title(context.Background(), []string{home}, sessionID, ""); len(got) != 4096 {
		t.Fatalf("title length = %d, want 4096", len(got))
	}
}

func TestProfileSearchIsBounded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	database(t, home, "Must not choose from incomplete search")

	for i := range 65 {
		if err := os.MkdirAll(filepath.Join(home, "profiles", strconv.Itoa(i)), 0o750); err != nil {
			t.Fatal(err)
		}
	}

	if got := hermes.Title(context.Background(), []string{home}, sessionID, ""); got != "" {
		t.Fatalf("unbounded search returned %q", got)
	}
}

func TestUnreadableCandidateDoesNotSelectAnotherProfile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	database(t, filepath.Join(home, "profiles", "work"), "Cannot establish ownership")

	if err := os.WriteFile(
		filepath.Join(home, "state.db"),
		[]byte("corrupt database"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if got := hermes.Title(context.Background(), []string{home}, sessionID, ""); got != "" {
		t.Fatalf("selected title despite unreadable candidate: %q", got)
	}
}

func TestLockedDatabaseFallsBack(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	database(t, home, "Locked title")

	writer, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = writer.Close() }()

	if _, err := writer.Exec(`BEGIN EXCLUSIVE`); err != nil {
		t.Fatal(err)
	}

	defer func() { _, _ = writer.Exec(`ROLLBACK`) }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if got := hermes.Title(ctx, []string{home}, sessionID, ""); got != "" {
		t.Fatalf("locked database returned %q", got)
	}

	if ctx.Err() != nil {
		t.Fatal("lookup exhausted caller's longer timeout")
	}
}

func TestCanceledLookupReturnsNoTitle(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	database(t, home, "Do not read")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if got := hermes.Title(ctx, []string{home}, sessionID, home); got != "" {
		t.Fatalf("canceled lookup returned %q", got)
	}
}

func TestDatabasePathIsURIQuoted(t *testing.T) {
	t.Parallel()
	home := filepath.Join(t.TempDir(), "home #with spaces")
	database(t, home, "Quoted path title")

	if got := hermes.Title(context.Background(), nil, sessionID, home); got != "Quoted path title" {
		t.Fatalf("quoted path title = %q", got)
	}
}

func TestExplicitHomeNeverFallsThrough(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	database(t, home, "Wrong profile")

	for _, explicit := range []string{t.TempDir(), "relative-home"} {
		if got := hermes.Title(
			context.Background(),
			[]string{home},
			sessionID,
			explicit,
		); got != "" {
			t.Fatalf("explicit home %q fell through: %q", explicit, got)
		}
	}
}
