package main

import (
	"os"
	"regexp"
	"testing"
)

// An install refuses a release binary that reports another version than the
// manifest, so the two disagreeing would fail every install of that release.
func TestVersionMatchesManifest(t *testing.T) {
	t.Parallel()

	manifest, err := os.ReadFile("../../herdr-plugin.toml")
	if err != nil {
		t.Fatal(err)
	}

	match := regexp.MustCompile(`(?m)^version\s*=\s*"([^"]*)"`).FindSubmatch(manifest)
	if match == nil {
		t.Fatal("herdr-plugin.toml has no version")
	}

	if got := string(match[1]); got != version {
		t.Errorf("herdr-plugin.toml says %q, the binary says %q", got, version)
	}
}
