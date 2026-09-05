package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// isolate takes a test off the developer's machine: HOME decides where the
// configuration file is looked for, and every variable Auto Title reads is
// removed so the test sees only what it sets.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	// Where os.UserConfigDir looks first on Linux, so HOME alone does not
	// isolate anything until it is out of the way.
	t.Setenv("XDG_CONFIG_HOME", "")

	names := []string{
		EnvDebug, EnvPoll, EnvMaxLength, EnvBranchMax,
		EnvPosition, EnvManual, EnvTranscript, EnvAgentName,
	}

	for _, name := range names {
		// Setenv first for its cleanup, which then also undoes what the
		// configuration file sets; an empty variable is not an absent one.
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
}

// writeConfig puts contents where LoadConfig looks for the configuration file.
func writeConfig(t *testing.T, contents string) {
	t.Helper()

	path := configPath()
	if path == "" {
		t.Fatal("no configuration directory")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	isolate(t)

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if cfg.Debug {
		t.Error("debug is on by default")
	}

	if cfg.Poll != DefaultPoll {
		t.Errorf("poll = %s, want %s", cfg.Poll, DefaultPoll)
	}

	if cfg.MaxLength != resolver.DefaultMaxLength {
		t.Errorf("max length = %d, want %d", cfg.MaxLength, resolver.DefaultMaxLength)
	}

	if cfg.BranchMax != resolver.DefaultBranchMaxLength {
		t.Errorf("branch max = %d, want %d", cfg.BranchMax, resolver.DefaultBranchMaxLength)
	}

	if !cfg.ShowPosition {
		t.Error("positions are off by default")
	}

	if !cfg.ShowAgentName {
		t.Error("agent names are off by default")
	}
}

func TestLoadConfigTurnsPositionsOff(t *testing.T) {
	isolate(t)
	t.Setenv(EnvPosition, "false")

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if cfg.ShowPosition {
		t.Error("positions are on despite being disabled")
	}
}

func TestLoadConfigTurnsTheAgentNameOff(t *testing.T) {
	isolate(t)
	t.Setenv(EnvAgentName, "false")

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if cfg.ShowAgentName {
		t.Error("agent names are shown despite being disabled")
	}
}

func TestLoadConfigFromEnvironment(t *testing.T) {
	isolate(t)
	t.Setenv(EnvDebug, "true")
	t.Setenv(EnvPoll, "250")
	t.Setenv(EnvMaxLength, "32")
	t.Setenv(EnvBranchMax, "20")

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if !cfg.Debug {
		t.Error("debug is off despite being enabled")
	}

	if cfg.Poll != 250*time.Millisecond {
		t.Errorf("poll = %s, want 250ms", cfg.Poll)
	}

	if cfg.MaxLength != 32 {
		t.Errorf("max length = %d, want 32", cfg.MaxLength)
	}

	if cfg.BranchMax != 20 {
		t.Errorf("branch max = %d, want 20", cfg.BranchMax)
	}
}

func TestABranchWidthOfZeroIsAValue(t *testing.T) {
	// Zero is how branches are turned off, so it must pass where zero is
	// rejected everywhere else — and without a warning.
	isolate(t)
	t.Setenv(EnvBranchMax, "0")

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if cfg.BranchMax != 0 {
		t.Errorf("branch max = %d, want 0", cfg.BranchMax)
	}
}

func TestANegativeBranchWidthIsRejected(t *testing.T) {
	isolate(t)
	t.Setenv(EnvBranchMax, "-1")

	cfg, warnings := LoadConfig()
	if cfg.BranchMax != resolver.DefaultBranchMaxLength {
		t.Errorf("branch max = %d, want the default", cfg.BranchMax)
	}

	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one", warnings)
	}

	if !strings.Contains(warnings[0], "cannot be negative") {
		t.Errorf("warning %q does not say what is wrong", warnings[0])
	}
}

func TestLoadConfigKeepsDefaultsOnBadValues(t *testing.T) {
	isolate(t)
	t.Setenv(EnvDebug, "yes please")
	t.Setenv(EnvPoll, "-5")
	t.Setenv(EnvMaxLength, "plenty")

	cfg, warnings := LoadConfig()
	if len(warnings) != 3 {
		t.Errorf("warnings = %v, want one per bad value", warnings)
	}

	if cfg.Debug {
		t.Error("debug was enabled by an unparseable value")
	}

	if cfg.Poll != DefaultPoll {
		t.Errorf("poll = %s, want the default %s", cfg.Poll, DefaultPoll)
	}

	if cfg.MaxLength != resolver.DefaultMaxLength {
		t.Errorf("max length = %d, want the default %d", cfg.MaxLength, resolver.DefaultMaxLength)
	}
}

func TestAnUnusableValueIsReportedInFull(t *testing.T) {
	isolate(t)
	t.Setenv(EnvMaxLength, "0")

	cfg, warnings := LoadConfig()
	if cfg.MaxLength != resolver.DefaultMaxLength {
		t.Errorf("max length = %d, want the default %d", cfg.MaxLength, resolver.DefaultMaxLength)
	}

	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one", warnings)
	}
	// The warning has to say which variable, what was in it, and what is being
	// used instead — it is all the user gets.
	for _, want := range []string{EnvMaxLength, `"0"`, "must be positive", strconv.Itoa(resolver.DefaultMaxLength)} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("warning %q does not mention %q", warnings[0], want)
		}
	}
}

func TestLoadConfigReadsTheFile(t *testing.T) {
	isolate(t)
	writeConfig(t, `# every setting the file can carry
HERDR_AUTO_TITLE_DEBUG=true
HERDR_AUTO_TITLE_POLL_MS=800
HERDR_AUTO_TITLE_MAX_LENGTH=32
HERDR_AUTO_TITLE_BRANCH_MAX=0
HERDR_AUTO_TITLE_POSITION=false
HERDR_AUTO_TITLE_TRANSCRIPT=false
HERDR_AUTO_TITLE_AGENT_NAME=false
HERDR_AUTO_TITLE_MANUAL_FILE=/tmp/names.json
`)

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if !cfg.Debug {
		t.Error("debug is off despite the file enabling it")
	}

	if cfg.Poll != 800*time.Millisecond {
		t.Errorf("poll = %s, want 800ms", cfg.Poll)
	}

	if cfg.MaxLength != 32 {
		t.Errorf("max length = %d, want 32", cfg.MaxLength)
	}

	if cfg.BranchMax != 0 {
		t.Errorf("branch max = %d, want 0", cfg.BranchMax)
	}

	if cfg.ShowPosition {
		t.Error("positions are on despite the file turning them off")
	}

	if cfg.ReadTranscripts {
		t.Error("transcripts are read despite the file turning them off")
	}

	if cfg.ShowAgentName {
		t.Error("agent names are shown despite the file turning them off")
	}

	if cfg.ManualPath != "/tmp/names.json" {
		t.Errorf("manual path = %q, want the one the file names", cfg.ManualPath)
	}
}

func TestTheEnvironmentBeatsTheFile(t *testing.T) {
	// The file is where a setting lives permanently; the environment is how it
	// is overridden for one run, which is what `make run` does with debug.
	isolate(t)
	writeConfig(t, "HERDR_AUTO_TITLE_POLL_MS=800\n")
	t.Setenv(EnvPoll, "250")

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if cfg.Poll != 250*time.Millisecond {
		t.Errorf("poll = %s, want the 250ms the environment asked for", cfg.Poll)
	}
}

func TestAMissingConfigFileIsSilent(t *testing.T) {
	isolate(t)

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if cfg.Poll != DefaultPoll {
		t.Errorf("poll = %s, want the default %s", cfg.Poll, DefaultPoll)
	}
}

func TestABrokenConfigFileCostsTheWholeFile(t *testing.T) {
	// godotenv parses the file or nothing, so a good line next to a bad one is
	// lost with it. The warning is all the user gets.
	isolate(t)
	writeConfig(t, "HERDR_AUTO_TITLE_POLL_MS=800\nHERDR_AUTO_TITLE_MAX_LENGTH=\"32\n")

	cfg, warnings := LoadConfig()
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one", warnings)
	}

	if !strings.Contains(warnings[0], ConfigFile) {
		t.Errorf("warning %q does not name the file", warnings[0])
	}

	if cfg.Poll != DefaultPoll {
		t.Errorf("poll = %s, want the default %s", cfg.Poll, DefaultPoll)
	}

	if cfg.MaxLength != resolver.DefaultMaxLength {
		t.Errorf("max length = %d, want the default %d", cfg.MaxLength, resolver.DefaultMaxLength)
	}
}

func TestAKeyOfSomeoneElsesIsIgnored(t *testing.T) {
	// godotenv puts every key in the file into the environment; only the ones
	// Auto Title reads mean anything to it.
	isolate(t)
	writeConfig(t, "HERDR_AUTO_TITLE_POL_MS=800\nSOMETHING_ELSE=1\n")

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if cfg.Poll != DefaultPoll {
		t.Errorf("poll = %s, want the default %s", cfg.Poll, DefaultPoll)
	}
}

func TestAnEmptyManualFileKeepsLocksInMemory(t *testing.T) {
	// The setting is a path and takes no sentinel, so asking for no file is
	// asking for no path. Only LookupEnv can tell that from not asking at all.
	isolate(t)
	t.Setenv(EnvManual, "")

	cfg, warnings := LoadConfig()
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if cfg.ManualPath != "" {
		t.Errorf("manual path = %q, want none so the locks stay in memory", cfg.ManualPath)
	}
}

func TestAnUnsetManualFileKeepsTheDefault(t *testing.T) {
	isolate(t)

	cfg, _ := LoadConfig()
	if cfg.ManualPath != state.DefaultManualPath() {
		t.Errorf("manual path = %q, want the default", cfg.ManualPath)
	}
}
