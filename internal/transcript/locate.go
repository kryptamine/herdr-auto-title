package transcript

import (
	"os"
	"path/filepath"
	"time"
)

// LocateRetry is how long a session whose file was not found is left alone.
// Herdr can name a session before the agent has written its file, so the
// search is repeated, just not twice a second for the pane's whole life.
const LocateRetry = 10 * time.Second

// Layout is where an agent files a session it found by id under one of its
// homes: usually under the directory the session started in, else anywhere.
// An agent that files a session by date rather than directory has no Filed.
type Layout struct {
	Filed    func(root, dir, sessionID string) string
	Anywhere func(root, sessionID string) string
	// Pick chooses among the files Anywhere matched, in the order Glob sorts
	// them. The first is taken when there is none. A session that can go on in
	// a newer file needs one, and its search is repeated after it was found.
	Pick func(matches []string) string
}

// Search is one session's file as far as it has been looked for. An empty
// Path is a session not found yet, and a reader clears it to look again.
type Search struct {
	Path       string
	searchedAt time.Time
}

// Find fills in s.Path and reports whether there is a file to read. Roots are
// searched in order, the pane's directory first within each; a search is made
// at most every LocateRetry, since its glob walks every project.
func (l Layout) Find(s *Search, roots []string, sessionID, dir string, now time.Time) bool {
	if s.Path != "" && l.Pick == nil {
		return true
	}

	if !s.searchedAt.IsZero() && now.Sub(s.searchedAt) < LocateRetry {
		return s.Path != ""
	}

	s.searchedAt = now

	// A file that is gone is noticed by the read, not by a search that misses.
	if path := l.locate(roots, sessionID, dir); path != "" {
		s.Path = path
	}

	return s.Path != ""
}

func (l Layout) locate(roots []string, sessionID, dir string) string {
	for _, root := range roots {
		if dir != "" && l.Filed != nil {
			if candidate := l.Filed(root, dir, sessionID); regular(candidate) {
				return candidate
			}
		}

		matches, err := filepath.Glob(l.Anywhere(root, sessionID))
		if err == nil && len(matches) > 0 {
			return l.pick(matches)
		}
	}

	return ""
}

func (l Layout) pick(matches []string) string {
	if l.Pick == nil {
		return matches[0]
	}

	return l.Pick(matches)
}

func regular(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.Mode().IsRegular()
}
