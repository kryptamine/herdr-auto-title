package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// socketPathEnv is where Herdr names the socket it made for this session. The
// path is never hard-coded.
const socketPathEnv = "HERDR_SOCKET_PATH"

// Client is what Auto Title asks of a session, whichever program holds it.
type Client interface {
	// Snapshot fetches the whole session: every tab with its label, every pane
	// with its context. There is deliberately no Subscribe beside it — see
	// docs/architecture/poll-loop.md.
	Snapshot(ctx context.Context) (Snapshot, error)
	// PaneProcesses reads what is running in a pane, the pane's own foreground
	// process last.
	PaneProcesses(ctx context.Context, paneID string) ([]PaneProcessInfoProcess, error)
	RenameTab(ctx context.Context, tabID, label string) error
	// RenamePane names a pane, which is what the goto panel lists a pane by. An
	// empty label clears the name rather than storing it, so nothing sends one.
	RenamePane(ctx context.Context, paneID, label string) error
	// ReportWorkspaceTopic shows topic as the workspace's `topic` token until ttl
	// runs out, or clears the token when topic is empty.
	ReportWorkspaceTopic(
		ctx context.Context,
		workspaceID, source, topic string,
		ttl time.Duration,
	) error
	// Path names the session, which is what its claim is kept under.
	Path() string
	// Server identifies the server holding the socket, or is "" while none
	// does. It changes when another server binds the path.
	Server() string
}

// SocketClient speaks NDJSON to the Herdr socket. Herdr closes the connection
// after answering, so each call dials its own — which is why there is nothing
// to reconnect anywhere in the plugin.
type SocketClient struct {
	path string
	seq  atomic.Uint64
}

var _ Client = (*SocketClient)(nil)

func socketPath() (string, error) {
	path := os.Getenv(socketPathEnv)
	if path == "" {
		return "", fmt.Errorf("%s is not set: Auto Title must be started by Herdr", socketPathEnv)
	}

	return path, nil
}

// New builds a client for the socket named by HERDR_SOCKET_PATH. It performs no
// I/O; the first connection is made by the first call.
func New() (*SocketClient, error) {
	path, err := socketPath()
	if err != nil {
		return nil, err
	}

	return newWithPath(path), nil
}

func newWithPath(path string) *SocketClient {
	return &SocketClient{path: path}
}

// Server reads the socket's identity the way Herdr tells its own socket from a
// successor's: the socket file's device and inode, or on Windows the marker
// written into the file. It performs no request.
func (c *SocketClient) Server() string {
	return serverIdentity(c.path)
}

// Path is the socket the client dials, which is what names a session.
func (c *SocketClient) Path() string {
	return c.path
}

// Snapshot asks Herdr for the whole session.
func (c *SocketClient) Snapshot(ctx context.Context) (Snapshot, error) {
	var res snapshotResult
	if err := c.call(ctx, MethodSessionSnapshot, emptyParams{}, &res); err != nil {
		return Snapshot{}, err
	}

	return res.Snapshot, nil
}

// PaneProcesses asks Herdr what a pane runs. PaneInfo has no process name and
// this is the only method that answers one, at 0.11 ms per pane.
func (c *SocketClient) PaneProcesses(
	ctx context.Context,
	paneID string,
) ([]PaneProcessInfoProcess, error) {
	var res processInfoResult
	if err := c.call(ctx, MethodPaneProcessInfo, PaneTarget{PaneID: paneID}, &res); err != nil {
		return nil, err
	}

	return foregroundLast(res.ProcessInfo), nil
}

// foregroundLast moves the foreground process to the end of the list. Herdr
// 0.8.2 on macOS listed it last and 0.9.0 on Linux lists it first, ahead of the
// servers an agent spawned, so only its pid says which one it is.
func foregroundLast(info PaneProcessInfo) []PaneProcessInfoProcess {
	processes := info.ForegroundProcesses
	if info.ForegroundProcessGroupID == 0 {
		return processes
	}

	for i, process := range processes {
		if process.PID != info.ForegroundProcessGroupID {
			continue
		}

		ordered := make([]PaneProcessInfoProcess, 0, len(processes))
		ordered = append(ordered, processes[:i]...)
		ordered = append(ordered, processes[i+1:]...)

		return append(ordered, process)
	}

	return processes
}

func (c *SocketClient) RenameTab(ctx context.Context, tabID, label string) error {
	return c.call(ctx, MethodTabRename, TabRenameParams{TabID: tabID, Label: label}, nil)
}

func (c *SocketClient) RenamePane(ctx context.Context, paneID, label string) error {
	return c.call(ctx, MethodPaneRename, PaneRenameParams{PaneID: paneID, Label: label}, nil)
}

// ShowNotification puts a notice in front of the user and reports whether it
// was shown, with Herdr's reason when it was not: nothing is shown without a
// client attached, and two within a second is one too many.
func (c *SocketClient) ShowNotification(
	ctx context.Context,
	title, body string,
) (NotificationResult, error) {
	var res NotificationResult

	err := c.call(ctx, MethodNotificationShow, NotificationParams{Title: title, Body: body}, &res)

	return res, err
}

// ReportWorkspaceTopic sets the token through Herdr, which draws it where the
// user's sidebar rows ask for one and never changes the label for it.
func (c *SocketClient) ReportWorkspaceTopic(
	ctx context.Context,
	workspaceID, source, topic string,
	ttl time.Duration,
) error {
	params := TopicReport(workspaceID, source, topic, ttl)

	return c.call(ctx, MethodWorkspaceReportMetadata, params, nil)
}

// call sends one request on a connection of its own and reads the single
// response Herdr answers with before closing. A nil result drops the answer.
func (c *SocketClient) call(ctx context.Context, method string, params any, result any) error {
	if params == nil {
		params = emptyParams{}
	}

	conn, err := dial(ctx, c.path)
	if err != nil {
		return fmt.Errorf("connect to herdr socket %s: %w", c.path, err)
	}
	defer conn.Close()

	// Unblock a call whose context is cancelled while it waits on the socket.
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()

	req := request{
		ID:     fmt.Sprintf("auto-title-%d", c.seq.Add(1)),
		Method: method,
		Params: params,
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return unanswered(ctx, fmt.Errorf("send %s: %w", method, err))
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return unanswered(ctx, fmt.Errorf("read %s response: %w", method, err))
	}

	var f frame
	if err := json.Unmarshal(line, &f); err != nil {
		return unanswered(ctx, fmt.Errorf("decode %s response: %w", method, err))
	}

	if f.Error != nil {
		return fmt.Errorf("%s: %w", method, f.Error)
	}

	if result != nil && len(f.Result) > 0 {
		if err := json.Unmarshal(f.Result, result); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}

	return nil
}

// TopicReport is the request that reports a topic: a null token clears it.
func TopicReport(workspaceID, source, topic string, ttl time.Duration) WorkspaceMetadataParams {
	var value *string
	if topic != "" {
		value = &topic
	}

	return WorkspaceMetadataParams{
		WorkspaceID: workspaceID,
		Source:      source,
		Tokens:      map[string]*string{TopicToken: value},
		TTLMs:       ttl.Milliseconds(),
	}
}

// unanswered reports a call that failed once its request was sent, as the
// cancellation that closed the connection when that is what happened.
func unanswered(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		err = ctxErr
	}

	return fmt.Errorf("%w: %w", ErrUnanswered, err)
}
