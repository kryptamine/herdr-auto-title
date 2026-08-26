package resolver

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// numberedCWD names a tab after its directory and puts its position in front.
func numberedCWD(maxLength int) *Numbered {
	return NewNumbered(New(maxLength, NewCWD()))
}

// atPosition builds a one-pane tab sitting at a position in its workspace.
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
