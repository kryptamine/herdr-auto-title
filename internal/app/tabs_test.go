package app

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func TestATabNotShownIsNamedAfterItsOwnFocusedPane(t *testing.T) {
	t.Parallel()

	// Herdr marks one pane focused in the whole session, so without its layout
	// a tab nobody is looking at would be named after another of its panes.
	app := newTestApp(t, testConfig())
	snapshot := herdr.Snapshot{
		Tabs: []herdr.TabInfo{{TabID: "wE:t1"}, {TabID: "wE:t2"}},
		Panes: []herdr.PaneInfo{
			{PaneID: "wE:p1", TabID: "wE:t1", Focused: true},
			{
				PaneID:      "wE:p2",
				TabID:       "wE:t2",
				Agent:       "claude",
				AgentStatus: herdr.AgentStatusWorking,
			},
			{PaneID: "wE:p3", TabID: "wE:t2"},
		},
		Layouts: []herdr.LayoutInfo{
			{TabID: "wE:t1", FocusedPaneID: "wE:p1"},
			{TabID: "wE:t2", FocusedPaneID: "wE:p3"},
		},
	}

	tabs := app.tabsIn(snapshot)

	for i, want := range []string{"wE:p1", "wE:p3"} {
		if got := tabs[i].Context.ID; got != want {
			t.Errorf("tab %d is named after %s, want its focused pane %s", i, got, want)
		}
	}
}
