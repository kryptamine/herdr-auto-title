package files

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/files/filestest"
)

type clock struct{ at time.Time }

func (c *clock) now() time.Time           { return c.at }
func (c *clock) advance(by time.Duration) { c.at = c.at.Add(by) }

func newTestMachine(t *testing.T) (*Machine, *filestest.Machine, *clock) {
	t.Helper()

	c := &clock{at: time.Unix(1_700_000_000, 0)}
	m := NewMachine()
	m.now = c.now

	return m, filestest.New(), c
}

func write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fetch(t *testing.T, m *Machine, remote *filestest.Machine) {
	t.Helper()

	if err := m.Fetch(t.Context(), remote.Exec); err != nil {
		t.Fatal(err)
	}
}

func TestAQuestionIsAnsweredByTheNextBatch(t *testing.T) {
	t.Parallel()

	m, remote, _ := newTestMachine(t)
	local := filepath.Join(t.TempDir(), "HEAD")
	path := filestest.Remote(local)
	write(t, local, "ref: refs/heads/main\n")

	// Not knowing is not the file being gone: a reader must not act on it.
	if _, err := m.ReadAt(path, 0, 64); !errors.Is(err, ErrUnfetched) || m.Misses() != 1 {
		t.Fatalf("first read = %v after %d misses, want ErrUnfetched", err, m.Misses())
	}

	fetch(t, m, remote)

	data, err := m.ReadAt(path, 0, 64)
	if err != nil || string(data) != "ref: refs/heads/main\n" {
		t.Errorf("answered read = %q, %v", data, err)
	}

	if _, err := m.Stat(path + "/gone"); !errors.Is(err, ErrUnfetched) {
		t.Errorf("an unasked question was answered: %v", err)
	}
}

func TestAnsweredQuestionsAreAskedOncePerRefresh(t *testing.T) {
	t.Parallel()

	// A batch every poll for what nobody changed was the cost to avoid.
	m, remote, c := newTestMachine(t)
	local := filepath.Join(t.TempDir(), "HEAD")
	path := filestest.Remote(local)
	write(t, local, "one\n")

	_, _ = m.ReadAt(path, 0, 64)
	fetch(t, m, remote)

	for range 5 {
		c.advance(time.Second / 2)

		_, _ = m.ReadAt(path, 0, 64)
		fetch(t, m, remote)
	}

	if remote.Calls() != 1 {
		t.Fatalf("batches before the refresh = %d, want 1", remote.Calls())
	}

	write(t, local, "two\n")
	c.advance(refresh)
	fetch(t, m, remote)

	if data, _ := m.ReadAt(path, 0, 64); string(data) != "two\n" {
		t.Errorf("after the refresh = %q, want the file as it is now", data)
	}
}

func TestAFailedBatchKeepsTheLastAnswers(t *testing.T) {
	t.Parallel()

	// A connection that drops for a second must not rename every tab and back.
	m, remote, c := newTestMachine(t)
	local := filepath.Join(t.TempDir(), "HEAD")
	path := filestest.Remote(local)
	write(t, local, "kept\n")

	_, _ = m.ReadAt(path, 0, 64)
	fetch(t, m, remote)
	_, _ = m.ReadAt(path, 0, 64)

	remote.SetDown(true)
	c.advance(refresh)

	if err := m.Fetch(t.Context(), remote.Exec); err == nil {
		t.Fatal("a failed batch reported success")
	}

	if data, err := m.ReadAt(path, 0, 64); err != nil || string(data) != "kept\n" {
		t.Errorf("after a failure = %q, %v, want the last answer", data, err)
	}

	c.advance(keep)

	_ = m.Fetch(t.Context(), remote.Exec)

	if _, err := m.ReadAt(path, 0, 64); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("past keep = %v, want nothing known", err)
	}
}

func TestAFailingMachineIsAskedOncePerRefresh(t *testing.T) {
	t.Parallel()

	m, remote, c := newTestMachine(t)
	remote.SetDown(true)

	_, _ = m.Stat("/srv")

	for range 5 {
		_ = m.Fetch(t.Context(), remote.Exec)

		c.advance(time.Second / 2)

		_, _ = m.Stat("/srv")
	}

	if remote.Calls() != 1 {
		t.Errorf("batches to a failing machine = %d, want 1", remote.Calls())
	}
}

func TestAMachineThatNeverAnsweredHasNothingToWaitFor(t *testing.T) {
	t.Parallel()

	// No python3 there must still let its tabs be named from what the app says.
	m, remote, c := newTestMachine(t)
	remote.SetDown(true)

	_, _ = m.Stat("/srv")
	_ = m.Fetch(t.Context(), remote.Exec)
	misses := m.Misses()

	if _, err := m.Stat("/srv"); !errors.Is(err, fs.ErrNotExist) || m.Misses() != misses {
		t.Fatalf("unanswered machine = %v, %d new misses", err, m.Misses()-misses)
	}

	// What was asked meanwhile is the first thing asked once it answers.
	remote.SetDown(false)
	c.advance(refresh)
	fetch(t, m, remote)

	if _, err := m.Stat("/srv"); errors.Is(err, ErrUnfetched) {
		t.Error("the question asked while the machine was down was dropped")
	}
}

func TestBulkReadsAreBoundedPerBatchAndNotRefreshed(t *testing.T) {
	t.Parallel()

	// machine.exec truncates at 8 MiB, and a truncated batch is a failed one.
	m, remote, c := newTestMachine(t)
	local := filepath.Join(t.TempDir(), "rollout.jsonl")
	path := filestest.Remote(local)
	write(t, local, "line\n")

	for i := range 3 {
		_, _ = m.ReadAt(path, int64(i), 2<<20)
	}

	fetch(t, m, remote)

	var answered []int64

	for i := range int64(3) {
		if _, err := m.ReadAt(path, i, 2<<20); err == nil {
			answered = append(answered, i)
		}
	}

	if len(answered) != 2 {
		t.Fatalf("bulk reads in one batch = %d, want 2 of 2 MiB", len(answered))
	}

	c.advance(refresh)
	fetch(t, m, remote)

	for _, from := range answered {
		if _, err := m.ReadAt(path, from, 2<<20); !errors.Is(err, ErrUnfetched) {
			t.Errorf("the bulk read from %d was asked again at the refresh: %v", from, err)
		}
	}
}

func TestAStalledMachineIsGivenUpOn(t *testing.T) {
	t.Parallel()

	m, remote, _ := newTestMachine(t)
	remote.SetStalled(true)

	_, _ = m.Stat("/srv")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	start := time.Now()

	if err := m.Fetch(ctx, remote.Exec); err == nil {
		t.Fatal("a stalled batch reported success")
	}

	if waited := time.Since(start); waited > 2*fetchTimeout {
		t.Errorf("a stalled batch held the poll for %v", waited)
	}
}
