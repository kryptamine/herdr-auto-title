// Package pi reads what a Pi session is about from the session file the agent
// appends to. Herdr says which file a pane's Pi holds; the file is where that
// session says what it is doing and where it started.
package pi

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

// Agent is the label Herdr gives Pi, and the only agent whose session file this
// package knows how to read.
const Agent = "pi"

// maxOpening bounds what an opening prompt contributes. It is generous because
// the cut that matters is made later, against the width of the tab bar.
const maxOpening = 200

// Topic is what a session says it is about.
type Topic struct {
	// Name is what the user called the session with /name, and empty until
	// they have: Pi derives no name of its own.
	Name string
	// Opening is the first prompt the user typed, which is what Pi's own
	// session list shows for a session that has no name.
	Opening string
	// Dir is where the session was started, which Pi records once in the
	// file's header.
	Dir string
}

// Text is what the topic contributes to a title, the name first.
func (t Topic) Text() string {
	if t.Name != "" {
		return t.Name
	}

	return t.Opening
}

// Reader reads session files, remembering how far it has read each one. A
// session file only grows, so a poll reads the bytes appended since the last
// one rather than the file.
type Reader struct {
	mu       sync.Mutex
	sessions map[string]*session
}

// session is one file and what has been read out of it. Pi records where a
// session started only in the file's header, so a catch-up reads the head too.
type session struct {
	follower transcript.Follower
	topic    Topic
}

func NewReader() *Reader {
	return &Reader{sessions: make(map[string]*session)}
}

// Topic reports what the session in the file at path is about, and the zero
// topic when nothing can be read. Only a .jsonl file is opened, and the path
// must be one herdr.AgentSessionInfo.Path accepted.
func (r *Reader) Topic(path string) Topic {
	if filepath.Ext(path) != ".jsonl" {
		return Topic{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	held, known := r.sessions[path]
	if !known {
		held = &session{follower: transcript.Follower{Head: true}}
		r.sessions[path] = held
	}

	// A file that is gone keeps what it said while it was here.
	held.follower.Follow(path, func() { held.topic = Topic{} }, held.absorb)

	return held.topic
}

// Retain forgets every session but these, so a pane that closed takes its file
// with it.
func (r *Reader) Retain(paths []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := make(map[string]*session, len(paths))
	for _, path := range paths {
		if held, known := r.sessions[path]; known {
			kept[path] = held
		}
	}

	r.sessions = kept
}

// entry is one session line. The file carries far more per line — ids,
// timestamps, model names, the whole assistant turn — and none of it says what
// the session is about.
type entry struct {
	Type    string `json:"type"`
	CWD     string `json:"cwd"`
	Name    string `json:"name"`
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// absorb updates the topic from one session line. The last name wins and the
// first opening does: a session is renamed as it goes, but it opened only once.
func (s *session) absorb(line string) {
	var read entry
	if json.Unmarshal([]byte(line), &read) != nil {
		return
	}

	switch read.Type {
	case "session":
		// Kept as the file spelled it: whether a path can be a checkout is
		// settled where one is read.
		if read.CWD != "" {
			s.topic.Dir = read.CWD
		}
	case "session_info":
		// A name entry without a name is the user clearing it, and the
		// session then goes back to being known by its opening.
		s.topic.Name = strings.TrimSpace(read.Name)
	case "message":
		if s.topic.Opening == "" && read.Message.Role == "user" {
			s.topic.Opening = opening(read.Message.Content)
		}
	}
}

// opening reduces a user's first prompt to what can name a session: its first
// line, which is also what Pi's own session list shows.
func opening(content json.RawMessage) string {
	text, ok := transcript.TextOf(content)
	if !ok {
		return ""
	}

	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return transcript.Truncate(strings.TrimSpace(line), maxOpening)
}
