package resolver

import (
	"strings"

	"github.com/rivo/uniseg"
)

// slideGap is what separates the end of a sliding name from its next round.
const slideGap = "   "

// slidePause is the ticks a sliding name holds its head still before it moves:
// long enough to read where the title starts, at the default poll rate.
const slidePause = 4

// DefaultScrollStep is how many columns a sliding title moves per tick.
const DefaultScrollStep = 1

// Slide shows width columns of a name too wide for them: the head first, then
// step columns further in per tick, round to the head again after a gap. The
// step is at least one.
func Slide(name string, width, tick, step int) string {
	if width <= 0 {
		return name
	}

	total := uniseg.StringWidth(name)
	if total <= width {
		return name
	}

	// A lap is the name and the gap behind it. Ticks are counted per lap so
	// that the head is held still at the start of every one, not only the first.
	lap := total + len(slideGap)
	moves := (lap + step - 1) / step
	offset := max(tick%(slidePause+moves)-slidePause, 0) * step

	_, rest := splitAtWidth(name+slideGap+name, offset)
	head, _ := splitAtWidth(rest, width)

	// A cluster two columns wide does not always land on the edge, and a
	// window one column short would shrink the tab it is sliding in.
	return head + strings.Repeat(" ", width-uniseg.StringWidth(head))
}
