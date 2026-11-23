package resolver

import (
	"slices"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// sessionIDWidth is how much of a session id a qualifier keeps: the first
// group of its UUID.
const sessionIDWidth = 8

// Qualifiers finds the tabs whose titles would read the same and says, by tab
// ID, what tells each one apart. The ladder it climbs is in
// docs/architecture/title-resolution.md.
func (d *Deterministic) Qualifiers(tabs []state.TabState) map[string]string {
	names := make(map[string]string, len(tabs))
	byName := make(map[string][]state.TabState, len(tabs))

	for _, tab := range tabs {
		name := d.Resolve(tab).Name
		names[tab.ID] = name
		byName[name] = append(byName[name], tab)
	}

	// In name order, so which group keeps a name another wanted does not
	// depend on map order.
	twins := make([]string, 0, len(byName))
	for name, group := range byName {
		if len(group) > 1 {
			twins = append(twins, name)
		}
	}

	slices.Sort(twins)

	qualifiers := make(map[string]string)
	for _, name := range twins {
		d.qualify(byName[name], names, qualifiers)
	}

	return qualifiers
}

// qualify takes the first rung of the ladder that gives every tab of one group
// a name no other tab carries, and leaves the group's names as they were when
// no rung does. names holds every tab's name, and is kept current.
func (d *Deterministic) qualify(group []state.TabState, names, qualifiers map[string]string) {
	before := make([]string, len(group))
	for i, tab := range group {
		before[i] = names[tab.ID]
	}

	for _, valueOf := range []func(state.TabState) string{
		d.boundedContextOf, d.contextOf, sessionOf,
	} {
		values := make([]string, len(group))

		for i, tab := range group {
			values[i] = valueOf(tab)
			tab.Qualifier = values[i]
			names[tab.ID] = d.Resolve(tab).Name
		}

		if unique(names, group) {
			for i, tab := range group {
				if values[i] != "" {
					qualifiers[tab.ID] = values[i]
				}
			}

			return
		}
	}

	for i, tab := range group {
		names[tab.ID] = before[i]
	}
}

// unique reports that no tab of group shares its name with any other tab.
func unique(names map[string]string, group []state.TabState) bool {
	count := make(map[string]int, len(names))
	for _, name := range names {
		count[name]++
	}

	for _, tab := range group {
		if count[names[tab.ID]] > 1 {
			return false
		}
	}

	return true
}

// contextOf is where the chain places a tab before any row above or a bound
// has taken it away. A hidden directory stays hidden: the user asked never to
// see it, so its twins go on to their sessions.
func (d *Deterministic) contextOf(tab state.TabState) string {
	found := d.collect(tab.Context)
	if found.directory && d.hideDirectory {
		return ""
	}

	return found.parts.Context
}

// boundedContextOf is the directory cut to DirectoryMax, tried before the whole
// one so the activity keeps its width; "" when no bound applies.
func (d *Deterministic) boundedContextOf(tab state.TabState) string {
	found := d.collect(tab.Context)
	if !found.directory || d.hideDirectory || d.directoryMax <= 0 {
		return ""
	}

	return truncateToGrapheme(found.parts.Context, d.directoryMax)
}

// sessionOf is the start of the session id the tab's agent holds, or "" when it
// reported none by id.
func sessionOf(tab state.TabState) string {
	if tab.Context == nil {
		return ""
	}

	id, ok := tab.Context.AgentSession.ID()
	if !ok {
		return ""
	}

	return id[:sessionIDWidth]
}
