package desktop

import "testing"

// hc's own choice of core is what the user configured; reading another session
// would rename nothing, or the wrong window's tabs.
func TestCoreEndpointFollowsHC(t *testing.T) {
	cases := []struct {
		name, socket, session, want string
	}{
		{"default session", "", "", coreSocketPath("/h", "default")},
		{"HC_SESSION", "", "devbox", coreSocketPath("/h", "devbox")},
		{"HC_SOCKET wins", "/custom/core.sock", "devbox", "/custom/core.sock"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HC_SOCKET", c.socket)
			t.Setenv("HC_SESSION", c.session)

			if got := coreEndpoint("/h"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
