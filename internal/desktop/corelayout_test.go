package desktop

import (
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/labels"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

const fakeDocument = `{"spaces":[
 {"id":"sp1","name":"","folder":"/repo","tabs":[
  {"id":"tb1","name":"","root":{"type":"row","id":"tb1","children":[
   {"type":"block","id":"p1","size":{"weight":1.5},"content":{"terminal":"t1"}},
   {"type":"column","id":"c1","children":[
    {"type":"block","id":"p2","content":{"terminal":"t2"}}]}]}},
  {"id":"tb2","name":"mine","root":{"type":"block","id":"p3","content":{"terminal":"t3"}}}]}]}`

const fakeCoreTerminals = `{"terminals":[
 {"address":"t1","state":"live","cwd":"/repo","title":"~/repo","turn":0,"foreground":null,
  "labels":{"herdr.space":"sp1","herdr.tab":"ovis-1"}},
 {"address":"t2","state":"live","cwd":"/repo","title":"✳ Claude Code","turn":4,"foreground":null,
  "status":{"app":"claude-code","state":"idle"},"labels":{"herdr.seen":"3"}},
 {"address":"t3","state":"live","cwd":"/repo","title":"","turn":0,"foreground":null,
  "status":{"app":"codex","state":"working"}}]}`

// fakeCore keeps one layout document and refuses a write made against an
// older version, as herdr-core does.
type fakeCore struct {
	mu        sync.Mutex
	value     json.RawMessage
	version   int
	conflict  int
	sets      int
	terminals []terminal
	failLabel string
}

func (f *fakeCore) reply(request map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	params, _ := request["params"].(map[string]any)

	switch request["method"] {
	case "terminal.list":
		result, err := json.Marshal(map[string]any{"terminals": f.terminals})
		if err != nil {
			return `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"bad terminals"}}`
		}

		return `{"jsonrpc":"2.0","id":1,"result":` + string(result) + `}`
	case "terminal.label":
		return f.label(params)
	case "doc.get":
		return `{"jsonrpc":"2.0","id":1,"result":{"value":` + string(f.value) +
			`,"version":` + strconv.Itoa(f.version) + `}}`
	case "doc.set":
		f.sets++
		if version, _ := params["version"].(float64); int(version) != f.version || f.conflict > 0 {
			f.conflict = max(f.conflict-1, 0)
			f.version++

			return `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"document is at a newer version",` +
				`"data":{"kind":"conflict"}}}`
		}

		value, err := json.Marshal(params["value"])
		if err != nil {
			return `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad value"}}`
		}

		f.value = value
		f.version++

		return `{"jsonrpc":"2.0","id":1,"result":{"version":` + strconv.Itoa(f.version) + `}}`
	default:
		return `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"no method"}}`
	}
}

func (f *fakeCore) label(params map[string]any) string {
	address, _ := params["terminal"].(string)
	labels, _ := params["labels"].(map[string]any)

	for i, t := range f.terminals {
		if t.Address != address || address == f.failLabel {
			continue
		}

		if t.Labels == nil {
			f.terminals[i].Labels = make(map[string]string)
		}

		for key, raw := range labels {
			if value, _ := raw.(string); value == "" {
				delete(f.terminals[i].Labels, key)
			} else {
				f.terminals[i].Labels[key] = value
			}
		}

		return `{"jsonrpc":"2.0","id":1,"result":{}}`
	}

	return `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"no terminal"}}`
}

// written is the document and how many writes were asked for, read under the
// lock: the race detector cannot see a socket ordering them.
func (f *fakeCore) written() (json.RawMessage, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.value, f.sets
}

func coreLayoutClient(t *testing.T, conflicts int, extra ...terminal) (*Client, *fakeCore) {
	t.Helper()

	var listed struct {
		Terminals []terminal `json:"terminals"`
	}
	if err := json.Unmarshal([]byte(fakeCoreTerminals), &listed); err != nil {
		t.Fatal(err)
	}

	core := &fakeCore{
		value: json.RawMessage(fakeDocument), version: 7, conflict: conflicts,
		terminals: append(listed.Terminals, extra...),
	}
	socket := serve(t, core.reply)

	client := newClient("", socket.path, true)
	client.sessions = newSessionFinder(nil)

	return client, core
}

// Protects the layout read on a machine with no window: tabs, their order and
// panes come from the document, and an unnamed tab reads as its position.
func TestCoreLayoutReadsTheDocument(t *testing.T) {
	t.Parallel()

	client, _ := coreLayoutClient(t, 0)
	snapshot := readSnapshot(t, client)

	labels := make([]string, 0, len(snapshot.Tabs))
	for _, tab := range snapshot.Tabs {
		labels = append(labels, tab.Label)
	}

	if strings.Join(labels, ",") != "1,mine" {
		t.Errorf("tab labels = %v", labels)
	}

	if got := paneByID(
		t,
		snapshot,
		"p2",
	); got.TabID != "tb1" ||
		got.AgentStatus != herdr.AgentStatusDone {
		t.Errorf("p2 = %+v, want tab tb1 and done past its seen turn", got)
	}

	if got := paneByID(t, snapshot, "p3"); got.AgentStatus != herdr.AgentStatusWorking {
		t.Errorf("p3 status = %q", got.AgentStatus)
	}

	if client.Path() != client.core {
		t.Errorf("claim path = %q, want the core's", client.Path())
	}
}

// Protects everything the app keeps in its document but Auto Title never reads.
func TestCoreLayoutRenameKeepsTheRestOfTheDocument(t *testing.T) {
	t.Parallel()

	client, core := coreLayoutClient(t, 0)

	if err := client.RenameTab(t.Context(), "tb1", "  fix login  "); err != nil {
		t.Fatalf("rename: %v", err)
	}

	value, _ := core.written()

	var got struct {
		Spaces []struct {
			Folder string `json:"folder"`
			Tabs   []struct {
				Name string `json:"name"`
				Root struct {
					Children []struct {
						Size struct {
							Weight float64 `json:"weight"`
						} `json:"size"`
					} `json:"children"`
				} `json:"root"`
			} `json:"tabs"`
		} `json:"spaces"`
	}

	if err := json.Unmarshal(value, &got); err != nil {
		t.Fatal(err)
	}

	space := got.Spaces[0]
	if tab := space.Tabs[0]; tab.Name != "fix login" || space.Folder != "/repo" ||
		tab.Root.Children[0].Size.Weight != 1.5 {
		t.Errorf("document after rename: %s", value)
	}
}

// Protects a rename racing a connected app's own write to the document.
func TestCoreLayoutRenameRetriesAConflict(t *testing.T) {
	t.Parallel()

	client, core := coreLayoutClient(t, 1)

	if err := client.RenameTab(t.Context(), "tb2", "x"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if value, sets := core.written(); sets != 2 || !strings.Contains(string(value), `"name":"x"`) {
		t.Errorf("sets = %d, document %s", sets, value)
	}
}

// labels forgets a tab that is gone, so a closed tab must say so.
func TestCoreLayoutRenameOfAGoneTab(t *testing.T) {
	t.Parallel()

	client, core := coreLayoutClient(t, 0)

	err := client.RenameTab(t.Context(), "tb9", "x")
	if _, sets := core.written(); herdr.ErrorCode(err) != herdr.CodeTabNotFound || sets != 0 {
		t.Errorf("gone tab: %v after %d writes", err, sets)
	}
}

func TestCoreLayoutReadsUnplacedTabs(t *testing.T) {
	t.Parallel()

	client, _ := coreLayoutClient(t, 0,
		terminal{Address: "t4", State: "live", Labels: map[string]string{
			"herdr.space": "sp1", "herdr.tab": "ovis-1", "where": " Main / Project / unused ",
			"ovis.tab": "hidden",
		}},
		terminal{Address: "t5", State: "live", Labels: map[string]string{
			"herdr.space": "sp1", "herdr.tab": "ovis-1", "ovis.tab": "second pane",
		}, Status: &coreStatus{App: "codex", State: "working"}},
		terminal{Address: "t6", State: "live", Labels: map[string]string{
			"herdr.space": "sp2", "herdr.tab": "ovis-2", "ovis.workspace": "Other space",
		}},
		terminal{Address: "t7", State: "live", Labels: map[string]string{
			"herdr.space": "sp2", "herdr.tab": "ovis-1", "ovis.tab": "Named",
		}},
		terminal{Address: "t8", State: "live", Labels: map[string]string{
			"herdr.space": "sp1", "herdr.tab": "tb2",
		}},
		terminal{Address: "t9", State: "live", Labels: map[string]string{"herdr.tab": "ovis-1"}},
		terminal{Address: "remote/t4", State: "live", Labels: map[string]string{
			"herdr.space": "sp2", "herdr.tab": "ovis-2",
		}},
		terminal{Address: "ta", State: "finished", Labels: map[string]string{
			"herdr.space": "sp2", "herdr.tab": "ovis-2",
		}},
	)

	snapshot := readSnapshot(t, client)

	tabs := make([]string, 0, len(snapshot.Tabs))
	for _, tab := range snapshot.Tabs {
		tabs = append(tabs, tab.WorkspaceID+"/"+tab.TabID+"/"+tab.Label)
	}

	panes := make([]string, 0, len(snapshot.Panes))
	for _, pane := range snapshot.Panes {
		panes = append(panes, pane.TabID+"/"+pane.PaneID)
	}

	wantTabs := []string{
		"sp1/tb1/1", "sp1/tb2/mine", "sp1/ovis-1/Project", "sp2/ovis-2/1", "sp2/sp2:t7/Named",
	}

	wantPanes := []string{
		"tb1/p1",
		"tb1/p2",
		"tb2/p3",
		"tb2/t8",
		"ovis-1/t4",
		"ovis-1/t5",
		"ovis-2/t6",
		"sp2:t7/t7",
	}
	if !slices.Equal(tabs, wantTabs) || !slices.Equal(panes, wantPanes) {
		t.Fatalf("tabs %v, panes %v", tabs, panes)
	}

	if workspace := snapshot.Workspaces[1]; workspace.Label != "Other space" {
		t.Errorf("label-only workspace = %+v", workspace)
	}

	if pane := paneByID(
		t,
		snapshot,
		"t5",
	); pane.Agent != "codex" ||
		pane.AgentStatus != herdr.AgentStatusWorking {
		t.Errorf("label-only pane = %+v", pane)
	}
}

func TestCoreLayoutLabelRenameReadsBackAndProtectsManualNames(t *testing.T) {
	t.Parallel()

	client, core := coreLayoutClient(t, 0,
		terminal{Address: "t4", State: "live", Labels: map[string]string{
			"herdr.space": "sp1", "herdr.tab": "ovis-1", "where": "Main / Old name",
		}},
		terminal{Address: "t5", State: "live", Labels: map[string]string{
			"herdr.space": "sp1", "herdr.tab": "ovis-1", "where": "Main / Other old name",
		}},
	)
	owner := labels.Load("", nil, slog.New(slog.DiscardHandler), false)
	poll := func(name string) {
		snapshot := readSnapshot(t, client)
		pass := owner.Pass(client, snapshot)
		pass.Tab(
			t.Context(),
			state.TabFrom(snapshot.Tabs[2], "", 3, nil, false),
			resolver.Decision{Name: name},
		)
		pass.Settle()
	}

	poll("Automatic")
	poll("Updated topic")

	terminals, err := listTerminals(t.Context(), client.core)
	if err != nil {
		t.Fatal(err)
	}

	for _, pane := range terminals[3:] {
		if pane.Labels["ovis.tab"] != "Updated topic" || pane.Labels["where"] != "Main" {
			t.Errorf("%s labels = %v", pane.Address, pane.Labels)
		}
	}

	if _, sets := core.written(); sets != 0 || terminals[0].Labels["ovis.tab"] != "" {
		t.Errorf(
			"label rename touched a placed pane or document: %v, %d writes",
			terminals[0].Labels,
			sets,
		)
	}

	for _, address := range []string{"t4", "t5"} {
		params := map[string]any{
			"terminal": address,
			"labels":   map[string]string{"ovis.tab": "My title"},
		}
		if err := rpc(t.Context(), client.core, "terminal.label", params, nil); err != nil {
			t.Fatal(err)
		}
	}

	poll("Would overwrite")

	if got := readSnapshot(t, client).Tabs[2].Label; got != "My title" {
		t.Errorf("manual name was changed to %q", got)
	}
}

func TestCoreLayoutLabelRenameWithoutDocument(t *testing.T) {
	t.Parallel()

	core := &fakeCore{
		value: json.RawMessage(`null`),
		terminals: []terminal{{Address: "t4", State: "live", Labels: map[string]string{
			"herdr.space": "sp1", "herdr.tab": "ovis-1",
		}}},
	}
	socket := serve(t, core.reply)
	client := newClient("", socket.path, true)
	client.sessions = newSessionFinder(nil)

	if err := client.RenameTab(t.Context(), "ovis-1", "New title"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	snapshot := readSnapshot(t, client)
	if len(snapshot.Tabs) != 1 || snapshot.Tabs[0].Label != "New title" {
		t.Errorf("tabs = %+v", snapshot.Tabs)
	}
}

func TestCoreLayoutLabelRenameReportsPartialWrite(t *testing.T) {
	t.Parallel()

	client, core := coreLayoutClient(
		t,
		0,
		terminal{
			Address: "t4",
			State:   "live",
			Labels:  map[string]string{"herdr.space": "sp1", "herdr.tab": "ovis-1"},
		},
		terminal{
			Address: "t5",
			State:   "live",
			Labels:  map[string]string{"herdr.space": "sp1", "herdr.tab": "ovis-1"},
		},
	)
	core.mu.Lock()
	core.failLabel = "t4"
	core.mu.Unlock()

	if err := client.RenameTab(
		t.Context(),
		"ovis-1",
		"Automatic",
	); !errors.Is(
		err,
		herdr.ErrUnanswered,
	) {
		t.Errorf("rename error = %v, want partial write marked as unanswered", err)
	}

	if got := readSnapshot(t, client).Tabs[2].Label; got != "3" {
		t.Errorf("visible label = %q, want unchanged position", got)
	}
}

// A connected machine's PIDs and paths must never reach the local readers.
func TestCoreLayoutLeavesOutAConnectedMachinesBlock(t *testing.T) {
	t.Parallel()

	core := &fakeCore{
		value: json.RawMessage(
			`{"spaces":[{"id":"sp1","tabs":[{"id":"tb1","root":{"type":"row","children":[
		 {"type":"block","id":"p1","content":{"terminal":"t1"}},
		 {"type":"block","id":"box/p9","content":{"terminal":"box/t9"}}]}}]}]}`,
		),
		terminals: []terminal{
			{Address: "t1", State: "live", PID: 10},
			{Address: "box/t9", State: "live", PID: 99, CWD: "/remote"},
		},
	}
	socket := serve(t, core.reply)
	client := newClient("", socket.path, true)
	client.sessions = newSessionFinder(nil)

	snapshot := readSnapshot(t, client)
	if len(snapshot.Panes) != 1 || snapshot.Panes[0].PaneID != "p1" {
		t.Errorf("panes = %+v, want only the local one", snapshot.Panes)
	}
}
