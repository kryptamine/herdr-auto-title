package pi

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	sessionID = "8852bfe0-8b24-4a23-a35e-7521d04da061"
	// started is the directory the session was opened in, which is what
	// Pi files its session under.
	started = "/work/dashboard"
)

// stamp is the start time in a session file's name, in the spelling Pi
// gives it. Nothing in the reader reads it; the id is what a file is
// found by.
const stamp = "2026-10-16T02-00-00-000Z"

// project is a Pi agent home built out of files, so the tests describe
// the session format they parse rather than depend on the agent.
type project struct {
	t    *testing.T
	root string
	dir  string
}

func newProject(t *testing.T) project {
	t.Helper()

	return project{t: t, root: t.TempDir(), dir: started}
}

// reader builds a reader over this project's home. The home is passed
// rather than set in the environment: nothing in this package reads one.
func (p project) reader() *Reader {
	return NewReader(p.root)
}

func (p project) path() string {
	return filepath.Join(p.root, "sessions", sessionDirName(p.dir), stamp+"_"+sessionID+".jsonl")
}

// write lays down a session file, replacing whatever was there.
func (p project) write(lines ...string) {
	p.t.Helper()

	path := p.path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		p.t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(joined(lines)), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

// appendLines adds to a session file the way the agent does.
func (p project) appendLines(lines ...string) {
	p.t.Helper()

	p.appendRaw(joined(lines))
}

// appendRaw adds text that is not whole lines, the way a write caught
// halfway looks.
func (p project) appendRaw(text string) {
	p.t.Helper()

	file, err := os.OpenFile(p.path(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		p.t.Fatal(err)
	}

	if _, err := file.WriteString(text); err != nil {
		p.t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		p.t.Fatal(err)
	}
}

func joined(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n") + "\n"
}

// The session file lines the reader looks for, as Pi writes them.
func header(dir string) string {
	return `{"type":"session","version":3,"id":"` + sessionID +
		`","timestamp":"2026-10-16T02:00:00.000Z","cwd":` + strconv.Quote(dir) + `}`
}

func named(name string) string {
	return `{"type":"session_info","id":"a1b2c3d4","parentId":null,` +
		`"timestamp":"2026-10-16T02:01:00.000Z","name":` + strconv.Quote(name) + `}`
}

func cleared() string {
	return `{"type":"session_info","id":"b2c3d4e5","parentId":"a1b2c3d4",` +
		`"timestamp":"2026-10-16T02:02:00.000Z"}`
}

func human(text string) string {
	return `{"type":"message","id":"c3d4e5f6","parentId":null,` +
		`"timestamp":"2026-10-16T02:00:01.000Z","message":{"role":"user","content":` +
		strconv.Quote(text) + `,"timestamp":1792125601000}}`
}

func assistant(text string) string {
	return `{"type":"message","id":"d4e5f6g7","parentId":"c3d4e5f6",` +
		`"timestamp":"2026-10-16T02:00:02.000Z","message":{"role":"assistant","content":` +
		strconv.Quote(text) + `,"timestamp":1792125602000}}`
}

func TestANameNamesASession(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), human("fix the redirect"), named("OAuth redirect fix"))

	if got := p.reader().Topic(sessionID, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the name the user gave the session", got)
	}
}

func TestTheLastNameWins(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), named("First guess"), named("What it turned into"))

	if got := p.reader().Topic(sessionID, started); got.Text() != "What it turned into" {
		t.Errorf("topic = %+v, want the last name", got)
	}
}

func TestAnUnnamedSessionIsNamedByItsOpening(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), human("fix the redirect"), assistant("On it"))

	if got := p.reader().Topic(sessionID, started); got.Text() != "fix the redirect" {
		t.Errorf("topic = %+v, want the opening prompt", got)
	}
}

func TestAClearedNameHandsTheSessionBackToItsOpening(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), human("fix the redirect"), named("OAuth redirect fix"), cleared())

	if got := p.reader().Topic(sessionID, started); got.Text() != "fix the redirect" {
		t.Errorf("topic = %+v, want the opening once the name is cleared", got)
	}
}

func TestOnlyTheFirstPromptOpensASession(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), human("fix the redirect"), human("and the tests"))

	if got := p.reader().Topic(sessionID, started); got.Text() != "fix the redirect" {
		t.Errorf("topic = %+v, want the first prompt only", got)
	}
}

func TestAppendedLinesAreReadOnTheNextPoll(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), human("fix the redirect"))

	reader := p.reader()
	if got := reader.Topic(sessionID, started); got.Text() != "fix the redirect" {
		t.Fatalf("topic = %+v before the name, want the opening", got)
	}

	p.appendLines(named("OAuth redirect fix"))

	if got := reader.Topic(sessionID, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v after the name, want it read", got)
	}
}

func TestAHalfWrittenLineIsReadWhenItIsWhole(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), human("fix the redirect"))

	reader := p.reader()
	reader.Topic(sessionID, started)

	line := named("OAuth redirect fix")
	half := len(line) / 2
	p.appendRaw(line[:half])

	if got := reader.Topic(sessionID, started); got.Title != "" {
		t.Errorf("topic = %+v, want no name from half a line", got)
	}

	p.appendRaw(line[half:] + "\n")

	if got := reader.Topic(sessionID, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the name once its line is whole", got)
	}
}

func TestAFileThatShrankIsReadAgain(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), human("fix the redirect"), named("OAuth redirect fix"))

	reader := p.reader()
	if got := reader.Topic(sessionID, started); got.Text() != "OAuth redirect fix" {
		t.Fatalf("topic = %+v, want the name before the file shrinks", got)
	}

	p.write(header(started), human("start over"))

	if got := reader.Topic(sessionID, started); got.Text() != "start over" {
		t.Errorf("topic = %+v, want the shorter file read from its start", got)
	}
}

func TestASessionFiledUnderAnotherDirectoryIsStillFound(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.dir = "/work/other"
	p.write(header(p.dir), named("Elsewhere"))

	if got := p.reader().Topic(sessionID, started); got.Text() != "Elsewhere" {
		t.Errorf("topic = %+v, want the session found across directories", got)
	}
}

func TestASessionInAFlatDirectoryIsFound(t *testing.T) {
	t.Parallel()

	// A session directory of Pi's own naming holds its files flat, with
	// no directory per session to file them under.
	root := t.TempDir()

	path := filepath.Join(root, stamp+"_"+sessionID+".jsonl")
	if err := os.WriteFile(
		path,
		[]byte(joined([]string{header(started), named("Flat")})),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if got := NewReader(root).Topic(sessionID, started); got.Text() != "Flat" {
		t.Errorf("topic = %+v, want the session in the flat directory", got)
	}
}

func TestAnIdThatIsNotASessionIsRefused(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), named("OAuth redirect fix"))

	for _, id := range []string{"", "..", "../" + sessionID, "a/b", `a\b`, "a b"} {
		if got := p.reader().Topic(id, started); got != (Topic{}) {
			t.Errorf("topic for id %q = %+v, want nothing", id, got)
		}
	}
}

func TestASessionWithNoFileSaysNothing(t *testing.T) {
	t.Parallel()

	p := newProject(t)

	if got := p.reader().Topic(sessionID, started); got != (Topic{}) {
		t.Errorf("topic = %+v, want nothing for a session with no file", got)
	}
}

func TestAFileThatWasNotThereYetIsLookedForAgain(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	reader := p.reader()

	clock := time.Date(2026, 10, 16, 2, 0, 0, 0, time.UTC)
	reader.now = func() time.Time { return clock }

	if got := reader.Topic(sessionID, started); got != (Topic{}) {
		t.Fatalf("topic = %+v, want nothing before the file exists", got)
	}

	p.write(header(started), named("Written late"))

	// Inside the retry window the miss is still the answer.
	if got := reader.Topic(sessionID, started); got != (Topic{}) {
		t.Errorf("topic = %+v, want the miss kept inside the retry window", got)
	}

	clock = clock.Add(locateRetry + time.Second)

	if got := reader.Topic(sessionID, started); got.Text() != "Written late" {
		t.Errorf("topic = %+v, want the file looked for again", got)
	}
}

func TestRetainForgetsTheSessionsARunOutlived(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header(started), named("Kept in mind"))

	reader := p.reader()
	reader.Topic(sessionID, started)
	reader.Retain(nil)

	if len(reader.sessions) != 0 {
		t.Errorf("sessions = %d after retaining none, want 0", len(reader.sessions))
	}
}

func TestTheDirectoryIsReadFromTheHeader(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(header("/work/agent-is-here"), human("fix the redirect"))

	if got := p.reader().Topic(sessionID, started); got.Dir != "/work/agent-is-here" {
		t.Errorf("dir = %q, want the directory the header records", got.Dir)
	}
}

func TestSessionDirNameMatchesHowPiNamesADirectory(t *testing.T) {
	t.Parallel()

	for dir, want := range map[string]string{
		"/work/dashboard":      "--work-dashboard--",
		"/home/u/a-b":          "--home-u-a-b--",
		`C:\Users\u\dashboard`: "--C--Users-u-dashboard--",
	} {
		if got := sessionDirName(dir); got != want {
			t.Errorf("sessionDirName(%q) = %q, want %q", dir, got, want)
		}
	}
}
