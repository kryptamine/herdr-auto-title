package desktop

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The labels Ovis puts on a terminal it starts without the app.
const (
	coreSpaceLabel = "herdr.space"
	coreTabLabel   = "herdr.tab"
	ovisTabName    = "ovis.tab"
)

// reservedSpace is Ovis's "Other terminals", the terminals no workspace made;
// it has no name to change, so a label naming it starts no space.
const reservedSpace = "hc-other"

// unplaced files the live local terminals no document tab holds into the tabs
// their labels name, as Ovis groups them:
// docs/architecture/desktop-api.md#a-core-with-no-window.
type unplaced struct {
	w *world
	// spaces indexes w.Spaces by id; taken holds every space and tab id in use.
	spaces map[string]int
	taken  map[string]bool
}

func addUnplaced(w *world, terminals []terminal) {
	u := unplaced{w: w, spaces: make(map[string]int), taken: map[string]bool{reservedSpace: true}}
	placed := make(map[string]bool)

	for i, s := range w.Spaces {
		u.spaces[s.ID], u.taken[s.ID] = i, true

		for _, t := range s.Tabs {
			u.taken[t.ID] = true

			for _, entry := range t.Terminals {
				placed[entry.Terminal] = true
			}
		}
	}

	for _, t := range terminals {
		if t.State == "live" && t.Address != "" && !remoteAddress(t.Address) && !placed[t.Address] {
			u.add(t)
		}
	}
}

func (u *unplaced) add(t terminal) {
	s := u.space(t.Labels[coreSpaceLabel])
	if s == nil {
		return
	}

	if s.Name == "" {
		workspace, _ := whereNames(t)
		s.Name = cmp.Or(
			strings.TrimSpace(workspace),
			t.Labels["ovis.workspace"],
			filepath.Base(cmp.Or(t.Foreground.CWD, t.CWD)),
		)
	}

	id := u.tabID(s, t)

	i := s.tabIndex(id)
	if i < 0 {
		_, name := whereNames(t)
		i = len(s.Tabs)
		s.Tabs = append(
			s.Tabs,
			tab{ID: id, Name: cmp.Or(name, t.Labels[ovisTabName], strconv.Itoa(i+1))},
		)
		u.taken[id] = true
	}

	s.Tabs[i].Terminals = append(s.Tabs[i].Terminals, corePane(t.Address, t.Address, t))
}

// space is the space a label names, started when the document has none. A
// terminal with no label, or one naming a tab's id, belongs to no space.
func (u *unplaced) space(id string) *space {
	if id == "" {
		return nil
	}

	i, exists := u.spaces[id]
	if !exists {
		if u.taken[id] {
			return nil
		}

		i = len(u.w.Spaces)
		u.spaces[id], u.taken[id] = i, true
		u.w.Spaces = append(u.w.Spaces, space{ID: id})
	}

	return &u.w.Spaces[i]
}

// tabID is the terminal's tab label, unless it has none or the id is in use
// outside this space: Ovis then gives the terminal a tab named after its address.
func (u *unplaced) tabID(s *space, t terminal) string {
	usable := func(id string) bool { return !u.taken[id] || s.tabIndex(id) >= 0 }

	if id := t.Labels[coreTabLabel]; id != "" && usable(id) {
		return id
	}

	base := s.ID + ":" + t.Address

	id := base
	for n := 2; !usable(id); n++ {
		id = base + ":" + strconv.Itoa(n)
	}

	return id
}

func (s space) tabIndex(id string) int {
	return slices.IndexFunc(s.Tabs, func(t tab) bool { return t.ID == id })
}

// whereNames splits Ovis's `where` label, `<space> / <tab>`.
func whereNames(t terminal) (workspace, name string) {
	workspace, rest, _ := strings.Cut(t.Labels["where"], " / ")
	name, _, _ = strings.Cut(rest, " / ")

	return workspace, strings.TrimSpace(name)
}
