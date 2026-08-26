package herdr

// Agent statuses Herdr reports. Every pane carries one: a pane with no agent
// reports AgentStatusUnknown.
const (
	AgentStatusIdle    = "idle"
	AgentStatusWorking = "working"
	AgentStatusBlocked = "blocked"
	AgentStatusDone    = "done"
	AgentStatusUnknown = "unknown"
)

// Wire types carry only the fields Auto Title reads: a field nothing uses makes
// the type claim a dependency the code does not have.

// WorkspaceInfo describes a workspace. Its label is what Herdr shows above the
// tabs, and it is usually the project every tab in it belongs to.
type WorkspaceInfo struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

// TabInfo describes a tab. Optional fields are decoded as plain strings, so a
// JSON null leaves them empty rather than failing.
type TabInfo struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

// PaneInfo describes a pane. It carries no foreground process name — only
// pane.process_info answers that — and every optional field decodes null to "".
type PaneInfo struct {
	PaneID   string `json:"pane_id"`
	TabID    string `json:"tab_id"`
	Focused  bool   `json:"focused"`
	Revision uint64 `json:"revision"`

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

// SessionRefID is the only reference kind Herdr has been seen to answer with.
// It accepts a path from an integration and reports the id anyway.
const SessionRefID = "id"

// AgentSessionInfo identifies an agent's own session, as its integration hook
// reported it. Which agent reported it decides how the reference is read, so
// the label is part of the reference.
type AgentSessionInfo struct {
	Agent string `json:"agent"`
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// IDFor reports the session id this reference holds, and false unless it names
// a session of the given agent by id. A nil reference is a pane whose agent has
// reported nothing, which is every pane until an integration is installed.
func (s *AgentSessionInfo) IDFor(agent string) (string, bool) {
	if s == nil || s.Agent != agent || s.Kind != SessionRefID {
		return "", false
	}

	return s.Value, true
}

// Dir is the directory a pane speaks for. The shell's own is preferred over
// the foreground process's, which follows whatever is running right now.
func (p PaneInfo) Dir() string {
	if p.CWD != "" {
		return p.CWD
	}

	return p.ForegroundCWD
}

// PaneProcessInfoProcess is one process running in a pane. Herdr reports more
// about each — its pid, its working directory, the joined command line — but a
// title is derived from the name and the arguments alone.
type PaneProcessInfoProcess struct {
	Name string   `json:"name"`
	Argv []string `json:"argv"`
}

// PaneProcessInfo is what pane.process_info answers. ForegroundProcesses holds
// the pane's foreground process and its descendants, so an editor that shelled
// out lists both.
type PaneProcessInfo struct {
	ForegroundProcesses []PaneProcessInfoProcess `json:"foreground_processes"`
}

// Snapshot is the whole session as session.snapshot reports it.
type Snapshot struct {
	Workspaces []WorkspaceInfo `json:"workspaces"`

	Tabs  []TabInfo  `json:"tabs"`
	Panes []PaneInfo `json:"panes"`
}

// snapshotResult wraps the snapshot in the method's result object.
type snapshotResult struct {
	Snapshot Snapshot `json:"snapshot"`
}

// processInfoResult wraps what pane.process_info answers.
type processInfoResult struct {
	ProcessInfo PaneProcessInfo `json:"process_info"`
}
