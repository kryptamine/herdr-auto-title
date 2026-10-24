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

	follower := transcript.Follower{CatchUp: transcript.HeadAndTail}

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

func TestAWholeCatchUpSkipsNoLine(t *testing.T) {
	t.Parallel()

	// Caught up on from its tail, a file past MaxScan loses every line before
	// the tail, which for an index of many sessions is most of them.
	path := filepath.Join(t.TempDir(), "index.jsonl")
	lines := []string{`{"name":"first"}`}

	for filler := strings.Repeat("x", 1000); len(lines)*len(filler) <= transcript.MaxScan; {
		lines = append(lines, filler)
	}

	lines = append(lines, `{"name":"last"}`)
	content := strings.Join(lines, "\n") + "\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	follower := transcript.Follower{CatchUp: transcript.Whole}

	var read []string

	absorb := func(line string) { read = append(read, line) }
	for range len(content)/transcript.MaxScan + 1 {
		follower.Follow(path, func() {}, absorb)
	}

	if strings.Join(read, "\n") != strings.Join(lines, "\n") {
		t.Errorf("read %d lines, want all %d of the file in order", len(read), len(lines))
	}
}

func TestAnotherPathIsReadFromItsStart(t *testing.T) {
	t.Parallel()

	// A follower handed another file would otherwise read it from where the
	// first one was left, and what it said before the switch is kept.
	dir := t.TempDir()
	first := filepath.Join(dir, "first.jsonl")
	second := filepath.Join(dir, "second.jsonl")

	if err := os.WriteFile(first, []byte("{\"n\":1}\n{\"n\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(second, []byte("{\"n\":3}\n{\"n\":4}\n{\"n\":5}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var follower transcript.Follower

	var read []string

	resets := 0
	reset := func() { resets++ }
	absorb := func(line string) { read = append(read, line) }
	follower.Follow(first, reset, absorb)
	follower.Follow(second, reset, absorb)

	want := []string{`{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":4}`, `{"n":5}`}
	if strings.Join(read, " ") != strings.Join(want, " ") {
		t.Errorf("lines = %q, want %q", read, want)
	}

	if resets != 0 {
		t.Errorf("reset ran %d times, want what the first file said kept", resets)
	}
}
