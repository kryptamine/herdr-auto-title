// Package pi reads what a Pi session is about from the session file the agent
// appends to. Herdr says which file a pane's Pi holds; the file is where that
// session says what it is doing and where it started.
package pi

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Agent is the label Herdr gives Pi, and the only agent whose session file this
// package knows how to read.
const Agent = "pi"

// maxScan bounds one read. A session first seen mid-flight is caught up on at
// both ends, so this bounds each of those reads rather than the file.
const maxScan = 2 << 20

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

// session is one file and what has been read out of it.
type session struct {
	offset int64
	topic  Topic
}

func NewReader() *Reader {
	return &Reader{sessions: make(map[string]*session)}
}

// Topic reports what the session in the file at path is about, and the zero
// topic when nothing can be read. The path arrives over the socket, so only an
// absolute, already clean path to a .jsonl file is opened.
func (r *Reader) Topic(path string) Topic {
	if !isSessionPath(path) {
		return Topic{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	held, known := r.sessions[path]
	if !known {
		held = &session{}
		r.sessions[path] = held
	}

	held.readInto(path)

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

func isSessionPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && filepath.Ext(path) == ".jsonl"
}

// readInto reads what has been appended since the last read and updates the
// topic from it. A file shorter than what was read out of it is no longer the
// file that was read, and one that is gone keeps what it said while it was here.
func (s *session) readInto(path string) {
	info, err := os.Stat(path)
	if err != nil {
		s.offset = 0

		return
	}

	size := info.Size()
	switch {
	case size == s.offset:
		return
	case size < s.offset:
		s.offset, s.topic = 0, Topic{}
	}

	//nolint:gosec // the path is Herdr's, and refused above unless absolute, clean and a .jsonl
	file, err := os.Open(path)
	if err != nil {
		return
	}
	// A read-only file has nothing to report on close.
	defer func() { _ = file.Close() }()

	// A session first seen long after it started is read at both ends: the head
	// holds where it started and its first prompt, the tail the name it was last
	// given, and what lies between says neither.
	fromTail := false

	if s.offset == 0 && size > maxScan {
		head, next, ok := readLines(file, 0, false)
		if !ok {
			return
		}

		s.absorb(head)
		s.offset = next

		if tail := size - maxScan; tail > next {
			s.offset, fromTail = tail, true
		}
	}

	lines, next, ok := readLines(file, s.offset, fromTail)
	if !ok {
		return
	}

	s.absorb(lines)
	s.offset = next
}

// readLines reads the whole lines in the next maxScan bytes from offset, and
// where the read after this one starts. The agent may have been writing the
// last line while this ran, so the rest of it arrives next poll.
func readLines(file *os.File, from int64, midLine bool) (string, int64, bool) {
	if _, err := file.Seek(from, io.SeekStart); err != nil {
		return "", from, false
	}

	content, err := io.ReadAll(io.LimitReader(file, maxScan))
	if err != nil {
		return "", from, false
	}

	end := bytes.LastIndexByte(content, '\n')
	if end < 0 {
		return "", from, false
	}

	complete := string(content[:end])
	if midLine {
		// A tail read starts mid-line, and that fragment is not JSON.
		_, complete, _ = strings.Cut(complete, "\n")
	}

	return complete, from + int64(end) + 1, true
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

// absorb updates the topic from a run of session lines. The last name wins and
// the first opening does: a session is renamed as it goes, but it opened only
// once.
func (s *session) absorb(lines string) {
	for line := range strings.SplitSeq(lines, "\n") {
		if line == "" {
			continue
		}

		var read entry
		if json.Unmarshal([]byte(line), &read) != nil {
			continue
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
}

// opening reduces a user's first prompt to what can name a session: its first
// line, which is also what Pi's own session list shows.
func opening(content json.RawMessage) string {
	text, ok := textOf(content)
	if !ok {
		return ""
	}

	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return truncate(strings.TrimSpace(line), maxOpening)
}

// textOf reads a message's content, which is a string when the user typed one
// and a list of blocks when an image or a file came with it.
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
