package labels

import (
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// herdrMirrorStreamer reports the `herdr-mirror pane` invocation.
func herdrMirrorStreamer(args []string) bool {
	return len(args) > 1 && args[1] == "pane"
}

// owner is one program that owns its tab and pane labels.
type owner struct {
	process string
	match   func(args []string) bool
}

// Owners is the table of such programs, built once from the built-ins and the
// names in HERDR_AUTO_TITLE_FOREIGN_PROCESSES.
type Owners []owner

// NewOwners builds the table. extra names are matched by process name alone.
func NewOwners(extra []string) Owners {
	owners := Owners{{process: "herdr-mirror", match: herdrMirrorStreamer}}

	for _, name := range extra {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		owners = append(owners, owner{process: name, match: func([]string) bool { return true }})
	}

	return owners
}

// Owns reports whether pane is driven by one of the owners.
func (owners Owners) Owns(pane *state.PaneState) bool {
	if pane == nil {
		return false
	}

	process, ok := pane.Foreground()
	if !ok {
		return false
	}

	for _, owner := range owners {
		if strings.EqualFold(process.Name, owner.process) && owner.match(process.Args) {
			return true
		}
	}

	return false
}
