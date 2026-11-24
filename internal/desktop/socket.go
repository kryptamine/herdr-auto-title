package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// coreEndpoint is the core hc reaches from the same environment: HC_SOCKET
// names its socket outright, else HC_SESSION names its session.
func coreEndpoint(home string) string {
	if socket := os.Getenv("HC_SOCKET"); socket != "" {
		return socket
	}

	session := os.Getenv("HC_SESSION")
	if session == "" {
		session = "default"
	}

	return coreSocketPath(home, session)
}

// ask sends one JSON line on a connection of its own and decodes the line that
// answers it. A failure after sending may still take effect, as on Herdr's.
func ask(ctx context.Context, path string, request, reply any) error {
	conn, err := dial(ctx, path)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", path, err)
	}
	defer func() { _ = conn.Close() }()

	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()

	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("%w: send to %s: %w", herdr.ErrUnanswered, path, err)
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return fmt.Errorf("%w: read from %s: %w", herdr.ErrUnanswered, path, err)
	}

	if err := json.Unmarshal(line, reply); err != nil {
		return fmt.Errorf("%w: decode from %s: %w", herdr.ErrUnanswered, path, err)
	}

	return nil
}

// coreError is the core refusing a request, as opposed to not answering it.
// kind is the refusal's data.kind, such as "conflict" for a stale write.
type coreError struct {
	method, message, kind string
}

func (e *coreError) Error() string { return e.method + ": " + e.message }

// rpc asks the core one JSON-RPC question on a connection of its own. A reply
// with no result leaves result as it was.
func rpc(ctx context.Context, path, method string, params, result any) error {
	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}

	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
			Data    struct {
				Kind string `json:"kind"`
			} `json:"data"`
		} `json:"error"`
	}

	if err := ask(ctx, path, request, &reply); err != nil {
		return err
	}

	if reply.Error != nil {
		return &coreError{method: method, message: reply.Error.Message, kind: reply.Error.Data.Kind}
	}

	if result == nil || len(reply.Result) == 0 {
		return nil
	}

	if err := json.Unmarshal(reply.Result, result); err != nil {
		return fmt.Errorf("%w: decode %s: %w", herdr.ErrUnanswered, method, err)
	}

	return nil
}
