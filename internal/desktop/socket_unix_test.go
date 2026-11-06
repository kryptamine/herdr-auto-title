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
		name, runtime, state, runUser, want string
	}{
		{"XDG_RUNTIME_DIR", "/rt", "/st", runUser, "/rt/herdr-core/default.sock"},
		{"/run/user", "", "/st", runUser, filepath.Join(runUser, "herdr-core", "default.sock")},
		{"XDG_STATE_HOME", "", "/st", missing, "/st/herdr-core/default/default.sock"},
		{"home", "", "", missing, "/h/.local/state/herdr-core/default/default.sock"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_RUNTIME_DIR", c.runtime)
			t.Setenv("XDG_STATE_HOME", c.state)

			if got := coreSocketIn("/h", c.runUser); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
