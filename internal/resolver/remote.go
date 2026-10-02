package resolver

import (
	"strings"

	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// remote is a program that runs a shell on another machine, so a pane running
// it is named after that machine. A new transport is one more row in remotes.
type remote struct {
	// kind is the mark in a title, `ssh › prod-01`, and the command a local
	// shell's title echoes while the session connects.
	kind    string
	process string
	host    func(args []string) string
	// skip reports a run that opens no remote shell, such as a tunnel.
	skip        func(args []string) bool
	titlePrefix string
}

var remotes = []remote{
	{kind: sshKind, process: "ssh", host: sshHost, skip: sshIsTunnel},
}

// remoteOf returns the remote session a pane is running and its arguments. Only
// the foreground process counts: git and agents start ssh of their own, and so
// does ssh for a jump host.
func remoteOf(pane *state.PaneState) (remote, []string, bool) {
	process, ok := pane.Foreground()
	if !ok {
		return remote{}, nil, false
	}

	for _, session := range remotes {
		if strings.EqualFold(process.Name, session.process) &&
			(session.skip == nil || !session.skip(process.Args)) {
			return session, process.Args, true
		}
	}

	return remote{}, nil, false
}

// runsRemote reports a process name that runs a remote session, whatever its
// arguments.
func runsRemote(name string) bool {
	for _, session := range remotes {
		if strings.EqualFold(name, session.process) {
			return true
		}
	}

	return false
}

// withoutPrefix drops the mark the program puts before the remote title.
func (r remote) withoutPrefix(title string) string {
	if r.titlePrefix == "" {
		return title
	}

	if rest, ok := strings.CutPrefix(strings.TrimSpace(title), r.titlePrefix); ok {
		return strings.TrimSpace(rest)
	}

	return title
}

// echoesCommand reports a title that is the local shell's command line for the
// session. It is matched by its first word, not the host: fish trims the
// command to twenty columns, `ssh deploy@productio`.
func (r remote) echoesCommand(title string) bool {
	command, _, _ := strings.Cut(strings.TrimSpace(title), " ")

	return strings.EqualFold(command, r.kind)
}

// Remote makes the host the tab's context: `ssh › prod-01`. A remote shell's
// directory and title describe the remote but never say which machine, which is
// what a row of identical shells needs. The user is dropped.
type Remote struct{}

var _ Source = Remote{}

func NewRemote() Remote { return Remote{} }

func (Remote) Name() string    { return "remote" }
func (Remote) Confidence() int { return ConfidenceRemote }

func (Remote) Resolve(pane *state.PaneState) (Parts, bool) {
	session, args, running := remoteOf(pane)
	if !running {
		return Parts{}, false
	}

	// With no host to bind it to the mark stands alone, but it still goes in
	// the context slot: an activity would be outranked by the remote shell's
	// own title, exactly when the tab most needs to say it is remote.
	host := Sanitize(session.host(args), 0)
	if host == "" {
		return Parts{Context: session.kind}, true
	}

	return Parts{Context: qualify(host, session.kind)}, true
}

// firstPositional returns the first word that is neither an option nor the
// value of one, or the word after `--`. takesNext says whether an option's
// value is the next word.
func firstPositional(words []string, takesNext func(option string) bool) string {
	for i := 0; i < len(words); i++ {
		word := words[i]
		switch {
		case word == "--":
			if i+1 < len(words) {
				return words[i+1]
			}

			return ""
		case len(word) > 1 && word[0] == '-':
			if takesNext(word) {
				i++
			}
		case word == "-":
			// Not a destination and not an option; ignore it.
		default:
			return word
		}
	}

	return ""
}
