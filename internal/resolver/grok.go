package resolver

import (
	"strings"
	"unicode"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// grokSuffix ends every title Grok Build sets: `<name> - grok` idle, and
// `<spinner> - <status> - <name> - grok` while it works.
const grokSuffix = " - grok"

// withoutGrokFrame reports the session name in a Grok pane's title, without
// the spinner and status a working Grok puts in front of it. A title in any
// other shape is declined rather than read as a name.
func withoutGrokFrame(pane *state.PaneState, title string) (string, bool) {
	rest, framed := strings.CutSuffix(strings.TrimSpace(title), grokSuffix)
	if !framed {
		return "", false
	}

	// Herdr's stripped title drops the braille spinner and keeps the dash.
	rest = strings.TrimLeftFunc(rest, func(r rune) bool {
		return unicode.Is(unicode.Braille, r) || unicode.IsSpace(r)
	})
	if status, working := strings.CutPrefix(rest, "- "); working {
		if _, rest, working = strings.Cut(status, " - "); !working {
			return "", false
		}
	}

	// Grok cuts a long name with an ellipsis; a summary that read the whole
	// of the same name names the pane better, one rung down.
	if cut, short := strings.CutSuffix(
		rest,
		"…",
	); short && strings.HasPrefix(pane.AgentTopic, cut) &&
		pane.AgentTopic != cut {
		return "", false
	}

	return rest, rest != ""
}
