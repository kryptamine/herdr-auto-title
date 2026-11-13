package reads_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/reads"
	"github.com/kryptamine/herdr-auto-title/internal/reads/readstest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

type remoteClient struct {
	*herdrtest.Client

	reply []byte
	err   error
	calls int
	input map[string][]byte
}

func (*remoteClient) PaneMachine(paneID string) string {
	if machine, _, remote := strings.Cut(paneID, "/"); remote {
		return machine
	}

	return ""
}

func (c *remoteClient) ExecMachine(
	_ context.Context, machine string, _ []string, input []byte,
) ([]byte, error) {
	c.calls++
	if c.input == nil {
		c.input = make(map[string][]byte)
	}

	c.input[machine] = input

	return c.reply, c.err
}

func remotePane(t *testing.T) (herdr.PaneInfo, *remoteClient) {
	t.Helper()

	dir := readstest.Repo(t, "local-only")
	info := herdr.PaneInfo{PaneID: "box/p", TabID: "box/tb", CWD: dir, Agent: "codex"}
	client := &remoteClient{Client: herdrtest.New(nil, []herdr.PaneInfo{info})}
	client.SetProcesses(info.PaneID, herdr.PaneProcessInfoProcess{
		PID: 123, Name: "codex", Argv: []string{"codex"}, CWD: dir,
	})

	return info, client
}

func TestRemoteReadsNeverUseTheLocalCheckoutOrTranscript(t *testing.T) {
	t.Parallel()

	info, client := remotePane(t)
	home := transcript(
		t,
		"{\"type\":\"ai-title\",\"aiTitle\":\"local-only topic\",\"sessionId\":\""+testSession+"\"}",
	)
	info.AgentSession = &herdr.AgentSessionInfo{
		Agent: "claude", Kind: herdr.SessionRefID, Value: testSession,
	}
	client.reply = []byte(
		"{\"box/p\":{\"topic\":\"native remote name\",\"git\":{\"branch\":\"feature/remote\",\"default\":\"main\",\"common_dir\":\"/remote/.git\"}}}",
	)
	reader := newReader(transcriptOptions(home))
	pane := state.PaneFrom(info, "", time.Time{})
	poll := reader.Poll(client, []herdr.PaneInfo{info}, nil)

	if !poll.Fill(t.Context(), pane) {
		t.Fatal("remote foreground was not read")
	}

	if pane.AgentTopic != "native remote name" || pane.Git.Branch != "feature/remote" {
		t.Errorf("remote topic = %q, branch = %q", pane.AgentTopic, pane.Git.Branch)
	}

	if client.calls != 1 {
		t.Errorf("remote reads = %d, want 1", client.calls)
	}

	poll.Fill(t.Context(), pane)

	if client.calls != 1 {
		t.Errorf("a second fill repeated the remote read: %d", client.calls)
	}
}

func TestFailedRemoteEnrichmentDoesNotReadLocalFiles(t *testing.T) {
	t.Parallel()

	for name, reply := range map[string][]byte{
		"failed exec": nil,
		"bad reply":   []byte("not JSON"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			info, client := remotePane(t)

			client.reply = reply
			if reply == nil {
				client.err = errors.New("remote helper unavailable")
			}

			pane := state.PaneFrom(info, "", time.Time{})
			newReader(
				branchOptions(),
			).Poll(client, []herdr.PaneInfo{info}, nil).
				Fill(t.Context(), pane)

			if pane.Git.Branch != "" || pane.AgentTopic != "" {
				t.Errorf("failed remote read used local files: %+v", pane)
			}
		})
	}
}

func TestRemoteEnrichmentRespectsDisabledReads(t *testing.T) {
	t.Parallel()

	info, client := remotePane(t)
	pane := state.PaneFrom(info, "", time.Time{})
	newReader(reads.Options{}).Poll(client, []herdr.PaneInfo{info}, nil).Fill(t.Context(), pane)

	if client.calls != 0 || pane.Git.Branch != "" || pane.AgentTopic != "" {
		t.Errorf("disabled reads: calls = %d, pane = %+v", client.calls, pane)
	}
}

func TestRemotePanesBatchByMachineAndRefreshEachPoll(t *testing.T) {
	t.Parallel()

	dir := readstest.Repo(t, "local-only")
	infos := []herdr.PaneInfo{
		{PaneID: "box/p1", CWD: dir, Agent: "codex"},
		{PaneID: "box/p2", CWD: dir, Agent: "codex"},
		{PaneID: "other/p1", CWD: dir, Agent: "codex"},
		{PaneID: "p1", CWD: dir},
	}

	client := &remoteClient{Client: herdrtest.New(nil, infos), reply: []byte(`{
		"box/p1":{"topic":"First conversation","git":{"branch":"first","common_dir":"/repo/.git"}},
		"box/p2":{"topic":"Second conversation","git":{"branch":"second","common_dir":"/repo/.git"}},
		"other/p1":{"topic":"Other machine","git":{"branch":"other","common_dir":"/repo/.git"}}
	}`)}
	for _, info := range infos {
		client.SetProcesses(info.PaneID, herdr.PaneProcessInfoProcess{
			PID: 123, Name: "codex", CWD: dir,
		})
	}

	reader := newReader(reads.Options{BranchMax: 24, ReadSessions: true})
	poll := reader.Poll(client, infos, nil)

	panes := make([]*state.PaneState, 0, len(infos))
	for _, info := range infos {
		pane := state.PaneFrom(info, "", time.Time{})
		poll.Fill(t.Context(), pane)
		panes = append(panes, pane)
	}

	if client.calls != 2 || len(client.input) != 2 {
		t.Errorf("remote batches = %d for %d machines, want 2", client.calls, len(client.input))
	}

	for machine, input := range client.input {
		var requests map[string]json.RawMessage
		if err := json.Unmarshal(input, &requests); err != nil {
			t.Fatal(err)
		}

		if want := (map[string]int{"box": 2, "other": 1})[machine]; len(requests) != want {
			t.Errorf("%s batch contains %d panes, want %d", machine, len(requests), want)
		}

		for paneID := range requests {
			if !strings.HasPrefix(paneID, machine+"/") {
				t.Errorf("%s batch contains another machine's pane: %s", machine, paneID)
			}
		}
	}

	if panes[0].AgentTopic != "First conversation" ||
		panes[1].AgentTopic != "Second conversation" ||
		panes[2].AgentTopic != "Other machine" ||
		panes[3].Git.Branch != "local-only" {
		t.Errorf("machine or pane replies crossed: %+v", panes)
	}

	if panes[0].Git.CommonDir == panes[2].Git.CommonDir {
		t.Error("identical repository paths on different machines share a checkout identity")
	}

	client.reply = []byte(`{"box/p1":{"topic":"Resumed conversation","git":{"branch":"changed"}}}`)
	current := state.PaneFrom(infos[0], "", time.Time{})
	reader.Poll(client, infos, nil).Fill(t.Context(), current)

	if client.calls != 3 || current.AgentTopic != "Resumed conversation" ||
		current.Git.Branch != "changed" {
		t.Errorf("next poll reused old remote data: calls = %d, pane = %+v", client.calls, current)
	}
}

func TestRemoteReadSettingsAreIndependent(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		opts reads.Options
	}{
		{name: "branches only", opts: reads.Options{BranchMax: 24}},
		{name: "sessions only", opts: reads.Options{ReadSessions: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			info, client := remotePane(t)
			client.reply = []byte(`{"box/p":{"topic":"Remote name","git":{"branch":"remote"}}}`)
			pane := state.PaneFrom(info, "", time.Time{})
			newReader(test.opts).Poll(client, []herdr.PaneInfo{info}, nil).Fill(t.Context(), pane)

			if (pane.AgentTopic != "") != test.opts.ReadSessions ||
				(pane.Git.Branch != "") != (test.opts.BranchMax > 0) {
				t.Errorf("disabled data entered the pane: %+v", pane)
			}

			var requests map[string]struct {
				ReadSessions bool `json:"read_sessions"`
				ReadBranches bool `json:"read_branches"`
			}
			if err := json.Unmarshal(client.input["box"], &requests); err != nil {
				t.Fatal(err)
			}

			request := requests[info.PaneID]
			if request.ReadSessions != test.opts.ReadSessions ||
				request.ReadBranches != (test.opts.BranchMax > 0) {
				t.Errorf("disabled reads were sent to the helper: %+v", request)
			}
		})
	}
}
