// Package herdrtest provides an in-memory Herdr client for tests. It sits
// beside the client rather than inside it so that the package the plugin ships
// exports nothing only a test reads.
package herdrtest

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// The objects Herdr wraps its answers in. They are declared here rather than
// shared with the client: a stub borrowing the client's own wrappers could not
// catch the two disagreeing about the wire.
type (
	snapshotResult struct {
		Snapshot herdr.Snapshot `json:"snapshot"`
	}

	processInfoResult struct {
		ProcessInfo herdr.PaneProcessInfo `json:"process_info"`
	}
)

// RenameCall records one tab.rename issued through a Client.
type RenameCall struct {
	TabID string
	Label string
}

// Client is an in-memory herdr.Client. Tests change the session it describes
// and inspect the renames it received.
type Client struct {
	mu         sync.Mutex
	workspaces []herdr.WorkspaceInfo
	tabs       map[string]herdr.TabInfo
	panes      map[string]herdr.PaneInfo
	processes  map[string][]herdr.PaneProcessInfoProcess
	renames    []RenameCall
	renameErr  error
	processErr error
	callErr    error
	polls      int
	reads      int
}

var _ herdr.Client = (*Client)(nil)

// New returns a client describing the given session.
func New(tabs []herdr.TabInfo, panes []herdr.PaneInfo) *Client {
	s := &Client{
		tabs:      make(map[string]herdr.TabInfo, len(tabs)),
		panes:     make(map[string]herdr.PaneInfo, len(panes)),
		processes: make(map[string][]herdr.PaneProcessInfoProcess),
	}
	for _, tab := range tabs {
		s.tabs[tab.TabID] = tab
	}

	for _, pane := range panes {
		s.panes[pane.PaneID] = pane
	}

	return s
}

// SetWorkspaces sets the workspaces the session reports.
func (s *Client) SetWorkspaces(workspaces ...herdr.WorkspaceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.workspaces = workspaces
}

// SetTab adds or replaces a tab.
func (s *Client) SetTab(tab herdr.TabInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tabs[tab.TabID] = tab
}

// SetProcesses sets what a read of this pane's processes will answer.
func (s *Client) SetProcesses(paneID string, processes ...herdr.PaneProcessInfoProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.processes[paneID] = processes
}

// SetPane adds or replaces a pane.
func (s *Client) SetPane(pane herdr.PaneInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.panes[pane.PaneID] = pane
}

// CloseTab and ClosePane remove an object from the session.
func (s *Client) CloseTab(tabID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tabs, tabID)
}

func (s *Client) ClosePane(paneID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.panes, paneID)
}

// SetRenameError makes subsequent tab.rename calls fail.
func (s *Client) SetRenameError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.renameErr = err
}

// SetProcessError makes subsequent pane.process_info calls fail.
func (s *Client) SetProcessError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.processErr = err
}

// SetCallError makes every subsequent call fail, as a dropped socket would.
func (s *Client) SetCallError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.callErr = err
}

// Renames returns the renames received so far.
func (s *Client) Renames() []RenameCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.renames)
}

// ProcessReads counts the pane.process_info calls received so far, which is how
// a test sees that a pane was not asked about twice.
func (s *Client) ProcessReads() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.reads
}

// Polls counts the snapshots asked for so far, whether or not they were
// answered.
func (s *Client) Polls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.polls
}

func (s *Client) Call(ctx context.Context, method string, params any, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Counted before the error, so a test can wait out a stretch of polls that
	// are failing as readily as one of polls that work.
	if method == herdr.MethodSessionSnapshot {
		s.polls++
	}

	if s.callErr != nil {
		return s.callErr
	}

	switch method {
	case herdr.MethodSessionSnapshot:
		return answer(result, snapshotResult{Snapshot: herdr.Snapshot{
			Workspaces: slices.Clone(s.workspaces),
			Tabs:       sorted(s.tabs, func(t herdr.TabInfo) string { return t.TabID }),
			Panes:      sorted(s.panes, func(p herdr.PaneInfo) string { return p.PaneID }),
		}})

	case herdr.MethodPaneProcessInfo:
		return s.processInfo(params, result)

	case herdr.MethodTabRename:
		return s.rename(params)

	default:
		return fmt.Errorf("stub client: unsupported method %s", method)
	}
}

// processInfo answers pane.process_info for a pane the session still holds.
func (s *Client) processInfo(params any, result any) error {
	if s.processErr != nil {
		return s.processErr
	}

	var target herdr.PaneTarget
	if err := decode(params, &target); err != nil {
		return err
	}

	if _, live := s.panes[target.PaneID]; !live {
		return &herdr.APIError{
			Code:    herdr.CodePaneNotFound,
			Message: "pane " + target.PaneID + " not found",
		}
	}

	s.reads++

	return answer(result, processInfoResult{
		ProcessInfo: herdr.PaneProcessInfo{ForegroundProcesses: s.processes[target.PaneID]},
	})
}

// rename applies a tab.rename to the stub session.
func (s *Client) rename(params any) error {
	if s.renameErr != nil {
		return s.renameErr
	}

	var call herdr.TabRenameParams
	if err := decode(params, &call); err != nil {
		return err
	}

	tab, live := s.tabs[call.TabID]
	if !live {
		return &herdr.APIError{
			Code:    herdr.CodeTabNotFound,
			Message: "tab " + call.TabID + " not found",
		}
	}
	// Herdr's label really does change, so the next poll must agree.
	tab.Label = call.Label
	s.tabs[call.TabID] = tab
	s.renames = append(s.renames, RenameCall(call))

	return nil
}

// answer encodes the stub's reply and decodes it into result the way the socket
// client does, so a session travels the wire shape rather than being handed
// over as a Go value: a json tag nothing else exercises is exercised here.
func answer(result any, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("stub client: encode result: %w", err)
	}

	if result == nil {
		return nil
	}

	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("stub client: decode result: %w", err)
	}

	return nil
}

// decode reads a request's parameters as Herdr would, which is what makes the
// stub answer the request that was actually sent rather than the Go value
// behind it.
func decode(params any, target any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("stub client: encode params: %w", err)
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("stub client: decode params: %w", err)
	}

	return nil
}

// sorted flattens a map into a slice ordered by each value's id, so a stub
// session is enumerated in the same order every time.
func sorted[T any](items map[string]T, id func(T) string) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}

	slices.SortFunc(out, func(a, b T) int { return strings.Compare(id(a), id(b)) })

	return out
}
