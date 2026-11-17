package desktop

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// layoutDocument is the core document the app keeps its spaces and tabs in:
// docs/architecture/desktop-api.md#a-core-with-no-window.
const layoutDocument = "herdr-app"

// A rename that keeps losing the document to other writers gives up after this.
const renameAttempts = 3

// docLayout reads the spaces and tabs from the core's layout document, for a
// core whose window runs on another computer. The document knows no focus.
type docLayout struct {
	core string
}

// document is what this reads of the app's layout. Renames rewrite the whole
// value, so they go through renameIn on the raw JSON instead.
type document struct {
	Spaces []docSpace `json:"spaces"`
}

type docSpace struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Tabs []docTab `json:"tabs"`
}

type docTab struct {
	ID   string     `json:"id"`
	Name string     `json:"name"`
	Root layoutNode `json:"root"`
}

// layoutNode is a split or a block; a block holds one terminal.
type layoutNode struct {
	ID      string `json:"id"`
	Content *struct {
		Terminal string `json:"terminal"`
	} `json:"content"`
	Children []layoutNode `json:"children"`
}

// docName and docWrite are doc.get's and doc.set's parameters.
type docName struct {
	Name string `json:"name"`
}

type docWrite struct {
	Name    string `json:"name"`
	Value   any    `json:"value"`
	Version uint64 `json:"version"`
}

// docRead is doc.get's answer, the value left raw so a write can keep it whole.
type docRead struct {
	Value   json.RawMessage `json:"value"`
	Version uint64          `json:"version"`
}

// coreStatus is a terminal's own report, as hc 0.1.18 and later carry it.
type coreStatus struct {
	App   string `json:"app"`
	State string `json:"state"`
}

func (l docLayout) claim() string { return l.core }

func (l docLayout) read(ctx context.Context) ([]space, string, error) {
	terminals, err := listTerminals(ctx, l.core)
	if err != nil {
		return nil, "", err
	}

	d, _, err := l.document(ctx)
	if err != nil {
		return nil, "", err
	}

	return d.world(terminals).join(terminals), "", nil
}

func (l docLayout) document(ctx context.Context) (document, docRead, error) {
	var got docRead
	if err := rpc(ctx, l.core, "doc.get", docName{layoutDocument}, &got); err != nil {
		return document{}, got, err
	}

	var d document
	if len(got.Value) == 0 {
		return d, got, nil
	}

	if err := json.Unmarshal(got.Value, &d); err != nil {
		return document{}, got, fmt.Errorf("read %s: %w", layoutDocument, err)
	}

	return d, got, nil
}

// renameTab writes a document tab's name, read again if another writer got
// there first; a tab only Ovis's labels place is renamed through them.
func (l docLayout) renameTab(ctx context.Context, tabID, label string) error {
	label = strings.TrimSpace(label)

	for attempt := 1; ; attempt++ {
		d, got, err := l.document(ctx)
		if err != nil {
			return err
		}

		if !d.holds(tabID) {
			return l.renameUnplacedTab(ctx, d, tabID, label)
		}

		value, err := renameIn(got.Value, tabID, label)
		if err != nil {
			return err
		}

		err = rpc(
			ctx,
			l.core,
			"doc.set",
			docWrite{Name: layoutDocument, Value: value, Version: got.Version},
			nil,
		)

		var refused *coreError
		if !errors.As(err, &refused) || refused.kind != "conflict" || attempt == renameAttempts {
			return err
		}
	}
}

func (l docLayout) renameUnplacedTab(ctx context.Context, d document, tabID, label string) error {
	terminals, err := listTerminals(ctx, l.core)
	if err != nil {
		return err
	}

	for _, s := range d.world(terminals).Spaces {
		if i := s.tabIndex(tabID); i >= 0 {
			return l.renameLabelPanes(ctx, s.Tabs[i].Terminals, terminals, label)
		}
	}

	return &herdr.APIError{Code: herdr.CodeTabNotFound, Message: noTabPrefix + strconv.Quote(tabID)}
}

func (l docLayout) renameLabelPanes(
	ctx context.Context, entries []paneEntry, terminals []terminal, label string,
) error {
	byAddress := terminalsByAddress(terminals)

	// The first pane supplies the displayed name, so change it last.
	for i, entry := range slices.Backward(entries) {
		labels := map[string]string{ovisTabName: label}
		if workspace, name := whereNames(byAddress[entry.Terminal]); name != "" {
			// Keep the workspace part so `where` no longer hides `ovis.tab`.
			labels["where"] = workspace
		}

		params := map[string]any{"terminal": entry.Terminal, "labels": labels}
		if err := rpc(ctx, l.core, "terminal.label", params, nil); err != nil {
			if i < len(entries)-1 {
				return fmt.Errorf("%w: %w", herdr.ErrUnanswered, err)
			}

			return err
		}
	}

	return nil
}

func (d document) holds(tabID string) bool {
	return slices.ContainsFunc(d.Spaces, func(s docSpace) bool {
		return slices.ContainsFunc(s.Tabs, func(t docTab) bool { return t.ID == tabID })
	})
}

// world is the document as `ls` would answer it, with the tabs only Ovis's
// labels place added after.
func (d document) world(terminals []terminal) world {
	byAddress := terminalsByAddress(terminals)

	var w world

	for _, s := range d.Spaces {
		listed := space{ID: s.ID, Name: s.Name}

		for i, t := range s.Tabs {
			listed.Tabs = append(listed.Tabs, tab{
				ID: t.ID, Name: cmp.Or(t.Name, strconv.Itoa(i+1)),
				Terminals: entriesOf(t.Root, byAddress),
			})
		}

		w.Spaces = append(w.Spaces, listed)
	}

	addUnplaced(&w, terminals)

	return w
}

// entriesOf is a tab's panes as `ls` would list them, in the tree's order. A
// connected machine's terminal is left out: its PIDs and paths are not local.
func entriesOf(root layoutNode, byAddress map[string]terminal) []paneEntry {
	var entries []paneEntry

	for _, block := range blocksOf(root) {
		if address := block.Content.Terminal; !remoteAddress(address) {
			entries = append(entries, corePane(address, block.ID, byAddress[address]))
		}
	}

	return entries
}

func corePane(address, pane string, t terminal) paneEntry {
	entry := paneEntry{Terminal: address, Pane: pane}
	if t.Status != nil {
		entry.Status, entry.App = t.appStatus(), t.Status.App
	}

	return entry
}

func blocksOf(node layoutNode) []layoutNode {
	if node.Content != nil && node.Content.Terminal != "" {
		return []layoutNode{node}
	}

	var blocks []layoutNode
	for _, child := range node.Children {
		blocks = append(blocks, blocksOf(child)...)
	}

	return blocks
}

// remoteAddress is a connected machine's terminal, whose address carries the
// machine's prefix.
func remoteAddress(address string) bool { return strings.Contains(address, "/") }

// appStatus is the app's word for a terminal at rest: done while its last turn
// is past the one someone looked at (`herdr.seen`), idle once they have.
func (t terminal) appStatus() string {
	state := t.Status.State
	if state != herdr.AgentStatusIdle && state != herdr.AgentStatusDone {
		return state
	}

	if seen, err := strconv.ParseUint(t.Labels["herdr.seen"], 10, 64); err == nil && t.Turn > seen {
		return herdr.AgentStatusDone
	}

	return herdr.AgentStatusIdle
}

// renameIn sets one tab's name in the document and keeps everything else in
// it, numbers included, exactly as the app wrote it.
func renameIn(raw json.RawMessage, tabID, name string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("read %s: %w", layoutDocument, err)
	}

	spaces, _ := value["spaces"].([]any)
	for _, s := range spaces {
		listed, _ := s.(map[string]any)
		tabs, _ := listed["tabs"].([]any)

		for _, t := range tabs {
			if shown, _ := t.(map[string]any); shown != nil && shown["id"] == tabID {
				shown["name"] = name
			}
		}
	}

	return value, nil
}
