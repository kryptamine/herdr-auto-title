package codex

import (
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

func TestATitleShowsTheSessionItNames(t *testing.T) {
	t.Parallel()

	named := transcript.Topic{Name: "Rework the poll loop so a pane that moved is read again"}

	// Codex shows the first 45 characters of a name past 48, then "...".
	for _, title := range []string{
		"Rework the poll loop so a pane that moved is read again | dashboard",
		"Rework the poll loop so a pane that moved is ... | dashboard",
		"[ ! ] Action Required | Rework the poll loop so a pane that moved is ... | dashboard",
		"",
	} {
		if !shows(title, named) {
			t.Errorf("title %q does not show %q", title, named.Name)
		}
	}

	// A session Codex has not named yet titles its terminal with the project.
	if !shows("dashboard", transcript.Topic{Opening: "rework the poll loop"}) {
		t.Error("an unnamed title does not show an unnamed session")
	}
}

func TestATitleThatMovedOnShowsAnotherSession(t *testing.T) {
	t.Parallel()

	// After /new or /resume Herdr still reports the session before, until the
	// next prompt, while the title already belongs to the next one.
	named := transcript.Topic{Name: "Reply with ok"}

	if shows("dashboard", named) {
		t.Error("a fresh session's title shows the named session before it")
	}

	if shows("Reply yes | dashboard", named) {
		t.Error("a resumed session's title shows the session before it")
	}

	// A name the title shows whole is matched whole: one session's name can
	// start another's, and a cut name alone is known only by its start.
	if shows("PR164 Alpha | dashboard", transcript.Topic{Name: "PR164 Alpha session"}) {
		t.Error("a whole shorter name shows the longer session it starts")
	}

	if shows("Rework the poll loop so a pane that moved is ... | dashboard", named) {
		t.Error("a cut name shows a session whose name starts differently")
	}

	if shows(
		"Reply with ok | dashboard",
		transcript.Topic{Opening: "reply with the single word ok"},
	) {
		t.Error("a named title shows an unnamed session")
	}
}

func TestAConfiguredTitleKeepsEverySessionRead(t *testing.T) {
	t.Parallel()

	// A title Codex was told to set otherwise carries no name to compare, and
	// reading it as stale would drop every session's name.
	h := newHome(t)
	h.rollout(meta(started), prompt("reply with ok"))
	h.index(named(session, "Reply with ok"))

	r := NewReader(h.dir)
	topic := r.Topic(session)

	if r.Shows("dashboard", topic) {
		t.Fatal("a default title of another session shows this one")
	}

	h.write(filepath.Join(h.dir, "config.toml"), "[tui]", `terminal_title = ["spinner", "project"]`)

	if !r.Shows("dashboard", topic) {
		t.Error("a configured title is read as another session's")
	}
}

func TestOnlyAnAssignedTerminalTitleCountsAsConfigured(t *testing.T) {
	t.Parallel()

	for config, want := range map[string]bool{
		"[tui]\nterminal_title = [\"project\"]": true,
		"tui.terminal_title = []":               true,
		"tui = { terminal_title = [] }":         true,
		"# terminal_title = [\"project\"]":      false,
		"[tui]\nnotifications = true":           false,
	} {
		if got := setsTerminalTitle(config); got != want {
			t.Errorf("setsTerminalTitle(%q) = %v, want %v", config, got, want)
		}
	}
}
