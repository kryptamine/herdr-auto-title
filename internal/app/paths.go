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
// that already holds it, so a file is read where it is, and the last one when
// none does. See docs/architecture/configuration.md for the ordering.
func ownPath(name string) string {
	dirs := configDirs()
	if len(dirs) == 0 {
		return ""
	}

	for _, dir := range dirs {
		path := filepath.Join(dir, ownDir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// A new file is machine state, so it goes where the platform keeps it and
	// never starts a ~/.config that a dotfiles repository would then sync.
	return filepath.Join(dirs[len(dirs)-1], ownDir, name)
}

// configDirs are the directories Auto Title's files are looked for in, in
// order: XDG_CONFIG_HOME, ~/.config, and what the platform offers, which is
// Application Support on macOS and %APPDATA% on Windows.
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

	// Last, because a new file is created in the last directory. On Linux this
	// repeats one of the two above, so a new file lands there as before.
	if dir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, dir)
	}

	return dirs
}
