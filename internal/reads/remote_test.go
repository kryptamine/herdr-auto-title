package reads_test

import (
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/reads/readstest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// A connected machine's paths and sessions can match this computer's by chance,
// and reading them here would name a remote tab after a local checkout.
func TestRemoteReadsNeverUseTheLocalCheckoutOrTranscript(t *testing.T) {
	t.Parallel()

	dir := readstest.Repo(t, "local-only")
	home := transcript(
		t,
		"{\"type\":\"ai-title\",\"aiTitle\":\"local-only topic\",\"sessionId\":\""+testSession+"\"}",
	)
	info := herdr.PaneInfo{
		PaneID: "box/p", TabID: "box/tb", CWD: dir, Agent: "claude", Machine: "box",
		AgentSession: &herdr.AgentSessionInfo{
			Agent: "claude", Kind: herdr.SessionRefID, Value: testSession,
		},
	}
	client := herdrtest.New(nil, []herdr.PaneInfo{info})
	client.SetProcesses(info.PaneID, herdr.PaneProcessInfoProcess{
		PID: 123, Name: "claude", Argv: []string{"claude"}, CWD: dir,
	})

	pane := state.PaneFrom(info, "", time.Time{})
	if !newReader(transcriptOptions(home)).Poll(client, []herdr.PaneInfo{info}, nil).
		Fill(t.Context(), pane) {
		t.Fatal("remote foreground was not read")
	}

	if pane.Dir != dir || len(pane.Processes) != 1 {
		t.Errorf("remote terminal metadata was lost: %+v", pane)
	}

	if pane.AgentTopic != "" || pane.Git.Branch != "" || pane.AgentGit.Branch != "" {
		t.Errorf(
			"remote pane read local files: topic %q, branch %q",
			pane.AgentTopic,
			pane.Git.Branch,
		)
	}
}
