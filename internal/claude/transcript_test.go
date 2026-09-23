package claude

import (
	"os"
	"path/filepath"
	"strconv"
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

func newProject(t *testing.T) project {
	t.Helper()

	return project{t: t, root: t.TempDir(), dir: started}
}

// reader builds a reader over this project's home, and over extra homes when
// the test names them. The homes are passed rather than set in the
// environment: nothing in this package reads one.
func (p project) reader(extra ...string) *Reader {
	return NewReader(append([]string{p.root}, extra...)...)
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

	if _, err := file.WriteString(joined(lines)); err != nil {
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
	t.Parallel()

	p := newProject(t)
	p.write(human("fix the redirect"), aiTitle("OAuth redirect fix"))

	if got := p.reader().Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the generated title", got)
	}
}

func TestTheLastTitleWins(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(aiTitle("First guess"), aiTitle("What it turned into"))

	if got := p.reader().Topic(session, started); got.Text() != "What it turned into" {
		t.Errorf("topic = %+v, want the last title", got)
	}
}

func TestASlashCommandNamesASessionWithNoTitle(t *testing.T) {
	t.Parallel()

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

	got := p.reader().Topic(session, started)
	if got.Text() != "grill-me" {
		t.Errorf("topic = %+v, want the command it opened with", got)
	}
}

func TestASlashCommandKeepsWhatItWasCalledWith(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(
		human(
			`<command-name>/code-review</command-name>\n<command-message>code-review</command-message>\n<command-args>spec.md</command-args>`,
		),
	)

	if got := p.reader().Topic(session, started); got.Text() != "code-review spec.md" {
		t.Errorf("topic = %+v, want the command and its argument", got)
	}
}

func TestACommandCalledWithNothingStandsAlone(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(
		human(
			`<command-name>/grill-me</command-name>\n<command-message>grill-me</command-message>\n<command-args></command-args>`,
		),
	)

	if got := p.reader().Topic(session, started); got.Text() != "grill-me" {
		t.Errorf("topic = %+v, want the command alone", got)
	}
}

func TestATitleOutranksTheOpening(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(human("rework the poll loop"), aiTitle("Poll loop rework"))

	got := p.reader().Topic(session, started)
	switch {
	case got.Text() != "Poll loop rework":
		t.Errorf("text = %q, want the title", got.Text())
	case got.Opening != "rework the poll loop":
		t.Errorf("opening = %q, want the first prompt kept", got.Opening)
	}
}

func TestOnlyTheUsersOwnPromptOpensASession(t *testing.T) {
	t.Parallel()

	// A slash command expands into the conversation as another user message.
	// Only the one the user typed says what the session is about.
	p := newProject(t)
	p.write(expanded("Base directory for this skill: /skills/grilling"), toolResult())

	if got := p.reader().Topic(session, started); got.Text() != "" {
		t.Errorf("topic = %+v, want nothing from an expansion", got)
	}
}

func TestAResumedSessionsCaveatIsNotItsOpening(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(
		human(
			`<local-command-caveat>Caveat: The messages below were generated by a command</local-command-caveat>`,
		),
	)

	if got := p.reader().Topic(session, started); got.Text() != "" {
		t.Errorf("topic = %+v, want nothing from a caveat", got)
	}
}

func TestOnlyTheFirstLineOfAPromptOpensASession(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(human(`rework the poll loop\nand say why in the commit`))

	if got := p.reader().Topic(session, started); got.Text() != "rework the poll loop" {
		t.Errorf("topic = %+v, want the first line alone", got)
	}
}

func TestAppendedLinesAreReadOnTheNextPoll(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(human("fix the redirect"))

	reader := p.reader()

	if got := reader.Topic(session, started); got.Text() != "fix the redirect" {
		t.Fatalf("topic = %+v, want the opening prompt", got)
	}

	p.appendLines(aiTitle("OAuth redirect fix"))

	if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the title that arrived since", got)
	}
}

func TestAHalfWrittenLineIsReadWhenItIsWhole(t *testing.T) {
	t.Parallel()

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

	reader := p.reader()
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
	t.Parallel()

	p := newProject(t)
	p.write(human("fix the redirect"), aiTitle("OAuth redirect fix"))

	reader := p.reader()
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
	t.Parallel()

	// The pane has changed directory since the session started, so the slug
	// does not lead to it and only the scan does.
	p := newProject(t)
	p.write(aiTitle("OAuth redirect fix"))

	if got := p.reader().Topic(session, "/somewhere/else"); got.Text() != "OAuth redirect fix" {
		t.Errorf("topic = %+v, want the transcript found by scanning", got)
	}
}

func TestAnIdThatIsNotASessionIsRefused(t *testing.T) {
	t.Parallel()

	// The id arrives over the socket and becomes part of a path.
	p := newProject(t)

	reader := p.reader()

	for _, id := range []string{"", "../../../etc/passwd", "8852bfe0", strings.Repeat("a", 36)} {
		if got := reader.Topic(id, started); got.Text() != "" {
			t.Errorf("id %q resolved to %+v", id, got)
		}
	}
}

func TestASessionWithNoTranscriptSaysNothing(t *testing.T) {
	t.Parallel()

	p := newProject(t)

	if got := p.reader().Topic(session, started); got.Text() != "" {
		t.Errorf("topic = %+v, want nothing", got)
	}
}

func TestATranscriptThatWasNotThereYetIsLookedForAgain(t *testing.T) {
	t.Parallel()

	// Herdr can name a session before the agent has written a line of it, so a
	// search that found nothing has to be repeated — but not every poll: it
	// walks every project directory the user has.
	p := newProject(t)
	reader := p.reader()
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
	t.Parallel()

	p := newProject(t)
	p.write(aiTitle("OAuth redirect fix"))

	reader := p.reader()
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
	t.Parallel()

	if got := slugOf("/Users/dev/.claude/skills"); got != "-Users-dev--claude-skills" {
		t.Errorf("slug = %q", got)
	}
}

func TestATranscriptThatWentMissingIsLookedForAgain(t *testing.T) {
	t.Parallel()

	// The path was found once and then kept for the pane's whole life, so a
	// transcript that was rotated away froze the topic on whatever it last
	// said. locateRetry exists for exactly this and could never apply.
	p := newProject(t)
	reader := p.reader()
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

// The transcript lines that carry the directory the agent is working in.
func humanIn(text, dir string) string {
	return `{"type":"user","origin":{"kind":"human"},"cwd":` + strconv.Quote(dir) +
		`,"message":{"role":"user","content":"` + text + `"}}`
}

func agentIn(dir string) string {
	return `{"type":"assistant","cwd":` + strconv.Quote(dir) + `}`
}

func TestTheDirectoryTheAgentIsWorkingInIsRead(t *testing.T) {
	t.Parallel()

	worktree := t.TempDir()

	p := newProject(t)
	p.write(humanIn("fix the redirect", started), agentIn(worktree))

	if got := p.reader().Topic(session, started); got.Dir != worktree {
		t.Errorf("dir = %q, want %q", got.Dir, worktree)
	}
}

func TestALineCarryingNoDirectoryLeavesTheLastOneStanding(t *testing.T) {
	t.Parallel()

	// The lines that name a session — a title, a mode change — carry no
	// directory at all, so the last one that did is still where the agent is.
	worktree := t.TempDir()

	p := newProject(t)
	p.write(agentIn(worktree), aiTitle("OAuth redirect fix"))

	got := p.reader().Topic(session, started)
	switch {
	case got.Dir != worktree:
		t.Errorf("dir = %q, want %q", got.Dir, worktree)
	case got.Text() != "OAuth redirect fix":
		t.Errorf("text = %q, want the title", got.Text())
	}
}

func TestADirectoryOutlivesTheReadThatCarriedIt(t *testing.T) {
	t.Parallel()

	// A poll is handed only the bytes appended since the last one, and a third
	// of transcript lines carry no directory, so one slice can hold none.
	worktree := t.TempDir()

	p := newProject(t)
	p.write(agentIn(worktree))

	reader := p.reader()
	if got := reader.Topic(session, started); got.Dir != worktree {
		t.Fatalf("dir = %q, want %q", got.Dir, worktree)
	}

	p.appendLines(aiTitle("OAuth redirect fix"))

	if got := reader.Topic(session, started); got.Dir != worktree {
		t.Errorf("dir = %q, want the directory the earlier read carried", got.Dir)
	}
}

func TestALaterReadFollowsTheAgentToItsNextDirectory(t *testing.T) {
	t.Parallel()

	// The branch follows the agent as it moves, with nothing damping it, so a
	// directory appended after a read replaces the one held from before it.
	first, second := t.TempDir(), t.TempDir()

	p := newProject(t)
	p.write(agentIn(first))

	reader := p.reader()
	if got := reader.Topic(session, started); got.Dir != first {
		t.Fatalf("dir = %q, want %q", got.Dir, first)
	}

	p.appendLines(agentIn(second))

	if got := reader.Topic(session, started); got.Dir != second {
		t.Errorf("dir = %q, want the directory the agent moved to", got.Dir)
	}
}

func TestATranscriptThatShrankForgetsTheDirectoryToo(t *testing.T) {
	t.Parallel()

	// A literal directory rather than t.TempDir(): the replacement below only
	// counts as a shrink if it is shorter, and a temporary path is short
	// enough where TMPDIR is /tmp to make the first transcript the shorter one.
	p := newProject(t)
	p.write(human("fix the redirect"), agentIn(started+"/.claude/worktrees/oauth"))

	reader := p.reader()
	if got := reader.Topic(session, started); got.Dir == "" {
		t.Fatal("dir is empty before the transcript was replaced")
	}

	// Only a file that is no longer the one that was read can be shorter than
	// what has already been read out of it.
	p.write(aiTitle("Something else"))

	if got := reader.Topic(session, started); got.Dir != "" {
		t.Errorf("dir = %q, want the replaced transcript's directory let go", got.Dir)
	}
}

func TestATranscriptThatWentMissingKeepsItsDirectory(t *testing.T) {
	t.Parallel()

	worktree := t.TempDir()

	p := newProject(t)
	p.write(agentIn(worktree))

	reader := p.reader()
	if got := reader.Topic(session, started); got.Dir != worktree {
		t.Fatalf("dir = %q, want %q", got.Dir, worktree)
	}

	if err := os.Remove(p.path()); err != nil {
		t.Fatal(err)
	}

	if got := reader.Topic(session, started); got.Dir != worktree {
		t.Errorf("dir = %q, want the last known one kept", got.Dir)
	}
}

func TestADirectoryIsReportedAsTheTranscriptSpelledIt(t *testing.T) {
	t.Parallel()

	// Whether a path can be a checkout is settled where one is read, so the
	// reader repairs nothing and judges nothing.
	for _, dir := range []string{"work/dashboard", "..", "/work/dashboard/"} {
		p := newProject(t)
		p.write(agentIn(dir))

		if got := p.reader().Topic(session, started); got.Dir != dir {
			t.Errorf("dir %q reported as %q", dir, got.Dir)
		}
	}
}

func TestALineCarryingAnEmptyDirectoryLeavesTheLastOneStanding(t *testing.T) {
	t.Parallel()

	// An empty value is the one the reader still refuses: it would erase the
	// directory the session last named.
	p := newProject(t)
	p.write(agentIn("/work/dashboard"), agentIn(""))

	if got := p.reader().Topic(session, started); got.Dir != "/work/dashboard" {
		t.Errorf("dir = %q, want the last directory the transcript named", got.Dir)
	}
}

func TestATranscriptNamingNoDirectorySaysNothingAboutOne(t *testing.T) {
	t.Parallel()

	p := newProject(t)
	p.write(human("fix the redirect"), aiTitle("OAuth redirect fix"))

	got := p.reader().Topic(session, started)
	switch {
	case got.Dir != "":
		t.Errorf("dir = %q, want none", got.Dir)
	case got.Text() != "OAuth redirect fix":
		t.Errorf("text = %q, want the title", got.Text())
	}
}

// newProjectIn builds a state directory the reader was not given as its own
// configuration home, so a test can name it through EnvExtraRoots instead.
func newProjectIn(t *testing.T, root string) project {
	t.Helper()

	return project{t: t, root: root, dir: started}
}

func TestASessionInAnExtraConfigHomeIsFound(t *testing.T) {
	t.Parallel()

	// The home a shell picks per directory is not the one the server passes
	// the plugin, so a session written there is only found by searching both.
	worktree := t.TempDir()
	p := newProject(t)

	other := t.TempDir()
	newProjectIn(t, other).write(agentIn(worktree), aiTitle("OAuth redirect fix"))

	got := p.reader(other).Topic(session, started)
	switch {
	case got.Text() != "OAuth redirect fix":
		t.Errorf("text = %q, want the title from the extra home", got.Text())
	case got.Dir != worktree:
		t.Errorf("dir = %q, want %q", got.Dir, worktree)
	}
}

func TestTheFirstConfigHomeAnswersWhenBothHoldTheSession(t *testing.T) {
	t.Parallel()

	// The homes are searched in order and the search stops at the first hit,
	// which is what keeps a one-home machine paying what it paid before.
	p := newProject(t)
	p.write(aiTitle("The home the server named"))

	other := t.TempDir()
	newProjectIn(t, other).write(aiTitle("The extra home"))

	if got := p.reader(other).Topic(session, started); got.Text() != "The home the server named" {
		t.Errorf("text = %q, want the first home's transcript", got.Text())
	}
}

func TestAConfigHomeThatCannotBeUsedIsSkipped(t *testing.T) {
	t.Parallel()

	// A home that is not there, or is not a directory at all, costs itself and
	// not the homes named beside it. Nothing here refuses a home for how it is
	// spelled: the search simply misses, which is why configuration can warn
	// about a home and still hand it over.
	p := newProject(t)

	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	other := t.TempDir()
	newProjectIn(t, other).write(aiTitle("OAuth redirect fix"))

	// The usable home comes last, so reaching it means every other was skipped.
	homes := []string{filepath.Join(t.TempDir(), "never-created"), file, "relative/home", other}

	got := p.reader(homes...).Topic(session, started)
	if got.Text() != "OAuth redirect fix" {
		t.Errorf("text = %q, want the usable home still read", got.Text())
	}
}

func TestNoExtraConfigHomesLeavesTheSearchWhereItWas(t *testing.T) {
	t.Parallel()

	p := newProject(t)

	elsewhere := t.TempDir()
	newProjectIn(t, elsewhere).write(aiTitle("OAuth redirect fix"))

	if got := p.reader().Topic(session, started); got.Text() != "" {
		t.Errorf("text = %q, want a home nobody named left unread", got.Text())
	}
}

func TestATranscriptInAnExtraConfigHomeKeepsBeingRead(t *testing.T) {
	t.Parallel()

	p := newProject(t)

	other := t.TempDir()
	q := newProjectIn(t, other)
	q.write(human("fix the redirect"))

	reader := p.reader(other)
	if got := reader.Topic(session, started); got.Text() != "fix the redirect" {
		t.Fatalf("text = %q, want the opening prompt", got.Text())
	}

	q.appendLines(aiTitle("OAuth redirect fix"))

	if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Errorf("text = %q, want the title that arrived since", got.Text())
	}
}

func TestATranscriptThatMovedHomesIsFoundAgain(t *testing.T) {
	t.Parallel()

	// A path let go because the transcript went missing is searched for in
	// every home again, not only in the one that answered before.
	p := newProject(t)
	p.write(aiTitle("OAuth redirect fix"))

	other := t.TempDir()

	reader := p.reader(other)
	clock := time.Now()
	reader.now = func() time.Time { return clock }

	if got := reader.Topic(session, started); got.Text() != "OAuth redirect fix" {
		t.Fatalf("text = %q, want the first home's transcript", got.Text())
	}

	if err := os.Remove(p.path()); err != nil {
		t.Fatal(err)
	}

	newProjectIn(t, other).write(aiTitle("Rework the poll loop"))
	reader.Topic(session, started)

	clock = clock.Add(locateRetry)

	if got := reader.Topic(session, started); got.Text() != "Rework the poll loop" {
		t.Errorf("text = %q, want the transcript found in the other home", got.Text())
	}
}
