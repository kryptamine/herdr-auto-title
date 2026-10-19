// Package claude reads what a Claude Code session is about from the transcript
// the agent appends to. Herdr says which session a pane holds; the transcript
// is where that session says what it is doing.
package claude

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

// Agent is the label Herdr gives Claude Code, and the only agent whose
// transcript this package knows how to read.
const Agent = "claude"

// maxOpening bounds what an opening prompt contributes. It is generous because
// the cut that matters is made later, against the width of the tab bar.
const maxOpening = 200

// Topic is what a session says it is about.
type Topic struct {
	// Custom is the name the session was given, at launch or with /rename. It
	// outranks Title, which Claude Code keeps writing beside it unchanged.
	Custom string
	// Title is what Claude Code generated from the first prompt, and leaves
	// empty until it has seen one. It does not change after that.
	Title string
	// Opening is what the session started with. It names a session that has
	// not earned a title yet: one opened with a slash command and answered by
	// the agent alone never gets one.
	Opening string
	// Dir is where the agent was last working, which is its own directory and
	// not the pane's: an agent sent into a worktree stays there.
	Dir string
}

// Text is what the topic contributes to a title: the name the session was
// given, then the generated one, then how it opened.
func (t Topic) Text() string {
	if t.Custom != "" {
		return t.Custom
	}

	if t.Title != "" {
		return t.Title
	}

	return t.Opening
}

// Reader reads session transcripts, remembering how far it has read each one.
// Transcripts are append-only, so a poll reads the bytes appended since the
// last one rather than the file.
type Reader struct {
	mu sync.Mutex
	// roots are the configuration directories a transcript is looked for in,
	// searched in order until one holds the session.
	roots    []string
	sessions map[string]*followed
	now      func() time.Time
}

// followed is one session's transcript and what has been read out of it.
type followed struct {
	search   transcript.Search
	follower transcript.Follower
	topic    Topic
}

// NewReader builds a reader over homes, the Claude Code configuration
// directories a transcript is looked for in, searched in the order given. No
// home at all is a reader that answers nothing, which is what a machine with
// no Claude Code on it should get.
func NewReader(homes ...string) *Reader {
	return &Reader{
		roots:    homes,
		sessions: make(map[string]*followed),
		now:      time.Now,
	}
}

// Topic reports what the session is about, or the zero topic when nothing can
// be read. dir is where the transcript is looked for first; the id becomes part
// of a path, so it must be one herdr.AgentSessionInfo.ID accepted.
func (r *Reader) Topic(sessionID, dir string) Topic {
	if len(r.roots) == 0 {
		return Topic{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	held, known := r.sessions[sessionID]
	if !known {
		held = &followed{}
		r.sessions[sessionID] = held
	}

	if !layout.Find(&held.search, r.roots, sessionID, dir, r.now()) {
		// What was read before a transcript went missing still names the
		// session better than nothing does.
		return held.topic
	}

	held.read()

	return held.topic
}

// Retain forgets every session but these, so a pane that closed takes its
// transcript with it.
func (r *Reader) Retain(sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := make(map[string]*followed, len(sessionIDs))
	for _, id := range sessionIDs {
		if held, known := r.sessions[id]; known {
			kept[id] = held
		}
	}

	r.sessions = kept
}

// layout is where Claude Code files a session: under the directory it started
// in, which is usually the pane's, else under whichever project it moved to.
var layout = transcript.Layout{
	Filed: func(root, dir, sessionID string) string {
		return filepath.Join(root, "projects", slugOf(dir), sessionID+".jsonl")
	},
	Anywhere: func(root, sessionID string) string {
		return filepath.Join(root, "projects", "*", sessionID+".jsonl")
	},
}

// slugOf is how Claude Code names a project directory: every character that is
// not a letter or a digit becomes a dash, the leading separator included.
func slugOf(dir string) string {
	var slug strings.Builder
	slug.Grow(len(dir))

	for _, r := range dir {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			slug.WriteRune(r)
		default:
			slug.WriteByte('-')
		}
	}

	return slug.String()
}

// read updates the topic from what the transcript has had appended since the
// last read. A transcript that is gone was rotated, deleted or refiled, and
// letting its path go puts the session back behind the search's retry.
func (s *followed) read() {
	reset := func() { s.topic = Topic{} }
	if !s.follower.Follow(s.search.Path, reset, s.absorb) {
		s.search.Path = ""
	}
}

// entry is one transcript line. A transcript carries far more per line — uuids,
// timestamps, token counts, the whole assistant turn — and none of it says what
// the session is about.
type entry struct {
	Type        string `json:"type"`
	CWD         string `json:"cwd"`
	AITitle     string `json:"aiTitle"`
	CustomTitle string `json:"customTitle"`
	Origin      *struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// humanOrigin marks a line the user typed, as against one a slash command
// expanded into the conversation or a tool answered with.
const humanOrigin = "human"

// absorb updates the topic from one transcript line. The last name of each
// kind wins and the first opening does: a session can be renamed, but it
// opened only once.
func (s *followed) absorb(line string) {
	var read entry
	if json.Unmarshal([]byte(line), &read) != nil {
		return
	}

	// Kept as the transcript spelled it: whether a path can be a checkout is
	// settled where one is read. Only an empty value is refused, which would
	// erase the directory the session last named.
	if read.CWD != "" {
		s.topic.Dir = read.CWD
	}

	switch {
	case read.Type == "custom-title" && read.CustomTitle != "":
		s.topic.Custom = read.CustomTitle
	case read.Type == "ai-title" && read.AITitle != "":
		s.topic.Title = read.AITitle
	case read.Type == "user" && s.topic.Opening == "" && read.Origin != nil && read.Origin.Kind == humanOrigin:
		s.topic.Opening = opening(read.Message.Content)
	}
}

// commandPattern matches the marker Claude Code wraps a slash command in.
var commandPattern = regexp.MustCompile(`<command-name>\s*/?([^<\s]+)\s*</command-name>`)

// argsPattern matches what a slash command was called with, which Claude Code
// records beside the command's name and leaves empty when there was nothing.
var argsPattern = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)

// markupPattern matches the blocks Claude Code wraps around text the user did
// not type: a command's expansion, the caveat on a resumed session.
var markupPattern = regexp.MustCompile(`(?s)<[a-z][a-z-]*>.*?</[a-z][a-z-]*>`)

// opening reduces a user's first message to what can name a session. Content is
// either the text itself or the blocks a message was assembled from.
func opening(content json.RawMessage) string {
	text, ok := transcript.TextOf(content)
	if !ok {
		return ""
	}

	// A session opened with a slash command is named after the command, which
	// is what Claude Code's own session list shows for it.
	if command := commandPattern.FindStringSubmatch(text); command != nil {
		return transcript.Truncate(withArgs(command[1], text), maxOpening)
	}

	text = strings.TrimSpace(markupPattern.ReplaceAllString(text, " "))
	if text == "" {
		return ""
	}

	line, _, _ := strings.Cut(text, "\n")

	return transcript.Truncate(strings.TrimSpace(line), maxOpening)
}

// withArgs appends what the command was called with: `/code-review spec.md`
// says which review is running, and the command alone does not.
func withArgs(command, text string) string {
	args := argsPattern.FindStringSubmatch(text)
	if args == nil {
		return command
	}

	line, _, _ := strings.Cut(strings.TrimSpace(args[1]), "\n")
	if line = strings.TrimSpace(line); line == "" {
		return command
	}

	return command + " " + line
}
