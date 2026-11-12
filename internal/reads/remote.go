package reads

import (
	"context"
	_ "embed"
	"encoding/json"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/codex"
	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// A machine round trip can exceed 500 ms even for file reads; a separate
// deadline keeps that wait from consuming the whole poll.
const remoteReadTimeout = time.Second

//go:embed remote.py
var remoteScript string

type machineClient interface {
	PaneMachine(paneID string) string
	ExecMachine(ctx context.Context, machine string, argv []string, input []byte) ([]byte, error)
}

type remoteRequest struct {
	PID          int    `json:"pid"`
	Dir          string `json:"dir"`
	ReadSessions bool   `json:"read_sessions"`
	ReadBranches bool   `json:"read_branches"`
}

type remoteReply struct {
	Topic string         `json:"topic"`
	Git   remoteCheckout `json:"git"`
}

type remoteCheckout struct {
	Branch    string `json:"branch"`
	Commit    string `json:"commit"`
	Default   string `json:"default"`
	CommonDir string `json:"common_dir"`
}

// fillRemote owns all filesystem reads for a remote pane, even when the helper
// is unavailable. Falling through would read a same-named local checkout.
func (p *Poll) fillRemote(
	ctx context.Context,
	pane *state.PaneState,
	processes []herdr.PaneProcessInfoProcess,
) bool {
	client, supported := p.client.(machineClient)
	if !supported {
		return false
	}

	machine := client.PaneMachine(pane.ID)
	if machine == "" {
		return false
	}

	readSessions := p.reader.readSessions && pane.Agent == codex.Agent
	if (!readSessions && !p.reader.readBranches) || spent(ctx) {
		return true
	}

	replies, done := p.remoteReads[machine]
	if !done {
		requests := p.remoteRequests(ctx, client, machine, pane, processes)

		var err error

		replies, err = readRemote(ctx, client, machine, requests)

		p.remoteReads[machine] = replies
		if err != nil {
			p.reader.log.Debug(
				"could not read remote pane metadata",
				"machine",
				machine,
				"error",
				err,
			)
		}
	}

	reply := replies[pane.ID]
	if readSessions {
		pane.AgentTopic = reply.Topic
	}

	if p.reader.readBranches {
		pane.Git = reply.Git.checkout(machine)
	}

	return true
}

func (p *Poll) remoteRequests(
	ctx context.Context,
	client machineClient,
	machine string,
	current *state.PaneState,
	currentProcesses []herdr.PaneProcessInfoProcess,
) map[string]remoteRequest {
	requests := make(map[string]remoteRequest)

	for _, info := range p.panes {
		if client.PaneMachine(info.PaneID) != machine {
			continue
		}

		processes := currentProcesses

		if info.PaneID != current.ID {
			var err error

			processes, err = p.client.PaneProcesses(ctx, info.PaneID)
			if err != nil {
				continue
			}
		}

		request := remoteRequest{
			Dir:          state.PaneDir(processes, info.CWD),
			ReadSessions: p.reader.readSessions && info.Agent == codex.Agent,
			ReadBranches: p.reader.readBranches,
		}
		if last := len(processes) - 1; last >= 0 {
			request.PID = processes[last].PID
		}

		requests[info.PaneID] = request
	}

	return requests
}

func readRemote(
	ctx context.Context,
	client machineClient,
	machine string,
	requests map[string]remoteRequest,
) (map[string]remoteReply, error) {
	ctx, cancel := context.WithTimeout(ctx, remoteReadTimeout)
	defer cancel()

	input, err := json.Marshal(requests)
	if err != nil {
		return nil, err
	}

	raw, err := client.ExecMachine(ctx, machine, []string{"python3", "-c", remoteScript}, input)
	if err != nil {
		return nil, err
	}

	var reply map[string]remoteReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}

	return reply, nil
}

func (c remoteCheckout) checkout(machine string) git.Checkout {
	common := c.CommonDir
	if common != "" {
		common = machine + "/" + common
	}

	return git.Checkout{Branch: c.Branch, Commit: c.Commit, Default: c.Default, CommonDir: common}
}
