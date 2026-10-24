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
	topic    transcript.Topic
}

func NewReader() *Reader {
	return &Reader{sessions: make(map[string]*session)}
}

// Topic reports what the session in the file at path is about, and the zero
// topic when nothing can be read. Only a .jsonl file is opened, and the path
// must be one herdr.AgentSessionInfo.Path accepted.
func (r *Reader) Topic(path string) transcript.Topic {
	if filepath.Ext(path) != ".jsonl" {
		return transcript.Topic{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	held, known := r.sessions[path]
	if !known {
		held = &session{follower: transcript.Follower{CatchUp: transcript.HeadAndTail}}
		r.sessions[path] = held
	}

	// A file that is gone keeps what it said while it was here.
	held.follower.Follow(path, func() { held.topic = transcript.Topic{} }, held.absorb)

	return held.topic
}

// Retain forgets every session but these, so a pane that closed takes its file
// with it.
func (r *Reader) Retain(paths []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions = transcript.Retain(r.sessions, paths)
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
		// Only the user names a session, with /name; an entry without a name
		// clears it, and the session goes back to being known by its opening.
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

	return transcript.FirstLine(text)
}
