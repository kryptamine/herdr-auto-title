package resolver

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// dshAgent is the agent Herdr reports for a DeepSeek Harness pane.
const dshAgent = "dsh-tui"

// dshMark follows the state in the title DeepSeek Harness sets,
// `<state> 🐋 <title>`, where the state is dshIdle or a braille spinner.
const (
	dshMark = "🐋"
	dshIdle = "✦"
)

// withoutDshFrame reports the session title in a DeepSeek Harness pane's
// title, and false for the frame alone. Herdr strips only the working braille
// spinner, so the frame changes with the state and names an untitled pane `✦ 🐋`.
func withoutDshFrame(title string) (string, bool) {
	frame := strings.TrimSpace(title)
	if state, rest, ok := strings.Cut(frame, " "); ok && isDshState(state) {
		frame = strings.TrimSpace(rest)
	}

	if frame != dshMark && !strings.HasPrefix(frame, dshMark+" ") {
		return title, true
	}

	name := strings.TrimSpace(strings.TrimPrefix(frame, dshMark))

	return name, name != ""
}

func isDshState(s string) bool {
	r, size := utf8.DecodeRuneInString(s)

	return s == dshIdle || (size == len(s) && unicode.Is(unicode.Braille, r))
}
