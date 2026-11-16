package herdrtest

import (
	"path/filepath"
	"runtime"
)

// Dir is an absolute directory as Herdr reports one on the platform the tests
// run on: under a drive on Windows, so that fixtures exercise filepath there
// rather than the Unix syntax a connected machine's directory arrives in.
func Dir(elems ...string) string {
	return filepath.Join(append([]string{Root(), "Users", "dev"}, elems...)...)
}

// Root is the filesystem root the fixtures hang off: `/`, or `C:\` on Windows.
func Root() string {
	if runtime.GOOS == "windows" {
		return `C:\`
	}

	return "/"
}
