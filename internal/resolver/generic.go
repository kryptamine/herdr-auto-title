package resolver

import (
	"regexp"
	"strings"
)

// genericValues name a program or a place rather than what the user is doing
// there, so a source that finds one declines. Shells are not repeated here:
// shellNames holds them, and two lists would disagree. Keys are lower-cased.
var genericValues = map[string]struct{}{
	// Terminals and runtimes.
	"shell":    {},
	"terminal": {},
	"node":     {},
	// Agents naming themselves instead of their work.
	"claude":       {},
	"claude code":  {},
	"agent":        {},
	"coding agent": {},
}

// isGeneric reports whether a lower-cased value names something rather than
// says what is being done with it.
func isGeneric(lowered string) bool {
	if _, generic := genericValues[lowered]; generic {
		return true
	}

	_, shell := shellNames[lowered]

	return shell
}

// uriPattern matches a scheme-qualified location such as oil:///home/dev.
var uriPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

// promptPattern matches the title a shell sets from its own prompt:
// `root@psi:`, `alex@macbook:~/work`. It says who and where, which the context
// already says, and never what the user is doing.
var promptPattern = regexp.MustCompile(`^[^\s@]+@[^\s@:]+:\S*$`)

// fallbackTitlePattern matches the title Herdr gives a Windows pane whose
// program has set none: `pwsh in dashboard`, which names the shell and where
// it is, and the context already says where.
var fallbackTitlePattern = regexp.MustCompile(`^(\S+) in .+$`)

// punctuation wraps and joins words inside titles such as
// `Makefile (~/work/dashboard) - Nvim`.
const punctuation = `()[]{}<>"'` + ",;:-–—|"

// Meaningful cleans an untrusted value for use in a title and reports whether
// anything useful survived: `auth.ts (~/work/src) - Nvim` keeps `auth.ts -
// Nvim`, while a bare `~` leaves nothing.
func Meaningful(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if isFallbackTitle(trimmed) {
		return "", false
	}

	cleaned := stripLocations(trimmed)
	if cleaned == "" {
		return "", false
	}

	if isGeneric(strings.ToLower(cleaned)) {
		return "", false
	}

	if promptPattern.MatchString(cleaned) {
		return "", false
	}

	return cleaned, true
}

// stripLocations removes every word that is an absolute path, a home-anchored
// path or a URI, then tidies the punctuation left behind. Relative paths
// survive: `Fix bug in src/auth.ts` describes work rather than a place.
func stripLocations(value string) string {
	words := strings.Fields(value)

	kept := make([]string, 0, len(words))
	for _, word := range words {
		if isLocation(strings.Trim(word, punctuation)) {
			continue
		}

		kept = append(kept, word)
	}

	return tidy(kept)
}

// isFallbackTitle reports a title that only says which shell sits where. The
// check runs before locations are stripped, because the place can be one.
func isFallbackTitle(value string) bool {
	match := fallbackTitlePattern.FindStringSubmatch(value)

	return match != nil && isGeneric(strings.ToLower(match[1]))
}

func isLocation(word string) bool {
	switch {
	case word == "~", strings.HasPrefix(word, "~/"), strings.HasPrefix(word, `~\`):
		return true
	case strings.HasPrefix(word, "/"), strings.HasPrefix(word, `\\`):
		return true
	case isDrivePath(word):
		return true
	default:
		return uriPattern.MatchString(word)
	}
}

// isDrivePath reports a path rooted on a Windows drive, `C:\work` or `C:/work`.
func isDrivePath(word string) bool {
	if len(word) < 3 || word[1] != ':' || (word[2] != '\\' && word[2] != '/') {
		return false
	}

	letter := word[0] | 0x20

	return letter >= 'a' && letter <= 'z'
}

// tidy joins words back together, dropping the punctuation that only made sense
// around a word that has been removed: `- (oil:///work) - Nvim` loses its path
// and must not become `- - Nvim`.
func tidy(words []string) string {
	kept := make([]string, 0, len(words))
	for _, word := range words {
		if !isPunctuationOnly(word) {
			kept = append(kept, word)
			continue
		}
		// A separator is only worth keeping between two real words.
		if len(kept) == 0 || isPunctuationOnly(kept[len(kept)-1]) {
			continue
		}

		kept = append(kept, word)
	}

	for len(kept) > 0 && isPunctuationOnly(kept[len(kept)-1]) {
		kept = kept[:len(kept)-1]
	}

	return strings.Join(kept, " ")
}

func isPunctuationOnly(word string) bool {
	return strings.Trim(word, punctuation) == ""
}
