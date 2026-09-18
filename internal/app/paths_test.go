package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeOwnFile puts contents into Auto Title's directory inside dir, as one of
// the places ownPath looks, and reports where it wrote it.
func writeOwnFile(t *testing.T, dir, name, contents string) string {
	t.Helper()

	path := filepath.Join(dir, ownDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

// pollFound is the poll interval LoadConfig read, which the tests below use to
// tell the configuration files they put in different directories apart.
func pollFound(t *testing.T) time.Duration {
	t.Helper()

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}

	return cfg.Poll
}

// dotConfig is the home's ~/.config, %USERPROFILE%\.config on Windows.
func dotConfig(t *testing.T) string {
	t.Helper()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	return filepath.Join(home, ".config")
}

// platformDir is what the platform offers, which is where a macOS or Windows
// install keeps Auto Title's files when nothing is in ~/.config.
func platformDir(t *testing.T) string {
	t.Helper()

	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	if dir == dotConfig(t) {
		t.Skip("the platform directory is ~/.config here, so there is no second place to look")
	}

	return dir
}

func TestXDGConfigHomeIsLookedInFirst(t *testing.T) {
	isolate(t)
	xdg := t.TempDir()
	t.Setenv(EnvXDGConfigHome, xdg)
	writeOwnFile(t, xdg, ConfigFile, "HERDR_AUTO_TITLE_POLL_MS=800\n")

	if poll := pollFound(t); poll != 800*time.Millisecond {
		t.Errorf("poll = %s, want the 800ms the file in XDG_CONFIG_HOME asks for", poll)
	}
}

func TestARelativeXDGConfigHomeIsIgnored(t *testing.T) {
	// The specification calls a relative value invalid, and honouring one would
	// put the file wherever the Herdr server happened to be started.
	isolate(t)
	t.Setenv(EnvXDGConfigHome, filepath.Join("somewhere", "relative"))

	path := ownPath(ConfigFile)
	if !filepath.IsAbs(path) {
		t.Errorf("configuration path = %q, want an absolute one", path)
	}

	if strings.Contains(path, "relative") {
		t.Errorf("configuration path = %q, want it not to come from a relative variable", path)
	}
}

func TestTheHomeConfigDirectoryIsLookedIn(t *testing.T) {
	// The point of the ordering: on macOS and Windows too, so one dotfiles
	// repository serves every machine.
	isolate(t)
	writeOwnFile(t, dotConfig(t), ConfigFile, "HERDR_AUTO_TITLE_POLL_MS=800\n")

	if poll := pollFound(t); poll != 800*time.Millisecond {
		t.Errorf("poll = %s, want the 800ms the file in ~/.config asks for", poll)
	}
}

func TestThePlatformDirectoryIsStillLookedIn(t *testing.T) {
	// An install that predates the ordering keeps working without being moved.
	isolate(t)
	writeOwnFile(t, platformDir(t), ConfigFile, "HERDR_AUTO_TITLE_POLL_MS=800\n")

	if poll := pollFound(t); poll != 800*time.Millisecond {
		t.Errorf("poll = %s, want the 800ms the platform directory asks for", poll)
	}
}

func TestTheHomeConfigDirectoryBeatsThePlatformOne(t *testing.T) {
	isolate(t)
	platform := platformDir(t)
	config := dotConfig(t)
	writeOwnFile(t, platform, ConfigFile, "HERDR_AUTO_TITLE_POLL_MS=900\n")
	writeOwnFile(t, config, ConfigFile, "HERDR_AUTO_TITLE_POLL_MS=800\n")

	if poll := pollFound(t); poll != 800*time.Millisecond {
		t.Errorf("poll = %s, want the 800ms ~/.config asks for", poll)
	}
}

func TestANewFileGoesToThePlatformDirectory(t *testing.T) {
	// Locks are machine state: a fresh install must not start a ~/.config that
	// a dotfiles repository would then sync.
	isolate(t)
	want := filepath.Join(platformDir(t), ownDir, manualFile)

	cfg, _ := LoadConfig()
	if cfg.ManualPath != want {
		t.Errorf("manual path = %q, want %q in the platform directory", cfg.ManualPath, want)
	}
}

func TestEachFileIsLookedForOnItsOwn(t *testing.T) {
	// A user who moves config.env into a dotfiles repository keeps the locks
	// the platform directory already holds, which are not dotfiles.
	isolate(t)
	platform := platformDir(t)
	config := dotConfig(t)
	locks := writeOwnFile(t, platform, manualFile, "{}\n")
	writeOwnFile(t, config, ConfigFile, "# moved into a dotfiles repository\n")

	cfg, _ := LoadConfig()
	if cfg.ManualPath != locks {
		t.Errorf(
			"manual path = %q, want the %q the platform directory holds",
			cfg.ManualPath,
			locks,
		)
	}
}
