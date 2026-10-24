package codex

import (
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/kryptamine/herdr-auto-title/internal/transcript"
)

// join joins the items of Codex's title, which by default reads
// `<activity> <thread name> | <project>` and drops the name until it has one.
const join = " | "

// attention are the two frames of the prefix Codex blinks in front of its
// title while it waits on the user.
var attention = []string{"[ ! ] Action Required", "[ . ] Action Required"}

// cutMark ends a thread name the title cut: Codex keeps 45 characters of a
// name past 48 and appends it, and shows a shorter name whole.
const cutMark = "..."

// ThreadName reports the thread name a Codex title carries, and false when it
// carries none. See docs/architecture/title-resolution.md for the layout this
// reads.
func ThreadName(title string) (string, bool) {
	// Herdr strips the spinner it recognizes, but not the voice mark before it.
	rest := strings.TrimLeftFunc(title, func(r rune) bool {
		return r == '●' || unicode.Is(unicode.Braille, r) || unicode.IsSpace(r)
	})

	for _, prefix := range attention {
		if after, found := strings.CutPrefix(rest, prefix); found {
			rest = strings.TrimPrefix(after, join)

			break
		}
	}

	// The project always comes last, and alone it is the whole title.
	cut := strings.LastIndex(rest, join)
	if cut < 0 {
		return "", false
	}

	rest = strings.TrimSpace(rest[:cut])

	return rest, rest != ""
}

// shows reports whether a terminal title can be the session topic was read
// from. Herdr learns of a session at its first prompt, so after /new or /resume
// the title has moved on while the session Herdr reports has not.
func shows(title string, topic transcript.Topic) bool {
	if title == "" {
		return true
	}

	name, named := ThreadName(title)
	if !named {
		return topic.Name == ""
	}

	// One session's whole name can start another's, so only a name the title
	// cut is matched by its start.
	if start, cut := strings.CutSuffix(name, cutMark); cut {
		return topic.Name != "" && strings.HasPrefix(topic.Name, start)
	}

	return topic.Name == name
}

// titleConfig is what config.toml says of the terminal title, read again only
// when the file changes.
type titleConfig struct {
	path   string
	stamp  time.Time
	size   int64
	custom bool
}

// customized reports whether config.toml sets tui.terminal_title, in which case
// the title says nothing ThreadName can read.
func (c *titleConfig) customized() bool {
	info, err := os.Stat(c.path)
	if err != nil {
		*c = titleConfig{path: c.path}

		return false
	}

	if info.ModTime().Equal(c.stamp) && info.Size() == c.size {
		return c.custom
	}

	content, err := os.ReadFile(c.path)
	if err != nil {
		return c.custom
	}

	c.stamp, c.size = info.ModTime(), info.Size()
	c.custom = setsTerminalTitle(string(content))

	return c.custom
}

// setsTerminalTitle scans rather than parses: any line assigning a key that
// names terminal_title counts, since a false match only skips a check while a
// missed one drops every session's name.
func setsTerminalTitle(config string) bool {
	for line := range strings.SplitSeq(config, "\n") {
		line, _, _ = strings.Cut(line, "#")
		if strings.Contains(line, "terminal_title") && strings.Contains(line, "=") {
			return true
		}
	}

	return false
}
