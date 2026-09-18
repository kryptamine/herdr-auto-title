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
	path      string
	pid       int
	displaced int
}

// Take claims the session for this process, displacing whoever held it. The
// claim works whatever happened to the file — an instance that cannot be
// displaced is still worth running — so the error is for a warning, not a stop.
func Take(path string) (*Claim, error) {
	c := &Claim{path: path, pid: os.Getpid()}
	if path == "" {
		return c, errors.New("no configuration directory to claim the session in")
	}

	if pid := holder(path); pid != c.pid {
		c.displaced = pid
	}

	//nolint:gosec // the path is Auto Title's own, never terminal-derived
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return c, err
	}

	return c, write(path, record{PID: c.pid})
}

// Displaced is the pid of the running instance this claim displaced, or 0.
func (c *Claim) Displaced() int {
	return c.displaced
}

// AwaitDisplaced waits for the displaced instance to exit and reports whether
// it did within timeout. It is never killed: it leaves on its own once it sees
// the claim, and an instance too old to look is left to the user.
func (c *Claim) AwaitDisplaced(ctx context.Context, timeout time.Duration) bool {
	if c.displaced == 0 {
		return true
	}

	return await(ctx, c.displaced, timeout)
}

// Taken reports whether another process has claimed the session since. A claim
// that cannot be read decides nothing: it may be half written, and the next
// look will read it whole. A claim that is gone is nobody's, and taken again.
func (c *Claim) Taken() bool {
	rec, ok := read(c.path)
	if !ok {
		if c.path != "" && !exists(c.path) {
			_ = write(c.path, record{PID: c.pid})
		}

		return false
	}

	return rec.PID != c.pid
}

// Ready marks this instance as having read the session, in the marker rather
// than the claim, so a claim taken meanwhile is never written over.
func (c *Claim) Ready() {
	if c.path == "" || c.names(marker(c.path)) {
		return
	}

	_ = write(marker(c.path), record{PID: c.pid})
}

// Release removes the claim and the marker if they are still this process's
// own, so leaving because it was displaced does not take the successor's along.
func (c *Claim) Release() {
	for _, path := range []string{c.path, marker(c.path)} {
		if c.names(path) {
			_ = os.Remove(path) //nolint:gosec // the path is Auto Title's own
		}
	}
}

// names reports whether the file at path holds this process's pid.
func (c *Claim) names(path string) bool {
	rec, ok := read(path)

	return ok && rec.PID == c.pid
}

// holder is the pid of the running instance the claim file names, or 0 when
// it names nobody or a process that has since exited.
func holder(path string) int {
	if rec, ok := read(path); ok && alive(rec.PID) {
		return rec.PID
	}

	return 0
}

// ready reports whether the instance holding the claim has read the session.
func ready(path string) bool {
	pid := holder(path)
	rec, ok := read(marker(path))

	return pid != 0 && ok && rec.PID == pid
}

// await reports whether the process left within timeout.
func await(ctx context.Context, pid int, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for alive(pid) {
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

func exists(path string) bool {
	_, err := os.Stat(path) //nolint:gosec // the path is Auto Title's own

	return err == nil
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
