package resolver

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/rivo/uniseg"
)

// Separator joins the parts of a title, and every part shares it: where a part
// came from is not something a separator can convey. A tab's position is not a
// part of its title and carries a mark of its own.
const Separator = " " + separatorRune + " "

const separatorRune = "›"

// zeroWidthJoiner is the one format character a title may keep: dropping it
// would break the emoji clusters truncation goes to such lengths to preserve.
const zeroWidthJoiner = '\u200d'

// trimmable are the characters a title must not start or end on: a separator
// left dangling by truncation says a part was lost without saying which.
const trimmable = " " + separatorRune

var (
	// ansiRe matches CSI sequences, OSC strings and single-character escapes.
	ansiRe = regexp.MustCompile(
		"\x1b\\[[0-9;?<>=]*[ -/]*[@-~]" +
			"|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)" +
			"|\x1b[@-Z\\\\-_]",
	)
	spaceRe      = regexp.MustCompile(`\s+`)
	sepRunRe     = regexp.MustCompile(separatorRune + `(?:\s*` + separatorRune + `)+`)
	sepSpacingRe = regexp.MustCompile(`\s*` + separatorRune + `\s*`)
)

// Sanitize turns an untrusted value into something safe to use as a tab label:
// escapes and invisible characters go, whitespace and separators are
// normalized, and the result is cut to maxLen columns. Empty means unusable.
func Sanitize(s string, maxLen int) string {
	s = ansiRe.ReplaceAllString(s, "")

	s = strings.Map(func(r rune) rune {
		// Every kind of space, the non-breaking and the ideographic included,
		// so the run below collapses them all.
		if unicode.IsSpace(r) {
			return ' '
		}

		if unicode.IsControl(r) {
			return -1
		}
		// Format characters are invisible and forge what the reader sees: a
		// bidi override reverses the label, a zero-width space makes a second
		// tab read identically. The joiner holds emoji clusters together.
		if unicode.Is(unicode.Cf, r) && r != zeroWidthJoiner {
			return -1
		}

		return r
	}, s)

	s = spaceRe.ReplaceAllString(s, " ")
	s = sepRunRe.ReplaceAllString(s, separatorRune)
	s = sepSpacingRe.ReplaceAllString(s, Separator)
	s = strings.Trim(s, trimmable)
	s = strings.TrimSpace(s)

	return truncate(s, maxLen)
}

// truncate cuts to maxWidth columns and leaves no dangling separator behind.
func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}

	head, rest := splitAtWidth(s, maxWidth)
	if rest == "" {
		return s
	}

	cut := strings.TrimRight(head, trimmable)

	return strings.TrimSpace(cut)
}

// splitAtWidth returns the longest prefix of s fitting in maxWidth terminal
// columns, cut between grapheme clusters. Neither runes nor bytes work here —
// see docs/architecture/sanitization.md.
func splitAtWidth(s string, maxWidth int) (head, rest string) {
	rest = s

	state := -1
	for width := 0; rest != ""; {
		_, next, clusterWidth, nextState := uniseg.FirstGraphemeClusterInString(rest, state)
		if width+clusterWidth > maxWidth {
			break
		}

		width, rest, state = width+clusterWidth, next, nextState
	}

	return s[:len(s)-len(rest)], rest
}

// Format assembles a title from its parts and sanitizes the result as a whole.
// The order is the one a title reads in: from the general to the particular.
func Format(parts Parts, maxLen int) string {
	var b strings.Builder

	for _, part := range []string{parts.Context, parts.Branch, parts.Activity} {
		if part == "" {
			continue
		}

		if b.Len() > 0 {
			b.WriteString(Separator)
		}

		b.WriteString(part)
	}

	return Sanitize(b.String(), maxLen)
}
