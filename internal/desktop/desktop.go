// Package desktop answers the Herdr client interface from the desktop app and
// herdr-core, so the rest of Auto Title names their tabs as it names Herdr
// TUI's, and offers newer releases: docs/architecture/desktop-api.md.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// Client speaks to the app's socket and the core's. Both answer one request
// per connection here, so like the TUI client it has nothing to reconnect.
type Client struct {
	app    string
	core   string
	layout layout

	mu sync.Mutex
	// foregrounds is what each pane was running at the last snapshot.
	foregrounds map[string]herdr.PaneProcessInfoProcess
	// initialLabels holds the first label until it changes; an empty value
	// means a launcher's name must never be inferred again for that tab.
	initialLabels map[string]string
	// focused is the pane each tab last had focused.
	focused map[string]string

	sessions *sessionFinder
}

var _ herdr.Client = (*Client)(nil)

// New builds a client for this user's app and core. It performs no I/O: the
// window comes and goes, and a poll finding it closed simply fails.
func New(claudeDirs []string, coreLayout bool) (*Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate the home directory: %w", err)
	}

	client := newClient(appSocketPath(home), coreSocketPath(home), coreLayout)
	client.sessions = newSessionFinder(claudeDirs)

	return client, nil
}

// newClient reads the layout from the app, or from the core's own document
// when no window runs beside it.
func newClient(app, core string, coreLayout bool) *Client {
	var from layout = appLayout{app: app, core: core}
	if coreLayout {
		from = docLayout{core: core}
	}

	return &Client{app: app, core: core, layout: from}
}

// layout is where the spaces and tabs come from and where a tab is renamed.
type layout interface {
	// read is the spaces joined to the core's terminals, and the focused pane.
	read(ctx context.Context) ([]space, string, error)
	renameTab(ctx context.Context, tabID, label string) error
	claim() string
}

func appSocketPath(home string) string {
	if dir := os.Getenv(appHomeEnv); dir != "" {
		return filepath.Join(dir, appSocket)
	}

	return filepath.Join(home, ".config", "herdr-app", appSocket)
}

// Server is empty because a window restart is not a successor: nothing else
// starts an Auto Title for the desktop app, so this one must stay.
func (*Client) Server() string { return "" }

// Path is the socket that names this session's claim.
func (c *Client) Path() string { return c.layout.claim() }

// Snapshot joins the app's view to the core's. What each pane runs is kept for
// PaneProcesses: terminal.list already carries it, at no extra cost.
func (c *Client) Snapshot(ctx context.Context) (herdr.Snapshot, error) {
	spaces, focusedPane, err := c.layout.read(ctx)
	if err != nil {
		return herdr.Snapshot{}, err
	}

	sessions := c.sessions.read(ctx, localPanesIn(spaces))

	c.mu.Lock()
	snapshot, foregrounds := c.build(spaces, focusedPane, sessions)
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

func (c *Client) RenameTab(ctx context.Context, tabID, label string) error {
	return c.layout.renameTab(ctx, tabID, label)
}

// appLayout is the window's own view, over the app's socket.
type appLayout struct {
	app  string
	core string
}

func (l appLayout) claim() string { return l.app }

func (l appLayout) read(ctx context.Context) ([]space, string, error) {
	var world world
	if err := ask(ctx, l.app, map[string]bool{"ls": true}, &world); err != nil {
		return nil, "", err
	}

	if err := appError("ls", world.Error); err != nil {
		return nil, "", err
	}

	var status appStatus
	if err := ask(ctx, l.app, map[string]bool{"status": true}, &status); err != nil {
		return nil, "", err
	}

	if err := appError("status", status.Error); err != nil {
		return nil, "", err
	}

	terminals, err := listTerminals(ctx, l.core)
	if err != nil {
		return nil, "", err
	}

	return world.join(terminals), status.FocusedPane, nil
}

func (l appLayout) renameTab(ctx context.Context, tabID, label string) error {
	request := map[string]any{
		"do":   "tab.rename",
		"args": map[string]string{"id": tabID, "name": label},
	}

	var reply struct {
		Error string `json:"error"`
	}

	if err := ask(ctx, l.app, request, &reply); err != nil {
		return err
	}

	if strings.HasPrefix(reply.Error, noTabPrefix) {
		return &herdr.APIError{Code: herdr.CodeTabNotFound, Message: reply.Error}
	}

	return appError("tab.rename", reply.Error)
}

// errRefused is the app's answer that it will not do what was asked, as
// opposed to no answer at all.
var errRefused = errors.New("the desktop app refused")

// appError is the app's error reply as an error: a slow window is unanswered,
// anything else a refusal. An `ls` or `status` that failed must fail the poll:
// read as an empty session, it drops every lock.
func appError(request, message string) error {
	switch message {
	case "":
		return nil
	case noAnswer:
		return fmt.Errorf("%w: %s: %s", herdr.ErrUnanswered, request, message)
	default:
		return fmt.Errorf("%w: %s: %s", errRefused, request, message)
	}
}

func listTerminals(ctx context.Context, core string) ([]terminal, error) {
	var listed struct {
		Terminals []terminal `json:"terminals"`
	}

	if err := rpc(ctx, core, "terminal.list", map[string]any{}, &listed); err != nil {
		return nil, err
	}

	return listed.Terminals, nil
}
