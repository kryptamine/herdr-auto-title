//go:build linux

package hermes

import (
	"io"
	"os"
	"strconv"
	"strings"
)

const maxEnvironmentBytes = 1 << 20

// ProcessHome reads only the foreground process's startup environment; child
// tools can belong to a different profile. No process is executed.
func ProcessHome(pid int) string {
	if pid <= 0 {
		return ""
	}

	file, err := os.Open("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return ""
	}

	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxEnvironmentBytes+1))
	if err != nil || len(data) > maxEnvironmentBytes {
		return ""
	}

	for entry := range strings.SplitSeq(string(data), "\x00") {
		if home, ok := strings.CutPrefix(entry, "HERMES_HOME="); ok {
			return home
		}
	}

	return ""
}
