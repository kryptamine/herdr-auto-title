package herdr

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// RenameCall records one tab.rename issued through a StubClient.
type RenameCall struct {
	TabID string
	Label string
}

// StubClient is an in-memory Client for tests. Tests change the session it
// describes and inspect the renames it received.
type StubClient struct {
	mu         sync.Mutex
	workspaces []WorkspaceInfo
	tabs       map[string]TabInfo
	panes      map[string]PaneInfo
	processes  map[string][]PaneProcessInfoProcess
	renames    []RenameCall
	renameErr  error
	processErr error
	callErr    error
	polls      int
	reads      int
}

var _ Client = (*StubClient)(nil)

// NewStub returns a client describing the given session.
func NewStub(tabs []TabInfo, panes []PaneInfo) *StubClient {
	s := &StubClient{
		tabs:      make(map[string]TabInfo, len(tabs)),
		panes:     make(map[string]PaneInfo, len(panes)),
		processes: make(map[string][]PaneProcessInfoProcess),
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
func (s *StubClient) SetWorkspaces(workspaces ...WorkspaceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.workspaces = workspaces
}

// SetTab adds or replaces a tab.
func (s *StubClient) SetTab(tab TabInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tabs[tab.TabID] = tab
}

// SetProcesses sets what a read of this pane's processes will answer.
func (s *StubClient) SetProcesses(paneID string, processes ...PaneProcessInfoProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.processes[paneID] = processes
}

// SetPane adds or replaces a pane.
func (s *StubClient) SetPane(pane PaneInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.panes[pane.PaneID] = pane
}

// CloseTab and ClosePane remove an object from the session.
func (s *StubClient) CloseTab(tabID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tabs, tabID)
}

func (s *StubClient) ClosePane(paneID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.panes, paneID)
}

// SetRenameError makes subsequent tab.rename calls fail.
func (s *StubClient) SetRenameError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.renameErr = err
}

// SetProcessError makes subsequent pane.process_info calls fail.
func (s *StubClient) SetProcessError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.processErr = err
}

// SetCallError makes every subsequent call fail, as a dropped socket would.
func (s *StubClient) SetCallError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.callErr = err
}

// Renames returns the renames received so far.
func (s *StubClient) Renames() []RenameCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.renames)
}

// ProcessReads counts the pane.process_info calls received so far, which is how
// a test sees that a pane was not asked about twice.
func (s *StubClient) ProcessReads() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.reads
}

// Polls counts the snapshots taken so far.
func (s *StubClient) Polls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.polls
}

func (s *StubClient) Call(ctx context.Context, method string, params any, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.callErr != nil {
		return s.callErr
	}

	switch method {
	case MethodSessionSnapshot:
		target, ok := result.(*snapshotResult)
		if !ok {
			return fmt.Errorf("stub client: unexpected result type for %s", method)
		}

		s.polls++
		target.Snapshot = Snapshot{
			Workspaces: slices.Clone(s.workspaces),
			Tabs:       sorted(s.tabs, func(t TabInfo) string { return t.TabID }),
			Panes:      sorted(s.panes, func(p PaneInfo) string { return p.PaneID }),
		}

		return nil

	case MethodPaneProcessInfo:
		if s.processErr != nil {
			return s.processErr
		}

		target, ok := params.(PaneTarget)
		if !ok {
			return fmt.Errorf("stub client: unexpected params for %s", method)
		}

		if _, ok := s.panes[target.PaneID]; !ok {
			return &APIError{
				Code:    CodePaneNotFound,
				Message: "pane " + target.PaneID + " not found",
			}
		}

		res, ok := result.(*processInfoResult)
		if !ok {
			return fmt.Errorf("stub client: unexpected result type for %s", method)
		}

		s.reads++
		res.ProcessInfo.ForegroundProcesses = s.processes[target.PaneID]

		return nil

	case MethodTabRename:
		if s.renameErr != nil {
			return s.renameErr
		}

		rename, ok := params.(TabRenameParams)
		if !ok {
			return fmt.Errorf("stub client: unexpected params for %s", method)
		}

		tab, ok := s.tabs[rename.TabID]
		if !ok {
			return &APIError{Code: CodeTabNotFound, Message: "tab " + rename.TabID + " not found"}
		}
		// Herdr's label really does change, so the next poll must agree.
		tab.Label = rename.Label
		s.tabs[rename.TabID] = tab
		s.renames = append(s.renames, RenameCall(rename))

		return nil

	default:
		return fmt.Errorf("stub client: unsupported method %s", method)
	}
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
