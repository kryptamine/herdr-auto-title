package herdr_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

const session = "8852bfe0-8b24-4a23-a35e-7521d04da061"

func TestASessionIDIsReported(t *testing.T) {
	t.Parallel()

	ref := &herdr.AgentSessionInfo{Agent: "claude", Kind: herdr.SessionRefID, Value: session}

	if got, ok := ref.ID(); !ok || got != session {
		t.Errorf("ID() = %q, %v, want %q", got, ok, session)
	}
}

func TestAnIDThatIsNotASessionIsRefused(t *testing.T) {
	t.Parallel()

	// The id arrives over the socket and a reader makes it part of a path.
	for _, value := range []string{
		"", "../../../etc/passwd", "../annotations/" + session, "8852bfe0", strings.Repeat("a", 36),
	} {
		ref := &herdr.AgentSessionInfo{Agent: "claude", Kind: herdr.SessionRefID, Value: value}
		if got, ok := ref.ID(); ok {
			t.Errorf("value %q reported as id %q", value, got)
		}
	}
}

func TestAReferenceThatIsNotAnIDIsRefused(t *testing.T) {
	t.Parallel()

	ref := &herdr.AgentSessionInfo{Agent: "claude", Kind: "path", Value: session}

	if got, ok := ref.ID(); ok {
		t.Errorf("ID() = %q, want none for kind %q", got, ref.Kind)
	}
}

func TestAPaneWithNoSessionHasNoID(t *testing.T) {
	t.Parallel()

	var ref *herdr.AgentSessionInfo

	if got, ok := ref.ID(); ok {
		t.Errorf("ID() = %q, want none for a nil reference", got)
	}
}

func TestASessionPathIsReported(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "session.jsonl")
	ref := &herdr.AgentSessionInfo{Agent: "pi", Kind: herdr.SessionRefPath, Value: path}

	if got, ok := ref.Path(); !ok || got != path {
		t.Errorf("Path() = %q, %v, want %q", got, ok, path)
	}
}

func TestAPathThatIsNotAbsoluteAndCleanIsRefused(t *testing.T) {
	t.Parallel()

	// The path arrives over the socket and a reader opens it as it is.
	path := filepath.Join(t.TempDir(), "session.jsonl")
	relative := filepath.Join("sessions", "session.jsonl")

	// Spelled by hand: filepath.Join would clean the dot-dot away.
	dir, sep := filepath.Dir(path), string(filepath.Separator)
	unclean := dir + sep + ".." + sep + filepath.Base(dir) + sep + filepath.Base(path)

	for name, value := range map[string]string{
		"relative": relative,
		"unclean":  unclean,
		"empty":    "",
	} {
		ref := &herdr.AgentSessionInfo{Agent: "pi", Kind: herdr.SessionRefPath, Value: value}
		if got, ok := ref.Path(); ok {
			t.Errorf("%s value %q reported as path %q", name, value, got)
		}
	}
}

func TestAnIDIsNotAPath(t *testing.T) {
	t.Parallel()

	ref := &herdr.AgentSessionInfo{Agent: "claude", Kind: herdr.SessionRefID, Value: session}

	if got, ok := ref.Path(); ok {
		t.Errorf("Path() = %q, want none for kind %q", got, ref.Kind)
	}
}
