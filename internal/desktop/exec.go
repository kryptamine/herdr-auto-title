package desktop

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ExecMachine uses the core's machine connection, with argv and stdin only.
// The caller's deadline is sent too, so the helper ends if this side gives up.
func (c *Client) ExecMachine(
	ctx context.Context,
	machine string,
	argv []string,
	input []byte,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	deadline, bounded := ctx.Deadline()
	if !bounded {
		return nil, errors.New("machine.exec needs a deadline")
	}

	timeout := time.Until(deadline).Seconds()
	if timeout <= 0 {
		return nil, context.DeadlineExceeded
	}

	params := map[string]any{"on": machine, "argv": argv, "input": input, "timeout": timeout}

	var result struct {
		Stdout    []byte `json:"stdout"`
		Code      *int   `json:"code"`
		TimedOut  bool   `json:"timed_out"`
		Truncated bool   `json:"truncated"`
	}

	if err := rpc(ctx, c.core, "machine.exec", params, &result); err != nil {
		return nil, err
	}

	if result.Code == nil || *result.Code != 0 || result.TimedOut || result.Truncated {
		return nil, fmt.Errorf("machine.exec on %s did not complete", machine)
	}

	return result.Stdout, nil
}
