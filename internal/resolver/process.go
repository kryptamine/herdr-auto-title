package resolver

import (
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// shellNames are the programs that run in a pane without being what the pane is
// for. A pane running a shell is described by what the shell is running.
var shellNames = map[string]struct{}{
	"bash": {}, "zsh": {}, "fish": {}, "sh": {}, "dash": {}, "ksh": {},
	"tcsh": {}, "csh": {}, "login": {},
}

// paneKind names what a pane is running, or "" when that cannot be said. The
// agent comes first because a process list never names one, and only a lone
// process names a pane: a build tool is `esbuild` and five `node`s.
func paneKind(pane *state.PaneState) string {
	if pane.HasAgent() {
		// An agent naming itself is not a report of its work, which is why the
		// name is generic as an activity. As a kind it names the program,
		// which is all a kind ever does.
		return Sanitize(pane.Agent, 0)
	}

	var kind string

	for _, process := range pane.Processes {
		name := strings.TrimSpace(process.Name)
		if name == "" {
			continue
		}

		if _, isShell := shellNames[strings.ToLower(name)]; isShell {
			continue
		}

		if kind != "" {
			// More than one candidate; nothing here names the pane.
			return ""
		}

		kind = name
	}

	switch strings.ToLower(kind) {
	case "":
		return ""
	case "ssh":
		// A remote session is marked on the host, where the mark cannot be
		// outranked. Saying it again here would only repeat it.
		return ""
	}

	if isGeneric(strings.ToLower(kind)) {
		// A runtime names the language, not the work.
		return ""
	}

	return Sanitize(kind, 0)
}

// qualify binds a kind to what a source found, so a title reads
// `nvim:auth.provider.ts`. A kind with nothing left to add stands alone, and
// without a kind the activity is unchanged.
func qualify(activity, kind string) string {
	if kind == "" {
		return activity
	}

	detail := stripKind(activity, kind)
	if detail == "" {
		return kind
	}

	return kind + Separator + detail
}

// stripKind removes the kind from a detail that already carries it, so a kind
// and its detail never say the same thing twice: Neovim titles its window
// `auth.provider.ts - Nvim`, and under the kind `nvim` the suffix is noise.
func stripKind(detail, kind string) string {
	trimmed := strings.TrimSpace(detail)
	if trimmed == "" || kind == "" {
		return trimmed
	}

	// Compared fold-wise rather than through a lower-cased copy: lower-casing
	// can change how many bytes a character takes, and the cut is by bytes.
	switch {
	case strings.EqualFold(trimmed, kind):
		return ""
	case len(trimmed) > len(kind) && strings.EqualFold(trimmed[len(trimmed)-len(kind):], kind):
		trimmed = trimmed[:len(trimmed)-len(kind)]
	case len(trimmed) > len(kind) && strings.EqualFold(trimmed[:len(kind)], kind):
		trimmed = trimmed[len(kind):]
	}

	return strings.Trim(trimmed, " -–—|:"+separatorRune)
}

// Process names a pane after the program running in it when nothing has said
// what that program is doing: an editor with no file open still reads
// `dashboard › nvim`.
type Process struct{}

var _ Source = Process{}

func NewProcess() Process { return Process{} }

func (Process) Name() string    { return "process" }
func (Process) Confidence() int { return ConfidenceProcess }

func (Process) Resolve(pane *state.PaneState) (Parts, bool) {
	kind := paneKind(pane)
	if kind == "" {
		return Parts{}, false
	}

	return Parts{Activity: kind}, true
}
