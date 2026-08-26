package app

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/resolver"
)

// isolate clears every variable Auto Title reads, so a test sees only what it
// sets and the environment the tests run in cannot change the outcome.
func isolate(t *testing.T) {
	t.Helper()
	for _, name := range []string{EnvDebug, EnvPoll, EnvMaxLength, EnvBranchMax, EnvPosition, EnvManual} {
		t.Setenv(name, "")
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
