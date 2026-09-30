package resolver

import (
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// TopicResolver says what a pane is doing, for the workspace row above it.
type TopicResolver interface {
	// Topic answers under maxLen columns, zero leaving it whole. An empty name
	// means there is nothing to say, never the fallback a tab would take.
	Topic(pane *state.PaneState, maxLen int) Decision
}

var _ TopicResolver = (*Deterministic)(nil)

// Places builds the resolver a workspace row is described by: the shipped chain
// without the foreground process, and the terminal title read without it too.
// Why the row must not follow the process is in title-resolution.md.
func Places(opts Options) *Deterministic {
	return New(opts,
		NewAgent(),
		NewUnboundTerminalTitle(),
		NewTranscript(),
		NewSSH(),
		NewGit(opts.BranchMax),
		NewCWD(opts.Home),
	)
}

// Topic is the agent and activity of a pane: the label names the project and
// Herdr draws the branch, so neither is repeated. An ssh session's host is the
// task rather than the project, so it leads.
func (d *Deterministic) Topic(pane *state.PaneState, maxLen int) Decision {
	if pane == nil {
		return Decision{}
	}

	found := d.collect(pane)
	parts := withoutRepetition(found.parts)

	topic := Parts{Agent: parts.Agent, Activity: parts.Activity}
	if parts.Activity == "" {
		// An agent's name alone says only that it is there.
		topic.Agent = ""
	}

	if _, remote := sshArgs(pane); remote {
		topic.Context = parts.Context
	}

	name := Format(topic, maxLen)
	if name == "" {
		return Decision{}
	}

	return Decision{Name: name, Confidence: found.confidence, Reason: found.reason}
}
