// Package transcript follows the append-only session files agents write, so
// each agent's package parses only what a line means to that agent.
package transcript

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// MaxScan bounds one read. A session file only grows, so this bounds the
// catch-up on a session first seen mid-flight, not the steady state.
const MaxScan = 2 << 20

// CatchUp is which part of a file first seen past MaxScan is read, which
// depends on what the file says only once.
type CatchUp int

const (
	// Tail reads the last MaxScan bytes alone, where a name the session was
	// given is repeated.
	Tail CatchUp = iota
	// HeadAndTail reads the first MaxScan bytes too, for a file that says where
	// the session started only up front.
	HeadAndTail
	// Whole skips nothing and reads MaxScan a poll until it has caught up, for
	// a file no line of which is repeated later: an index of many sessions.
	Whole
)

// Follower reads one file a poll at a time, from where the last read of that
// file stopped. Another path is read from its start, over what the last one
// said: a session that went on in another file is still that session.
type Follower struct {
	CatchUp CatchUp
	path    string
	offset  int64
}

// Follow hands absorb each complete line appended to the file at path since
// the last call. A file shorter than what was read is a new file, so reset
// runs first; a file that is gone reports false and is read from the start.
func (f *Follower) Follow(path string, reset func(), absorb func(line string)) bool {
	if path != f.path {
		f.path, f.offset = path, 0
	}

	info, err := os.Stat(path)
	if err != nil {
		f.offset = 0

		return false
	}

	size := info.Size()
	switch {
	case size == f.offset:
		return true
	case size < f.offset:
		f.offset = 0

		reset()
	}

	//nolint:gosec // each caller refuses a path it did not build or check itself
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	// A read-only file has nothing to report on close.
	defer func() { _ = file.Close() }()

	// What lies between the head and the tail says nothing.
	if f.CatchUp == HeadAndTail && f.offset == 0 && size > MaxScan {
		next, err := readLines(file, 0, false, absorb)
		if err != nil {
			return true
		}

		// A head with no whole line leaves next at zero, and the tail is read
		// anyway rather than the same head again every poll.
		f.offset = next
	}

	from, fromTail := f.offset, f.CatchUp != Whole && size-f.offset > MaxScan
	if fromTail {
		from = size - MaxScan
	}

	next, err := readLines(file, from, fromTail, absorb)
	if err != nil {
		return true
	}

	if next > from {
		f.offset = next
	}

	return true
}

// readLines hands absorb the whole lines in the MaxScan bytes from offset, and
// reports where the read after this one starts. The agent may be writing the
// last line, so its rest arrives next poll.
func readLines(file *os.File, from int64, midLine bool, absorb func(string)) (int64, error) {
	if _, err := file.Seek(from, io.SeekStart); err != nil {
		return from, err
	}

	content, err := io.ReadAll(io.LimitReader(file, MaxScan))
	if err != nil {
		return from, err
	}

	end := bytes.LastIndexByte(content, '\n')
	if end < 0 {
		return from, nil
	}

	complete := string(content[:end])
	if midLine {
		// A tail read starts mid-line, and that fragment is not JSON.
		_, complete, _ = strings.Cut(complete, "\n")
	}

	for line := range strings.SplitSeq(complete, "\n") {
		if line != "" {
			absorb(line)
		}
	}

	return from + int64(end) + 1, nil
}

// TextOf reads a message's content, which is a string when the user typed one
// and a list of blocks when it was assembled from several.
func TextOf(content json.RawMessage) (string, bool) {
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

// Truncate cuts value to at most limit runes.
func Truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	return string(runes[:limit])
}
