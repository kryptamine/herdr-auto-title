package pi

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

// started is the directory the session was opened in, which Pi records in the
// file's header.
const started = "/work/dashboard"

// sessionFile is a Pi session built out of files, so the tests describe the
// format they parse rather than depend on the agent.
type sessionFile struct {
	t    *testing.T
	path string
}

func newSessionFile(t *testing.T) sessionFile {
	t.Helper()

	return sessionFile{
		t:    t,
		path: filepath.Join(t.TempDir(), "2026-10-12T04-22-57-889Z_01a12e59.jsonl"),
	}
}

// write lays down a session, replacing whatever was there.
func (f sessionFile) write(lines ...string) {
	f.t.Helper()

	if err := os.WriteFile(f.path, []byte(joined(lines)), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// appendLines adds to a session the way the agent does.
func (f sessionFile) appendLines(lines ...string) {
	f.t.Helper()

	f.appendRaw(joined(lines))
}

func (f sessionFile) appendRaw(text string) {
	f.t.Helper()

	file, err := os.OpenFile(f.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		f.t.Fatal(err)
	}

	if _, err := file.WriteString(text); err != nil {
		f.t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		f.t.Fatal(err)
	}
}

func joined(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n") + "\n"
}

// The session lines the reader looks for, as Pi writes them.
func header() string {
	return `{"type":"session","version":3,"id":"01a12e59","timestamp":"2026-10-12T04:22:57.889Z","cwd":` +
		strconv.Quote(
			started,
		) + `}`
}

func named(name string) string {
	return `{"type":"session_info","id":"k1l2m3n4","parentId":"j0k1l2m3","name":` + strconv.Quote(
		name,
	) + `}`
}

func cleared() string {
	return `{"type":"session_info","id":"k1l2m3n4","parentId":"j0k1l2m3"}`
}

func prompt(text string) string {
	return `{"type":"message","id":"a1b2c3d4","parentId":null,"message":{"role":"user","content":` +
		strconv.Quote(text) + `,"timestamp":1791779582599}}`
}

func promptWithImage(text string) string {
	return `{"type":"message","id":"a1b2c3d4","parentId":null,"message":{"role":"user","content":[` +
		`{"type":"image","data":"...","mimeType":"image/png"},{"type":"text","text":` + strconv.Quote(
		text,
	) +
		`}],"timestamp":1791779582599}}`
}

func answer(text string) string {
	return `{"type":"message","id":"b2c3d4e5","parentId":"a1b2c3d4","message":{"role":"assistant","content":[` +
		`{"type":"text","text":` + strconv.Quote(
		text,
	) + `}],"provider":"openai","model":"gpt-6.1-sol"}}`
}

func TestTheFirstPromptNamesAnUnnamedSession(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(
		header(),
		prompt("rework the poll loop"),
		answer("On it."),
		prompt("and the tests"),
	)

	got := NewReader().Topic(f.path)
	if got.Text() != "rework the poll loop" {
		t.Errorf("topic = %+v, want the first prompt", got)
	}

	if got.Dir != started {
		t.Errorf("dir = %q, want the header's %q", got.Dir, started)
	}
}

func TestANameOutranksTheOpening(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), prompt("rework the poll loop"), named("Poll loop rework"))

	if got := NewReader().Topic(f.path); got.Text() != "Poll loop rework" {
		t.Errorf("topic = %+v, want the name", got)
	}
}

func TestTheLastNameWins(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), named("First guess"), named("What it turned into"))

	if got := NewReader().Topic(f.path); got.Text() != "What it turned into" {
		t.Errorf("topic = %+v, want the last name", got)
	}
}

func TestAClearedNameFallsBackToTheOpening(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), prompt("rework the poll loop"), named("Poll loop rework"), cleared())

	if got := NewReader().Topic(f.path); got.Text() != "rework the poll loop" {
		t.Errorf("topic = %+v, want the opening once the name is cleared", got)
	}
}

func TestAPromptWithAnImageIsReadByItsText(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), promptWithImage("what is wrong in this screenshot"))

	if got := NewReader().Topic(f.path); got.Text() != "what is wrong in this screenshot" {
		t.Errorf("topic = %+v, want the text block", got)
	}
}

func TestOnlyTheFirstLineOfAPromptNamesTheSession(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), prompt("  rework the poll loop\n\nit starves its own timer"))

	if got := NewReader().Topic(f.path); got.Text() != "rework the poll loop" {
		t.Errorf("topic = %+v, want the first line alone", got)
	}
}

func TestALongPromptIsCut(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), prompt(strings.Repeat("é", transcript.MaxOpening+50)))

	if got := NewReader().Topic(f.path); len([]rune(got.Text())) != transcript.MaxOpening {
		t.Errorf("opening is %d runes, want %d", len([]rune(got.Text())), transcript.MaxOpening)
	}
}

func TestANameAppendedLaterIsRead(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), prompt("rework the poll loop"))

	r := NewReader()
	if got := r.Topic(f.path); got.Text() != "rework the poll loop" {
		t.Fatalf("topic = %+v, want the opening first", got)
	}

	f.appendLines(answer("On it."), named("Poll loop rework"))

	if got := r.Topic(f.path); got.Text() != "Poll loop rework" {
		t.Errorf("topic = %+v, want the name appended since", got)
	}
}

func TestAPartialLastLineWaitsForItsRest(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header())
	// The agent is still writing the line.
	f.appendRaw(`{"type":"session_info","id":"k1l2m3n4","name":"Poll lo`)

	r := NewReader()
	if got := r.Topic(f.path); got.Text() != "" {
		t.Fatalf("topic = %+v, want nothing from a half-written line", got)
	}

	f.appendRaw("op rework\"}\n")

	if got := r.Topic(f.path); got.Text() != "Poll loop rework" {
		t.Errorf("topic = %+v, want the line once it is whole", got)
	}
}

func TestAFileShorterThanWhatWasReadIsReadAgain(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), prompt("rework the poll loop"), named("Poll loop rework"))

	r := NewReader()
	if got := r.Topic(f.path); got.Text() != "Poll loop rework" {
		t.Fatalf("topic = %+v, want the name", got)
	}

	// Not a session file Pi would write, but a file that shrank is no longer
	// the one that was read, whatever did it.
	f.write(header(), prompt("something else"))

	if got := r.Topic(f.path); got.Text() != "something else" {
		t.Errorf("topic = %+v, want the shorter file read from the start", got)
	}
}

func TestAFileThatIsGoneKeepsWhatItSaid(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), named("Poll loop rework"))

	r := NewReader()
	if got := r.Topic(f.path); got.Text() != "Poll loop rework" {
		t.Fatalf("topic = %+v, want the name", got)
	}

	if err := os.Remove(f.path); err != nil {
		t.Fatal(err)
	}

	if got := r.Topic(f.path); got.Text() != "Poll loop rework" {
		t.Errorf("topic = %+v, want what was read before the file went", got)
	}

	// The file is back, and it is read whole again rather than from an offset
	// that counted into the one that went.
	f.write(header(), named("Back again"))

	if got := r.Topic(f.path); got.Text() != "Back again" {
		t.Errorf("topic = %+v, want the file that came back", got)
	}
}

func TestASessionFirstSeenMidFlightIsReadAtBothEnds(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), prompt("rework the poll loop"))

	filler := answer(strings.Repeat("x", 4000))
	for written := 0; written < transcript.MaxScan+len(filler); written += len(filler) + 1 {
		f.appendLines(filler)
	}

	f.appendLines(named("Poll loop rework"))

	got := NewReader().Topic(f.path)
	if got.Name != "Poll loop rework" {
		t.Errorf("name = %q, want the one at the tail", got.Name)
	}

	if got.Opening != "rework the poll loop" {
		t.Errorf("opening = %q, want the one at the head", got.Opening)
	}

	if got.Dir != started {
		t.Errorf("dir = %q, want the header's", got.Dir)
	}
}

func TestAPathThatIsNotASessionFileIsRefused(t *testing.T) {
	t.Parallel()

	f := newSessionFile(t)
	f.write(header(), named("Poll loop rework"))

	// Only Pi's own format is parsed, so a file of any other kind is not opened.
	for name, path := range map[string]string{
		"not a session": strings.TrimSuffix(f.path, ".jsonl") + ".txt",
		"empty":         "",
	} {
		if got := NewReader().Topic(path); got != (transcript.Topic{}) {
			t.Errorf("%s path %q: topic = %+v, want nothing", name, path, got)
		}
	}
}

func TestRetainKeepsOnlyTheSessionsStillHeld(t *testing.T) {
	t.Parallel()

	kept, gone := newSessionFile(t), newSessionFile(t)
	kept.write(header(), named("Kept"))
	gone.write(header(), named("Gone"))

	r := NewReader()
	r.Topic(kept.path)
	r.Topic(gone.path)

	r.Retain([]string{kept.path, "/somewhere/else.jsonl"})

	if _, held := r.sessions[gone.path]; held {
		t.Error("a session no pane holds must be forgotten")
	}

	if _, held := r.sessions[kept.path]; !held {
		t.Error("a session a pane still holds must be kept")
	}
}
