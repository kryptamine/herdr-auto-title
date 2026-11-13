package resolver

import (
	"strings"

	"github.com/rivo/uniseg"
)

// dshAgent is the agent Herdr reports for a DeepSeek Harness pane.
const dshAgent = "dsh-tui"

// dshMark follows the state in the title DeepSeek Harness sets,
// `<state> 🐋 <title>`, where the state is `✦` idle or a braille spinner.
const dshMark = "🐋"

// withoutDshFrame reports the session title in a DeepSeek Harness pane's
// title, and false for the frame alone. Herdr strips only the working braille
// spinner, so the frame changes with the state and names an untitled pane `✦ 🐋`.
func withoutDshFrame(title string) (string, bool) {
	head, rest, _ := strings.Cut(strings.TrimSpace(title), " ")
	if head != dshMark {
		if uniseg.GraphemeClusterCount(head) != 1 {
			return title, true
		}

		head, rest, _ = strings.Cut(strings.TrimSpace(rest), " ")
		if head != dshMark {
			return title, true
		}
	}

	rest = strings.TrimSpace(rest)

	return rest, rest != ""
}
