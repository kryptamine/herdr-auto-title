package resolver

import "strings"

const (
	// moshKind marks a mosh session the way sshKind marks an ssh one, on the
	// host: `mosh › prod-01`, or `mosh` alone when the host cannot be read.
	moshKind = "mosh"

	// moshTitlePrefix is what mosh-client puts before the remote title unless
	// MOSH_TITLE_NOPREFIX is set. The context already says it.
	moshTitlePrefix = "[mosh]"

	// moshSSHOption names mosh's --ssh option. It equals sshKind only by
	// coincidence: one is an option name, the other a mark in a title.
	moshSSHOption = "ssh"
)

// moshValueOptions are mosh's options whose value may be a separate word.
var moshValueOptions = []string{
	"client", "server", "predict", "port", "family", "p", moshSSHOption, "bind-server",
	"experimental-remote-ip",
}

// moshHost reads the destination out of the one argument the `mosh` wrapper
// passes as `-# <its own arguments> |`, ahead of the server's address and port.
// The wrapper joined them with spaces, so a quoted --ssh value is split too.
func moshHost(args []string) string {
	if len(args) < 2 {
		return ""
	}

	line, ok := strings.CutPrefix(strings.TrimSpace(args[1]), "-#")
	if !ok {
		return ""
	}

	line, _ = strings.CutSuffix(line, "|")

	// The words after --ssh are read as ssh's options, so the key in
	// `--ssh=ssh -i key` is not taken for the host.
	inSSH := false

	return hostOf(firstPositional(strings.Fields(line), func(option string) bool {
		long, isLong := strings.CutPrefix(option, "--")
		if !isLong && inSSH {
			return sshTakesNext(option)
		}

		name, _, attached := strings.Cut(strings.TrimPrefix(long, "-"), "=")
		inSSH = name == moshSSHOption

		return !attached && moshTakesValue(name)
	}))
}

// moshTakesValue reads a long option as Getopt::Long does: after one dash or
// two, and abbreviated to any prefix. An ambiguous one makes the wrapper exit
// before mosh-client runs, so it never reaches a pane.
func moshTakesValue(name string) bool {
	for _, option := range moshValueOptions {
		if strings.HasPrefix(option, name) {
			return true
		}
	}

	return false
}
