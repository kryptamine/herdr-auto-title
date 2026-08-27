package resolver

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// DefaultBranchMaxLength bounds what a branch adds to a title, in columns of
// the tab bar. Twelve holds a tracker key, or a word and part of the next —
// see docs/architecture/title-resolution.md.
const DefaultBranchMaxLength = 12

// trackerKey matches an issue key such as MC-13675. Two to six letters and at
// least two digits keeps it clear of hyphenated words and of fragments like
// `utf-8`.
var trackerKey = regexp.MustCompile(`(?i)\b[a-z]{2,6}-\d{2,6}\b`)

// branchSeparators are the characters branch names are built out of. Cutting a
// long branch at one of them ends it on a whole word.
const branchSeparators = "-_./ "

// Git names the branch the pane's repository has checked out. It qualifies the
// context rather than the activity, and why that matters is in
// docs/architecture/title-resolution.md.
type Git struct {
	maxLength int
}

var _ Source = Git{}

// NewGit builds the source. A maxLength of zero or less leaves branches out of
// titles entirely.
func NewGit(maxLength int) Git { return Git{maxLength: maxLength} }

func (Git) Name() string    { return "git" }
func (Git) Confidence() int { return ConfidenceGit }

func (g Git) Resolve(pane *state.PaneState) (Parts, bool) {
	if g.maxLength <= 0 {
		return Parts{}, false
	}

	// The branch is read from the directory ssh was launched in, which says
	// nothing about the machine the tab is showing.
	if _, remote := sshArgs(pane); remote {
		return Parts{}, false
	}

	branch := g.label(pane.Git)
	if branch == "" {
		return Parts{}, false
	}

	return Parts{Branch: branch}, true
}

// label reduces a checkout to what a tab says about it, or "" when it says
// nothing worth the width.
func (g Git) label(checkout git.Checkout) string {
	if checkout.Branch == "" {
		// A detached HEAD is the one place a bare hash belongs in a title: it
		// is where commits get lost, and no name is being left behind.
		return Sanitize(checkout.Commit, 0)
	}
	// Compared exactly, because git refs are: a branch named `Main` beside a
	// `main` trunk is a different branch and has something to say.
	if checkout.Branch == checkout.Default {
		return ""
	}

	return shortenBranch(Sanitize(checkout.Branch, 0), g.maxLength)
}

// shortenBranch reduces an over-long branch name to the part worth a tab's
// width. The rules, and why none of them is a list of known prefixes, are in
// docs/architecture/title-resolution.md.
func shortenBranch(branch string, maxLength int) string {
	branch = strings.Trim(branch, branchSeparators)
	if branch == "" || maxLength <= 0 {
		return ""
	}

	if _, over := splitAtWidth(branch, maxLength); over == "" {
		return branch
	}

	// A key is atomic: cutting it leaves something that identifies nothing, so
	// it is the one value allowed past maxLength.
	if key := trackerKey.FindString(branch); key != "" {
		return strings.ToUpper(key)
	}

	if cut := strings.LastIndex(branch, "/"); cut >= 0 && cut+1 < len(branch) {
		branch = branch[cut+1:]
	}

	return cutAtSeparator(branch, maxLength)
}

// cutAtSeparator shortens a value to maxWidth columns, ending on the last
// separator that fits so the result is a whole word rather than a fragment.
func cutAtSeparator(value string, maxWidth int) string {
	head, rest := splitAtWidth(value, maxWidth)
	if rest == "" {
		return strings.Trim(value, branchSeparators)
	}

	// When the character that did not fit is itself a separator, the head
	// already ends on a whole word and cutting again would throw one away.
	next, _ := utf8.DecodeRuneInString(rest)
	if !strings.ContainsRune(branchSeparators, next) {
		if cut := strings.LastIndexAny(head, branchSeparators); cut > 0 {
			head = head[:cut]
		}
	}

	return strings.Trim(head, branchSeparators)
}
