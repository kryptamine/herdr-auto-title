package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync/atomic"
)

// socketPathEnv is where Herdr names the socket it made for this session. The
// path is never hard-coded.
const socketPathEnv = "HERDR_SOCKET_PATH"

// Client is the subset of the Herdr socket API that Auto Title uses.
type Client interface {
	// Call issues one request and decodes its result into result, which may be
	// nil when the caller does not need it.
	Call(ctx context.Context, method string, params any, result any) error
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

// Call sends one request on a connection of its own and reads the single
// response Herdr answers with before closing.
func (c *SocketClient) Call(ctx context.Context, method string, params any, result any) error {
	if params == nil {
		params = emptyParams{}
	}

	var d net.Dialer

	conn, err := d.DialContext(ctx, "unix", c.path)
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
		return withContextErr(ctx, fmt.Errorf("send %s: %w", method, err))
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return withContextErr(ctx, fmt.Errorf("read %s response: %w", method, err))
	}

	var f frame
	if err := json.Unmarshal(line, &f); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
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

// withContextErr reports cancellation as such rather than as the socket error
// that closing the connection produced.
func withContextErr(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	return err
}

// SessionSnapshot fetches the whole session: every tab with its label, every
// pane with its context. There is deliberately no Subscribe beside it — see
// docs/architecture/poll-loop.md.
func SessionSnapshot(ctx context.Context, c Client) (Snapshot, error) {
	var res snapshotResult
	if err := c.Call(ctx, MethodSessionSnapshot, emptyParams{}, &res); err != nil {
		return Snapshot{}, err
	}

	return res.Snapshot, nil
}

// PaneProcesses reads what is running in a pane. PaneInfo has no process name
// and this is the only method that answers one, at 0.11 ms per pane.
func PaneProcesses(ctx context.Context, c Client, paneID string) ([]PaneProcessInfoProcess, error) {
	var res processInfoResult
	if err := c.Call(ctx, MethodPaneProcessInfo, PaneTarget{PaneID: paneID}, &res); err != nil {
		return nil, err
	}

	return res.ProcessInfo.ForegroundProcesses, nil
}

func RenameTab(ctx context.Context, c Client, tabID, label string) error {
	return c.Call(ctx, MethodTabRename, TabRenameParams{TabID: tabID, Label: label}, nil)
}
