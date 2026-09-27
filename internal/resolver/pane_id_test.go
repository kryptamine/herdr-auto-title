package resolver

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func TestPaneIDLeadsEveryPaneLabel(t *testing.T) {
	t.Parallel()

	tab := tabOf([]*state.PaneState{
		{ID: "wE:p1", Dir: dashboard, Focused: true},
		{ID: "wE:p2", Dir: api},
	})

	got := NewPaneIDs(defaultChain(), DefaultMaxLength).ResolvePanes(tab)

	want := []string{"[wE:p1] dashboard", "[wE:p2] api"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("pane %d = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestPaneIDLeadsTheFallback(t *testing.T) {
	t.Parallel()

	tab := tabOf([]*state.PaneState{{ID: "wE:p1", Dir: herdrtest.Root(), Focused: true}})

	got := NewPaneIDs(defaultChain(), DefaultMaxLength).ResolvePanes(tab)
	if want := "[wE:p1] " + GenericFallback; got[0].Name != want {
		t.Errorf("name = %q, want %q", got[0].Name, want)
	}
}

func TestPaneIDKeepsTheDecisionItWraps(t *testing.T) {
	t.Parallel()

	got := NewPaneIDs(defaultChain(), DefaultMaxLength).ResolvePanes(tabWithCWD(dashboard))
	if got[0].Reason != "cwd" {
		t.Errorf("reason = %q, want cwd", got[0].Reason)
	}

	if got[0].Confidence != ConfidenceCWD {
		t.Errorf("confidence = %d, want %d", got[0].Confidence, ConfidenceCWD)
	}
}

func TestAPaneIDIsCountedAgainstTheWidth(t *testing.T) {
	t.Parallel()

	const maxLength = 16

	long := herdrtest.Dir("work", strings.Repeat("a", 40))
	chain := New(Options{MaxLength: maxLength}, NewCWD(""))

	got := NewPaneIDs(chain, maxLength).ResolvePanes(tabWithCWD(long))
	if width := uniseg.StringWidth(got[0].Name); width > maxLength {
		t.Errorf("name %q is %d columns wide, want at most %d", got[0].Name, width, maxLength)
	}

	if !strings.HasPrefix(got[0].Name, "[wE:p1] ") {
		t.Errorf("name = %q, want it to lead with its pane ID", got[0].Name)
	}
}

func TestAPaneIDWiderThanTheLabelIsLeftOut(t *testing.T) {
	t.Parallel()

	// A label with no room left for a name would say only which pane it is,
	// which Herdr's own goto panel already shows.
	const maxLength = 6

	chain := New(Options{MaxLength: maxLength}, NewCWD(""))

	got := NewPaneIDs(chain, maxLength).ResolvePanes(tabWithCWD(api))
	if got[0].Name != "api" {
		t.Errorf("name = %q, want the name alone", got[0].Name)
	}
}
