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
type Layout struct {
	Filed    func(root, dir, sessionID string) string
	Anywhere func(root, sessionID string) string
}

// Search is one session's file as far as it has been looked for. An empty
// Path is a session not found yet, and a reader clears it to look again.
type Search struct {
	Path       string
	searchedAt time.Time
}

// Find fills in s.Path and reports whether there is a file to read. Roots are
// searched in order, the pane's directory first within each; a search that came
// up empty waits LocateRetry, since its glob walks every project.
func (l Layout) Find(s *Search, roots []string, sessionID, dir string, now time.Time) bool {
	if s.Path != "" {
		return true
	}

	if !s.searchedAt.IsZero() && now.Sub(s.searchedAt) < LocateRetry {
		return false
	}

	s.searchedAt = now

	for _, root := range roots {
		if dir != "" {
			if candidate := l.Filed(root, dir, sessionID); regular(candidate) {
				s.Path = candidate

				return true
			}
		}

		matches, err := filepath.Glob(l.Anywhere(root, sessionID))
		if err == nil && len(matches) > 0 {
			s.Path = matches[0]

			return true
		}
	}

	return false
}

func regular(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.Mode().IsRegular()
}
