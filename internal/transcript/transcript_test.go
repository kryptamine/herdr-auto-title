package transcript_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

func TestAHeadWithNoWholeLineIsSkippedForTheTail(t *testing.T) {
	t.Parallel()

	// With no newline in the head, the head read used to give up with the
	// offset still at zero, so every poll read the same megabytes again.
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := strings.Repeat("x", transcript.MaxScan+1) + "\n" + `{"name":"tail"}` + "\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	follower := transcript.Follower{Head: true}

	var read []string

	absorb := func(line string) { read = append(read, line) }
	follower.Follow(path, func() {}, absorb)

	if len(read) != 1 || read[0] != `{"name":"tail"}` {
		t.Fatalf("lines = %q, want the tail's one whole line", read)
	}

	follower.Follow(path, func() {}, absorb)

	if len(read) != 1 {
		t.Errorf("lines = %q, want nothing read again from a file that did not grow", read)
	}
}
