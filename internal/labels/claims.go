package labels

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// claims is what is remembered about one kind of thing Herdr labels. Herdr
// numbers each kind apart, so each keeps claims of its own.
type claims struct {
	labels *Labels
	kind   kind
	seen   map[string]seenLabels
	// locked is the label a thing carried when the user claimed it. The label,
	// not the id, is what makes a reloaded lock safe: Herdr reuses ids.
	locked map[string]string
	// departed is the labels of things gone unclaimed since the last poll that
	// saw everything; a poll cut short must not lose them.
	departed map[string]struct{}
}

// seenLabels is what is known of one thing's label: the one it carried when
// last looked at, and those of renames whose call got no answer. Herdr may
// apply one of those seconds later, and it is Auto Title's label all the same.
type seenLabels struct {
	current string
	sent    map[string]struct{}
}

func newClaims(l *Labels, kind kind) *claims {
	return &claims{
		labels:   l,
		kind:     kind,
		seen:     make(map[string]seenLabels),
		locked:   make(map[string]string),
		departed: make(map[string]struct{}),
	}
}

// claimsFile is the on-disk form: locks outlive the process because Herdr can
// restart a plugin mid-session.
type claimsFile struct {
	Locked      map[string]string `json:"locked_tabs"`
	LockedPanes map[string]string `json:"locked_panes"`
}

// isLocked reports whether the user has claimed this one.
func (c *claims) isLocked(id string) bool {
	c.labels.mu.Lock()
	defer c.labels.mu.Unlock()

	_, locked := c.locked[id]

	return locked
}

// sighting is what one poll saw of a tab or a pane: the label it
// carries, the name chosen for it, and what Herdr names one nobody has claimed.
type sighting struct {
	id        string
	current   string
	desired   string
	byDefault string
}

// verdict is what a poll makes of a label it saw.
type verdict int

const (
	// verdictName says nobody claims the label, so the resolver's name may go on.
	verdictName verdict = iota
	// verdictClaimed says the user put the label there, and it is left alone.
	verdictClaimed
)

// observe records what a poll saw and says whether the user put that label
// there: docs/architecture/manual-rename-protection.md.
func (c *claims) observe(s sighting) verdict {
	l := c.labels

	l.mu.Lock()
	defer l.mu.Unlock()

	previous, known := c.seen[s.id]
	_, moved := c.departed[s.current]
	ours := c.ours(s) || (!known && moved)
	c.record(s, previous)

	return c.claimedOnChange(s, previous, known, ours)
}

// settle marks the end of a poll that saw everything. After the first,
// something unseen did not exist before; after any, a moved thing has been seen.
func (l *Labels) settle() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.settled = true

	for _, c := range []*claims{l.tabs, l.panes} {
		clear(c.departed)
	}
}

// applied records a label Auto Title has just set, so the next poll does not
// read its own work as the user's.
func (c *claims) applied(id, label string) {
	c.labels.mu.Lock()
	defer c.labels.mu.Unlock()

	seen := c.seen[id]
	seen.current = label
	c.seen[id] = seen
}

// sent records the label of a rename whose call got no answer, which Herdr
// may still apply once it answers again — by when the name wanted may have
// moved on.
func (c *claims) sent(id, label string) {
	c.labels.mu.Lock()
	defer c.labels.mu.Unlock()

	seen := c.seen[id]
	if seen.sent == nil {
		seen.sent = make(map[string]struct{})
	}

	seen.sent[label] = struct{}{}
	c.seen[id] = seen
}

// retain drops everything about what the session no longer holds, and releases
// a lock whose owner now carries a different label — which is what stops a
// reloaded lock from claiming an unrelated tab or pane that inherited its id.
func (c *claims) retain(live map[string]string) {
	l := c.labels

	l.mu.Lock()
	defer l.mu.Unlock()

	changed := false

	if c.kind.followsMoves {
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
		l.saveLocked()
	}
}

// keepDeparted adds the labels of things gone from live that the user had not
// claimed. It runs before retain forgets them, so the locks are still there.
func (c *claims) keepDeparted(live map[string]string) {
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
func (c *claims) record(s sighting, previous seenLabels) {
	delete(previous.sent, s.current)
	c.seen[s.id] = seenLabels{current: s.current, sent: previous.sent}
}

// claimedOnChange is the tab and pane rule: a label that moved between two
// polls, and was not this plugin's doing, is the user's.
func (c *claims) claimedOnChange(s sighting, previous seenLabels, known, ours bool) verdict {
	switch {
	case ours:
		return verdictName
	case s.current == "", s.current == s.byDefault:
		// Nobody has named it. A tab can wear either spelling -- clearing a name
		// empties the label rather than restoring the position, and a position
		// slides down when a tab to its left closes. A pane has only the empty.
		return verdictName
	case known:
		if s.current == previous.current {
			return verdictName
		}
	case !c.labels.settled:
		// The first poll, where nothing carries a name Auto Title has set.
		return verdictName
	}

	return c.claim(s)
}

func (c *claims) claim(s sighting) verdict {
	c.locked[s.id] = s.current

	c.labels.saveLocked()

	return verdictClaimed
}

// ours reports whether Auto Title put this label there: it is the name wanted
// now, or that of a rename whose call got no answer, which Herdr applied late.
func (c *claims) ours(s sighting) bool {
	_, sent := c.seen[s.id].sent[s.current]
	return sent || s.current == s.desired
}

// saveLocked writes the locks out through a temporary file, so a crash cannot
// leave a half-written one. The caller holds the mutex; failure is silent.
func (l *Labels) saveLocked() {
	if l.path == "" {
		return
	}

	if os.MkdirAll(filepath.Dir(l.path), 0o700) != nil {
		return
	}

	// encoding/json sorts map keys itself, so the file is diffable already.
	raw, err := json.MarshalIndent(
		claimsFile{
			Locked:      l.tabs.locked,
			LockedPanes: l.panes.locked,
		},
		"",
		"  ",
	)
	if err != nil {
		return
	}

	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) != nil {
		return
	}

	if os.Rename(tmp, l.path) != nil {
		_ = os.Remove(tmp)
	}
}
