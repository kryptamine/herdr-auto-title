package resolver

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

func TestSlideShowsAWindowThatMovesAStepPerTick(t *testing.T) {
	const name = "abcdefgh"

	tests := []struct {
		step int
		tick int
		want string
	}{
		{1, 0, "abcde"},
		{1, slidePause, "abcde"},
		{1, slidePause + 1, "bcdef"},
		{1, slidePause + 3, "defgh"},
		{1, slidePause + 4, "efgh "},
		{1, slidePause + 7, "h   a"},
		{1, slidePause + 8, "   ab"},
		{1, slidePause + 10, " abcd"},
		{1, slidePause + 11, "abcde"},
		{1, slidePause + 12, "abcde"},
		{2, slidePause, "abcde"},
		{2, slidePause + 1, "cdefg"},
		{2, slidePause + 2, "efgh "},
		{2, slidePause + 4, "   ab"},
		// The lap is eleven columns and two does not divide it, so the last
		// move lands one column short of the head rather than past it.
		{2, slidePause + 5, " abcd"},
		{2, slidePause + 6, "abcde"},
		{2, slidePause + 7, "abcde"},
	}

	for _, tc := range tests {
		if got := Slide(name, 5, tc.tick, tc.step); got != tc.want {
			t.Errorf(
				"Slide(%q, 5, %d, step %d) = %q, want %q",
				name,
				tc.tick,
				tc.step,
				got,
				tc.want,
			)
		}
	}
}

func TestSlideLeavesAloneWhatFits(t *testing.T) {
	tests := []struct {
		name  string
		value string
		width int
	}{
		{"a name narrower than the bar", "abc", 5},
		{"a name exactly as wide as the bar", "abcde", 5},
		{"no bound at all", "abcdefgh", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for tick := range 20 {
				if got := Slide(tc.value, tc.width, tick, 1); got != tc.value {
					t.Errorf("tick %d: got %q, want %q untouched", tick, got, tc.value)
				}
			}
		})
	}
}

func TestEverySlidingWindowIsTheSameWidth(t *testing.T) {
	// A window one column short would shrink the tab it slides in, which is
	// what a name of two-column clusters does to a width they do not divide.
	tests := []struct {
		name  string
		value string
	}{
		{"two-column clusters", "탭 제목 슬라이드 처리 " + strings.Repeat("x", 10)},
		{"an emoji cluster", "日本語のタイトル 👨‍👩‍👧 " + strings.Repeat("x", 10)},
		{"one-column clusters", strings.Repeat("abcde", 6)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for tick := range 60 {
				got := Slide(tc.value, 9, tick, 1)
				if width := uniseg.StringWidth(got); width != 9 {
					t.Errorf("tick %d: %q is %d columns wide, want 9", tick, got, width)
				}
			}
		})
	}
}

func TestASlidingWindowIsCutBetweenGraphemeClusters(t *testing.T) {
	// Never mid-cluster: a torn emoji is a different emoji, and half a Hangul
	// syllable is a different syllable.
	name := "日本語のタイトル 👨‍👩‍👧 " + strings.Repeat("x", 10)

	for tick := range 60 {
		got := strings.TrimRight(Slide(name, 8, tick, 1), " ")
		if !strings.Contains(name+slideGap+name, got) {
			t.Errorf("tick %d: %q is not a piece of the name", tick, got)
		}
	}
}
