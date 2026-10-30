// Package herdrtest provides an in-memory Herdr client for tests. It sits
// beside the client rather than inside it so that the package the plugin ships
// exports nothing only a test reads.
package herdrtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

type RenameCall struct {
	TabID string
	Label string
}

type PaneRenameCall struct {
	PaneID string
	Label  string
}

// WorkspaceReportCall is one workspace.report_metadata the stub received. A nil
// token value is a clear.
type WorkspaceReportCall struct {
	WorkspaceID string
	Source      string
	Tokens      map[string]*string
	TTLMs       int64
}

// Client is an in-memory herdr.Client. Tests change the session it describes
// and inspect the renames it received.
type Client struct {
	mu               sync.Mutex
	workspaces       []herdr.WorkspaceInfo
	tabs             map[string]herdr.TabInfo
	panes            map[string]herdr.PaneInfo
	processes        map[string][]herdr.PaneProcessInfoProcess
	handles          map[string]string
	renames          []RenameCall
	paneRenames      []PaneRenameCall
	workspaceReports []WorkspaceReportCall
	renameErr        error
	reportErr        error
	processErr       error
	processErrOnce   error
	callErr          error
	reads            int
	server           string
}

var _ herdr.Client = (*Client)(nil)

func New(tabs []herdr.TabInfo, panes []herdr.PaneInfo) *Client {
	s := &Client{
		tabs:      make(map[string]herdr.TabInfo, len(tabs)),
		panes:     make(map[string]herdr.PaneInfo, len(panes)),
		processes: make(map[string][]herdr.PaneProcessInfoProcess),
		handles:   make(map[string]string),
		server:    "herdrtest",
	}
	for _, tab := range tabs {
		s.tabs[tab.TabID] = tab
	}

	for _, pane := range panes {
		s.panes[pane.PaneID] = pane
	}

	return s
}

// SetServer changes which server the stub reports on the socket: "" for none,
// another name for a successor.
func (s *Client) SetServer(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.server = id
}

func (s *Client) Server() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.server
}

func (s *Client) SetWorkspaces(workspaces ...herdr.WorkspaceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.workspaces = workspaces
}

func (s *Client) SetTab(tab herdr.TabInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tabs[tab.TabID] = tab
}

func (s *Client) SetProcesses(paneID string, processes ...herdr.PaneProcessInfoProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.processes[paneID] = processes
}

// SetHandle names the agent in a pane, as `herdr agent rename` does; "" clears
// the name.
func (s *Client) SetHandle(paneID, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.handles[paneID] = name
}

func (s *Client) SetPane(pane herdr.PaneInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.panes[pane.PaneID] = pane
}

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

// SetRenameError makes every subsequent tab rename fail with err. One wrapping
// herdr.ErrUnanswered lands all the same, as a request Herdr read but did not
// answer does. Pane renames are unaffected.
func (s *Client) SetRenameError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.renameErr = err
}

// SetReportError makes every subsequent workspace report fail with err. One
// wrapping herdr.ErrUnanswered lands all the same, as a rename's does.
func (s *Client) SetReportError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reportErr = err
}

func (s *Client) SetProcessError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.processErr = err
}

// FailNextProcessRead makes only the next pane.process_info call fail, as a
// Herdr busy for a moment does.
func (s *Client) FailNextProcessRead(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.processErrOnce = err
}

// SetCallError makes every subsequent call fail, as a dropped socket would.
func (s *Client) SetCallError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.callErr = err
}

func (s *Client) Renames() []RenameCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.renames)
}

func (s *Client) PaneRenames() []PaneRenameCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.paneRenames)
}

func (s *Client) WorkspaceReports() []WorkspaceReportCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.workspaceReports)
}

// ProcessReads counts the pane.process_info calls received so far, which is how
// a test sees that a pane was not asked about twice.
func (s *Client) ProcessReads() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.reads
}

func (s *Client) Path() string { return "herdrtest" }

// Snapshot travels the wire shape rather than being handed over as a Go value:
// a json tag nothing else exercises is exercised here.
func (s *Client) Snapshot(ctx context.Context) (herdr.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.refused(ctx); err != nil {
		return herdr.Snapshot{}, err
	}

	raw, err := json.Marshal(herdr.Snapshot{
		Workspaces: slices.Clone(s.workspaces),
		Tabs:       sorted(s.tabs, func(t herdr.TabInfo) string { return t.TabID }),
		Panes:      sorted(s.panes, func(p herdr.PaneInfo) string { return p.PaneID }),
		Agents:     s.agents(),
	})
	if err != nil {
		return herdr.Snapshot{}, fmt.Errorf("stub client: encode snapshot: %w", err)
	}

	var snapshot herdr.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return herdr.Snapshot{}, fmt.Errorf("stub client: decode snapshot: %w", err)
	}

	return snapshot, nil
}

func (s *Client) PaneProcesses(
	ctx context.Context,
	paneID string,
) ([]herdr.PaneProcessInfoProcess, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.refused(ctx); err != nil {
		return nil, err
	}

	if s.processErr != nil {
		return nil, s.processErr
	}

	if err := s.processErrOnce; err != nil {
		s.processErrOnce = nil
		return nil, err
	}

	if _, live := s.panes[paneID]; !live {
		return nil, &herdr.APIError{
			Code:    herdr.CodePaneNotFound,
			Message: "pane " + paneID + " not found",
		}
	}

	s.reads++

	return slices.Clone(s.processes[paneID]), nil
}

func (s *Client) RenameTab(ctx context.Context, tabID, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.refused(ctx); err != nil {
		return err
	}

	if s.renameErr != nil && !errors.Is(s.renameErr, herdr.ErrUnanswered) {
		return s.renameErr
	}

	tab, live := s.tabs[tabID]
	if !live {
		return &herdr.APIError{
			Code:    herdr.CodeTabNotFound,
			Message: "tab " + tabID + " not found",
		}
	}
	// Herdr's label really does change, so the next poll must agree.
	tab.Label = label
	s.tabs[tabID] = tab
	s.renames = append(s.renames, RenameCall{TabID: tabID, Label: label})

	return s.renameErr
}

func (s *Client) RenamePane(ctx context.Context, paneID, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.refused(ctx); err != nil {
		return err
	}

	pane, live := s.panes[paneID]
	if !live {
		return &herdr.APIError{
			Code:    herdr.CodePaneNotFound,
			Message: "pane " + paneID + " not found",
		}
	}
	// Herdr's label really does change, so the next poll must agree.
	pane.Label = label
	s.panes[paneID] = pane
	s.paneRenames = append(s.paneRenames, PaneRenameCall{PaneID: paneID, Label: label})

	return nil
}

func (s *Client) ReportWorkspaceTopic(
	ctx context.Context,
	workspaceID, source, topic string,
	ttl time.Duration,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.refused(ctx); err != nil {
		return err
	}

	if s.reportErr != nil && !errors.Is(s.reportErr, herdr.ErrUnanswered) {
		return s.reportErr
	}

	call := herdr.TopicReport(workspaceID, source, topic, ttl)

	for i, workspace := range s.workspaces {
		if workspace.WorkspaceID != call.WorkspaceID {
			continue
		}
		// The tokens really do change, so the next snapshot must show them.
		s.workspaces[i].Tokens = merged(workspace.Tokens, call.Tokens)
		s.workspaceReports = append(s.workspaceReports, WorkspaceReportCall(call))

		return s.reportErr
	}

	return &herdr.APIError{
		Code:    herdr.CodeWorkspaceNotFound,
		Message: "workspace " + call.WorkspaceID + " not found",
	}
}

// refused is why the stub will not answer this call, if it will not.
func (s *Client) refused(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.callErr
}

// merged applies reported tokens the way Herdr does: each key replaces its own
// value, a nil value clears it, and keys not reported stay.
func merged(current map[string]string, reported map[string]*string) map[string]string {
	next := make(map[string]string, len(current))
	maps.Copy(next, current)

	for key, value := range reported {
		if value == nil || *value == "" {
			delete(next, key)
		} else {
			next[key] = *value
		}
	}

	if len(next) == 0 {
		return nil
	}

	return next
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

// agents lists every pane running an agent, as Herdr does, with the name only
// where one was given.
func (s *Client) agents() []herdr.AgentInfo {
	var agents []herdr.AgentInfo

	for _, pane := range sorted(s.panes, func(p herdr.PaneInfo) string { return p.PaneID }) {
		if pane.Agent != "" {
			agents = append(
				agents,
				herdr.AgentInfo{PaneID: pane.PaneID, Name: s.handles[pane.PaneID]},
			)
		}
	}

	return agents
}
