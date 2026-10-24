package resolver

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func TestStatusMarksTheTitle(t *testing.T) {
	t.Parallel()

	r := NewStatusTitles(New(Options{}, NewCWD("")), 0)

	tests := []struct {
		status string
		want   string
	}{
		{herdr.AgentStatusIdle, "◉ dashboard"},
		{herdr.AgentStatusWorking, "◐ dashboard"},
		{herdr.AgentStatusBlocked, "● dashboard"},
		{herdr.AgentStatusDone, "✓ dashboard"},
		// A tab without an agent must not carry a mark that means nothing.
		{"unknown", "dashboard"},
		{"", "dashboard"},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()

			tab := tabWithCWD(dashboard)
			tab.AgentStatus = tc.status

			if got := r.Resolve(tab); got.Name != tc.want {
				t.Errorf("name = %q, want %q", got.Name, tc.want)
			}
		})
	}
}

func TestStatusSitsBetweenThePositionAndTheIcon(t *testing.T) {
	// The position is a key and keeps the lead; the status changes while the
	// icon holds still, so it reads first after the key.
	t.Parallel()

	inner := NewStatusTitles(NewIconedTitles(New(Options{}, NewCWD("")), 0), 0)
	tab := atPosition(2, dashboard)
	tab.AgentStatus = herdr.AgentStatusWorking

	want := "2 · ◐ " + iconShell + " dashboard"
	if got := NewNumbered(inner, 0).Resolve(tab); got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestStatusCountsAgainstTheLength(t *testing.T) {
	// A mark must not make a title wider than the bound the user set.
	t.Parallel()

	const maxLength = 6

	r := NewStatusTitles(New(Options{MaxLength: maxLength}, NewCWD("")), maxLength)
	tab := tabWithCWD(dashboard)
	tab.AgentStatus = herdr.AgentStatusBlocked

	if got := r.Resolve(tab); got.Name != "● dash" {
		t.Errorf("name = %q, want %q", got.Name, "● dash")
	}
}
