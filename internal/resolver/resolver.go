// Package resolver turns a pane's read state into a title, for the tab that
// pane speaks for or for the pane itself. Resolution is deterministic: no
// network call and no LLM, and identical state yields an identical decision.
package resolver

import (
	"cmp"
	"slices"
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// DefaultMaxLength bounds a generated title, in columns of the tab bar.
const DefaultMaxLength = 50

// GenericFallback names a tab whose context tells us nothing.
const GenericFallback = "Shell"

// Confidence levels form the resolution ladder, and the resolver orders itself
// by them. A source never overrides a field a higher one already supplied. The
// gaps are what make room for the next source.
const (
	ConfidenceFallback      = 10
	ConfidenceCWD           = 30
	ConfidenceGit           = 40
	ConfidenceSSH           = 60
	ConfidenceProcess       = 70
	ConfidenceTranscript    = 75
	ConfidenceTerminalTitle = 80
	ConfidenceAgent         = 90
)

// Parts are the components a source contributes to a title, formatted as
// "<context> › <branch> › <agent> › <activity>". A source may supply any of
// them.
type Parts struct {
	Context string
	// Branch qualifies the context rather than standing on its own: a branch
	// is part of where the user is, not of what they are doing.
	Branch string
	// Agent names the agent running in the pane. It stands apart from the
	// activity because the user can turn it off, which is a decision the whole
	// title makes rather than the source that read it.
	Agent    string
	Activity string
}

// activityFrom turns an untrusted value into the activity of a title, bound to
// the kind of program the pane is running. No limit is applied: truncation
// belongs to the assembled name.
func activityFrom(pane *state.PaneState, value string) (Parts, bool) {
	activity, ok := Meaningful(Sanitize(value, 0))
	if !ok {
		return Parts{}, false
	}

	if echoesAgentName(pane, activity) {
		return Parts{}, false
	}

	return partsFrom(pane, paneKind(pane), activity), true
}

// partsFrom places a pane's kind: an agent's name is a field of its own, any
// other kind qualifies the activity. An agent's activity is stripped of the
// name whether or not it is shown, so it cannot come back as text.
func partsFrom(pane *state.PaneState, kind, activity string) Parts {
	if pane.HasAgent() {
		return Parts{Agent: kind, Activity: stripKind(activity, kind)}
	}

	return Parts{Activity: qualify(activity, kind)}
}

// echoesAgentName reports an activity that is no more than the agent's own
// name. That is as generic as anything in genericValues, but the name differs
// per agent, so it is compared against the pane instead of being listed.
func echoesAgentName(pane *state.PaneState, activity string) bool {
	return strings.EqualFold(activity, pane.Agent) ||
		strings.EqualFold(activity, pane.DisplayAgent)
}

// Source contributes title parts from a pane's context.
type Source interface {
	// Name identifies the source in the rename reason.
	Name() string
	// Confidence is the source's place on the resolution ladder. It belongs to
	// the source rather than to each result it returns: a source is trusted for
	// what it reads, not for what it happened to find this time.
	Confidence() int
	// Resolve reports the parts this source derives, or false when the pane
	// carries nothing this source recognizes. The pane is never nil.
	Resolve(pane *state.PaneState) (Parts, bool)
}

type Decision struct {
	Name       string
	Confidence int
	Reason     string
}

type TitleResolver interface {
	Resolve(tab state.TabState) Decision
}

// PaneResolver names the panes of a tab rather than the tab. Herdr's goto panel
// lists a pane under its tab, so a pane is named for what tells it from that tab.
type PaneResolver interface {
	// ResolvePanes names every pane of tab, in the order tab.Panes holds them.
	// tab.Context is read from too, so it must be filled first.
	ResolvePanes(tab state.TabState) []Decision
}

// Options are the settings a title is assembled under, as opposed to the ones
// a single source reads.
type Options struct {
	MaxLength int
	BranchMax int
	// HideAgentName leaves the agent's name out of a title. It is stated this
	// way round so that the zero value keeps the name, which is what a resolver
	// built without options wants.
	HideAgentName bool
}

// Deterministic resolves titles from a fixed priority list of sources.
type Deterministic struct {
	sources       []Source
	maxLength     int
	hideAgentName bool
}

var (
	_ TitleResolver = (*Deterministic)(nil)
	_ PaneResolver  = (*Deterministic)(nil)
)

// New builds a resolver from sources, ordering them by confidence rather than
// by the order they are listed in. Equal confidences keep the order given.
func New(opts Options, sources ...Source) *Deterministic {
	if opts.MaxLength <= 0 {
		opts.MaxLength = DefaultMaxLength
	}

	ordered := slices.Clone(sources)
	slices.SortStableFunc(ordered, func(a, b Source) int {
		return cmp.Compare(b.Confidence(), a.Confidence())
	})

	return &Deterministic{
		sources:       ordered,
		maxLength:     opts.MaxLength,
		hideAgentName: opts.HideAgentName,
	}
}

// Default builds the chain Auto Title ships with, so nothing else has to list
// what it contains.
func Default(opts Options) *Deterministic {
	return New(opts,
		NewAgent(),
		NewTerminalTitle(),
		NewTranscript(),
		NewProcess(),
		NewSSH(),
		NewGit(opts.BranchMax),
		NewCWD(),
	)
}

// Resolve names a tab in three steps: ask the sources what they see, drop the
// parts that only repeat something already on screen, and assemble the rest.
func (d *Deterministic) Resolve(tab state.TabState) Decision {
	return d.name(d.collect(tab.Context), Parts{Context: tab.WorkspaceName})
}

// ResolvePanes names each pane of a tab by what tells it from that tab. The row
// above a pane is the tab's parts, not its finished title, for the reason in
// docs/architecture/title-resolution.md.
func (d *Deterministic) ResolvePanes(tab state.TabState) []Decision {
	workspace := Parts{Context: tab.WorkspaceName}
	above := d.collect(tab.Context).parts

	decisions := make([]Decision, len(tab.Panes))
	for i, pane := range tab.Panes {
		decisions[i] = d.name(d.collect(pane), workspace, above)
	}

	return decisions
}

// name assembles what the chain found into a title, dropping in turn what each
// row shown above it already carries.
func (d *Deterministic) name(found collected, rows ...Parts) Decision {
	parts := withoutRepetition(found.parts)
	for _, row := range rows {
		parts = withoutAbove(parts, row)
	}

	name := Format(parts, d.maxLength)
	if name == "" {
		return Decision{
			Name:       GenericFallback,
			Confidence: ConfidenceFallback,
			Reason:     "generic_fallback",
		}
	}

	return Decision{Name: name, Confidence: found.confidence, Reason: found.reason}
}

// collected is what the chain produced: the parts of a title, and the source
// that answers for it.
type collected struct {
	parts      Parts
	reason     string
	confidence int
}

// collect walks the sources in ladder order, filling each field with the first
// source that supplies it. The two are filled independently, so a low source
// can complete a title a higher one only half answered.
func (d *Deterministic) collect(pane *state.PaneState) collected {
	var found collected

	// A tab with no panes has nothing for any source to read, which is what
	// lets every one of them take a pane it can dereference.
	if pane == nil {
		return found
	}

	for _, source := range d.sources {
		parts, ok := source.Resolve(pane)
		if !ok {
			continue
		}

		found.take(source, parts)

		if found.complete() {
			break
		}
	}

	// Dropped before any repetition check, so a title left with nothing but its
	// directory keeps it rather than losing it to a name it will not show.
	if d.hideAgentName {
		found.parts.Agent = ""
	}

	return found
}

// take fills whatever this source supplies and nothing already has.
func (c *collected) take(source Source, parts Parts) {
	// The activity is what a title is about, so its source answers for the
	// title whenever one turns up. Every other part is credited only while
	// nothing has been.
	if c.parts.Activity == "" && parts.Activity != "" {
		c.parts.Activity = parts.Activity
		c.credit(source)
	}

	if c.parts.Agent == "" && parts.Agent != "" {
		c.parts.Agent = parts.Agent
		if c.reason == "" {
			c.credit(source)
		}
	}

	if c.parts.Branch == "" && parts.Branch != "" {
		c.parts.Branch = parts.Branch
		if c.reason == "" {
			c.credit(source)
		}
	}

	if c.parts.Context == "" && parts.Context != "" {
		c.parts.Context = parts.Context
		if c.reason == "" {
			c.credit(source)
		}
	}
}

func (c *collected) credit(source Source) {
	c.reason = source.Name()
	c.confidence = source.Confidence()
}

// complete stops the walk once both halves of a title are answered, an agent's
// name counting as the activity half. The branch is not required: a tab outside
// a repository has none, and waiting for one only walks outranked sources.
func (c *collected) complete() bool {
	return c.parts.Context != "" && (c.parts.Activity != "" || c.parts.Agent != "")
}

// withoutRepetition drops the parts of a title that only say again what another
// part of it says.
func withoutRepetition(parts Parts) Parts {
	// A shell that titles its window after its directory would otherwise
	// produce `dashboard › dashboard`.
	if strings.EqualFold(parts.Activity, parts.Context) {
		parts.Activity = ""
	}

	// A prompt that carries the branch in the window title would otherwise
	// produce `feat/oauth › feat/oauth`.
	if parts.Branch != "" && strings.EqualFold(parts.Activity, parts.Branch) {
		parts.Activity = ""
	}

	// A worktree is usually named after the branch checked out in it, so
	// `git worktree add ../feat-oauth feat-oauth` would otherwise produce
	// `feat-oauth › feat-oauth`. The directory leads, so the branch goes.
	if parts.Branch != "" && strings.EqualFold(parts.Branch, parts.Context) {
		parts.Branch = ""
	}

	return parts
}

// withoutAbove drops the parts a row shown above the title already carries: the
// workspace above a tab, the tab above a pane. The activity is what a row is
// for and always stays, and a title left with nothing keeps what it had.
func withoutAbove(parts, above Parts) Parts {
	kept := parts

	if strings.EqualFold(kept.Context, above.Context) {
		kept.Context = ""
	}

	if strings.EqualFold(kept.Branch, above.Branch) {
		kept.Branch = ""
	}

	if strings.EqualFold(kept.Agent, above.Agent) {
		kept.Agent = ""
	}

	if kept == (Parts{}) {
		return parts
	}

	return kept
}
