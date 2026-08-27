package resolver

import "github.com/kryptamine/herdr-auto-title/internal/state"

// TerminalTitle derives the activity from the pane's terminal title: a title a
// program went out of its way to set usually says what is happening. A lone
// program in the pane qualifies it, `nvim › auth.provider.ts`.
type TerminalTitle struct{}

var _ Source = TerminalTitle{}

// NewTerminalTitle builds the source.
func NewTerminalTitle() TerminalTitle { return TerminalTitle{} }

func (TerminalTitle) Name() string    { return "terminal_title" }
func (TerminalTitle) Confidence() int { return ConfidenceTerminalTitle }

func (TerminalTitle) Resolve(pane *state.PaneState) (Parts, bool) {
	// Herdr strips escapes and decorative prefixes for us; the raw field is
	// only a fallback for when it has not.
	title := pane.TerminalTitle
	if title == "" {
		title = pane.TerminalTitleRaw
	}

	return activityFrom(pane, title)
}
