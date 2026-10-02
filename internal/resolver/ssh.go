package resolver

import "strings"

const (
	// sshKind marks an ssh session: `ssh › prod-01`, or `ssh` alone when the
	// host cannot be read. It goes in the context, never the activity the
	// terminal title outranks — see docs/architecture/title-resolution.md.
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

		tunnel, consumesNext := sshFlagGroup(arg)
		if tunnel {
			return true
		}

		if consumesNext {
			i++
		}
	}

	return false
}

// sshFlagGroup reads one cluster of short flags such as -fNp: whether it holds
// -N, and whether its last flag takes its value from the next argument.
func sshFlagGroup(arg string) (tunnel, consumesNext bool) {
	for j := 1; j < len(arg); j++ {
		if _, takesValue := sshFlagsWithValue[arg[j]]; takesValue {
			return false, j == len(arg)-1
		}

		if arg[j] == 'N' {
			return true, false
		}
	}

	return false, false
}

// sshHost extracts the destination: the first argument that is not an option or
// an option's value. Everything after it is the remote command, left to the
// terminal title to report.
func sshHost(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return hostOf(firstPositional(args[1:], sshTakesNext))
}

// sshTakesNext reports an option whose value is the next argument: the last
// flag of a cluster takes one, unless the value is attached, as in -p2222.
func sshTakesNext(option string) bool {
	if option == "" {
		return false
	}

	_, takesValue := sshFlagsWithValue[option[len(option)-1]]

	return takesValue
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
