package reads

import (
	"context"
	"strconv"
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/codex"
	"github.com/kryptamine/herdr-auto-title/internal/files"
	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// maxCmdline bounds what is read of a process's argv.
const maxCmdline = 4 << 10

// machineExec is a client that can run a program on a connected machine.
type machineExec interface {
	ExecMachine(ctx context.Context, machine string, argv []string, input []byte) ([]byte, error)
}

// remote is what is kept of one connected machine: its files as last fetched,
// a Codex reader per home found there, and each pane's last whole read.
type remote struct {
	files *files.Machine
	codex map[string]*codex.Reader
	held  []string
	panes map[string]remoteRead
}

// remoteRead is what a pane's last read that saw every file it asked for said.
type remoteRead struct {
	topic         string
	git, agentGit git.Checkout
	agentDir      string
}

func newRemote() *remote {
	return &remote{
		files: files.NewMachine(),
		codex: make(map[string]*codex.Reader),
		panes: make(map[string]remoteRead),
	}
}

// retainRemotes forgets the machines, panes and Codex sessions the snapshot no
// longer holds.
func (r *Reader) retainRemotes(panes []herdr.PaneInfo) {
	held := make(map[string]map[string]bool)

	for _, pane := range panes {
		if pane.Machine == "" {
			continue
		}

		if held[pane.Machine] == nil {
			held[pane.Machine] = make(map[string]bool)
		}

		held[pane.Machine][pane.PaneID] = true
	}

	for name, machine := range r.remotes {
		if held[name] == nil {
			delete(r.remotes, name)

			continue
		}

		for pane := range machine.panes {
			if !held[name][pane] {
				delete(machine.panes, pane)
			}
		}

		// The sessions the last poll found are the ones still held.
		for _, reader := range machine.codex {
			reader.Retain(machine.held)
		}

		machine.held = nil
	}
}

// remoteSource is the connected machine's files, asked at most once a poll,
// or nil when nothing would be read there.
func (p *Poll) remoteSource(ctx context.Context, name string) *remote {
	client, supported := p.client.(machineExec)
	if !supported || (!p.reader.readBranches && !p.reader.readSessions) {
		return nil
	}

	machine, known := p.reader.remotes[name]
	if !known {
		machine = newRemote()
		p.reader.remotes[name] = machine
	}

	if !p.fetched[name] && !spent(ctx) {
		p.fetched[name] = true

		exec := func(ctx context.Context, argv []string, input []byte) ([]byte, error) {
			return client.ExecMachine(ctx, name, argv, input)
		}
		if err := machine.files.Fetch(ctx, exec); err != nil && ctx.Err() == nil {
			p.reader.log.Debug("could not read a connected machine", "machine", name, "error", err)
		}
	}

	return machine
}

// misses counts what readers could not be told on machine so far, and is zero
// for this computer, where every question is answered.
func (p *Poll) misses(machine string) int {
	if remote, known := p.reader.remotes[machine]; known {
		return remote.files.Misses()
	}

	return 0
}

// remoteTopic reads what the Codex session a remote pane's foreground process
// holds is about, through the same reader a local session takes.
func (r *Reader) remoteTopic(
	ctx context.Context,
	machine *remote,
	pane *state.PaneState,
	processes []herdr.PaneProcessInfoProcess,
) sessionTopic {
	if !r.readSessions || spent(ctx) || pane.Agent != codex.Agent || len(processes) == 0 {
		return sessionTopic{}
	}

	proc := "/proc/" + strconv.Itoa(processes[len(processes)-1].PID)

	cmdline, err := machine.files.ReadAt(proc+"/cmdline", 0, maxCmdline)
	if err != nil {
		return sessionTopic{}
	}

	open, err := machine.files.Links(proc + "/fd")
	if err != nil {
		return sessionTopic{}
	}

	argv := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")

	sessionID, home, held := codex.HeldSession(machine.files, argv, open)
	if !held {
		return sessionTopic{}
	}

	machine.held = append(machine.held, sessionID)

	reader, known := machine.codex[home]
	if !known {
		reader = codex.NewReader(machine.files, home)
		machine.codex[home] = reader
	}

	topic := reader.Topic(sessionID)
	if !reader.Shows(pane.TerminalTitle, topic) {
		return sessionTopic{}
	}

	return r.namedTopic(topic)
}
