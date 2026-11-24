//go:build !windows

package desktop

import (
	"path/filepath"
	"testing"
)

// Each branch is a place herdr-core puts its socket; a wrong one reads as a
// window that is never open.
func TestCoreSocketPath(t *testing.T) {
	runUser := t.TempDir()
	missing := filepath.Join(runUser, "missing")

	cases := []struct {
		name, runtime, state, runUser, session, want string
	}{
		{"XDG_RUNTIME_DIR", "/rt", "/st", runUser, "default", "/rt/herdr-core/default.sock"},
		{
			"/run/user",
			"",
			"/st",
			runUser,
			"default",
			filepath.Join(runUser, "herdr-core", "default.sock"),
		},
		{"XDG_STATE_HOME", "", "/st", missing, "default", "/st/herdr-core/default/default.sock"},
		{"home", "", "", missing, "default", "/h/.local/state/herdr-core/default/default.sock"},
		{"named session", "/rt", "/st", runUser, "devbox", "/rt/herdr-core/devbox.sock"},
		{
			"named session in state",
			"",
			"/st",
			missing,
			"devbox",
			"/st/herdr-core/devbox/devbox.sock",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_RUNTIME_DIR", c.runtime)
			t.Setenv("XDG_STATE_HOME", c.state)

			if got := coreSocketIn("/h", c.runUser, c.session); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
