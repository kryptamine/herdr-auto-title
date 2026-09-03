package resolver

import (
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

const (
	// sshKind marks a session as remote: `ssh › prod-01`, or `ssh` alone when
	// the host cannot be read. It goes in the context, never the activity,
	// which the terminal title would outrank — see docs/architecture.
	sshKind = "ssh"
)

// sshFlagsWithValue are the options whose value is a separate argument, so
// `ssh -p 2222 prod-01` does not read 2222 as the destination. Everything else
// starting with a dash is a switch or carries its value attached.
var sshFlagsWithValue = map[byte]struct{}{
	'B': {}, 'b': {}, 'c': {}, 'D': {}, 'E': {}, 'e': {}, 'F': {}, 'I': {},
	'i': {}, 'J': {}, 'L': {}, 'l': {}, 'm': {}, 'O': {}, 'o': {}, 'P': {},
	'p': {}, 'Q': {}, 'R': {}, 'S': {}, 'W': {}, 'w': {},
}

// SSH makes the host the tab's context: `ssh › prod-01`. A remote shell's
// directory and title describe the remote but never say which machine, which is
// what a row of identical shells needs. The user is dropped.
type SSH struct{}

var _ Source = SSH{}

func NewSSH() SSH { return SSH{} }

func (SSH) Name() string    { return "ssh" }
func (SSH) Confidence() int { return ConfidenceSSH }

func (SSH) Resolve(pane *state.PaneState) (Parts, bool) {
	args, running := sshArgs(pane)
	if !running {
		return Parts{}, false
	}

	// With no host to bind it to the mark stands alone, but it still goes in
	// the context slot: an activity would be outranked by the remote shell's
	// own title, exactly when the tab most needs to say it is remote.
	host := Sanitize(sshHost(args), 0)
	if host == "" {
		return Parts{Context: sshKind}, true
	}

	return Parts{Context: qualify(host, sshKind)}, true
}

// sshArgs finds an ssh process in the pane and returns its arguments. A
// tunnel (`ssh -N`) runs no remote shell and is skipped.
func sshArgs(pane *state.PaneState) ([]string, bool) {
	for _, process := range pane.Processes {
		if strings.EqualFold(process.Name, "ssh") && !sshIsTunnel(process.Args) {
			return process.Args, true
		}
	}

	return nil, false
}

// sshIsTunnel reports whether -N appears before the destination; -pN is a port.
func sshIsTunnel(args []string) bool {
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-" {
			continue
		}

		if len(arg) < 2 || arg[0] != '-' {
			return false
		}

		for j := 1; j < len(arg); j++ {
			if _, takesValue := sshFlagsWithValue[arg[j]]; takesValue {
				if j == len(arg)-1 {
					i++
				}

				break
			}

			if arg[j] == 'N' {
				return true
			}
		}
	}

	return false
}

// sshHost extracts the destination: the first argument that is not an option or
// an option's value. Everything after it is the remote command, left to the
// terminal title to report.
func sshHost(args []string) string {
	if len(args) == 0 {
		return ""
	}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			// Everything after this is positional.
			if i+1 < len(args) {
				return hostOf(args[i+1])
			}

			return ""
		case len(arg) > 1 && arg[0] == '-':
			// A flag whose value is a separate argument consumes the next one,
			// unless the value is attached: -p2222 carries its own.
			last := arg[len(arg)-1]
			if _, takesValue := sshFlagsWithValue[last]; takesValue {
				i++
			}
		case arg == "-":
			// Not a destination and not a flag; ignore it.
		default:
			return hostOf(arg)
		}
	}

	return ""
}

// hostOf reduces an ssh destination to the host alone, dropping the scheme, the
// user and the port from `ssh://deploy@prod-01:2222`.
func hostOf(destination string) string {
	host := strings.TrimSpace(destination)

	host = strings.TrimPrefix(host, "ssh://")
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	// A URL destination can carry a path.
	if slash := strings.Index(host, "/"); slash >= 0 {
		host = host[:slash]
	}

	// An IPv6 literal is bracketed, and only a port may follow the bracket.
	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end >= 0 {
			return host[1:end]
		}

		return ""
	}
	// A bare colon is a port; a host with several is an unbracketed IPv6
	// literal, which has no port to strip.
	if strings.Count(host, ":") == 1 {
		host, _, _ = strings.Cut(host, ":")
	}

	return host
}
