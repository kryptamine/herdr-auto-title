package reads_test

import (
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/reads/readstest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func TestSessionOnlyClaudeRequiresName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, line, want string }{
		{"opening", "", ""},
		{"generated", `{"type":"ai-title","aiTitle":"Generated name","sessionId":"` + testSession + `"}`, "Generated name"},
		{"custom", `{"type":"custom-title","customTitle":"Chosen name","sessionId":"` + testSession + `"}`, "Chosen name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := transcript(
				t,
				`{"type":"user","origin":{"kind":"human"},"message":{"role":"user","content":"raw opening prompt"}}`,
				tc.line,
			)
			opts := transcriptOptions(home)
			opts.SessionOnly = true
			pane := agentPaneAt("wE:p1", testSession, t.TempDir())
			readOne(opts, pane)

			if pane.AgentTopic != tc.want {
				t.Fatalf("topic = %q, want %q", pane.AgentTopic, tc.want)
			}
		})
	}
}

func TestSessionOnlyCodexRequiresIndexName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "Indexed name"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := codexOptions(t)
			opts.SessionOnly = true
			readstest.CodexRollout(
				t,
				opts.CodexHome,
				testSession,
				readstest.CodexPrompt("raw opening prompt"),
			)

			if name != "" {
				readstest.CodexName(t, opts.CodexHome, testSession, name)
			}

			pane := codexPaneAt(t.TempDir(), "")
			readOne(opts, pane)

			if pane.AgentTopic != name {
				t.Fatalf("topic = %q, want %q", pane.AgentTopic, name)
			}
		})
	}
}

func TestSessionOnlyPiRequiresName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, line, want string }{
		{"opening", "", ""},
		{"named", `{"type":"session_info","name":"Chosen name"}`, "Chosen name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := readstest.PiSession(
				t,
				readstest.PiStartedIn(t.TempDir()),
				readstest.PiPrompt("raw opening prompt"),
				tc.line,
			)
			opts := transcriptOptions()
			opts.SessionOnly = true
			pane := piPaneAt("wE:p2", path, t.TempDir())
			readOne(opts, pane)

			if pane.AgentTopic != tc.want {
				t.Fatalf("topic = %q, want %q", pane.AgentTopic, tc.want)
			}
		})
	}
}

func TestSessionOnlyCavemanRequiresName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, line, want string }{
		{"opening", "", ""},
		{"named", `{"type":"session_info","name":"Chosen name"}`, "Chosen name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := readstest.PiSession(
				t,
				readstest.PiStartedIn(dir),
				readstest.PiPrompt("raw opening prompt"),
				tc.line,
			)
			opts := transcriptOptions()
			opts.Home = t.TempDir()
			opts.SessionOnly = true
			readstest.CavemanSidecar(t, opts.Home, cavemanPID, path)

			pane := state.PaneFrom(cavemanInfoAt("wE:p2", dir), "", time.Time{})
			readCavemanOne(opts, pane)

			if pane.AgentTopic != tc.want {
				t.Fatalf("topic = %q, want %q", pane.AgentTopic, tc.want)
			}
		})
	}
}
