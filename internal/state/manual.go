package state

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Manual remembers which tabs, panes and workspaces the user renamed by hand,
// so Auto Title stops naming them. A rename is not an event but a label that
// moved between two polls; see docs/architecture/manual-rename-protection.md.
type Manual struct {
	Tabs       *Claims
	Panes      *Claims
	Workspaces *Claims

	mu   sync.Mutex
	path string
	// settled is false until the first poll has finished, while nothing can yet
	// be judged. One poll looks at every kind together, so they share it.
	settled bool
}

// Claims is what is remembered about one kind of thing Herdr labels. Herdr
// numbers each kind apart, so each keeps claims of its own.
type Claims struct {
	manual *Manual
	// lockUnseen claims a label the first poll has never seen. Only a workspace
	// carries it: Herdr labels an unnamed one after its directory, so anything
	// else is a name its owner chose. See manual-rename-protection.md.
	lockUnseen bool
	seen       map[string]labels
	// pending holds a workspace sighted with no default yet -- no pane, so no
	// directory to compare against. It is judged on the first poll that has one.
	pending map[string]struct{}
	// written is the label this last wrote, kept on disk for the kind claimed
	// on sight: a restarted plugin finds its own work wearing neither the
	// default nor a name it remembers, and must not claim it as the owner's.
	written map[string]string
	// locked is the label a thing carried when the user claimed it. The label,
	// not the id, is what makes a reloaded lock safe: Herdr reuses ids.
	locked map[string]string
}

// labels is what is known of one thing's label: the one it carried when last
// looked at, and those of renames whose call got no answer. Herdr may apply one
// of those seconds later, and it is Auto Title's label all the same.
type labels struct {
	current string
	sent    map[string]struct{}
}

func newClaims(m *Manual, lockUnseen bool) *Claims {
	return &Claims{
		manual:     m,
		lockUnseen: lockUnseen,
		seen:       make(map[string]labels),
		pending:    make(map[string]struct{}),
		written:    make(map[string]string),
		locked:     make(map[string]string),
	}
}

// manualFile is the on-disk form: locks outlive the process because Herdr can
// restart a plugin mid-session.
type manualFile struct {
	Locked           map[string]string `json:"locked_tabs"`
	LockedPanes      map[string]string `json:"locked_panes"`
	LockedWorkspaces map[string]string `json:"locked_workspaces"`
	// WrittenWorkspaces is what this last named each workspace, which a lock
	// cannot say: it is the label a restart must not mistake for the owner's.
	WrittenWorkspaces map[string]string `json:"written_workspaces"`
}

// LoadManual reads persisted locks from path. Anything unreadable yields an
// empty set: this is a convenience, not a reason to refuse to start.
func LoadManual(path string) *Manual {
	m := &Manual{path: path}
	m.Tabs = newClaims(m, false)
	m.Panes = newClaims(m, false)
	m.Workspaces = newClaims(m, true)

	raw, err := os.ReadFile(path) //nolint:gosec // the path is configured, never terminal-derived
	if err != nil {
		return m
	}

	var stored manualFile
	if json.Unmarshal(raw, &stored) != nil {
		return m
	}

	maps.Copy(m.Tabs.locked, stored.Locked)
	maps.Copy(m.Panes.locked, stored.LockedPanes)
	maps.Copy(m.Workspaces.locked, stored.LockedWorkspaces)
	maps.Copy(m.Workspaces.written, stored.WrittenWorkspaces)

	return m
}

// DefaultManualPath is where locks are kept when nothing says otherwise.
func DefaultManualPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	return filepath.Join(dir, "herdr-auto-title", "manual-names.json")
}

// Locked reports whether the user has claimed this one.
func (c *Claims) Locked(id string) bool {
	c.manual.mu.Lock()
	defer c.manual.mu.Unlock()

	_, locked := c.locked[id]

	return locked
}

// Sighting is what one poll saw of a tab or a pane: the label it carries, what
// the resolver would name it, and what Herdr names one nobody has claimed.
type Sighting struct {
	ID      string
	Current string
	Desired string
	Default string
}

// SightingFrom is what a poll saw of a tab, given the name the resolver chose
// for it. What Herdr calls an unclaimed tab is its position, which is this
// package's to know.
func SightingFrom(tab TabState, desired string) Sighting {
	return Sighting{
		ID:      tab.ID,
		Current: tab.CurrentName,
		Desired: desired,
		Default: strconv.Itoa(tab.Position),
	}
}

// PaneSightingFrom is what a poll saw of a pane. A pane has one spelling for
// unnamed and no second default: pane.rename clears an empty label rather than
// storing it, so Default stays empty.
func PaneSightingFrom(pane *PaneState, desired string) Sighting {
	return Sighting{
		ID:      pane.ID,
		Current: pane.CurrentName,
		Desired: desired,
	}
}

// Observe records what a poll saw and reports whether the user put that label
// there.
func (c *Claims) Observe(s Sighting) bool {
	m := c.manual

	m.mu.Lock()
	defer m.mu.Unlock()

	previous, known := c.seen[s.ID]
	ours := c.ours(s)

	// A rename that landed late is as much this plugin's work as one that
	// answered, and a restart must find it recorded as such.
	if _, landed := previous.sent[s.Current]; landed && c.lockUnseen {
		c.written[s.ID] = s.Current

		m.saveLocked()
	}

	delete(previous.sent, s.Current)
	c.seen[s.ID] = labels{current: s.Current, sent: previous.sent}

	// The poll a workspace is told apart on: the first, or for one that showed
	// no default then, the first that does. Until then it stays unseen, so that
	// poll still finds it new.
	_, pending := c.pending[s.ID]
	judging := c.lockUnseen && !known && (!m.settled || pending)

	if judging {
		// Its own work, found again after a restart: neither the default nor
		// the owner's, and named on rather than claimed.
		if written, ok := c.written[s.ID]; ok && written == s.Current {
			delete(c.pending, s.ID)

			return false
		}

		if s.Default == "" {
			c.pending[s.ID] = struct{}{}
			delete(c.seen, s.ID)

			return false
		}

		delete(c.pending, s.ID)
	}

	// Nobody has named it. A tab can wear either spelling -- clearing a name
	// empties the label rather than restoring the position. A pane has only
	// the empty, and a workspace the basename Herdr derived for it.
	if s.Current == "" || s.Current == s.Default {
		return false
	}

	// Everything Herdr writes itself was let through above, so a workspace
	// reaching here on its judging poll wears a name its owner wrote. One opened
	// later is not claimed for being unseen: manual-rename-protection.md.
	if c.lockUnseen && !known {
		if !judging {
			return false
		}

		c.locked[s.ID] = s.Current

		m.saveLocked()

		return true
	}

	switch {
	case ours:
		return false
	case known:
		if s.Current == previous.current {
			return false
		}
	case !m.settled:
		// The first poll, where nothing carries a name Auto Title has set.
		return false
	}

	c.locked[s.ID] = s.Current

	m.saveLocked()

	return true
}

// ours reports whether Auto Title put this label there: it is the name wanted
// now, or that of a rename whose call got no answer, which Herdr applied late.
func (c *Claims) ours(s Sighting) bool {
	_, sent := c.seen[s.ID].sent[s.Current]
	return sent || s.Current == s.Desired
}

// Pending reports whether this one is still waiting for a default to be judged
// against. It has no basename yet, so nothing may be read into its label -- and
// nothing written over it, or the judging poll would find the plugin's own work.
func (c *Claims) Pending(id string) bool {
	c.manual.mu.Lock()
	defer c.manual.mu.Unlock()

	_, pending := c.pending[id]

	return pending
}

// Settled marks the end of a poll. Only the first matters: after it, something
// unseen is something that did not exist before.
func (m *Manual) Settled() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.settled = true
}

// Applied records a label Auto Title has just set, so the next poll does not
// read its own work as the user's.
func (c *Claims) Applied(id, label string) {
	c.manual.mu.Lock()
	defer c.manual.mu.Unlock()

	seen := c.seen[id]
	seen.current = label
	c.seen[id] = seen

	if c.lockUnseen {
		c.written[id] = label

		c.manual.saveLocked()
	}
}

// Sent records the label of a rename whose call got no answer, which Herdr
// may still apply once it answers again — by when the name wanted may have
// moved on.
func (c *Claims) Sent(id, label string) {
	c.manual.mu.Lock()
	defer c.manual.mu.Unlock()

	seen := c.seen[id]
	if seen.sent == nil {
		seen.sent = make(map[string]struct{})
	}

	seen.sent[label] = struct{}{}
	c.seen[id] = seen
}

// Retain drops everything about what the session no longer holds, and releases
// a lock whose owner now carries a different label — which is what stops a
// reloaded lock from claiming an unrelated tab or pane that inherited its id.
func (c *Claims) Retain(live map[string]string) {
	m := c.manual

	m.mu.Lock()
	defer m.mu.Unlock()

	changed := false

	for id, label := range c.locked {
		if current, alive := live[id]; !alive || current != label {
			delete(c.locked, id)

			changed = true
		}
	}

	for id := range c.seen {
		if _, alive := live[id]; !alive {
			delete(c.seen, id)
		}
	}

	for id := range c.pending {
		if _, alive := live[id]; !alive {
			delete(c.pending, id)
		}
	}

	for id := range c.written {
		if _, alive := live[id]; !alive {
			delete(c.written, id)

			changed = true
		}
	}

	if changed {
		m.saveLocked()
	}
}

// saveLocked writes the locks out through a temporary file, so a crash cannot
// leave a half-written one. The caller holds the mutex; failure is silent.
func (m *Manual) saveLocked() {
	if m.path == "" {
		return
	}

	if os.MkdirAll(filepath.Dir(m.path), 0o700) != nil {
		return
	}

	// encoding/json sorts map keys itself, so the file is diffable already.
	raw, err := json.MarshalIndent(
		manualFile{
			Locked:            m.Tabs.locked,
			LockedPanes:       m.Panes.locked,
			LockedWorkspaces:  m.Workspaces.locked,
			WrittenWorkspaces: m.Workspaces.written,
		},
		"",
		"  ",
	)
	if err != nil {
		return
	}

	tmp := m.path + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) != nil {
		return
	}

	if os.Rename(tmp, m.path) != nil {
		_ = os.Remove(tmp)
	}
}
