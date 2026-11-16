package desktop

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

const fakeDocument = `{"spaces":[
 {"id":"sp1","name":"","folder":"/repo","tabs":[
  {"id":"tb1","name":"","root":{"type":"row","id":"tb1","children":[
   {"type":"block","id":"p1","size":{"weight":1.5},"content":{"terminal":"t1"}},
   {"type":"column","id":"c1","children":[
    {"type":"block","id":"p2","content":{"terminal":"t2"}}]}]}},
  {"id":"tb2","name":"mine","root":{"type":"block","id":"p3","content":{"terminal":"t3"}}}]}]}`

const fakeCoreTerminals = `{"terminals":[
 {"address":"t1","cwd":"/repo","title":"~/repo","turn":0,"foreground":null},
 {"address":"t2","cwd":"/repo","title":"✳ Claude Code","turn":4,"foreground":null,
  "status":{"app":"claude-code","state":"idle"},"labels":{"herdr.seen":"3"}},
 {"address":"t3","cwd":"/repo","title":"","turn":0,"foreground":null,
  "status":{"app":"codex","state":"working"}}]}`

// fakeCore keeps one layout document and refuses a write made against an
// older version, as herdr-core does.
type fakeCore struct {
	mu       sync.Mutex
	value    json.RawMessage
	version  int
	conflict int
	sets     int
}

func (f *fakeCore) reply(request map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	params, _ := request["params"].(map[string]any)

	switch request["method"] {
	case "terminal.list":
		return `{"jsonrpc":"2.0","id":1,"result":` + fakeCoreTerminals + `}`
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

// written is the document and how many writes were asked for, read under the
// lock: the race detector cannot see a socket ordering them.
func (f *fakeCore) written() (json.RawMessage, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.value, f.sets
}

func coreLayoutClient(t *testing.T, conflicts int) (*Client, *fakeCore) {
	t.Helper()

	core := &fakeCore{value: json.RawMessage(fakeDocument), version: 7, conflict: conflicts}
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

// A connected machine's PIDs and paths must never reach the local readers.
func TestCoreLayoutLeavesOutAConnectedMachinesBlock(t *testing.T) {
	t.Parallel()

	socket := serve(t, func(request map[string]any) string {
		if request["method"] == "terminal.list" {
			return `{"jsonrpc":"2.0","id":1,"result":{"terminals":[
			 {"address":"t1","pid":10},{"address":"box/t9","pid":99,"cwd":"/remote"}]}}`
		}

		return `{"jsonrpc":"2.0","id":1,"result":{"version":1,"value":{"spaces":[{"id":"sp1","tabs":[
		 {"id":"tb1","root":{"type":"row","children":[
		  {"type":"block","id":"p1","content":{"terminal":"t1"}},
		  {"type":"block","id":"box/p9","content":{"terminal":"box/t9"}}]}}]}]}}}`
	})
	client := newClient("", socket.path, true)
	client.sessions = newSessionFinder(nil)

	snapshot := readSnapshot(t, client)
	if len(snapshot.Panes) != 1 || snapshot.Panes[0].PaneID != "p1" {
		t.Errorf("panes = %+v, want only the local one", snapshot.Panes)
	}
}
