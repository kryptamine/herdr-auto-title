// Package resolver turns a tab's read state into a tab title. Resolution is
// deterministic: identical state always yields an identical decision. No
// network call and no LLM: every source names a tab from state already read.
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
// "<context> › <branch> › <activity>". A source may supply any of them.
type Parts struct {
	Context string
	// Branch qualifies the context rather than standing on its own: a branch
	// is part of where the user is, not of what they are doing.
	Branch   string
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

	return Parts{Activity: qualify(activity, paneKind(pane))}, true
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
	// carries nothing this source recognizes. The pane is never nil: a tab
	// with no panes is declined by collect, before any source is asked.
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

// Deterministic resolves titles from a fixed priority list of sources.
type Deterministic struct {
	sources   []Source
	maxLength int
}

var _ TitleResolver = (*Deterministic)(nil)

// New builds a resolver from sources, ordering them by confidence rather than
// by the order they are listed in. Equal confidences keep the order given.
func New(maxLength int, sources ...Source) *Deterministic {
	if maxLength <= 0 {
		maxLength = DefaultMaxLength
	}

	ordered := slices.Clone(sources)
	slices.SortStableFunc(ordered, func(a, b Source) int {
		return cmp.Compare(b.Confidence(), a.Confidence())
	})

	return &Deterministic{sources: ordered, maxLength: maxLength}
}

// Default builds the chain Auto Title ships with, so nothing else has to list
// what it contains.
func Default(maxLength, branchMax int) *Deterministic {
	return New(maxLength,
		NewAgent(),
		NewTerminalTitle(),
		NewTranscript(),
		NewProcess(),
		NewSSH(),
		NewGit(branchMax),
		NewCWD(),
	)
}

// Resolve names a tab in three steps: ask the sources what they see, drop the
// parts that only repeat something already on screen, and assemble the rest.
func (d *Deterministic) Resolve(tab state.TabState) Decision {
	found := d.collect(state.SelectContextPane(tab))
	found.parts = withoutRepetition(found.parts, tab.WorkspaceName)

	name := Format(found.parts, d.maxLength)
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
	// title whenever one turns up. A context and a branch are credited only
	// while no activity has been found.
	if c.parts.Activity == "" && parts.Activity != "" {
		c.parts.Activity = parts.Activity
		c.credit(source)
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

// complete stops the walk once both halves of a title are answered. The branch
// is not required: a tab outside a repository has none, and waiting for one
// would only walk sources that have already been outranked.
func (c *collected) complete() bool {
	return c.parts.Context != "" && c.parts.Activity != ""
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

	// Herdr shows the workspace above its tabs, so repeating it wastes half the
	// width. Dropped only when something else remains — the branch counts,
	// which is what keeps `feat/oauth › nvim` from losing where it is.
	if (parts.Activity != "" || parts.Branch != "") && strings.EqualFold(parts.Context, workspace) {
		parts.Context = ""
	}

	return parts
}
