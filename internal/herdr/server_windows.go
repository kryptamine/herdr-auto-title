package herdr

import "os"

// serverIdentity is the marker Herdr writes into the socket file when it binds
// the pipe, its pid and start time, so a successor's differs from its
// predecessor's; "" says no server holds the path.
func serverIdentity(path string) string {
	//nolint:gosec // the path is HERDR_SOCKET_PATH, never terminal-derived
	marker, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(marker)
}
