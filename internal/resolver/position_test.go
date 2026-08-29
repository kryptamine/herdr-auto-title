package resolver

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// numberedCWD names a tab after its directory and puts its position in front.
func numberedCWD(maxLength int) *Numbered {
	return NewNumbered(New(maxLength, NewCWD()), maxLength)
}

func atPosition(position int, dir string) state.TabState {
	tab := tabWithCWD(dir)
	tab.Position = position

	return tab
}

func TestPositionLeadsTheTitle(t *testing.T) {
	r := numberedCWD(DefaultMaxLength)

	tests := []struct {
		name     string
		position int
		dir      string
		want     string
	}{
		{"the first tab", 1, "/Users/dev/work/dashboard", "1 · dashboard"},
		{"a tab past the ninth", 12, "/Users/dev/work/dashboard", "12 · dashboard"},
		{"a tab with nothing to say", 3, "/", "3 · " + GenericFallback},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.Resolve(atPosition(tc.position, tc.dir)); got.Name != tc.want {
				t.Errorf("name = %q, want %q", got.Name, tc.want)
			}
		})
	}
}

func TestPositionKeepsTheDecisionItWraps(t *testing.T) {
	got := numberedCWD(DefaultMaxLength).Resolve(atPosition(1, "/Users/dev/work/dashboard"))
	if got.Reason != "cwd" {
		t.Errorf("reason = %q, want cwd", got.Reason)
	}

	if got.Confidence != ConfidenceCWD {
		t.Errorf("confidence = %d, want %d", got.Confidence, ConfidenceCWD)
	}
}

func TestAPositionIsCountedAgainstTheWidth(t *testing.T) {
	const maxLength = 16

	long := "/Users/dev/work/" + strings.Repeat("a", 40)

	got := numberedCWD(maxLength).Resolve(atPosition(7, long))
	if width := uniseg.StringWidth(got.Name); width > maxLength {
		t.Errorf("name %q is %d columns wide, want at most %d", got.Name, width, maxLength)
	}

	if !strings.HasPrefix(got.Name, "7 · ") {
		t.Errorf("name = %q, want it to lead with its position", got.Name)
	}
}

// A tab bar narrower than the position itself keeps the name over the number:
// a title cut down to nothing has lost more than the position is worth.
func TestAPositionWithNoRoomIsDropped(t *testing.T) {
	got := numberedCWD(3).Resolve(atPosition(10, "/Users/dev/work/dashboard"))
	if got.Name != "das" {
		t.Errorf("name = %q, want the bare title", got.Name)
	}
}

func TestNumberedWithoutAWidthTakesTheDefault(t *testing.T) {
	// Zero means "no bound" to Sanitize but would leave no room at all here,
	// so every tab would quietly lose the position instead.
	got := NewNumbered(New(0, NewCWD()), 0).Resolve(atPosition(2, "/Users/dev/work/api"))
	if got.Name != "2 · api" {
		t.Errorf("name = %q, want the position kept", got.Name)
	}
}

// fixedResolver names every tab the same, which is all Numbered needs of what
// it wraps.
type fixedResolver struct {
	decision Decision
}

func (f fixedResolver) Resolve(state.TabState) Decision { return f.decision }

func TestAnyResolverCanBeNumbered(t *testing.T) {
	// Numbered asks what it wraps for a name and nothing else, so a resolver
	// that is not the shipped chain is numbered just the same.
	inner := fixedResolver{decision: Decision{
		Name:       "release notes",
		Confidence: ConfidenceAgent,
		Reason:     "test_source",
	}}

	got := NewNumbered(inner, DefaultMaxLength).Resolve(atPosition(4, "/Users/dev/work/api"))
	want := Decision{
		Name:       "4 · release notes",
		Confidence: ConfidenceAgent,
		Reason:     "test_source",
	}

	if got != want {
		t.Errorf("decision = %+v, want %+v", got, want)
	}
}

// Making room for the position is the one thing Numbered does to a name, and a
// cut that lands on a separator says a part was lost without saying which.
func TestANumberedTitleLeavesNoDanglingSeparator(t *testing.T) {
	inner := fixedResolver{decision: Decision{Name: "dashboard › nvim"}}

	got := NewNumbered(inner, 16).Resolve(atPosition(1, "/Users/dev/work/dashboard"))
	if got.Name != "1 · dashboard" {
		t.Errorf("name = %q, want %q", got.Name, "1 · dashboard")
	}
}
