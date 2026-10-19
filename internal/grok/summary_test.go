package grok

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	session = "01a143aa-66ab-7082-a367-6c8c9566b580"
	started = "/work/dashboard"
)

func TestAGeneratedTitleNamesTheSession(t *testing.T) {
	t.Parallel()

	home := writeSummary(t, started, "Fix the login redirect")

	got := NewReader(home).Title(session, started)
	if got != "Fix the login redirect" {
		t.Errorf("title = %q", got)
	}
}

func TestThePaneDirectoryIsTriedBeforeAScan(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeSummaryIn(t, home, "/work/other", "The other one")
	writeSummaryIn(t, home, started, "This pane")

	got := NewReader(home).Title(session, started)
	if got != "This pane" {
		t.Errorf("title = %q, want the pane's own project", got)
	}
}

func TestAMovedSessionIsFoundByScanning(t *testing.T) {
	t.Parallel()

	home := writeSummary(t, "/work/elsewhere", "Still this session")

	got := NewReader(home).Title(session, started)
	if got != "Still this session" {
		t.Errorf("title = %q", got)
	}
}

func TestAMissingSummaryIsNotTheSession(t *testing.T) {
	t.Parallel()

	if got := NewReader(t.TempDir()).Title(session, started); got != "" {
		t.Errorf("title = %q, want nothing", got)
	}
}

func TestARewriteReplacesTheTitle(t *testing.T) {
	t.Parallel()

	home := writeSummary(t, started, "First cut")
	reader := NewReader(home)

	if got := reader.Title(session, started); got != "First cut" {
		t.Fatalf("title = %q", got)
	}

	// Grok rewrites the file. A clock that does not move would hide that.
	time.Sleep(10 * time.Millisecond)
	writeSummaryIn(t, home, started, "Renamed")

	if got := reader.Title(session, started); got != "Renamed" {
		t.Errorf("title = %q, want the rewrite", got)
	}
}

func TestAnUnchangedSummaryIsReadOnce(t *testing.T) {
	t.Parallel()

	home := writeSummary(t, started, "Fix the login redirect")
	path := summaryPath(home, started, session)
	reader := NewReader(home)

	reader.Title(session, started)

	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	if got := reader.Title(session, started); got != "Fix the login redirect" {
		t.Errorf("title = %q, want the title kept without re-reading", got)
	}
}

func TestNoHomeAnswersNothing(t *testing.T) {
	t.Parallel()

	if got := NewReader("").Title(session, started); got != "" {
		t.Errorf("title = %q, want nothing", got)
	}
}

func TestRetainForgetsAClosedSession(t *testing.T) {
	t.Parallel()

	home := writeSummary(t, started, "Fix the login redirect")
	reader := NewReader(home)
	reader.Title(session, started)

	reader.Retain(nil)

	if err := os.Remove(summaryPath(home, started, session)); err != nil {
		t.Fatal(err)
	}

	if got := reader.Title(session, started); got != "" {
		t.Errorf("title = %q, want the forgotten session re-found as missing", got)
	}
}

func writeSummary(t *testing.T, dir, title string) string {
	t.Helper()

	home := t.TempDir()
	writeSummaryIn(t, home, dir, title)

	return home
}

func writeSummaryIn(t *testing.T, home, dir, title string) {
	t.Helper()

	path := summaryPath(home, dir, session)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	body := `{"generated_title":"` + title + `","cwd":"` + dir + `"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func summaryPath(home, dir, sessionID string) string {
	return filepath.Join(home, "sessions", slugOf(dir), sessionID, summaryName)
}
