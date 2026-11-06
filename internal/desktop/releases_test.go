package desktop

import (
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// A release offered where there is no mod is logged and done with, while a
// slow or closed window must fail the show, so that the next check offers it
// again.
func TestShowModTreatsOnlyARefusalAsDone(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)

	var reply atomic.Value

	app := serve(t, func(map[string]any) string {
		answer, _ := reply.Load().(string)
		return answer
	})
	client := &Client{app: app.path}

	reply.Store(`{"ok":true}`)

	if err := client.showMod(t.Context(), log); err != nil {
		t.Fatalf("open: %v", err)
	}

	args, _ := app.last()["args"].(map[string]any)
	if app.last()["do"] != "mod.open" || args["name"] != modName {
		t.Errorf("sent %v", app.last())
	}

	reply.Store(`{"error":"no mod folder \"auto-title\""}`)

	if err := client.showMod(t.Context(), log); err != nil {
		t.Errorf("no mod: %v", err)
	}

	reply.Store(`{"error":"the window did not answer"}`)

	if err := client.showMod(t.Context(), log); err == nil {
		t.Error("slow window: no error")
	}

	closed := &Client{app: filepath.Join(t.TempDir(), "gone.sock")}
	if err := closed.showMod(t.Context(), log); err == nil {
		t.Error("closed window: no error")
	}
}
