package desktop

import (
	"context"
	"fmt"
	"time"
)

const machineExecTimeout = time.Second

// ExecMachine uses the core's machine connection, with argv and stdin only.
// Its server-side deadline also ends the helper if this connection gives up.
func (c *Client) ExecMachine(
	ctx context.Context,
	machine string,
	argv []string,
	input []byte,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	timeout := machineExecTimeout.Seconds()
	if deadline, bounded := ctx.Deadline(); bounded {
		timeout = min(timeout, time.Until(deadline).Seconds())
	}

	if timeout <= 0 {
		return nil, context.DeadlineExceeded
	}

	request := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "machine.exec",
		"params": map[string]any{
			"on": machine, "argv": argv, "input": input, "timeout": timeout,
		},
	}

	var reply struct {
		Result struct {
			Stdout    []byte `json:"stdout"`
			Code      *int   `json:"code"`
			TimedOut  bool   `json:"timed_out"`
			Truncated bool   `json:"truncated"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := ask(ctx, c.core, request, &reply); err != nil {
		return nil, err
	}

	if reply.Error != nil {
		return nil, fmt.Errorf("machine.exec: %s", reply.Error.Message)
	}

	result := reply.Result
	if result.Code == nil || *result.Code != 0 || result.TimedOut || result.Truncated {
		return nil, fmt.Errorf("machine.exec on %s did not complete", machine)
	}

	return result.Stdout, nil
}
