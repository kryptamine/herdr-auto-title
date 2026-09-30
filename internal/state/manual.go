package state

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Manual remembers which tabs and panes the user renamed by hand, so Auto Title
// stops naming them. A rename is not an event but a label that
// moved between two polls; see docs/architecture/manual-rename-protection.md.
type Manual struct {
	Tabs  *Claims
	Panes *Claims

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
	seen   map[string]labels
	// locked is the label a thing carried when the user claimed it. The label,
	// not the id, is what makes a reloaded lock safe: Herdr reuses ids.
	locked map[string]string
	// followsMoves takes a thing sighted wearing an unclaimed departed label as
	// the same one, moved. Only a pane: Herdr moves one across workspaces under
	// a new id, label intact, while a moved tab keeps its id.
	followsMoves bool
	// departed is the labels of things gone unclaimed since the last poll that
	// saw everything; a poll cut short must not lose them.
	departed map[string]struct{}
}

// labels is what is known of one thing's label: the one it carried when last
// looked at, and those of renames whose call got no answer. Herdr may apply one
// of those seconds later, and it is Auto Title's label all the same.
type labels struct {
	current string
	sent    map[string]struct{}
}

func newClaims(m *Manual, followsMoves bool) *Claims {
	return &Claims{
		manual:       m,
		followsMoves: followsMoves,
		seen:         make(map[string]labels),
		locked:       make(map[string]string),
		departed:     make(map[string]struct{}),
	}
}

// manualFile is the on-disk form: locks outlive the process because Herdr can
// restart a plugin mid-session.
type manualFile struct {
	Locked      map[string]string `json:"locked_tabs"`
	LockedPanes map[string]string `json:"locked_panes"`
}

// LoadManual reads persisted locks from path. Anything unreadable yields an
// empty set: this is a convenience, not a reason to refuse to start.
func LoadManual(path string) *Manual {
	m := &Manual{path: path}
	m.Tabs = newClaims(m, false)
	m.Panes = newClaims(m, true)

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

	return m
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

// Verdict is what a poll makes of a label it saw.
type Verdict int

const (
	// VerdictName says nobody claims the label, so the resolver's name may go on.
	VerdictName Verdict = iota
	// VerdictClaimed says the user put the label there, and it is left alone.
	VerdictClaimed
)

// Observe records what a poll saw and says whether the user put that label
// there: docs/architecture/manual-rename-protection.md.
func (c *Claims) Observe(s Sighting) Verdict {
	m := c.manual

	m.mu.Lock()
	defer m.mu.Unlock()

	previous, known := c.seen[s.ID]
	_, moved := c.departed[s.Current]
	ours := c.ours(s) || (!known && moved)
	c.record(s, previous)

	return c.claimedOnChange(s, previous, known, ours)
}

// Settled marks the end of a poll that saw everything. After the first,
// something unseen did not exist before; after any, a moved thing has been seen.
func (m *Manual) Settled() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.settled = true

	for _, c := range []*Claims{m.Tabs, m.Panes} {
		clear(c.departed)
	}
}

// Applied records a label Auto Title has just set, so the next poll does not
// read its own work as the user's.
func (c *Claims) Applied(id, label string) {
	c.manual.mu.Lock()
	defer c.manual.mu.Unlock()

	seen := c.seen[id]
	seen.current = label
	c.seen[id] = seen
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

	if c.followsMoves {
		c.keepDeparted(live)
	}

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

	if changed {
		m.saveLocked()
	}
}

// keepDeparted adds the labels of things gone from live that the user had not
// claimed. It runs before Retain forgets them, so the locks are still there.
func (c *Claims) keepDeparted(live map[string]string) {
	for id, seen := range c.seen {
		_, alive := live[id]
		_, claimed := c.locked[id]

		if !alive && !claimed && seen.current != "" {
			c.departed[seen.current] = struct{}{}
		}
	}
}

// record remembers the label a poll saw. A rename that got no answer and has
// now landed is no longer waited for.
func (c *Claims) record(s Sighting, previous labels) {
	delete(previous.sent, s.Current)
	c.seen[s.ID] = labels{current: s.Current, sent: previous.sent}
}

// claimedOnChange is the tab and pane rule: a label that moved between two
// polls, and was not this plugin's doing, is the user's.
func (c *Claims) claimedOnChange(s Sighting, previous labels, known, ours bool) Verdict {
	switch {
	case ours:
		return VerdictName
	case s.Current == "", s.Current == s.Default:
		// Nobody has named it. A tab can wear either spelling -- clearing a name
		// empties the label rather than restoring the position, and a position
		// slides down when a tab to its left closes. A pane has only the empty.
		return VerdictName
	case known:
		if s.Current == previous.current {
			return VerdictName
		}
	case !c.manual.settled:
		// The first poll, where nothing carries a name Auto Title has set.
		return VerdictName
	}

	return c.claim(s)
}

func (c *Claims) claim(s Sighting) Verdict {
	c.locked[s.ID] = s.Current

	c.manual.saveLocked()

	return VerdictClaimed
}

// ours reports whether Auto Title put this label there: it is the name wanted
// now, or that of a rename whose call got no answer, which Herdr applied late.
func (c *Claims) ours(s Sighting) bool {
	_, sent := c.seen[s.ID].sent[s.Current]
	return sent || s.Current == s.Desired
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
			Locked:      m.Tabs.locked,
			LockedPanes: m.Panes.locked,
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
