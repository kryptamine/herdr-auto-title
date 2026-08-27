package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	session = "8852bfe0-8b24-4a23-a35e-7521d04da061"
	// started is the directory the session was opened in, which is what Claude
	// Code files its transcript under.
	started = "/work/dashboard"
)

// project is a Claude Code state directory built out of files, so the tests
// describe the transcript format they parse rather than depend on the agent.
type project struct {
	t    *testing.T
	root string
	dir  string
}

// newProject points the reader at a temporary state directory.
func newProject(t *testing.T) project {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)

	return project{t: t, root: root, dir: started}
}

func (p project) path() string {
	return filepath.Join(p.root, "projects", slugOf(p.dir), session+".jsonl")
}

// write lays down a transcript, replacing whatever was there.
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

// appendLines adds to a transcript the way the agent does.
func (p project) appendLines(lines ...string) {
	p.t.Helper()

	file, err := os.OpenFile(p.path(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		p.t.Fatal(err)
	}
	defer file.Close() // the write is checked below

	if _, err := file.WriteString(joined(lines)); err != nil {
		p.t.Fatal(err)
	}
}

func joined(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n") + "\n"
}

// The transcript lines the reader looks for, as Claude Code writes them.
func aiTitle(title string) string {
	return `{"type":"ai-title","aiTitle":"` + title + `","sessionId":"` + session + `"}`
}

func human(text string) string {
	return `{"type":"user","origin":{"kind":"human"},"message":{"role":"user","content":"` + text + `"}}`
}

func expanded(text string) string {
	return `{"type":"user","origin":null,"message":{"role":"user","content":"` + text + `"}}`
}

func toolResult() string {
	return `{"type":"user","origin":null,"message":{"role":"user","content":[{"type":"tool_result"}]}}`
}

func TestTitleNamesASession(t *testing.T) {
	p := newProject(t)
	p.write(human("fix the redirect"), aiTitle("OAuth redirect fix"))

	if got := NewReader().Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the generated title", got)
	}
}

func TestTheLastTitleWins(t *testing.T) {
	p := newProject(t)
	p.write(aiTitle("First guess"), aiTitle("What it turned into"))

	if got := NewReader().Topic(session, started); got.Text() != "What it turned into" {
		t.Errorf("topic = %+v, want the last title", got)
	}
}

func TestASlashCommandNamesASessionWithNoTitle(t *testing.T) {
	// The session this whole source exists for: opened with a command and
	// answered by the agent alone, so Claude Code never titles it.
	p := newProject(t)
	p.write(
		human(
			`<command-message>grill-me</command-message>\n<command-name>/grill-me</command-name>`,
		),
		expanded("Run a `/grilling` session."),
		toolResult(),
	)

	got := NewReader().Topic(session, started)
	if got.Text() != "grill-me" {
		t.Errorf("topic = %+v, want the command it opened with", got)
	}
}

func TestASlashCommandKeepsWhatItWasCalledWith(t *testing.T) {
	p := newProject(t)
	p.write(
		human(
			`<command-name>/code-review</command-name>\n<command-message>code-review</command-message>\n<command-args>spec.md</command-args>`,
		),
	)

	if got := NewReader().Topic(session, started); got.Text() != "code-review spec.md" {
		t.Errorf("topic = %+v, want the command and its argument", got)
	}
}

func TestACommandCalledWithNothingStandsAlone(t *testing.T) {
	p := newProject(t)
	p.write(
		human(
			`<command-name>/grill-me</command-name>\n<command-message>grill-me</command-message>\n<command-args></command-args>`,
		),
	)

	if got := NewReader().Topic(session, started); got.Text() != "grill-me" {
		t.Errorf("topic = %+v, want the command alone", got)
	}
}

func TestATitleOutranksTheOpening(t *testing.T) {
	p := newProject(t)
	p.write(human("rework the poll loop"), aiTitle("Poll loop rework"))

	got := NewReader().Topic(session, started)
	switch {
	case got.Text() != "Poll loop rework":
		t.Errorf("text = %q, want the title", got.Text())
	case got.Opening != "rework the poll loop":
		t.Errorf("opening = %q, want the first prompt kept", got.Opening)
	}
}

func TestOnlyTheUsersOwnPromptOpensASession(t *testing.T) {
	// A slash command expands into the conversation as another user message.
	// Only the one the user typed says what the session is about.
	p := newProject(t)
	p.write(expanded("Base directory for this skill: /skills/grilling"), toolResult())

	if got := NewReader().Topic(session, started); got.Text() != "" {
		t.Errorf("topic = %+v, want nothing from an expansion", got)
	}
}

func TestAResumedSessionsCaveatIsNotItsOpening(t *testing.T) {
	p := newProject(t)
	p.write(
		human(
			`<local-command-caveat>Caveat: The messages below were generated by a command</local-command-caveat>`,
		),
	)

	if got := NewReader().Topic(session, started); got.Text() != "" {
		t.Errorf("topic = %+v, want nothing from a caveat", got)
	}
}

func TestOnlyTheFirstLineOfAPromptOpensASession(t *testing.T) {
	p := newProject(t)
	p.write(human(`rework the poll loop\nand say why in the commit`))

	if got := NewReader().Topic(session, started); got.Text() != "rework the poll loop" {
		t.Errorf("topic = %+v, want the first line alone", got)
	}
}

func TestAppendedLinesAreReadOnTheNextPoll(t *testing.T) {
	p := newProject(t)
	p.write(human("fix the redirect"))

	reader := NewReader()

	if got := reader.Topic(session, started); got.Text() != "fix the redirect" {
		t.Fatalf("topic = %+v, want the opening prompt", got)
	}

	p.appendLines(aiTitle("OAuth redirect fix"))

	if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the title that arrived since", got)
	}
}

func TestAHalfWrittenLineIsReadWhenItIsWhole(t *testing.T) {
	// The agent may be mid-write when a poll reads. The fragment must not be
	// parsed, and must not be skipped once the rest of it lands either.
	p := newProject(t)
	p.write(human("fix the redirect"))
	path := p.path()

	half := aiTitle("OAuth redirect fix")
	if err := os.WriteFile(
		path,
		[]byte(joined([]string{human("fix the redirect")})+half[:20]),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	reader := NewReader()
	if got := reader.Topic(session, started); got.Text() != "fix the redirect" {
		t.Fatalf("topic = %+v, want the fragment ignored", got)
	}

	if err := os.WriteFile(
		path,
		[]byte(joined([]string{human("fix the redirect"), half})),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the line read once it was whole", got)
	}
}

func TestATranscriptThatShrankIsReadAgain(t *testing.T) {
	p := newProject(t)
	p.write(human("fix the redirect"), aiTitle("OAuth redirect fix"))

	reader := NewReader()
	if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Fatalf("topic = %+v", got)
	}

	// Only a file that is no longer the one that was read can be shorter than
	// what has already been read out of it.
	p.write(human("something else"))

	if got := reader.Topic(session, started); got.Text() != "something else" {
		t.Errorf("topic = %+v, want the transcript read from the start again", got)
	}
}

func TestASessionFiledUnderAnotherDirectoryIsStillFound(t *testing.T) {
	// The pane has changed directory since the session started, so the slug
	// does not lead to it and only the scan does.
	p := newProject(t)
	p.write(aiTitle("OAuth redirect fix"))

	if got := NewReader().Topic(session, "/somewhere/else"); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the transcript found by scanning", got)
	}
}

func TestAnIdThatIsNotASessionIsRefused(t *testing.T) {
	// The id arrives over the socket and becomes part of a path.
	newProject(t)

	reader := NewReader()

	for _, id := range []string{"", "../../../etc/passwd", "8852bfe0", strings.Repeat("a", 36)} {
		if got := reader.Topic(id, started); got.Text() != "" {
			t.Errorf("id %q resolved to %+v", id, got)
		}
	}
}

func TestASessionWithNoTranscriptSaysNothing(t *testing.T) {
	newProject(t)

	if got := NewReader().Topic(session, started); got.Text() != "" {
		t.Errorf("topic = %+v, want nothing", got)
	}
}

func TestATranscriptThatWasNotThereYetIsLookedForAgain(t *testing.T) {
	// Herdr can name a session before the agent has written a line of it, so a
	// search that found nothing has to be repeated — but not every poll: it
	// walks every project directory the user has.
	p := newProject(t)
	reader := NewReader()
	clock := time.Now()
	reader.now = func() time.Time { return clock }

	if got := reader.Topic(session, started); got.Text() != "" {
		t.Fatalf("topic = %+v, want nothing before the transcript exists", got)
	}

	p.write(aiTitle("OAuth redirect fix"))

	if got := reader.Topic(session, started); got.Text() != "" {
		t.Errorf("topic = %+v, want the failed search left alone until it expires", got)
	}

	clock = clock.Add(locateRetry)

	if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the transcript found on the next search", got)
	}
}

func TestRetainForgetsTheSessionsARunOutlived(t *testing.T) {
	p := newProject(t)
	p.write(aiTitle("OAuth redirect fix"))

	reader := NewReader()
	reader.Topic(session, started)

	if len(reader.sessions) != 1 {
		t.Fatalf("sessions = %d, want the read one remembered", len(reader.sessions))
	}

	reader.Retain(nil)

	if len(reader.sessions) != 0 {
		t.Errorf("sessions = %d, want the session let go", len(reader.sessions))
	}
}

func TestSlugMatchesHowClaudeCodeNamesAProject(t *testing.T) {
	if got := slugOf("/Users/dev/.claude/skills"); got != "-Users-dev--claude-skills" {
		t.Errorf("slug = %q", got)
	}
}

func TestATranscriptThatWentMissingIsLookedForAgain(t *testing.T) {
	// The path was found once and then kept for the pane's whole life, so a
	// transcript that was rotated away froze the topic on whatever it last
	// said. locateRetry exists for exactly this and could never apply.
	p := newProject(t)
	reader := NewReader()
	clock := time.Now()
	reader.now = func() time.Time { return clock }

	p.write(aiTitle("OAuth redirect fix"))

	if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Fatalf("topic = %+v, want the transcript read", got)
	}

	if err := os.Remove(p.path()); err != nil {
		t.Fatal(err)
	}

	// The first read finds it gone and lets the path go; the second finds the
	// search still on cooldown. Both keep the name the session had.
	for i := range 2 {
		if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
			t.Errorf("read %d: topic = %+v, want the last known one kept", i, got)
		}
	}

	p.write(aiTitle("Rework the poll loop"))

	clock = clock.Add(locateRetry)

	if got := reader.Topic(session, started); got.Text() != "Rework the poll loop" {
		t.Errorf("topic = %+v, want the replacement transcript found and read", got)
	}
}
