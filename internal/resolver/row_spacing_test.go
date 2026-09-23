package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// A row the user wrote was never sanitized, so its segments may carry the
// spaces Sanitize would have collapsed. Compared raw, "group  ›  dashboard"
// splits into "group " and " dashboard", and the tab under it says dashboard
// again.
func TestATabDoesNotRepeatARowTheUserSpacedLoosely(t *testing.T) {
	t.Parallel()

	tab := tabWithPane(&state.PaneState{Dir: dashboard, TerminalTitle: "auth.ts"})
	tab.WorkspaceName = "group  ›  dashboard"

	if got, want := namingChain().Resolve(tab).Name, "auth.ts"; got != want {
		t.Errorf("name = %q under row %q, want %q", got, tab.WorkspaceName, want)
	}
}
