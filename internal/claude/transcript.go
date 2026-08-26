// Package claude reads what a Claude Code session is about from the transcript
// the agent appends to. Herdr says which session a pane holds; the transcript
// is where that session says what it is doing.
package claude

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Agent is the label Herdr gives Claude Code, and the only agent whose
// transcript this package knows how to read.
const Agent = "claude"

// maxScan bounds one read. A transcript is append-only, so this bounds the
// catch-up on a session first seen mid-flight, not the steady state.
const maxScan = 2 << 20

// maxOpening bounds what an opening prompt contributes. A title is 50 columns;
// the rest is only carried so the truncation can happen against the tab bar.
const maxOpening = 200

// locateRetry is how long a session whose transcript was not found is left
// alone. Herdr can name a session before the agent has written its first line,
// so the search is repeated — just not twice a second for the pane's whole life.
const locateRetry = 10 * time.Second

// Topic is what a session says it is about.
type Topic struct {
	// Title is what Claude Code named the session, which it derives from the
	// user's own prompts and leaves empty until it has seen one.
	Title string
	// Opening is what the session started with. It names a session that has
	// not earned a title yet: one opened with a slash command and answered by
	// the agent alone never gets one.
	Opening string
}

// Text is what the topic contributes to a title, the generated name first.
func (t Topic) Text() string {
	if t.Title != "" {
		return t.Title
	}

	return t.Opening
}

// Reader reads session transcripts, remembering how far it has read each one.
// Transcripts are append-only, so a poll reads the bytes appended since the
// last one rather than the file.
type Reader struct {
	mu       sync.Mutex
	root     string
	sessions map[string]*transcript
	now      func() time.Time
}

// transcript is one session's file and what has been read out of it. A path
// that is still empty is a session whose transcript has been looked for and not
// found, and searchedAt is when that search last ran.
type transcript struct {
	path       string
	offset     int64
	topic      Topic
	searchedAt time.Time
}

// NewReader builds a reader over Claude Code's configuration directory.
func NewReader() *Reader {
	return &Reader{root: root(), sessions: make(map[string]*transcript), now: time.Now}
}

// root is where Claude Code keeps its state. The environment variable is what
// a user who moved it sets.
func root() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".claude")
}

// Topic reports what the session is about, and the zero topic when nothing can
// be read: no transcript, an unreadable one, or one that has said nothing yet.
// dir is the pane's directory, where the transcript is looked for first.
func (r *Reader) Topic(sessionID, dir string) Topic {
	if r.root == "" || !isSessionID(sessionID) {
		return Topic{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	session, known := r.sessions[sessionID]
	if !known {
		session = &transcript{}
		r.sessions[sessionID] = session
	}

	if session.path == "" && !r.find(session, sessionID, dir) {
		return Topic{}
	}

	r.readInto(session)

	return session.topic
}

// find fills in the session's transcript path, and reports whether there is one
// to read. A search that came up empty is not repeated until locateRetry has
// passed, because the scan behind it walks every project directory.
func (r *Reader) find(session *transcript, sessionID, dir string) bool {
	now := r.now()
	if !session.searchedAt.IsZero() && now.Sub(session.searchedAt) < locateRetry {
		return false
	}

	session.searchedAt = now

	path, found := r.locate(sessionID, dir)
	if !found {
		return false
	}

	session.path = path

	return true
}

// Retain forgets every session but these, which is how the sessions a run has
// outlived are let go.
func (r *Reader) Retain(sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := make(map[string]*transcript, len(sessionIDs))
	for _, id := range sessionIDs {
		if session, known := r.sessions[id]; known {
			kept[id] = session
		}
	}

	r.sessions = kept
}

// sessionIDPattern is the shape of a session id. The id arrives over the socket
// and becomes part of a path, so anything else is refused rather than cleaned.
var sessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

func isSessionID(value string) bool {
	return sessionIDPattern.MatchString(value)
}

// locate finds the transcript. Claude Code files a session under the directory
// it was started in, which is usually the pane's, so that is tried before the
// scan across every project.
func (r *Reader) locate(sessionID, dir string) (string, bool) {
	name := sessionID + ".jsonl"
	projects := filepath.Join(r.root, "projects")

	if dir != "" {
		candidate := filepath.Join(projects, slugOf(dir), name)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, true
		}
	}

	// A pane that has changed directory since the session started files it
	// elsewhere, and only the whole projects directory says where.
	matches, err := filepath.Glob(filepath.Join(projects, "*", name))
	if err != nil || len(matches) == 0 {
		return "", false
	}

	return matches[0], true
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

// readInto reads what has been appended since the last read and updates the
// topic from it. A transcript only grows, so one that is shorter than what has
// been read out of it is no longer the file that was read.
func (r *Reader) readInto(session *transcript) {
	info, err := os.Stat(session.path)
	if err != nil {
		return
	}

	size := info.Size()
	switch {
	case size == session.offset:
		return
	case size < session.offset:
		session.offset, session.topic = 0, Topic{}
	}

	file, err := os.Open(session.path)
	if err != nil {
		return
	}
	defer file.Close() //nolint:errcheck // a read-only file has nothing to report on close

	// A session first seen long after it started is caught up on from its tail,
	// which is where a title it has already been given is repeated.
	from, fromTail := session.offset, size-session.offset > maxScan
	if fromTail {
		from = size - maxScan
	}

	if _, err := file.Seek(from, io.SeekStart); err != nil {
		return
	}

	content, err := io.ReadAll(io.LimitReader(file, maxScan))
	if err != nil {
		return
	}

	// Only whole lines are read: the agent may have been writing one while
	// this ran, and the rest of it arrives next poll.
	end := bytes.LastIndexByte(content, '\n')
	if end < 0 {
		return
	}

	session.offset = from + int64(end) + 1

	complete := string(content[:end])
	if fromTail {
		// A tail read starts mid-line, and that fragment is not JSON.
		_, complete, _ = strings.Cut(complete, "\n")
	}

	session.absorb(complete)
}

// entry is one transcript line. A transcript carries far more per line — uuids,
// timestamps, token counts, the whole assistant turn — and none of it says what
// the session is about.
type entry struct {
	Type    string `json:"type"`
	AITitle string `json:"aiTitle"`
	Origin  *struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// humanOrigin marks a line the user typed, as against one a slash command
// expanded into the conversation or a tool answered with.
const humanOrigin = "human"

// absorb updates the topic from a run of transcript lines. The last title wins
// and the first opening does: a session is renamed as it goes, but it opened
// only once.
func (t *transcript) absorb(lines string) {
	for line := range strings.SplitSeq(lines, "\n") {
		if line == "" {
			continue
		}

		var read entry
		if json.Unmarshal([]byte(line), &read) != nil {
			continue
		}

		switch {
		case read.Type == "ai-title" && read.AITitle != "":
			t.topic.Title = read.AITitle
		case read.Type == "user" && t.topic.Opening == "" && read.Origin != nil && read.Origin.Kind == humanOrigin:
			t.topic.Opening = opening(read.Message.Content)
		}
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
	text, ok := textOf(content)
	if !ok {
		return ""
	}

	// A session opened with a slash command is named after the command, which
	// is what Claude Code's own session list shows for it.
	if command := commandPattern.FindStringSubmatch(text); command != nil {
		return truncate(withArgs(command[1], text), maxOpening)
	}

	text = strings.TrimSpace(markupPattern.ReplaceAllString(text, " "))
	if text == "" {
		return ""
	}

	line, _, _ := strings.Cut(text, "\n")

	return truncate(strings.TrimSpace(line), maxOpening)
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

// textOf reads a message's content, which is a string when the user typed one
// and a list of blocks when it was assembled from several.
func textOf(content json.RawMessage) (string, bool) {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text, true
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return "", false
	}

	var joined []string

	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			joined = append(joined, block.Text)
		}
	}

	if len(joined) == 0 {
		return "", false
	}

	return strings.Join(joined, " "), true
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	return string(runes[:limit])
}
