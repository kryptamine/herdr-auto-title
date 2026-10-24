package herdr

import (
	"path/filepath"
	"regexp"
)

// Agent statuses a tab is named differently for. Herdr reports one more,
// unknown, which a pane without an agent carries and nothing here marks.
const (
	AgentStatusIdle    = "idle"
	AgentStatusWorking = "working"
	AgentStatusBlocked = "blocked"
	AgentStatusDone    = "done"
)

// WorkspaceInfo describes a workspace. Its label is what Herdr shows above the
// tabs, and it is usually the project every tab in it belongs to.
type WorkspaceInfo struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	// ActiveTabID is the tab the workspace shows. TabInfo carries a focused
	// flag too, but it marks only the one tab a client is looking at.
	ActiveTabID string `json:"active_tab_id"`
	// Tokens is each token's latest value, keyed by name alone whichever source
	// reported it, with neither source nor expiry.
	Tokens map[string]string `json:"tokens"`
}

// TabInfo describes a tab. Optional fields are decoded as plain strings, so a
// JSON null leaves them empty rather than failing.
type TabInfo struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	// AgentStatus is Herdr's own over the tab's panes, so a finished agent
	// outranks a working one — see docs/architecture/herdr-socket-api.md.
	AgentStatus string `json:"agent_status"`
}

// PaneInfo describes a pane. It carries no foreground process name — only
// pane.process_info answers that — and every optional field decodes null to "".
type PaneInfo struct {
	PaneID   string `json:"pane_id"`
	TabID    string `json:"tab_id"`
	Focused  bool   `json:"focused"`
	Revision uint64 `json:"revision"`

	// Label is the name the pane carries, which Herdr omits entirely while it
	// has none. Unlike a tab, a pane has one spelling for unnamed: the empty
	// string, because pane.rename clears rather than stores one.
	Label string `json:"label"`

	CWD                   string `json:"cwd"`
	ForegroundCWD         string `json:"foreground_cwd"`
	TerminalTitle         string `json:"terminal_title"`
	TerminalTitleStripped string `json:"terminal_title_stripped"`

	// Title is the agent's own title, not the terminal's. Herdr leaves it null
	// for agents that report their topic through the terminal title instead.
	Title        string `json:"title"`
	Agent        string `json:"agent"`
	DisplayAgent string `json:"display_agent"`
	AgentStatus  string `json:"agent_status"`

	// AgentSession identifies the conversation the pane's agent is holding.
	// It is null until the agent's Herdr integration reports one.
	AgentSession *AgentSessionInfo `json:"agent_session"`
}

// The reference kinds Herdr answers with, decided per agent: Claude Code's
// session arrives by id even when its hook reported the path too, and Pi's by
// the path to its session file — see docs/architecture/herdr-socket-api.md.
const (
	SessionRefID   = "id"
	SessionRefPath = "path"
)

// AgentSessionInfo identifies an agent's own session, as its integration hook
// reported it. Which agent reported it decides how the reference is read, so
// the label is part of the reference.
type AgentSessionInfo struct {
	Agent string `json:"agent"`
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// sessionIDPattern is the shape of a session id. A reader makes the id part of
// a path, so anything else is refused rather than cleaned.
var sessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

// ID reports the session id this reference holds, and false unless it names a
// session by an id shaped like a UUID. A nil reference is a pane whose agent
// has reported nothing, which is every pane until an integration is installed.
func (s *AgentSessionInfo) ID() (string, bool) {
	if s == nil || s.Kind != SessionRefID || !sessionIDPattern.MatchString(s.Value) {
		return "", false
	}

	return s.Value, true
}

// Path reports the session file this reference holds, and false unless it
// names a session by an absolute, already clean path. A reader opens it as it
// is, so anything else is refused rather than cleaned.
func (s *AgentSessionInfo) Path() (string, bool) {
	if s == nil || s.Kind != SessionRefPath || !filepath.IsAbs(s.Value) ||
		filepath.Clean(s.Value) != s.Value {
		return "", false
	}

	return s.Value, true
}

// PaneProcessInfoProcess is one process running in a pane. Herdr reports the
// joined command line too; a title is derived from the name, the arguments and
// the directory the process itself is in, and the pid finds the foreground one.
type PaneProcessInfoProcess struct {
	PID  int      `json:"pid"`
	Name string   `json:"name"`
	Argv []string `json:"argv"`
	CWD  string   `json:"cwd"`
}

// PaneProcessInfo is what pane.process_info answers. ForegroundProcesses holds
// the pane's foreground process and its descendants in an order Herdr does not
// keep the same everywhere; ForegroundProcessGroupID is the foreground one's pid.
type PaneProcessInfo struct {
	ForegroundProcessGroupID int                      `json:"foreground_process_group_id"`
	ForegroundProcesses      []PaneProcessInfoProcess `json:"foreground_processes"`
}

// AgentInfo is one entry of the snapshot's agents list, the only place the
// name `herdr agent rename` gives an agent appears: no pane object carries it.
type AgentInfo struct {
	PaneID string `json:"pane_id"`
	Name   string `json:"name"`
}

// Snapshot is the whole session as session.snapshot reports it.
type Snapshot struct {
	Workspaces []WorkspaceInfo `json:"workspaces"`

	Tabs   []TabInfo   `json:"tabs"`
	Panes  []PaneInfo  `json:"panes"`
	Agents []AgentInfo `json:"agents"`
}

type snapshotResult struct {
	Snapshot Snapshot `json:"snapshot"`
}

type processInfoResult struct {
	ProcessInfo PaneProcessInfo `json:"process_info"`
}
