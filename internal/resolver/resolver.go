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

// PaneResolver names one pane rather than a tab. Herdr's goto panel lists a
// pane under its tab, so a pane is named for what tells it from that tab.
type PaneResolver interface {
	// ResolvePane names one pane of tab, or returns an empty decision when the
	// pane has nothing to say that its tab does not. The pane is never nil.
	ResolvePane(pane *state.PaneState, tab state.TabState) Decision
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
	return d.name(state.SelectContextPane(tab), tab.WorkspaceName)
}

// ResolvePane names one pane of a tab by what tells it from that tab: the
// panes of a tab share a directory and an agent, and the goto panel puts the
// pane's row under the tab's, so repeating either says nothing twice over.
func (d *Deterministic) ResolvePane(pane *state.PaneState, tab state.TabState) Decision {
	found := d.collect(pane)

	parts := found.parts
	if d.hideAgentName {
		parts.Agent = ""
	}

	parts = withoutRepetition(parts, tab.WorkspaceName)

	// What tells this pane from its tab is the best name it can have. A pane
	// left with nothing still gets one — Herdr would list it as the agent in
	// it, which is the same word on every row and the reason this exists.
	if name := Format(withoutTheTabs(parts, d.tabParts(tab)), d.maxLength); name != "" {
		return Decision{Name: name, Confidence: found.confidence, Reason: found.reason}
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

// tabParts is what the tab was built from, before the parts that only repeat
// the workspace were dropped: a pane must not say again what the tab dropped
// for want of width, because the workspace above them both still says it.
func (d *Deterministic) tabParts(tab state.TabState) Parts {
	parts := d.collect(state.SelectContextPane(tab)).parts
	if d.hideAgentName {
		parts.Agent = ""
	}

	return parts
}

// withoutTheTabs drops the parts of a pane's title that its tab already carries.
// Where the pane is stays on the tab's row above; what it is doing is the whole
// of what a pane row is for, so the activity is kept whatever the tab says.
func withoutTheTabs(parts, tab Parts) Parts {
	if strings.EqualFold(parts.Context, tab.Context) {
		parts.Context = ""
	}

	if strings.EqualFold(parts.Branch, tab.Branch) {
		parts.Branch = ""
	}

	if strings.EqualFold(parts.Agent, tab.Agent) {
		parts.Agent = ""
	}

	return parts
}

// name is the resolution both entry points share, given the pane to read and
// the surrounding label a title must not merely repeat.
func (d *Deterministic) name(pane *state.PaneState, context string) Decision {
	found := d.collect(pane)

	parts := found.parts
	if d.hideAgentName {
		// Dropped before the repetition check, so a tab left with nothing but
		// its directory keeps it rather than losing it to a name it will not
		// show.
		parts.Agent = ""
	}

	parts = withoutRepetition(parts, context)

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

// withoutRepetition drops the parts of a title that only say again what the
// reader can already see.
func withoutRepetition(parts Parts, workspace string) Parts {
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

	// Herdr shows the workspace above its tabs, so repeating it wastes half the
	// width. Dropped only when something else remains: a branch counts, and so
	// does an agent's name, which by here is gone if it is not to be shown.
	if (parts.Activity != "" || parts.Branch != "" || parts.Agent != "") &&
		strings.EqualFold(parts.Context, workspace) {
		parts.Context = ""
	}

	return parts
}
