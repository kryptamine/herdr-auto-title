package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// ask sends one JSON line on a connection of its own and decodes the line that
// answers it. A failure after sending may still take effect, as on Herdr's.
func ask(ctx context.Context, path string, request, reply any) error {
	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "unix", path)
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
