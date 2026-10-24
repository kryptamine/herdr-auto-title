package transcript

import "strings"

// MaxOpening bounds what an opening prompt contributes. It is generous because
// the cut that matters is made later, against the width of the tab bar.
const MaxOpening = 200

// Topic is what a session says it is about, for an agent that keeps one name.
type Topic struct {
	// Name is what the session is called, and empty until it has one.
	Name string
	// Opening is the first prompt the user typed, which is what the agent's own
	// session list shows for a session that has no name.
	Opening string
	// Dir is where the agent works, as the file spelled it.
	Dir string
}

// Text is what the topic contributes to a title, the name first.
func (t Topic) Text() string {
	if t.Name != "" {
		return t.Name
	}

	return t.Opening
}

// FirstLine reduces a prompt to what can name a session: its first line.
func FirstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return Truncate(strings.TrimSpace(line), MaxOpening)
}

// Retain keeps the entries of held that ids name, so a session whose pane
// closed takes what was read of it along.
func Retain[V any](held map[string]V, ids []string) map[string]V {
	kept := make(map[string]V, len(ids))
	for _, id := range ids {
		if value, known := held[id]; known {
			kept[id] = value
		}
	}

	return kept
}
