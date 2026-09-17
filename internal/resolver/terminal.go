package resolver

import "github.com/kryptamine/herdr-auto-title/internal/state"

// TerminalTitle derives the activity from the pane's terminal title: a title a
// program went out of its way to set usually says what is happening. A lone
// program in the pane qualifies it, `nvim › auth.provider.ts`.
type TerminalTitle struct {
	// unbound reads the title without the program that set it, for a chain
	// that leaves the foreground process out and must not learn it from here.
	unbound bool
}

var _ Source = TerminalTitle{}

func NewTerminalTitle() TerminalTitle { return TerminalTitle{} }

// NewUnboundTerminalTitle reads the title as it is, `auth.ts` rather than
// `nvim › auth.ts`. Places builds on it, for the reason given there.
func NewUnboundTerminalTitle() TerminalTitle { return TerminalTitle{unbound: true} }

func (TerminalTitle) Name() string    { return "terminal_title" }
func (TerminalTitle) Confidence() int { return ConfidenceTerminalTitle }

func (s TerminalTitle) Resolve(pane *state.PaneState) (Parts, bool) {
	// Herdr strips escapes and decorative prefixes for us; the raw field is
	// only a fallback for when it has not.
	title := pane.TerminalTitle
	if title == "" {
		title = pane.TerminalTitleRaw
	}

	// A shell titles its window with the command it runs, so until the remote
	// shell sets a title this one only repeats the ssh the context already names.
	if echoesSSHCommand(pane, title) {
		return Parts{}, false
	}

	// An agent is a field of its own rather than a bound kind, so it stays.
	if s.unbound && !pane.HasAgent() {
		return activityAs(pane, title, "")
	}

	return activityFrom(pane, title)
}
