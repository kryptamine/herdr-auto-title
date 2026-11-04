package resolver

import "strings"

// openCodeAgent is the agent Herdr reports for an OpenCode pane.
const openCodeAgent = "opencode"

// openCodePrefix starts the title OpenCode sets for a named session,
// `OC | <name>`; an unnamed one and the home screen read `OpenCode` instead.
const openCodePrefix = "OC | "

// withoutOpenCodeFrame reports the session name in an OpenCode pane's title,
// and false for the frame alone. A title OpenCode did not frame passes through.
func withoutOpenCodeFrame(title string) (string, bool) {
	trimmed := strings.TrimSpace(title)
	if trimmed == strings.TrimSpace(openCodePrefix) {
		return "", false
	}

	name, framed := strings.CutPrefix(trimmed, openCodePrefix)
	if !framed {
		return title, true
	}

	name = strings.TrimSpace(name)

	return name, name != ""
}
