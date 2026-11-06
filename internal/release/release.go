// Package release finds out when a newer Auto Title is published, for the
// desktop mode, whose popup installs it: docs/architecture/desktop-api.md#updates.
package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Releases is where Auto Title's releases are published.
const Releases = "https://github.com/kryptamine/herdr-auto-title/releases"

const (
	// firstCheck lets a login settle, and the network come up, before asking.
	firstCheck = time.Minute
	checkEvery = 24 * time.Hour
	retryAfter = time.Hour
	askTimeout = 30 * time.Second
)

// tagPrefix is what releases/latest redirects to, ahead of the version.
const tagPrefix = "/releases/tag/v"

var errNoRedirect = errors.New("releases/latest did not name a release")

// version is the only spelling taken from the network, since it reaches the
// popup and the install script's download URL.
var version = regexp.MustCompile(`^[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}$`)

// latestRelease is the latest published release, read from where releases/latest
// redirects: a page has no rate limit for callers without a token, as the API has.
func latestRelease(ctx context.Context, releases string) (string, error) {
	client := &http.Client{
		Timeout: askTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, releases+"/latest", nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	_ = resp.Body.Close()

	location := resp.Header.Get("Location")

	_, found, ok := strings.Cut(location, tagPrefix)
	if !ok || !version.MatchString(found) {
		return "", fmt.Errorf("%w: %s %q", errNoRedirect, resp.Status, location)
	}

	return found, nil
}

// newer is whether version a comes after version b. A spelling that is not a
// version is never newer.
func newer(a, b string) bool {
	x, okA := parts(a)
	y, okB := parts(b)

	if !okA || !okB {
		return false
	}

	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}

	return false
}

func parts(v string) ([3]int, bool) {
	var p [3]int

	if !version.MatchString(v) {
		return p, false
	}

	for i, s := range strings.Split(v, ".") {
		p[i], _ = strconv.Atoi(s)
	}

	return p, true
}

// Pending is the release a check offered, if it is newer than current, or "".
func Pending(path, current string) string {
	data, err := os.ReadFile(path) //nolint:gosec // Auto Title's own state file
	if err != nil {
		return ""
	}

	if found := strings.TrimSpace(string(data)); newer(found, current) {
		return found
	}

	return ""
}

// Watcher checks for a newer release from time to time, and offers each one
// found once: Path keeps the last one offered, across restarts.
type Watcher struct {
	Current  string
	Releases string
	Path     string
	Show     func(ctx context.Context) error
	Log      *slog.Logger
}

// Run checks until ctx ends: a day apart, or an hour after a failed check.
func (w Watcher) Run(ctx context.Context) {
	wait := firstCheck

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		wait = checkEvery

		if err := w.Check(ctx); err != nil {
			w.Log.Debug("the check for a newer release failed", "error", err)

			wait = retryAfter
		}
	}
}

// Check asks for the latest release once, and offers it if it is newer than
// the one running and not offered yet. It is kept before it is shown, since
// what is shown reads it, and dropped if it could not be, to be offered again.
func (w Watcher) Check(ctx context.Context) error {
	latest, err := latestRelease(ctx, w.Releases)
	if err != nil {
		return err
	}

	if !newer(latest, w.Current) || Pending(w.Path, w.Current) == latest {
		return nil
	}

	//nolint:gosec // the directory is Auto Title's own, never terminal-derived
	if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(w.Path, []byte(latest+"\n"), 0o600); err != nil {
		return err
	}

	if err := w.Show(ctx); err != nil {
		_ = os.Remove(w.Path)

		return fmt.Errorf("show release %s: %w", latest, err)
	}

	w.Log.Info("offered a newer release", "version", latest, "running", w.Current)

	return nil
}
