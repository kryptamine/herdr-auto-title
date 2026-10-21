//go:build linux

package reads_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/reads"
)

func TestHermesProcessHomeOverridesSearch(t *testing.T) {
	t.Parallel()
	home, custom := t.TempDir(), t.TempDir()
	hermesDB(t, filepath.Join(home, ".hermes"), "Wrong profile")
	hermesDB(t, custom, "Process profile title")

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ready, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = ready.Close() })

	child, err := os.StartProcess(
		exe,
		[]string{exe, "-test.run=^TestHermesHomeFixture$"},
		&os.ProcAttr{
			Env:   []string{"HERMES_HOME=" + custom, "HERDR_TITLE_TEST_CHILD=1"},
			Files: []*os.File{nil, writer, nil},
		},
	)
	_ = writer.Close()

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = child.Kill(); _, _ = child.Wait() })

	if _, err := io.ReadFull(ready, make([]byte, len("ready"))); err != nil {
		t.Fatal(err)
	}

	pane := hermesPane()
	info := herdr.PaneInfo{PaneID: pane.ID}
	client := herdrtest.New(nil, []herdr.PaneInfo{info})
	client.SetProcesses(
		pane.ID,
		herdr.PaneProcessInfoProcess{PID: child.Pid, Name: "hermes", Argv: []string{"hermes"}},
	)

	reader := newReader(reads.Options{Home: home, ReadSessions: true})
	reader.Poll(client, []herdr.PaneInfo{info}, nil).Fill(context.Background(), pane)

	if pane.AgentTopic != "Process profile title" {
		t.Fatalf("topic = %q, want process HERMES_HOME", pane.AgentTopic)
	}
}

func TestHermesHomeFixture(t *testing.T) {
	t.Parallel()

	if os.Getenv("HERDR_TITLE_TEST_CHILD") == "1" {
		if _, err := os.Stdout.WriteString("ready"); err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Minute)
	}
}
