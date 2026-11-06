package desktop

import (
	"hash/fnv"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// world is what `ls` answers: spaces, their tabs, and each tab's terminals.
type world struct {
	Spaces []space `json:"spaces"`
	Error  string  `json:"error"`
}

type space struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Machine is null for this computer. Any other's transcripts and checkouts
	// are out of reach from here, so its tabs are left alone.
	Machine *string `json:"machine"`
	Tabs    []tab   `json:"tabs"`
}

type tab struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Active    bool        `json:"active"`
	Terminals []paneEntry `json:"terminals"`
	// panes are those of Terminals that the core also lists.
	panes []localPane
}

// paneEntry is a pane as `ls` lists it, whose status is the app's own word:
// `done` there is still `idle` in the core.
type paneEntry struct {
	Terminal string `json:"terminal"`
	Pane     string `json:"pane"`
	Status   string `json:"status"`
	App      string `json:"app"`
}

type appStatus struct {
	FocusedPane string `json:"focused_pane"`
	Error       string `json:"error"`
}

// terminal is what the core knows of a pane and `ls` does not.
type terminal struct {
	Address string `json:"address"`
	PID     int    `json:"pid"`
	CWD     string `json:"cwd"`
	Title   string `json:"title"`
	Turn    uint64 `json:"turn"`
	// Argv is what the terminal was started with, known from its first moment.
	Argv []string `json:"argv"`
	// Foreground is null for the first moments of a terminal, which decodes as
	// no process at all.
	Foreground herdr.PaneProcessInfoProcess `json:"foreground"`
}

// localPane is a pane on this computer and what the core knows of it.
type localPane struct {
	entry    paneEntry
	terminal terminal
}

// versionRe is a process name that is its version: Claude Code's reads 2.1.314.
var versionRe = regexp.MustCompile(`^\d+(\.\d+)+$`)

// launcherNames are what the app's `+` menu names a tab it starts an agent in.
// Nobody chose them, so such a tab counts as unnamed despite carrying one; an
// agent missing here locks its tab: docs/architecture/desktop-api.md.
var launcherNames = []string{
	"Claude Code", "Codex", "Pi", "PiG", "OpenCode", "Cursor", "Copilot", "Amp",
	"Grok", "DeepSeek Harness", "Antigravity", "Kimi", "Kiro", "Hermes", "Qoder",
	"Crush", "Gemini",
}

// statusRank orders what a tab reports, most urgent last, as the app does.
var statusRank = []string{
	herdr.AgentStatusIdle,
	herdr.AgentStatusWorking,
	herdr.AgentStatusDone,
	herdr.AgentStatusBlocked,
}

// local is this computer's spaces, each tab's panes joined to their terminals. A
// pane the core does not list yet has nothing to say, so another may name its tab.
func (w world) local(terminals []terminal) []space {
	byAddress := make(map[string]terminal, len(terminals))
	for _, t := range terminals {
		byAddress[t.Address] = t
	}

	var spaces []space

	for _, space := range w.Spaces {
		if space.Machine != nil {
			continue
		}

		for i := range space.Tabs {
			tab := &space.Tabs[i]

			for _, entry := range tab.Terminals {
				if t, listed := byAddress[entry.Terminal]; listed {
					tab.panes = append(tab.panes, localPane{entry: entry, terminal: t})
				}
			}
		}

		spaces = append(spaces, space)
	}

	return spaces
}

func panesIn(spaces []space) []localPane {
	var panes []localPane

	for _, space := range spaces {
		for _, tab := range space.Tabs {
			panes = append(panes, tab.panes...)
		}
	}

	return panes
}

// build is the snapshot a poll reads, and what each pane is running beside it.
func (c *Client) build(
	spaces []space,
	focusedPane string,
	sessions map[string]*herdr.AgentSessionInfo,
) (herdr.Snapshot, map[string]herdr.PaneProcessInfoProcess) {
	c.rememberLabels(spaces)
	c.rememberFocus(spaces, focusedPane)

	var snapshot herdr.Snapshot

	foregrounds := make(map[string]herdr.PaneProcessInfoProcess)

	for _, space := range spaces {
		workspace := herdr.WorkspaceInfo{WorkspaceID: space.ID, Label: space.Name}

		for _, tab := range space.Tabs {
			if tab.Active {
				workspace.ActiveTabID = tab.ID
			}

			if focused, known := c.focused[tab.ID]; known {
				snapshot.Layouts = append(snapshot.Layouts, herdr.LayoutInfo{
					TabID: tab.ID, FocusedPaneID: focused,
				})
			}

			snapshot.Tabs = append(snapshot.Tabs, herdr.TabInfo{
				TabID:       tab.ID,
				WorkspaceID: space.ID,
				Label:       tab.Name,
				Unnamed:     c.initialLabels[tab.ID] == tab.Name && launcherNamed(tab),
				AgentStatus: statusOf(tab),
			})

			for _, local := range tab.panes {
				pane := paneFrom(tab.ID, local, focusedPane)
				pane.AgentSession = sessions[local.entry.Pane]
				snapshot.Panes = append(snapshot.Panes, pane)
				foregrounds[local.entry.Pane] = foregroundOf(local.terminal)
			}
		}

		snapshot.Workspaces = append(snapshot.Workspaces, workspace)
	}

	return snapshot, foregrounds
}

// rememberLabels recognizes a launcher's unchanged initial name, never a name
// the user later returns to. Closed tabs leave no history behind.
func (c *Client) rememberLabels(spaces []space) {
	initialLabels := make(map[string]string)

	for _, space := range spaces {
		for _, tab := range space.Tabs {
			initial, known := c.initialLabels[tab.ID]
			if !known {
				initial = tab.Name
			} else if initial != tab.Name {
				initial = ""
			}

			initialLabels[tab.ID] = initial
		}
	}

	c.initialLabels = initialLabels
}

// rememberFocus keeps the pane each tab last had focused, since the app names
// only the shown tab's; a tab not shown would otherwise be named after another
// pane until it is shown again. Closed tabs and closed panes are forgotten.
func (c *Client) rememberFocus(spaces []space, focusedPane string) {
	focused := make(map[string]string)

	for _, space := range spaces {
		for _, tab := range space.Tabs {
			holds := func(pane string) bool {
				return slices.ContainsFunc(
					tab.Terminals,
					func(e paneEntry) bool { return e.Pane == pane },
				)
			}

			switch last := c.focused[tab.ID]; {
			case focusedPane != "" && holds(focusedPane):
				focused[tab.ID] = focusedPane
			case last != "" && holds(last):
				focused[tab.ID] = last
			}
		}
	}

	c.focused = focused
}

func foregroundOf(t terminal) herdr.PaneProcessInfoProcess {
	process := t.Foreground
	if versionRe.MatchString(process.Name) && len(process.Argv) > 0 {
		process.Name = filepath.Base(process.Argv[0])
	}

	return process
}

// launcherNamed is a launcher's name on a tab an agent was started in. The
// agent reports itself about a second late, so the argv tab.new was given
// counts too, or the first poll would take the name for the user's.
func launcherNamed(tab tab) bool {
	if !slices.Contains(launcherNames, tab.Name) {
		return false
	}

	return slices.ContainsFunc(tab.Terminals, func(e paneEntry) bool { return e.App != "" }) ||
		slices.ContainsFunc(
			tab.panes,
			func(p localPane) bool { return startsProgram(p.terminal.Argv) },
		)
}

// startsProgram is whether argv runs more than a shell given only flags, which
// tab.new's wrapper around the program it starts always does.
func startsProgram(argv []string) bool {
	return len(argv) > 1 && slices.ContainsFunc(argv[1:], func(arg string) bool {
		return !strings.HasPrefix(arg, "-")
	})
}

func statusOf(tab tab) string {
	best, status := -1, ""

	for _, entry := range tab.Terminals {
		if rank := slices.Index(statusRank, entry.Status); rank > best {
			best, status = rank, entry.Status
		}
	}

	return status
}

func paneFrom(tabID string, local localPane, focusedPane string) herdr.PaneInfo {
	entry, t := local.entry, local.terminal

	agent := entry.App
	if known, listed := agents[agent]; listed {
		agent = known.name
	}

	stripped := withoutSpinner(t.Title)

	return herdr.PaneInfo{
		PaneID:                entry.Pane,
		TabID:                 tabID,
		Focused:               entry.Pane == focusedPane,
		Revision:              revisionOf(entry, t, stripped),
		CWD:                   t.CWD,
		ForegroundCWD:         t.Foreground.CWD,
		TerminalTitle:         t.Title,
		TerminalTitleStripped: stripped,
		Agent:                 agent,
		DisplayAgent:          agent,
		AgentStatus:           entry.Status,
	}
}

// revisionOf stands in for the revision the core does not report. It moves on
// what a pane says and runs, and cannot see a pane merely drawing.
func revisionOf(entry paneEntry, t terminal, title string) uint64 {
	hash := fnv.New64a()

	parts := []string{
		entry.Status, title, strconv.FormatUint(t.Turn, 10),
		strconv.Itoa(t.Foreground.PID), t.Foreground.Name,
	}

	_, _ = hash.Write([]byte(strings.Join(parts, "\x00")))

	return hash.Sum64()
}

// withoutSpinner stands in for Herdr's terminal_title_stripped, which the core
// lacks. Frames differ, as in `⠇ ⠇ | name` from Codex; Grok's dash stays
// because the resolver reads its status line through it.
func withoutSpinner(title string) string {
	rest := strings.TrimLeftFunc(title, func(r rune) bool {
		return unicode.Is(unicode.Braille, r) || isCircleSpinner(r) || unicode.IsSpace(r)
	})

	if rest == title {
		return title
	}

	return strings.TrimPrefix(rest, "| ")
}

// isCircleSpinner reports the half and quadrant circles spinners cycle through,
// and the star Claude Code puts in front of its title.
func isCircleSpinner(r rune) bool {
	return (r >= '◐' && r <= '◓') || (r >= '◴' && r <= '◷') || r == '✳'
}
