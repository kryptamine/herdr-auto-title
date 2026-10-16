// Package grok reads what a Grok Build session is about from the summary the
// agent writes beside its transcript. Herdr says which session a pane holds;
// the summary is where that session names itself.
package grok

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Agent is the label Herdr gives Grok Build.
const Agent = "grok"

// summaryName is the file Grok rewrites with the session's current title.
const summaryName = "summary.json"

// locateRetry is how long a session whose summary was not found is left alone.
// Herdr can name a session before Grok has written the file.
const locateRetry = 10 * time.Second

// Topic is what a session says it is about.
type Topic struct {
	// Title is the name Grok generated, or the one the user set with /rename.
	Title string
}

// Text is what the topic contributes to a title.
func (t Topic) Text() string {
	return t.Title
}

// Reader reads session summaries, remembering where each one was found.
type Reader struct {
	mu sync.Mutex
	// roots are the Grok homes a summary is looked for in, in order.
	roots    []string
	sessions map[string]*summary
	now      func() time.Time
}

// summary is one session's file and the title last read out of it. A path
// that is still empty is a session that has been looked for and not found.
type summary struct {
	path       string
	mod        time.Time
	topic      Topic
	searchedAt time.Time
}

// NewReader builds a reader over homes, searched in the order given. No home
// at all answers nothing.
func NewReader(homes ...string) *Reader {
	return &Reader{
		roots:    homes,
		sessions: make(map[string]*summary),
		now:      time.Now,
	}
}

// Topic reports what the session is about. dir is the pane's directory, where
// the summary is looked for first. See docs/architecture/title-resolution.md.
func (r *Reader) Topic(sessionID, dir string) Topic {
	if len(r.roots) == 0 || strings.TrimSpace(sessionID) == "" ||
		strings.Contains(sessionID, "..") {
		return Topic{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	session, known := r.sessions[sessionID]
	if !known {
		session = &summary{}
		r.sessions[sessionID] = session
	}

	if session.path == "" && !r.find(session, sessionID, dir) {
		return session.topic
	}

	r.readInto(session)

	return session.topic
}

// Retain forgets every session but these.
func (r *Reader) Retain(sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := make(map[string]*summary, len(sessionIDs))
	for _, id := range sessionIDs {
		if session, known := r.sessions[id]; known {
			kept[id] = session
		}
	}

	r.sessions = kept
}

func (r *Reader) find(session *summary, sessionID, dir string) bool {
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

func (r *Reader) locate(sessionID, dir string) (string, bool) {
	for _, root := range r.roots {
		if path, found := locateUnder(root, sessionID, dir); found {
			return path, true
		}
	}

	return "", false
}

// locateUnder finds the summary in one Grok home. Grok files a session under
// the directory it was started in, with every separator percent-encoded.
func locateUnder(root, sessionID, dir string) (string, bool) {
	sessions := filepath.Join(root, "sessions")

	if dir != "" {
		candidate := filepath.Join(sessions, slugOf(dir), sessionID, summaryName)
		if regular(candidate) {
			return candidate, true
		}
	}

	matches, err := filepath.Glob(filepath.Join(sessions, "*", sessionID, summaryName))
	if err != nil || len(matches) == 0 {
		return "", false
	}

	return matches[0], true
}

func regular(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.Mode().IsRegular()
}

// slugOf is how Grok names a project directory: "/" and "\" become %2F.
func slugOf(dir string) string {
	return strings.NewReplacer("/", "%2F", "\\", "%2F").Replace(filepath.Clean(dir))
}

// readInto re-reads a summary whose mtime moved. Grok rewrites the file, so a
// missing or shorter one is the same session under a new name.
func (r *Reader) readInto(session *summary) {
	info, err := os.Stat(session.path)
	if err != nil {
		session.path, session.mod = "", time.Time{}

		return
	}

	if !info.ModTime().After(session.mod) && session.topic.Title != "" {
		return
	}

	// A summary carries the model, the cwd and the message counts too, and
	// none of that names the session.
	var file struct {
		Title string `json:"generated_title"`
	}

	content, err := os.ReadFile(session.path)
	if err != nil {
		return
	}

	if err := json.Unmarshal(content, &file); err != nil {
		return
	}

	session.mod = info.ModTime()
	session.topic.Title = strings.TrimSpace(file.Title)
}
