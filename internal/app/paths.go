package app

import (
	"os"
	"path/filepath"
)

// EnvXDGConfigHome names the directory a user following the XDG convention
// keeps configuration in.
const EnvXDGConfigHome = "XDG_CONFIG_HOME"

// The files Auto Title keeps of its own, and the directory holding them inside
// one of the configuration directories configDirs lists.
const (
	ownDir     = "herdr-auto-title"
	ConfigFile = "config.env"
	manualFile = "manual-names.json"
)

// ownPath is where Auto Title's file called name lives: the first directory
// that already holds it, so a file is read where it is, and the preferred one
// when none does. See docs/architecture/configuration.md for the ordering.
func ownPath(name string) string {
	var preferred string

	for _, dir := range configDirs() {
		path := filepath.Join(dir, ownDir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}

		if preferred == "" {
			preferred = path
		}
	}

	return preferred
}

// configDirs are the directories Auto Title's files are looked for in, the
// preferred one first: XDG_CONFIG_HOME, ~/.config, and what the platform
// offers, which is Application Support on macOS and %APPDATA% on Windows.
func configDirs() []string {
	var dirs []string

	// A relative XDG_CONFIG_HOME is against the specification, and would put
	// the file wherever the Herdr server happened to be started.
	if xdg := os.Getenv(EnvXDGConfigHome); filepath.IsAbs(xdg) {
		dirs = append(dirs, xdg)
	}

	// ~/.config is where cross-platform tools keep configuration, Windows
	// included, as %USERPROFILE%\.config.
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config"))
	}

	// On Linux this repeats one of the two above, which costs a stat and
	// decides nothing: the first directory holding the file still wins.
	if dir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, dir)
	}

	return dirs
}
