package resolver

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// CWD derives the tab's context from the basename of the pane's working
// directory, which is normally the project name. The home directory, the
// filesystem root and a relative path all yield nothing.
type CWD struct {
	home string
}

var _ Source = CWD{}

// NewCWD builds the source for a user whose home directory is home, which
// names no project. An empty home is one that could not be found.
func NewCWD(home string) CWD {
	if home == "" {
		return CWD{}
	}

	return CWD{home: filepath.Clean(home)}
}

func (CWD) Name() string    { return "cwd" }
func (CWD) Confidence() int { return ConfidenceCWD }

func (c CWD) Resolve(pane *state.PaneState) (Parts, bool) {
	name := pane.DirName()
	if name == "" || c.home != "" && sameDir(pane.Dir, c.home) {
		return Parts{}, false
	}

	return Parts{Context: name}, true
}

// sameDir compares two clean paths the way the platform does: on Windows a
// path spelled in another case is the same directory.
func sameDir(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}

	return a == b
}
