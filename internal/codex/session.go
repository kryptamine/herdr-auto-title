// Package codex reads what a Codex session is about from the files the agent
// keeps under its home. Herdr says which session a pane holds; the session's
// rollout says how it opened and where, and the session index what it is called.
package codex

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

// Agent is the label Herdr gives Codex, and the only agent whose sessions this
// package knows how to read.
const Agent = "codex"

// Reader reads Codex's session files, remembering how far it has read each
// one. Both kinds only grow, so a poll reads the bytes appended since the last.
type Reader struct {
	mu       sync.Mutex
	roots    []string
	names    names
	title    titleConfig
	sessions map[string]*rollout
	now      func() time.Time
}

// rollout is one session's rollout file and what has been read out of it.
type rollout struct {
	search   transcript.Search
	follower transcript.Follower
	topic    transcript.Topic
}

// NewReader builds a reader over home, the directory Codex keeps its sessions
// in. An empty home is a reader that answers nothing.
func NewReader(home string) *Reader {
	var roots []string
	if home != "" {
		roots = []string{home}
	}

	return &Reader{
		roots: roots,
		names: names{
			path:     filepath.Join(home, "session_index.jsonl"),
			follower: transcript.Follower{CatchUp: transcript.Whole},
		},
		title:    titleConfig{path: filepath.Join(home, "config.toml")},
		sessions: make(map[string]*rollout),
		now:      time.Now,
	}
}

// Topic reports what the session is about, or the zero topic when nothing can
// be read. The id becomes part of a path, so it must be one
// herdr.AgentSessionInfo.ID accepted.
func (r *Reader) Topic(sessionID string) transcript.Topic {
	if len(r.roots) == 0 {
		return transcript.Topic{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.names.read()

	held, known := r.sessions[sessionID]
	if !known {
		// Codex records where a session started and how it opened up front.
		held = &rollout{follower: transcript.Follower{CatchUp: transcript.HeadAndTail}}
		r.sessions[sessionID] = held
	}

	// A rollout that went missing is looked for again, and what was read of
	// it still names the session better than nothing does until it is found.
	// A newer rollout the search turns up is read from its start.
	if layout.Find(&held.search, r.roots, sessionID, "", r.now()) &&
		!held.follower.Follow(
			held.search.Path,
			func() { held.topic = transcript.Topic{} },
			held.absorb,
		) {
		held.search.Path = ""
	}

	topic := held.topic
	topic.Name = r.names.of[sessionID]

	return topic
}

// Shows reports whether a pane titled title can still hold the session topic
// was read from; a title Codex was configured to set otherwise always can.
func (r *Reader) Shows(title string, topic transcript.Topic) bool {
	if len(r.roots) == 0 {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.title.customized() || shows(title, topic)
}

// Retain forgets every session but these, so a pane that closed takes its
// rollout with it. Names stay: the index holds them all in one file.
func (r *Reader) Retain(sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions = transcript.Retain(r.sessions, sessionIDs)
}

// layout is where Codex files a rollout: under the day the session started,
// never its directory. A reverted session goes on in a newer file whose name
// adds a rollout id after the session's, which is why the layout picks.
var layout = transcript.Layout{
	Anywhere: func(root, sessionID string) string {
		return filepath.Join(root, "sessions", "*", "*", "*", "rollout-*-"+sessionID+"*.jsonl")
	},
	// A rollout is named after its date, so the newest sorts last.
	Pick: func(matches []string) string { return matches[len(matches)-1] },
}

// names is the session index, the one file every session's name is appended
// to, and what it has said so far, the last entry for an id winning. No name
// is repeated later in it, so it is read whole.
type names struct {
	path     string
	follower transcript.Follower
	of       map[string]string
}

func (n *names) read() {
	// A file that shrank was rewritten without some session's names.
	n.follower.Follow(n.path, func() { n.of = nil }, func(line string) {
		var read struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal([]byte(line), &read) != nil || read.ID == "" {
			return
		}

		if n.of == nil {
			n.of = make(map[string]string)
		}

		n.of[read.ID] = strings.TrimSpace(read.ThreadName)
	})
}

// entry is one rollout line. The file carries far more per line — the
// instructions, every tool call, token counts — and none of it says what the
// session is about.
type entry struct {
	Type    string `json:"type"`
	Payload struct {
		Type    string `json:"type"`
		CWD     string `json:"cwd"`
		Message string `json:"message"`
	} `json:"payload"`
}

// absorb updates the topic from one rollout line. The header says where the
// session started and each turn where it went on; the first prompt is kept.
func (s *rollout) absorb(line string) {
	var read entry
	if json.Unmarshal([]byte(line), &read) != nil {
		return
	}

	switch read.Type {
	case "session_meta", "turn_context":
		// Kept as the file spelled it: whether a path can be a checkout is
		// settled where one is read.
		if read.Payload.CWD != "" {
			s.topic.Dir = read.Payload.CWD
		}
	case "event_msg":
		if s.topic.Opening == "" && read.Payload.Type == "user_message" {
			s.topic.Opening = transcript.FirstLine(read.Payload.Message)
		}
	}
}
