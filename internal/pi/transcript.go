// Package pi reads what a Pi session is about from the session file the
// agent appends to. Herdr says which session a pane holds; the session
// file is where that session says what it is called.
package pi

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

// Agent is the label Herdr gives Pi, and the only agent whose session
// files this package knows how to read.
const Agent = "pi"

// maxScan bounds one read. A session file is append-only, so this bounds
// the catch-up on a session first seen mid-flight, not the steady state.
const maxScan = 2 << 20

// maxOpening bounds what an opening prompt contributes. It is generous
// because the cut that matters is made later, against the width of the
// tab bar.
const maxOpening = 200

// locateRetry is how long a session whose file was not found is left
// alone. Herdr can name a session before Pi has written its first line,
// so the search is repeated — just not twice a second for the pane's
// whole life.
const locateRetry = 10 * time.Second

// Topic is what a session says it is about.
type Topic struct {
	// Title is the name the user gave the session, with /name or --name.
	// It is empty until they do, and a later entry can clear it again.
	Title string
	// Opening is the first prompt the user typed. It names a session that
	// has not been given a name, which is what Pi's own session list
	// shows for one.
	Opening string
	// Dir is where the session was opened, which its header records. It
	// is the agent's own directory, not the pane's.
	Dir string
}

// Text is what the topic contributes to a title, the given name first.
func (t Topic) Text() string {
	if t.Title != "" {
		return t.Title
	}

	return t.Opening
}

// Reader reads session files, remembering how far it has read each one.
// Session files are append-only, so a poll reads the bytes appended since
// the last one rather than the file.
type Reader struct {
	mu sync.Mutex
	// roots are the agent homes a session file is looked for in, searched
	// in order until one holds the session. A root Pi was pointed at with
	// a session directory of its own holds its files flat instead.
	roots    []string
	sessions map[string]*session
	now      func() time.Time
}

// session is one session's file and what has been read out of it. A path
// that is still empty is a session whose file has been looked for and not
// found, and searchedAt is when that search last ran.
type session struct {
	path       string
	offset     int64
	topic      Topic
	searchedAt time.Time
}

// NewReader builds a reader over homes, the Pi agent homes session files
// are looked for in, searched in the order given. No home at all is a
// reader that answers nothing, which is what a machine with no Pi on it
// should get.
func NewReader(homes ...string) *Reader {
	return &Reader{
		roots:    homes,
		sessions: make(map[string]*session),
		now:      time.Now,
	}
}

// Topic reports what the session is about, and the zero topic when
// nothing can be read: no session file, an unreadable one, or one that
// has said nothing yet. dir is the pane's directory, where the session
// is looked for first.
func (r *Reader) Topic(sessionID, dir string) Topic {
	if len(r.roots) == 0 || !isSessionID(sessionID) {
		return Topic{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	held, known := r.sessions[sessionID]
	if !known {
		held = &session{}
		r.sessions[sessionID] = held
	}

	if held.path == "" && !r.find(held, sessionID, dir) {
		// What was read before a session file went missing still names
		// the session better than nothing does.
		return held.topic
	}

	r.readInto(held)

	return held.topic
}

// Retain forgets every session but these, so a pane that closed takes
// its session file with it.
func (r *Reader) Retain(sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := make(map[string]*session, len(sessionIDs))
	for _, id := range sessionIDs {
		if held, known := r.sessions[id]; known {
			kept[id] = held
		}
	}

	r.sessions = kept
}

// find fills in the session's file path, and reports whether there is
// one to read. A search that came up empty is not repeated until
// locateRetry has passed, because the scan behind it walks every
// session directory.
func (r *Reader) find(held *session, sessionID, dir string) bool {
	now := r.now()
	if !held.searchedAt.IsZero() && now.Sub(held.searchedAt) < locateRetry {
		return false
	}

	held.searchedAt = now

	path, found := r.locate(sessionID, dir)
	if !found {
		return false
	}

	held.path = path

	return true
}

// sessionIDPattern is the shape of a session id. The id arrives over the
// socket and becomes part of a path, so anything that could not sit in
// a file name is refused rather than cleaned. Pi writes a UUID unless a
// caller chose the id, and a chosen one is held to the same characters.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func isSessionID(value string) bool {
	return len(value) <= 128 && !strings.Contains(value, "..") &&
		sessionIDPattern.MatchString(value)
}

// locate finds the session file in the first home that holds it, so a
// session in the first costs no search of the ones after it. A file is
// named for when the session started and the id it was given, so the id
// is the part of the name to search by.
func (r *Reader) locate(sessionID, dir string) (string, bool) {
	suffix := "_" + sessionID + ".jsonl"

	for _, root := range r.roots {
		if path, found := locateUnder(root, suffix, dir); found {
			return path, true
		}
	}

	return "", false
}

// locateUnder finds the session file in one home. Pi files a session
// under the directory it was opened in, which is usually the pane's, so
// that is tried before the scan across every session directory.
func locateUnder(root, suffix, dir string) (string, bool) {
	sessions := filepath.Join(root, "sessions")

	if dir != "" {
		if path, found := firstMatch(
			filepath.Join(sessions, sessionDirName(dir), "*"+suffix),
		); found {
			return path, true
		}
	}

	if path, found := firstMatch(filepath.Join(sessions, "*", "*"+suffix)); found {
		return path, true
	}

	// A home that is a session directory itself — the one
	// PI_CODING_AGENT_SESSION_DIR names — holds its files flat.
	return firstMatch(filepath.Join(root, "*"+suffix))
}

func firstMatch(pattern string) (string, bool) {
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", false
	}

	return matches[0], true
}

// sessionDirName is how Pi names the directory a session is filed
// under: the leading separator dropped, and every separator and drive
// colon replaced by a dash, the whole wrapped in dashes.
func sessionDirName(dir string) string {
	trimmed := strings.TrimPrefix(dir, "/")
	trimmed = strings.TrimPrefix(trimmed, `\`)

	replaced := strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(trimmed)

	return "--" + replaced + "--"
}

func (r *Reader) readInto(held *session) {
	info, err := os.Stat(held.path)
	if err != nil {
		// Deleted, or refiled under another home. Letting the path go
		// puts the session back behind locateRetry, and the offset goes
		// with the file it counted into.
		held.path, held.offset = "", 0

		return
	}

	size := info.Size()
	switch {
	case size == held.offset:
		return
	case size < held.offset:
		held.offset, held.topic = 0, Topic{}
	}

	file, err := os.Open(held.path)
	if err != nil {
		return
	}
	// A read-only file has nothing to report on close.
	defer func() { _ = file.Close() }()

	// A session first seen long after it started is caught up on from
	// its tail, which is where a name it has already been given is
	// repeated.
	from, fromTail := held.offset, size-held.offset > maxScan
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

	// Only whole lines are read: the agent may have been writing one
	// while this ran, and the rest of it arrives next poll.
	end := bytes.LastIndexByte(content, '\n')
	if end < 0 {
		return
	}

	held.offset = from + int64(end) + 1

	complete := string(content[:end])
	if fromTail {
		// A tail read starts mid-line, and that fragment is not JSON.
		_, complete, _ = strings.Cut(complete, "\n")
	}

	held.absorb(complete)
}

// entry is one session file line. A file carries far more per line — the
// whole conversation, its tree links, its token counts — and none of it
// says what the session is called.
type entry struct {
	Type string `json:"type"`
	Name string `json:"name"`
	CWD  string `json:"cwd"`

	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// absorb updates the topic from a run of session file lines. The last
// name wins and the first prompt does: a session is renamed as it goes,
// but it opened only once.
func (s *session) absorb(lines string) {
	for line := range strings.SplitSeq(lines, "\n") {
		if line == "" {
			continue
		}

		var read entry
		if json.Unmarshal([]byte(line), &read) != nil {
			continue
		}

		switch {
		case read.Type == "session" && read.CWD != "":
			s.topic.Dir = read.CWD
		case read.Type == "session_info":
			// An entry carrying no name is a name cleared, and the
			// session is back to being named by how it opened.
			s.topic.Title = read.Name
		case read.Type == "message" && read.Message.Role == "user" && s.topic.Opening == "":
			s.topic.Opening = opening(read.Message.Content)
		}
	}
}

// opening reduces the user's first prompt to what can name a session.
// Content is either the text itself or the blocks a message was
// assembled from, and a prompt is named by its first line.
func opening(content json.RawMessage) string {
	text, ok := textOf(content)
	if !ok {
		return ""
	}

	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return truncate(strings.TrimSpace(line), maxOpening)
}

// textOf reads a message's content, which is a string when the user
// typed one and a list of blocks when it was assembled from several.
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
