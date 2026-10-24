package codex

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

const (
	session = "01a070db-95a3-70b3-bd9d-a9ee3186a65b"
	other   = "01a070db-93d3-7b51-830c-889411bdc194"
	started = "/work/dashboard"
)

// home is a Codex home built out of files, so the tests describe the format
// they parse rather than depend on the agent.
type home struct {
	t   *testing.T
	dir string
}

func newHome(t *testing.T) home {
	t.Helper()

	return home{t: t, dir: t.TempDir()}
}

// rolloutPath is where Codex files a session started on day, under name.
func (h home) rolloutPath(day, name string) string {
	return filepath.Join(h.dir, "sessions", "2026", "10", day, name)
}

func (h home) rollout(lines ...string) string {
	h.t.Helper()

	path := h.rolloutPath("19", "rollout-2026-10-19T09-17-14-"+session+".jsonl")
	h.write(path, lines...)

	return path
}

func (h home) index(lines ...string) {
	h.t.Helper()

	h.write(filepath.Join(h.dir, "session_index.jsonl"), lines...)
}

func (h home) write(path string, lines ...string) {
	h.t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		h.t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(joined(lines)), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h home) appendTo(path string, lines ...string) {
	h.t.Helper()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		h.t.Fatal(err)
	}

	if _, err := file.WriteString(joined(lines)); err != nil {
		h.t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		h.t.Fatal(err)
	}
}

func joined(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n") + "\n"
}

// The lines the reader looks for, as Codex writes them.
func meta(dir string) string {
	return `{"timestamp":"2026-10-19T09:17:14.053Z","type":"session_meta","payload":{"id":"` +
		session + `","cwd":` + strconv.Quote(dir) + `,"originator":"codex_cli_rs"}}`
}

func turn(dir string) string {
	return `{"timestamp":"2026-10-19T09:18:02.112Z","type":"turn_context","payload":{"cwd":` +
		strconv.Quote(dir) + `,"model":"gpt-6"}}`
}

func prompt(text string) string {
	return `{"timestamp":"2026-10-19T09:17:20.001Z","type":"event_msg","payload":{"type":"user_message","message":` +
		strconv.Quote(
			text,
		) + `}}`
}

func echoed(text string) string {
	return `{"timestamp":"2026-10-19T09:17:20.001Z","type":"response_item","payload":{"type":"message","role":"user",` +
		`"content":[{"type":"input_text","text":` + strconv.Quote(
		text,
	) + `}]}}`
}

func named(id, name string) string {
	return `{"id":"` + id + `","thread_name":` + strconv.Quote(name) +
		`,"updated_at":"2026-10-19T09:20:00Z"}`
}

func TestTheFirstPromptNamesAnUnnamedSession(t *testing.T) {
	t.Parallel()

	h := newHome(t)
	h.rollout(
		meta(started),
		echoed("<environment_context>"),
		prompt("rework the poll loop\nand the tests"),
		prompt("then the docs"),
	)

	got := NewReader(h.dir).Topic(session)

	if got.Text() != "rework the poll loop" {
		t.Errorf("text = %q, want the first line of the first prompt", got.Text())
	}

	if got.Dir != started {
		t.Errorf("dir = %q, want %q", got.Dir, started)
	}
}

func TestTheIndexNameOutranksTheOpening(t *testing.T) {
	t.Parallel()

	h := newHome(t)
	h.rollout(meta(started), prompt("rework the poll loop"))
	h.index(named(session, "Rework poll loop"), named(other, "Fix login form"))

	if got := NewReader(h.dir).Topic(session).Text(); got != "Rework poll loop" {
		t.Errorf("text = %q, want this session's name", got)
	}
}

func TestTheLastNameInTheIndexWins(t *testing.T) {
	t.Parallel()

	// The index is append-only: a /rename adds a line rather than editing one.
	h := newHome(t)
	h.rollout(meta(started), prompt("rework the poll loop"))
	h.index(named(session, "Rework poll loop"))

	r := NewReader(h.dir)
	r.Topic(session)

	h.appendTo(filepath.Join(h.dir, "session_index.jsonl"), named(session, "Poll loop rewrite"))

	if got := r.Topic(session).Text(); got != "Poll loop rewrite" {
		t.Errorf("text = %q, want the newer name", got)
	}
}

func TestANameRemovedFromTheIndexHandsBackTheOpening(t *testing.T) {
	t.Parallel()

	// Codex removes a session's names by rewriting the index without them.
	h := newHome(t)
	h.rollout(meta(started), prompt("rework the poll loop"))
	h.index(named(other, "Fix login form"), named(session, "Rework poll loop"))

	r := NewReader(h.dir)
	r.Topic(session)

	h.index(named(other, "Fix login form"))

	if got := r.Topic(session).Text(); got != "rework the poll loop" {
		t.Errorf("text = %q, want the opening back", got)
	}
}

func TestAPromptAppendedLaterOpensASessionThatHadNone(t *testing.T) {
	t.Parallel()

	h := newHome(t)
	path := h.rollout(meta(started))

	r := NewReader(h.dir)
	if got := r.Topic(session).Text(); got != "" {
		t.Fatalf("text = %q before any prompt", got)
	}

	h.appendTo(path, prompt("rework the poll loop"))

	if got := r.Topic(session).Text(); got != "rework the poll loop" {
		t.Errorf("text = %q, want the appended prompt", got)
	}
}

func TestATurnInAnotherDirectoryMovesTheSession(t *testing.T) {
	t.Parallel()

	h := newHome(t)
	h.rollout(meta(started), turn("/work/dashboard/.worktrees/oauth"))

	if got := NewReader(h.dir).Topic(session).Dir; got != "/work/dashboard/.worktrees/oauth" {
		t.Errorf("dir = %q, want the latest turn's", got)
	}
}

func TestARolloutWrittenAfterTheSessionWasNamedIsFound(t *testing.T) {
	t.Parallel()

	// Herdr can report a session before Codex has filed its rollout.
	h := newHome(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	r := NewReader(h.dir)
	r.now = func() time.Time { return now }

	if got := r.Topic(session).Text(); got != "" {
		t.Fatalf("text = %q with no rollout", got)
	}

	h.rollout(meta(started), prompt("rework the poll loop"))

	if got := r.Topic(session).Text(); got != "" {
		t.Errorf("text = %q, want the search held off until the retry", got)
	}

	now = now.Add(transcript.LocateRetry)

	if got := r.Topic(session).Text(); got != "rework the poll loop" {
		t.Errorf("text = %q, want the rollout found on retry", got)
	}
}

func TestARevertedSessionIsReadFromItsNewestRollout(t *testing.T) {
	t.Parallel()

	h := newHome(t)
	h.rollout(meta(started), prompt("rework the poll loop"))
	h.write(
		h.rolloutPath(
			"20",
			"rollout-2026-10-20T08-01-00-"+session+"_01a070dc-0000-7000-8000-000000000001.jsonl",
		),
		meta("/work/reverted"),
	)

	if got := NewReader(h.dir).Topic(session).Dir; got != "/work/reverted" {
		t.Errorf("dir = %q, want the reverted rollout's", got)
	}
}

func TestARolloutWrittenAfterTheFirstReadIsSwitchedTo(t *testing.T) {
	t.Parallel()

	// A revert during a session files a newer rollout, so a path found once
	// cannot be kept for the pane's life as Claude's is.
	h := newHome(t)
	now := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	r := NewReader(h.dir)
	r.now = func() time.Time { return now }

	h.rollout(meta(started), prompt("rework the poll loop"))

	if got := r.Topic(session).Dir; got != started {
		t.Fatalf("dir = %q, want the first rollout's", got)
	}

	h.write(
		h.rolloutPath(
			"20",
			"rollout-2026-10-20T08-01-00-"+session+"_01a070dc-0000-7000-8000-000000000001.jsonl",
		),
		meta("/work/reverted"),
	)

	if got := r.Topic(session).Dir; got != started {
		t.Errorf("dir = %q, want the search held off until the retry", got)
	}

	now = now.Add(transcript.LocateRetry)

	got := r.Topic(session)
	if got.Dir != "/work/reverted" {
		t.Errorf("dir = %q, want the newer rollout's", got.Dir)
	}

	// The newer rollout carries no prompt, and the opening is still the session's.
	if got.Text() != "rework the poll loop" {
		t.Errorf("text = %q, want the opening kept across the switch", got.Text())
	}
}

func TestAnIndexPastMaxScanKeepsEveryName(t *testing.T) {
	t.Parallel()

	// Read from its ends alone, as a rollout is, an index that outgrew two reads
	// would lose every session named between them.
	h := newHome(t)
	h.rollout(meta(started), prompt("rework the poll loop"))

	var filler []string
	for line := named(other, strings.Repeat("x", 1000)); len(filler)*len(line) <= transcript.MaxScan; {
		filler = append(filler, line)
	}

	lines := append(append(slices.Clone(filler), named(session, "Rework poll loop")), filler...)
	h.index(lines...)

	r := NewReader(h.dir)
	for range len(joined(lines))/transcript.MaxScan + 1 {
		r.Topic(session)
	}

	if got := r.Topic(session).Text(); got != "Rework poll loop" {
		t.Errorf("text = %q, want the name in the middle of the index", got)
	}
}

func TestAnotherSessionsRolloutIsNotRead(t *testing.T) {
	t.Parallel()

	h := newHome(t)
	h.write(
		h.rolloutPath("19", "rollout-2026-10-19T09-17-14-"+other+".jsonl"),
		meta(started), prompt("fix the login form"),
	)

	if got := NewReader(h.dir).Topic(session); got != (transcript.Topic{}) {
		t.Errorf("topic = %+v, want nothing", got)
	}
}

func TestNoHomeAnswersNothing(t *testing.T) {
	t.Parallel()

	if got := NewReader("").Topic(session); got != (transcript.Topic{}) {
		t.Errorf("topic = %+v, want nothing", got)
	}
}

func TestARetainedSessionKeepsWhatWasRead(t *testing.T) {
	t.Parallel()

	h := newHome(t)
	path := h.rollout(meta(started), prompt("rework the poll loop"))

	r := NewReader(h.dir)
	r.Topic(session)
	r.Retain([]string{session})

	// Gone from disk, the rollout is known only by what was read of it.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if got := r.Topic(session).Text(); got != "rework the poll loop" {
		t.Errorf("text = %q, want what was read before", got)
	}

	r.Retain(nil)

	if got := r.Topic(session).Text(); got != "" {
		t.Errorf("text = %q, want a forgotten session read afresh", got)
	}
}
