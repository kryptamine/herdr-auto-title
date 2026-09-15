// Package instance keeps one Auto Title per session: a running instance claims
// the session in a file, a newer claim tells the older one to leave, and a
// restart is a new claim waiting for the old holder — docs/architecture/poll-loop.md.
package instance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tick is how often a wait looks again at a claim or at a process.
const tick = 50 * time.Millisecond

// record is what a claim file and a ready marker hold: one pid.
type record struct {
	PID int `json:"pid"`
}

// File names the claim for the session behind socket, under dir. A session has
// one socket and a user may run several sessions, so the socket path names
// the claim; hashed, because a path is not a file name.
func File(dir, socket string) string {
	if dir == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(socket))

	return filepath.Join(dir, "instances", hex.EncodeToString(sum[:8])+".json")
}

// marker names the ready marker beside a claim. It is a file of its own so
// that marking ready never writes over a claim taken in the meantime.
func marker(path string) string {
	if path == "" {
		return ""
	}

	return strings.TrimSuffix(path, filepath.Ext(path)) + ".ready.json"
}

// Claim is this process's hold on a session.
type Claim struct {
	path  string
	pid   int
	ready bool
}

// Take claims the session for this process, displacing whoever held it, and
// waits up to leave for that instance to go, killing nothing; it returns the
// pid that stayed, or 0, and an error worth a warning rather than a stop.
func Take(ctx context.Context, path string, leave time.Duration) (*Claim, int, error) {
	c := &Claim{path: path, pid: os.Getpid()}
	if path == "" {
		return c, 0, errors.New("no configuration directory to claim the session in")
	}

	displaced := holder(path)
	if displaced == c.pid {
		displaced = 0
	}

	//nolint:gosec // the path is Auto Title's own, never terminal-derived
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return c, displaced, err
	}

	if err := write(path, record{PID: c.pid}); err != nil {
		return c, displaced, err
	}

	if displaced == 0 || waitUntil(ctx, leave, func() bool { return !alive(displaced) }) {
		return c, 0, nil
	}

	return c, displaced, nil
}

// Taken reports whether another process has claimed the session since. A claim
// that cannot be read decides nothing: it may be half written, and the next
// look will read it whole. A claim that is gone is nobody's.
func (c *Claim) Taken() bool {
	rec, ok := read(c.path)

	return ok && rec.PID != c.pid
}

// Ready marks this instance as having read the session, in a marker rather
// than the claim, so a claim taken meanwhile is never written over.
func (c *Claim) Ready() {
	if c.path == "" || c.ready {
		return
	}

	c.ready = write(marker(c.path), record{PID: c.pid}) == nil
}

// holder is the pid of the running instance the claim file names, or 0 when
// it names nobody or a process that has since exited.
func holder(path string) int {
	if rec, ok := read(path); ok && alive(rec.PID) {
		return rec.PID
	}

	return 0
}

// marked reports whether pid has said it read the session.
func marked(path string, pid int) bool {
	rec, ok := read(marker(path))

	return ok && rec.PID == pid
}

// waitUntil reports whether done became true within timeout. A cancelled
// context and a deadline are told apart by the caller, through ctx.Err().
func waitUntil(ctx context.Context, timeout time.Duration, done func() bool) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for !done() {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}

	return true
}

// read is the file's record, or false when the file says nothing usable yet.
func read(path string) (record, bool) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is Auto Title's own
	if err != nil {
		return record{}, false
	}

	var rec record
	if json.Unmarshal(raw, &rec) != nil || rec.PID <= 0 {
		return record{}, false
	}

	return rec, true
}

// write puts the record down in one call. The file is a line, and a reader
// catching it half written treats that as nothing said.
func write(path string, rec record) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	return os.WriteFile(path, raw, 0o600) //nolint:gosec // the path is Auto Title's own
}
