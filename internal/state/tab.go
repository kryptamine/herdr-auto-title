// Package state turns a Herdr session snapshot into the shape the resolver
// names tabs from. Each poll builds what it needs and throws it away again.
package state

import (
	"slices"
	"strings"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// PaneState is one pane's context as Herdr reported it when it was last read.
type PaneState struct {
	ID string

	// Dir is the directory this pane speaks for, already chosen between the
	// shell's and the foreground process's.
	Dir string
	// TerminalTitle is Herdr's cleaned title; TerminalTitleRaw still carries
	// escapes and decorative prefixes and is only a fallback.
	TerminalTitle    string
	TerminalTitleRaw string

	// Agent is the agent Herdr recognized, empty when there is none.
	// AgentTitle is what it says it is working on, which many agents leave
	// empty and report through the terminal title instead.
	Agent        string
	DisplayAgent string
	AgentTitle   string
	AgentStatus  string
	// AgentTopic is what the agent's own session says it is about, read from
	// the transcript Herdr pointed at. Empty unless the agent's integration
	// hook is installed — see docs/architecture/title-resolution.md.
	AgentTopic string
	// AgentSession names the conversation that agent holds, which is how the
	// transcript behind AgentTopic is found. Nil until the agent's integration
	// reports one.
	AgentSession *herdr.AgentSessionInfo

	// Processes are the pane's foreground process and its descendants.
	Processes []Process

	// Git is what the repository holding the pane's directory has checked out,
	// zero outside a repository.
	Git git.Checkout

	Focused bool
	// ChangedAt is when a poll last saw this pane's revision advance.
	// Snapshots carry no timestamp, so it is the only ordering available.
	ChangedAt time.Time
}

// Process is one command running in a pane. Args is the whole argument vector,
// program name included, and may be empty.
type Process struct {
	Name string
	Args []string
}

// Reads is what a poll learned about a pane that its snapshot entry does not
// carry: each field costs a request or a file of its own, which is why only
// the pane that names its tab is read.
type Reads struct {
	Processes []herdr.PaneProcessInfoProcess
	Git       git.Checkout
	// Topic is what the agent's own session says it is about.
	Topic string
}

// PaneFrom builds pane context from a snapshot entry and when a poll last saw
// the pane's revision move. What the snapshot cannot say is filled in by Read,
// and for most panes never is.
func PaneFrom(info herdr.PaneInfo, changedAt time.Time) *PaneState {
	return &PaneState{
		ID:               info.PaneID,
		Dir:              info.Dir(),
		TerminalTitle:    info.TerminalTitleStripped,
		TerminalTitleRaw: info.TerminalTitle,
		Agent:            info.Agent,
		DisplayAgent:     info.DisplayAgent,
		AgentTitle:       info.Title,
		AgentStatus:      info.AgentStatus,
		AgentSession:     info.AgentSession,
		Focused:          info.Focused,
		ChangedAt:        changedAt,
	}
}

// Read fills in what a snapshot does not carry. It is a step of its own so a
// poll can leave it out: a tab is named from one pane, and every read behind
// this costs a request or a file — see docs/architecture/poll-loop.md.
func (p *PaneState) Read(reads Reads) {
	p.Processes = processesFrom(reads.Processes)
	p.Git = reads.Git
	p.AgentTopic = reads.Topic
}

func processesFrom(processes []herdr.PaneProcessInfoProcess) []Process {
	if len(processes) == 0 {
		return nil
	}

	out := make([]Process, 0, len(processes))
	for _, p := range processes {
		out = append(out, Process{Name: p.Name, Args: p.Argv})
	}

	return out
}

// HasAgent reports whether Herdr recognizes an agent in the pane.
func (p *PaneState) HasAgent() bool {
	return p != nil && p.Agent != ""
}

// AgentIsActive reports whether the pane's agent is running or waiting on the
// user. An idle or finished one is no more interesting than any other pane.
func (p *PaneState) AgentIsActive() bool {
	if !p.HasAgent() {
		return false
	}

	switch p.AgentStatus {
	case herdr.AgentStatusWorking, herdr.AgentStatusBlocked:
		return true
	default:
		return false
	}
}

// TabState is one tab as it was last read: its current label and its panes.
type TabState struct {
	ID string
	// CurrentName lets a poll skip a rename that would change nothing.
	CurrentName string
	// WorkspaceName is the label Herdr shows above this tab.
	WorkspaceName string
	// Position is the tab's place in its workspace, counted from one, which is
	// both the key that switches to it and the label Herdr gives it while it
	// is unnamed. Not TabInfo.number, which is a counter that never repeats —
	// see docs/architecture/herdr-socket-api.md.
	Position int
	// Panes are ordered by ID, which is what makes every traversal of them
	// yield the same answer from the same session.
	Panes []*PaneState
}

func TabFrom(info herdr.TabInfo, workspaceName string, position int, panes []*PaneState) TabState {
	ordered := slices.Clone(panes)
	slices.SortFunc(ordered, func(a, b *PaneState) int { return strings.Compare(a.ID, b.ID) })

	return TabState{
		ID:            info.TabID,
		CurrentName:   info.Label,
		WorkspaceName: workspaceName,
		Position:      position,
		Panes:         ordered,
	}
}

// SelectContextPane picks the one pane a title is built from: the focused one,
// then one running an active agent, then whichever changed last. Ties break on
// pane ID, so identical state always yields the same choice.
func SelectContextPane(tab TabState) *PaneState {
	panes := tab.Panes
	if len(panes) == 0 {
		return nil
	}

	for _, p := range panes {
		if p.Focused {
			return p
		}
	}

	// A split with an agent running is about that agent, whatever moved last.
	if agent := mostRecent(panes, (*PaneState).AgentIsActive); agent != nil {
		return agent
	}

	return mostRecent(panes, nil)
}

// mostRecent returns the last-changed pane of an ID-ordered slice that keep
// accepts, or nil when it accepts none. A nil keep takes every pane; the
// strict comparison keeps the lowest ID when timestamps tie.
func mostRecent(panes []*PaneState, keep func(*PaneState) bool) *PaneState {
	var best *PaneState

	for _, p := range panes {
		if keep != nil && !keep(p) {
			continue
		}

		if best == nil || p.ChangedAt.After(best.ChangedAt) {
			best = p
		}
	}

	return best
}
