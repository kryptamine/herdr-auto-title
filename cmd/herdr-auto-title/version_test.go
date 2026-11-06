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

// The gallery mod installs the release it names, and that release has to be
// this one, or the mod offers a binary the repository no longer describes.
func TestModInstallsThisRelease(t *testing.T) {
	t.Parallel()

	mod, err := os.ReadFile("../../mod/mods/auto-title/service.js")
	if err != nil {
		t.Fatal(err)
	}

	match := regexp.MustCompile(`(?m)^export const VERSION = "([^"]*)"`).FindSubmatch(mod)
	if match == nil {
		t.Fatal("the mod names no VERSION")
	}

	if got := string(match[1]); got != version {
		t.Errorf("the mod installs %q, the binary is %q", got, version)
	}
}
