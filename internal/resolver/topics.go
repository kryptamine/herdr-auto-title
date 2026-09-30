package resolver

import (
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// Topics says what a pane is doing, for the workspace row above its tab. Its
// bound is its own: zero leaves a topic whole, where New would impose the tab
// bar's default.
type Topics struct {
	chain  *Deterministic
	maxLen int
}

func NewTopics(opts Options, maxLen int) *Topics {
	return &Topics{chain: topicChain(opts), maxLen: maxLen}
}

// topicChain is the shipped chain without the foreground process, and the
// terminal title read without it too. Why a topic must not follow the process
// is in title-resolution.md.
func topicChain(opts Options) *Deterministic {
	return New(opts,
		NewAgent(),
		NewUnboundTerminalTitle(),
		NewTranscript(),
		NewSSH(),
		NewGit(opts.BranchMax),
		NewCWD(opts.Home),
	)
}

// Topic is the agent and activity of a pane, or "" when there is nothing to say.
// The label names the project and Herdr draws the branch, so neither is repeated;
// an ssh session's host is the task rather than the project, so it leads.
func (t *Topics) Topic(pane *state.PaneState) string {
	if pane == nil {
		return ""
	}

	parts := withoutRepetition(t.chain.collect(pane).parts)

	topic := Parts{Agent: parts.Agent, Activity: parts.Activity}
	if parts.Activity == "" {
		// An agent's name alone says only that it is there.
		topic.Agent = ""
	}

	if _, remote := sshArgs(pane); remote {
		topic.Context = parts.Context
	}

	return Format(topic, t.maxLen)
}
