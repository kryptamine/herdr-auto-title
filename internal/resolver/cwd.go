package resolver

import (
	"os"
	"path/filepath"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// CWD derives the tab's context from the basename of the pane's working
// directory, which is normally the project name. The home directory, the
// filesystem root and a relative path all yield nothing.
type CWD struct {
	home string
}

var _ Source = CWD{}

// NewCWD builds the source, resolving the user's home directory once.
func NewCWD() CWD {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	return CWD{home: filepath.Clean(home)}
}

func (CWD) Name() string    { return "cwd" }
func (CWD) Confidence() int { return ConfidenceCWD }

func (c CWD) Resolve(pane *state.PaneState) (Parts, bool) {
	name := c.base(pane.Dir)
	if name == "" {
		return Parts{}, false
	}

	return Parts{Context: name}, true
}

// base returns the meaningful basename of dir, or "" when the path carries no
// useful context.
func (c CWD) base(dir string) string {
	if dir == "" {
		return ""
	}

	clean := filepath.Clean(dir)
	if !filepath.IsAbs(clean) {
		return ""
	}

	if clean == string(filepath.Separator) {
		return ""
	}

	if c.home != "" && clean == c.home {
		return ""
	}

	base := filepath.Base(clean)
	switch base {
	case ".", "..", string(filepath.Separator):
		return ""
	}

	return base
}
