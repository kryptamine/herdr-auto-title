package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// incoming is one request as the test server saw it.
type incoming struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// testServer imitates Herdr: one request per connection, answered and closed.
type testServer struct {
	t    *testing.T
	ln   net.Listener
	path string

	mu          sync.Mutex
	requests    []incoming
	connections int

	// reply returns the line to send back.
	reply func(req incoming) string
}

func newTestServer(t *testing.T, reply func(incoming) string) *testServer {
	t.Helper()

	// Unix socket paths are short; keep well clear of the platform limit.
	dir, err := os.MkdirTemp("", "at")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	path := filepath.Join(dir, "h.sock")

	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen on %s: %v", path, err)
	}

	s := &testServer{t: t, ln: ln, path: path, reply: reply}
	go s.accept()

	t.Cleanup(func() {
		ln.Close()
		os.RemoveAll(dir)
	})

	return s
}

func (s *testServer) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}

		go s.serve(conn)
	}
}

func (s *testServer) serve(conn net.Conn) {
	s.mu.Lock()
	s.connections++
	s.mu.Unlock()

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		conn.Close()
		return
	}

	var req incoming
	if err := json.Unmarshal(line, &req); err != nil {
		conn.Close()
		return
	}

	s.mu.Lock()
	s.requests = append(s.requests, req)
	reply := s.reply
	s.mu.Unlock()

	if response := reply(req); response != "" {
		_, _ = io.WriteString(conn, response+"\n")
	}
	// Herdr closes the connection once a method has been answered.
	conn.Close()
}

func (s *testServer) client() *SocketClient {
	return newWithPath(s.path)
}

func (s *testServer) connectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.connections
}

func (s *testServer) seen() []incoming {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]incoming(nil), s.requests...)
}

// respondOK answers every method with an empty result.
func respondOK(req incoming) string {
	return `{"id":"` + req.ID + `","result":{}}`
}

func TestCallDecodesResult(t *testing.T) {
	srv := newTestServer(t, func(req incoming) string {
		return `{"id":"` + req.ID + `","result":{"version":"0.8.2"}}`
	})

	var got struct {
		Version string `json:"version"`
	}
	if err := srv.client().
		Call(context.Background(), MethodSessionSnapshot, nil, &got); err != nil {
		t.Fatalf("Call: %v", err)
	}

	if got.Version != "0.8.2" {
		t.Errorf("version = %q, want 0.8.2", got.Version)
	}

	seen := srv.seen()
	if len(seen) != 1 || seen[0].Method != MethodSessionSnapshot {
		t.Fatalf("server saw %+v, want one ping", seen)
	}
	// Herdr requires params on every request.
	if string(seen[0].Params) != "{}" {
		t.Errorf("params = %s, want {}", seen[0].Params)
	}
}

func TestEachCallUsesItsOwnConnection(t *testing.T) {
	srv := newTestServer(t, respondOK)
	client := srv.client()

	// Herdr closes the connection after answering, so a reused connection would
	// fail on the second call.
	for i := range 3 {
		if err := client.Call(context.Background(), MethodSessionSnapshot, nil, nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}

	if got := srv.connectionCount(); got != 3 {
		t.Errorf("server accepted %d connections, want 3", got)
	}
}

func TestCallReturnsAPIError(t *testing.T) {
	srv := newTestServer(t, func(req incoming) string {
		return `{"id":"` + req.ID + `","error":{"code":"not_found","message":"no such tab"}}`
	})

	err := RenameTab(context.Background(), srv.client(), "wE:t1", "dashboard")
	if err == nil {
		t.Fatal("Call succeeded, want an error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}

	if apiErr.Code != "not_found" {
		t.Errorf("code = %q, want not_found", apiErr.Code)
	}
}

func TestCallReportsAnUncorrelatedError(t *testing.T) {
	// Herdr answers a malformed request with an error frame carrying no id and
	// then drops the connection.
	srv := newTestServer(t, func(incoming) string {
		return `{"id":"","error":{"code":"invalid_request","message":"unknown variant"}}`
	})

	if err := srv.client().Call(context.Background(), MethodSessionSnapshot, nil, nil); err == nil {
		t.Error("Call succeeded despite an error frame")
	}
}

func TestSessionSnapshotDecodesTheWrapper(t *testing.T) {
	srv := newTestServer(t, func(req incoming) string {
		return `{"id":"` + req.ID + `","result":{"snapshot":{"version":"0.8.2","protocol":20,` +
			`"tabs":[{"tab_id":"wE:t1","workspace_id":"wE","label":"1","number":1}],` +
			`"panes":[{"pane_id":"wE:p1","tab_id":"wE:t1","terminal_id":"t","workspace_id":"wE","cwd":"/work/api","focused":true}]}}}`
	})

	snapshot, err := SessionSnapshot(context.Background(), srv.client())
	if err != nil {
		t.Fatalf("SessionSnapshot: %v", err)
	}
	// The response carries version, protocol and a tab number, none of which
	// the wire types mirror: what nothing reads must decode to nothing.
	if len(snapshot.Tabs) != 1 || snapshot.Tabs[0].Label != "1" {
		t.Errorf("tabs = %+v, want one tab labelled 1", snapshot.Tabs)
	}

	if len(snapshot.Panes) != 1 || snapshot.Panes[0].CWD != "/work/api" {
		t.Errorf("panes = %+v, want one pane in /work/api", snapshot.Panes)
	}
}

func TestRenameTabSendsTabAndLabel(t *testing.T) {
	srv := newTestServer(t, respondOK)

	if err := RenameTab(
		context.Background(),
		srv.client(),
		"wE:t1",
		"dashboard › Tests",
	); err != nil {
		t.Fatalf("RenameTab: %v", err)
	}

	seen := srv.seen()
	if len(seen) != 1 || seen[0].Method != MethodTabRename {
		t.Fatalf("server saw %+v, want one tab.rename", seen)
	}

	var params TabRenameParams
	if err := json.Unmarshal(seen[0].Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}

	if params.TabID != "wE:t1" || params.Label != "dashboard › Tests" {
		t.Errorf("params = %+v, want {wE:t1 dashboard › Tests}", params)
	}
}

func TestNullFieldsDecodeAsEmpty(t *testing.T) {
	// Herdr sends null for every optional field of a pane running a plain
	// shell, and a snapshot is full of them.
	var got snapshotResult

	raw := `{"snapshot":{"tabs":[{"tab_id":"wE:t1","label":null}],"panes":[
		{"pane_id":"wE:p1","tab_id":"wE:t1","revision":3,"cwd":null,
		 "foreground_cwd":null,"terminal_title":null,"terminal_title_stripped":null,
		 "title":null,"agent":null,"display_agent":null,"agent_status":"unknown"}]}}`
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	pane := got.Snapshot.Panes[0]
	if pane.CWD != "" || pane.TerminalTitle != "" || pane.Agent != "" || pane.Title != "" {
		t.Errorf("null pane fields decoded as %+v, want empty strings", pane)
	}

	if label := got.Snapshot.Tabs[0].Label; label != "" {
		t.Errorf("null label decoded as %q, want empty", label)
	}
}

func TestSocketPathRequiresTheEnvironment(t *testing.T) {
	t.Setenv(socketPathEnv, "")

	if _, err := socketPath(); err == nil {
		t.Error("SocketPath succeeded without the environment variable")
	}

	t.Setenv(socketPathEnv, "/tmp/herdr.sock")

	got, err := socketPath()
	if err != nil {
		t.Fatalf("SocketPath: %v", err)
	}

	if got != "/tmp/herdr.sock" {
		t.Errorf("path = %q, want /tmp/herdr.sock", got)
	}
}

func TestErrorCode(t *testing.T) {
	srv := newTestServer(t, func(req incoming) string {
		return `{"id":"` + req.ID + `","error":{"code":"tab_not_found","message":"tab wE:t1 not found"}}`
	})

	err := RenameTab(context.Background(), srv.client(), "wE:t1", "dashboard")
	if got := ErrorCode(err); got != CodeTabNotFound {
		t.Errorf("ErrorCode = %q, want %q", got, CodeTabNotFound)
	}

	if got := ErrorCode(errors.New("plain")); got != "" {
		t.Errorf("ErrorCode of a plain error = %q, want empty", got)
	}
}
