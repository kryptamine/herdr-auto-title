package herdrtest

import (
	"path/filepath"
	"runtime"
)

// Dir is an absolute directory as Herdr reports one on the platform the tests
// run on. A fixture spelled `/Users/dev/work` is relative on Windows, where
// filepath.IsAbs wants a drive, and a relative directory names no tab.
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
