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

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

// Agent is the label Herdr gives Grok Build.
const Agent = "grok"

// summaryName is the file Grok rewrites with the session's current title.
const summaryName = "summary.json"

// Reader reads session summaries, remembering where each one was found.
type Reader struct {
	mu       sync.Mutex
	roots    []string
	sessions map[string]*summary
	now      func() time.Time
}

// summary is one session's file and the title last read out of it.
type summary struct {
	search transcript.Search
	mod    time.Time
	title  string
}

// NewReader builds a reader over home, the Grok Build home. An empty home
// answers nothing.
func NewReader(home string) *Reader {
	var roots []string
	if home != "" {
		roots = []string{home}
	}

	return &Reader{
		roots:    roots,
		sessions: make(map[string]*summary),
		now:      time.Now,
	}
}

// Title reports the name Grok generated for the session, or the one the user
// set with /rename. dir is where the summary is looked for first; the id
// becomes part of a path, so it must be one herdr.AgentSessionInfo.ID accepted.
func (r *Reader) Title(sessionID, dir string) string {
	if len(r.roots) == 0 {
		return ""
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	held, known := r.sessions[sessionID]
	if !known {
		held = &summary{}
		r.sessions[sessionID] = held
	}

	if !layout.Find(&held.search, r.roots, sessionID, dir, r.now()) {
		return held.title
	}

	held.read()

	return held.title
}

// Retain forgets every session but these.
func (r *Reader) Retain(sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := make(map[string]*summary, len(sessionIDs))
	for _, id := range sessionIDs {
		if held, known := r.sessions[id]; known {
			kept[id] = held
		}
	}

	r.sessions = kept
}

// layout is where Grok files a session: under the directory it was started in,
// every separator percent-encoded, else under whichever project it moved to.
var layout = transcript.Layout{
	Filed: func(root, dir, sessionID string) string {
		return filepath.Join(root, "sessions", slugOf(dir), sessionID, summaryName)
	},
	Anywhere: func(root, sessionID string) string {
		return filepath.Join(root, "sessions", "*", sessionID, summaryName)
	},
}

// slugOf is how Grok names a project directory: "/" and "\" become %2F.
func slugOf(dir string) string {
	return strings.NewReplacer("/", "%2F", "\\", "%2F").Replace(filepath.Clean(dir))
}

// read re-reads a summary whose mtime moved. Grok rewrites the file whole, so
// it is read whole; one that is gone sends the session back to the search.
func (s *summary) read() {
	info, err := os.Stat(s.search.Path)
	if err != nil {
		s.search.Path, s.mod = "", time.Time{}

		return
	}

	if !info.ModTime().After(s.mod) && s.title != "" {
		return
	}

	// A summary carries the model, the cwd and the message counts too, and
	// none of that names the session.
	var file struct {
		Title string `json:"generated_title"`
	}

	content, err := os.ReadFile(s.search.Path)
	if err != nil {
		return
	}

	if err := json.Unmarshal(content, &file); err != nil {
		return
	}

	s.mod = info.ModTime()
	s.title = strings.TrimSpace(file.Title)
}
