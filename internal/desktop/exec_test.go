package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"maps"
	"testing"
	"time"
)

func TestMachineExecKeepsArgumentsAndInputOutOfAShell(t *testing.T) {
	t.Parallel()

	reply := fixtureJSON(t, map[string]any{"result": map[string]any{
		"code": 0, "stdout": []byte("metadata"),
	}})
	core := serve(t, func(map[string]any) string { return reply })
	client := &Client{core: core.path}
	input := []byte("a folder's name with $(literal) characters")

	output, err := client.ExecMachine(
		t.Context(),
		"box",
		[]string{"python3", "-c", "print('ok')"},
		input,
	)
	if err != nil || !bytes.Equal(output, []byte("metadata")) {
		t.Fatalf("exec output = %q, %v", output, err)
	}

	request := core.last()
	params, _ := request["params"].(map[string]any)
	encoded, _ := params["input"].(string)

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !bytes.Equal(decoded, input) || params["on"] != "box" ||
		params["line"] != nil {
		t.Errorf("input or routing changed: %+v, %v", params, err)
	}

	timeout, _ := params["timeout"].(float64)
	if timeout <= 0 || timeout > machineExecTimeout.Seconds() {
		t.Errorf("server deadline = %v", timeout)
	}
}

func TestMachineExecRejectsPartialOrFailedOutput(t *testing.T) {
	t.Parallel()

	for name, fields := range map[string]map[string]any{
		"nonzero":   {"code": 1},
		"signaled":  {"code": nil},
		"timed out": {"timed_out": true},
		"truncated": {"truncated": true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := map[string]any{"code": 0, "stdout": []byte("partial")}
			maps.Copy(result, fields)

			reply := fixtureJSON(t, map[string]any{"result": result})
			core := serve(t, func(map[string]any) string { return reply })

			client := &Client{core: core.path}
			if raw, err := client.ExecMachine(
				t.Context(),
				"box",
				[]string{"python3"},
				nil,
			); err == nil ||
				raw != nil {
				t.Errorf("failed output accepted: %q, %v", raw, err)
			}
		})
	}
}

func TestCancelledMachineExecSendsNothing(t *testing.T) {
	t.Parallel()

	core := serve(t, func(map[string]any) string { return "{}" })
	client := &Client{core: core.path}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := client.ExecMachine(
		ctx,
		"box",
		[]string{"python3"},
		nil,
	); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Errorf("cancelled exec = %v", err)
	}

	core.mu.Lock()
	seen := len(core.seen)
	core.mu.Unlock()

	if seen != 0 {
		t.Errorf("cancelled exec sent %d requests", seen)
	}
}

func TestMachineExecUsesTheCallersShorterDeadline(t *testing.T) {
	t.Parallel()

	reply := fixtureJSON(t, map[string]any{"result": map[string]any{"code": 0}})
	core := serve(t, func(map[string]any) string { return reply })
	client := &Client{core: core.path}

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	if _, err := client.ExecMachine(ctx, "box", []string{"python3"}, nil); err != nil {
		t.Fatal(err)
	}

	params, _ := core.last()["params"].(map[string]any)

	timeout, _ := params["timeout"].(float64)
	if timeout <= 0 || timeout > 0.5 {
		t.Errorf("server deadline = %v, want no more than the caller's 0.5 seconds", timeout)
	}
}
