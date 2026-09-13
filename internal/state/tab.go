// Package state turns a Herdr session snapshot into the shape the resolver
// names tabs from. Each poll builds what it needs and throws it away again.
package state

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// PaneState is one pane's context as Herdr reported it when it was last read.
type PaneState struct {
	ID string
	// CurrentName is the label the pane carries, empty while nobody has named
	// it. Herdr falls back to the agent's name for display and stores nothing.
	CurrentName string

	// Dir is the directory this pane speaks for: its foreground process's own
	// once a poll has read it, and the snapshot's guess until then.
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

// PaneDir is the directory a pane speaks for, settled by what it is running:
// the list is deepest first, so the pane's own foreground process is last and
// the directory it is in is the pane's. A read saying nothing leaves guess.
func PaneDir(processes []herdr.PaneProcessInfoProcess, guess string) string {
	if last := len(processes) - 1; last >= 0 && processes[last].CWD != "" {
		return cleanDir(processes[last].CWD)
	}

	return guess
}

// snapshotDir is the directory a snapshot guesses a pane speaks for. A subshell
// leaves cwd behind in the directory it was started from, so the foreground one
// is preferred — but it is a descendant's, and only PaneDir is exact.
func snapshotDir(info herdr.PaneInfo) string {
	if info.ForegroundCWD != "" {
		return cleanDir(info.ForegroundCWD)
	}

	return cleanDir(info.CWD)
}

// cleanDir is a reported directory as filepath.Clean spells it, the trailing
// separator Windows adds gone. "" stays "": a pane without a directory is real,
// and Clean would spell it ".".
func cleanDir(dir string) string {
	if dir == "" {
		return ""
	}

	return filepath.Clean(dir)
}

// PaneFrom builds pane context from a snapshot entry and when a poll last saw
// the pane's revision move. What the snapshot cannot say is filled in by Read,
// and for most panes never is.
func PaneFrom(info herdr.PaneInfo, changedAt time.Time) *PaneState {
	return &PaneState{
		ID:               info.PaneID,
		CurrentName:      info.Label,
		Dir:              snapshotDir(info),
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

// ProcessesFrom is a process read as a pane holds it: the name and the whole
// argument vector. The directory a process is in is read by PaneDir and not
// carried.
func ProcessesFrom(processes []herdr.PaneProcessInfoProcess) []Process {
	if len(processes) == 0 {
		return nil
	}

	out := make([]Process, 0, len(processes))
	for _, p := range processes {
		out = append(out, Process{Name: programName(p.Name), Args: p.Argv})
	}

	return out
}

// exeSuffix is what Windows spells a program name with, and it says nothing
// about the program: `pwsh.exe` is the shell every other platform calls pwsh.
const exeSuffix = ".exe"

func programName(name string) string {
	if len(name) > len(exeSuffix) && strings.EqualFold(name[len(name)-len(exeSuffix):], exeSuffix) {
		return name[:len(name)-len(exeSuffix)]
	}

	return name
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
	// Position is the tab's place in its workspace, counted from one: the key
	// that switches to it, and the label Herdr gives it while it is unnamed.
	// Not TabInfo.number — see docs/architecture/herdr-socket-api.md.
	Position int
	// Panes are ordered by ID, which is what makes every traversal of them
	// yield the same answer from the same session.
	Panes []*PaneState
	// Context is the pane the tab is named after, nil only when it has no
	// panes. Picked once here, so what is read and what is named cannot differ.
	Context *PaneState
}

// TabFrom builds a tab and picks its context pane. With preferAgent a pane
// running an agent, in any state, outranks focus.
func TabFrom(
	info herdr.TabInfo,
	workspaceName string,
	position int,
	panes []*PaneState,
	preferAgent bool,
) TabState {
	ordered := slices.Clone(panes)
	slices.SortFunc(ordered, func(a, b *PaneState) int { return strings.Compare(a.ID, b.ID) })

	rules := contextRules
	if preferAgent {
		rules = agentFirstRules
	}

	return TabState{
		ID:            info.TabID,
		CurrentName:   info.Label,
		WorkspaceName: workspaceName,
		Position:      position,
		Panes:         ordered,
		Context:       contextPane(ordered, rules),
	}
}

// paneRule is a condition a pane must meet to name its tab.
type paneRule func(*PaneState) bool

func focused(p *PaneState) bool { return p.Focused }

func anyPane(*PaneState) bool { return true }

// contextRules rank the panes a tab may be named after: the focused one, then
// one running an active agent, then any.
var (
	contextRules    = []paneRule{focused, (*PaneState).AgentIsActive, anyPane}
	agentFirstRules = append([]paneRule{(*PaneState).HasAgent}, contextRules...)
)

// contextPane is the last-changed pane of the first rule any pane passes.
func contextPane(panes []*PaneState, rules []paneRule) *PaneState {
	for _, keep := range rules {
		if pane := mostRecent(panes, keep); pane != nil {
			return pane
		}
	}

	return nil
}

// mostRecent returns the last-changed pane keep accepts, or nil when it
// accepts none. Panes arrive ordered by ID, and the strict comparison keeps
// the lowest of them when timestamps tie.
func mostRecent(panes []*PaneState, keep paneRule) *PaneState {
	var best *PaneState

	for _, p := range panes {
		if !keep(p) {
			continue
		}

		if best == nil || p.ChangedAt.After(best.ChangedAt) {
			best = p
		}
	}

	return best
}
