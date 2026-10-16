// Package antigravity reads what an Antigravity CLI session is about from the
// annotation the agent keeps beside it. Herdr says which session a pane holds;
// the annotation is where the agent stores the title it generated for it.
package antigravity

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// Agent is the label Herdr gives Antigravity CLI.
const Agent = "agy"

// titlePattern matches the title field of an annotation, which is protobuf
// text format holding one quoted string.
var titlePattern = regexp.MustCompile(`(?m)^\s*title\s*:\s*("(?:[^"\\\n]|\\.)*")`)

// sessionIDPattern is the shape of a session id. The id arrives over the socket
// and becomes part of a path, so anything else is refused rather than cleaned.
var sessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

type Reader struct {
	root string
}

// NewReader builds a reader over root, the Antigravity CLI state directory.
func NewReader(root string) *Reader {
	return &Reader{root: root}
}

// Title reports what the agent named the session, or "" when nothing can be
// read.
func (r *Reader) Title(sessionID string) string {
	if r.root == "" || !sessionIDPattern.MatchString(sessionID) {
		return ""
	}

	//nolint:gosec // the root is configured and the id is checked before use
	content, err := os.ReadFile(filepath.Join(r.root, "annotations", sessionID+".pbtxt"))
	if err != nil {
		return ""
	}

	match := titlePattern.FindSubmatch(content)
	if match == nil {
		return ""
	}

	title, err := strconv.Unquote(string(match[1]))
	if err != nil {
		return ""
	}

	return title
}
