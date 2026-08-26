package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// Environment variables Auto Title reads. The configuration file holds the
// same names; see docs/architecture/configuration.md.
const (
	EnvDebug      = "HERDR_AUTO_TITLE_DEBUG"
	EnvPoll       = "HERDR_AUTO_TITLE_POLL_MS"
	EnvMaxLength  = "HERDR_AUTO_TITLE_MAX_LENGTH"
	EnvBranchMax  = "HERDR_AUTO_TITLE_BRANCH_MAX"
	EnvPosition   = "HERDR_AUTO_TITLE_POSITION"
	EnvManual     = "HERDR_AUTO_TITLE_MANUAL_FILE"
	EnvTranscript = "HERDR_AUTO_TITLE_TRANSCRIPT"
)

// ConfigFile is the configuration file, read from the same directory the
// manual-rename locks are kept in.
const ConfigFile = "config.env"

// DefaultPoll is how often the session is read. A six-pane snapshot measured
// 0.47 ms and 6 KB, so twice a second costs about a thousandth of a core, and
// a rename lands while the user is still looking at the tab.
const DefaultPoll = 500 * time.Millisecond

// Config is Auto Title's runtime configuration.
type Config struct {
	Debug bool
	Poll  time.Duration
	// MaxLength and BranchMax are measured in columns of the tab bar rather
	// than in characters: a CJK character or an emoji takes two.
	MaxLength int
	// BranchMax bounds what a git branch may add to a title. Zero leaves
	// branches out of titles entirely.
	BranchMax int
	// ShowPosition puts each tab's position in front of its title, which is
	// the key that switches to that tab.
	ShowPosition bool
	// ManualPath is where tabs the user renamed by hand are remembered across
	// restarts. Empty keeps them in memory only.
	ManualPath string
	// ReadTranscripts lets a title come from an agent's own session transcript
	// when the agent has not titled its terminal. It reads what the user has
	// been saying to that agent, so it can be turned off.
	ReadTranscripts bool
}

// LoadConfig reads configuration from the configuration file and the
// environment. Unusable values are reported as warnings and the default is
// kept, so a typo never stops the plugin from running.
func LoadConfig() (Config, []string) {
	var warnings []string
	if warning := readConfigFile(); warning != "" {
		warnings = append(warnings, warning)
	}

	cfg := Config{
		Poll:            DefaultPoll,
		MaxLength:       resolver.DefaultMaxLength,
		BranchMax:       resolver.DefaultBranchMaxLength,
		ShowPosition:    true,
		ManualPath:      state.DefaultManualPath(),
		ReadTranscripts: true,
	}

	cfg.Debug = fromEnv(&warnings, EnvDebug, cfg.Debug, boolean)
	cfg.Poll = fromEnv(&warnings, EnvPoll, cfg.Poll, milliseconds)
	cfg.MaxLength = fromEnv(&warnings, EnvMaxLength, cfg.MaxLength, count)
	// Zero is meaningful here, and only here: it leaves branches out of titles.
	cfg.BranchMax = fromEnv(&warnings, EnvBranchMax, cfg.BranchMax, countOrNone)
	cfg.ShowPosition = fromEnv(&warnings, EnvPosition, cfg.ShowPosition, boolean)
	cfg.ReadTranscripts = fromEnv(&warnings, EnvTranscript, cfg.ReadTranscripts, boolean)
	// A path needs neither parsing nor checking, so it does not go through
	// fromEnv: any string the user set is the path they meant.
	if raw := os.Getenv(EnvManual); raw != "" {
		cfg.ManualPath = raw
	}

	return cfg, warnings
}

// readConfigFile puts the file's settings into the environment, which is how a
// setting reaches a plugin the Herdr server starts: that process inherits the
// server's environment, never the user's shell.
func readConfigFile() string {
	path := configPath()
	if path == "" {
		return ""
	}
	// Load leaves a variable already in the environment alone, which is what
	// makes the environment win over the file.
	err := godotenv.Load(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	// One bad line costs the whole file: godotenv parses it or nothing.
	return fmt.Sprintf("%s %s, so nothing in it is used", path, err)
}

// configPath is where the configuration file lives, or empty when the user has
// no configuration directory at all.
func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	return filepath.Join(dir, "herdr-auto-title", ConfigFile)
}

// fromEnv returns what the environment says name is, or fallback when it says
// nothing usable. Nothing here fails: a typo must not stop the plugin starting,
// which is why it warns instead, and is the only place a warning is worded.
func fromEnv[T any](warnings *[]string, name string, fallback T, convert converter[T]) T {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	value, err := convert(raw)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s=%q %s, using %v", name, raw, err, fallback))
		return fallback
	}

	return value
}

// converter turns the raw text of a variable into a value, or says what is
// wrong with it.
type converter[T any] func(raw string) (T, error)

// The reasons a variable is rejected. Each reads as the middle of the warning
// it lands in: `HERDR_AUTO_TITLE_POLL_MS="0" must be positive, using 500ms`.
var (
	errNotBoolean  = errors.New("is not a boolean")
	errNotNumber   = errors.New("is not a number")
	errNotPositive = errors.New("must be positive")
	errNegative    = errors.New("cannot be negative")
)

func boolean(raw string) (bool, error) {
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, errNotBoolean
	}

	return value, nil
}

// count reads a number that must be positive. Zero would stop the plugin doing
// anything: a poll interval of zero spins, and a title of no length is no title.
func count(raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errNotNumber
	}

	if value <= 0 {
		return 0, errNotPositive
	}

	return value, nil
}

// countOrNone reads a number that may be zero, for a setting where zero means
// "none of this" rather than a value that would stop the plugin working.
func countOrNone(raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errNotNumber
	}

	if value < 0 {
		return 0, errNegative
	}

	return value, nil
}

// milliseconds reads a duration written as a count of them, which is easier to
// pass through a shell than "500ms".
func milliseconds(raw string) (time.Duration, error) {
	value, err := count(raw)
	if err != nil {
		return 0, err
	}

	return time.Duration(value) * time.Millisecond, nil
}
