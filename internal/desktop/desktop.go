// Package desktop answers the Herdr client interface from the desktop app and
// herdr-core, so the rest of Auto Title names their tabs as it names Herdr
// TUI's. What each socket reports is in docs/architecture/desktop-api.md.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

const (
	appHomeEnv = "HERDR_APP_HOME"
	appSocket  = "herdr-app.sock"

	// A tab.rename sends these spellings back: the app has one way to say a tab
	// is gone, and one to say the window took too long, whose request stays queued.
	noTabPrefix = "no tab "
	noAnswer    = "the window did not answer"
)

// errNoEquivalent answers what the app has no place for, which the desktop mode's
// configuration never asks of it.
var errNoEquivalent = errors.New("the desktop app has no equivalent")

// Client speaks to the app's socket and the core's. Both answer one request
// per connection here, so like the TUI client it has nothing to reconnect.
type Client struct {
	app  string
	core string

	mu sync.Mutex
	// foregrounds is what each pane was running at the last snapshot.
	foregrounds map[string]herdr.PaneProcessInfoProcess

	sessions *sessionFinder
}

var _ herdr.Client = (*Client)(nil)

// New builds a client for this user's app and core. It performs no I/O: the
// window comes and goes, and a poll finding it closed simply fails.
func New(claudeDirs []string) (*Client, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.New("the desktop mode does not support Windows yet")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate the home directory: %w", err)
	}

	return &Client{
		app:      appSocketPath(home),
		core:     coreSocketPath(home),
		sessions: newSessionFinder(claudeDirs),
	}, nil
}

func appSocketPath(home string) string {
	if dir := os.Getenv(appHomeEnv); dir != "" {
		return filepath.Join(dir, appSocket)
	}

	return filepath.Join(home, ".config", "herdr-app", appSocket)
}

// coreSocketPath follows the rules in herdr-core's docs/security.md for its
// default session.
func coreSocketPath(home string) string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		candidate := fmt.Sprintf("/run/user/%d", os.Getuid())
		if _, err := os.Stat(candidate); err == nil {
			runtimeDir = candidate
		}
	}

	if runtimeDir != "" {
		return filepath.Join(runtimeDir, "herdr-core", "default.sock")
	}

	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(state, "herdr-core", "default", "default.sock")
}

// Server is empty because a window restart is not a successor: nothing else
// starts an Auto Title for the desktop app, so this one must stay.
func (*Client) Server() string { return "" }

// Path is the app's socket, which is what names this session's claim.
func (c *Client) Path() string { return c.app }

// Snapshot joins the app's view to the core's. What each pane runs is kept for
// PaneProcesses: terminal.list already carries it, at no extra cost.
func (c *Client) Snapshot(ctx context.Context) (herdr.Snapshot, error) {
	var world world
	if err := ask(ctx, c.app, map[string]bool{"ls": true}, &world); err != nil {
		return herdr.Snapshot{}, err
	}

	var status appStatus
	if err := ask(ctx, c.app, map[string]bool{"status": true}, &status); err != nil {
		return herdr.Snapshot{}, err
	}

	terminals, err := c.terminals(ctx)
	if err != nil {
		return herdr.Snapshot{}, err
	}

	local := world.local(terminals)
	snapshot, foregrounds := build(local, status.FocusedPane, c.sessions.read(ctx, panesIn(local)))

	c.mu.Lock()
	c.foregrounds = foregrounds
	c.mu.Unlock()

	return snapshot, nil
}

// PaneProcesses reports the pane's foreground process, the only one the core
// names. A pane that has only just started reports none yet.
func (c *Client) PaneProcesses(
	_ context.Context,
	paneID string,
) ([]herdr.PaneProcessInfoProcess, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	process, known := c.foregrounds[paneID]
	if !known {
		return nil, &herdr.APIError{Code: herdr.CodePaneNotFound, Message: "no pane " + paneID}
	}

	if process.PID == 0 {
		return nil, nil
	}

	return []herdr.PaneProcessInfoProcess{process}, nil
}

func (*Client) RenamePane(context.Context, string, string) error { return errNoEquivalent }

func (*Client) ReportWorkspaceTopic(
	context.Context,
	string, string, string,
	time.Duration,
) error {
	return errNoEquivalent
}

func (c *Client) RenameTab(ctx context.Context, tabID, label string) error {
	request := map[string]any{
		"do":   "tab.rename",
		"args": map[string]string{"id": tabID, "name": label},
	}

	var reply struct {
		Error string `json:"error"`
	}

	if err := ask(ctx, c.app, request, &reply); err != nil {
		return err
	}

	switch {
	case reply.Error == "":
		return nil
	case strings.HasPrefix(reply.Error, noTabPrefix):
		return &herdr.APIError{Code: herdr.CodeTabNotFound, Message: reply.Error}
	case reply.Error == noAnswer:
		return fmt.Errorf("%w: %s", herdr.ErrUnanswered, reply.Error)
	default:
		return fmt.Errorf("tab.rename: %s", reply.Error)
	}
}

func (c *Client) terminals(ctx context.Context) ([]terminal, error) {
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "terminal.list",
		"params":  map[string]any{},
	}

	var reply struct {
		Result struct {
			Terminals []terminal `json:"terminals"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := ask(ctx, c.core, request, &reply); err != nil {
		return nil, err
	}

	if reply.Error != nil {
		return nil, fmt.Errorf("terminal.list: %s", reply.Error.Message)
	}

	return reply.Result.Terminals, nil
}
