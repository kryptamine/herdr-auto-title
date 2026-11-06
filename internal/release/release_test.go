package release

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// releases answers releases/latest the way GitHub does, redirecting to location.
func releases(t *testing.T, location string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Location", location)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/releases"
}

func TestLatestReleaseReadsTheRedirect(t *testing.T) {
	t.Parallel()

	got, err := latestRelease(
		t.Context(),
		releases(t, "https://github.com/o/r/releases/tag/v0.21.0"),
	)
	if err != nil || got != "0.21.0" {
		t.Fatalf("latestRelease = %q, %v; want 0.21.0", got, err)
	}
}

// The version reaches the popup and the install script's URL, so nothing but
// a version is taken from the network.
func TestLatestReleaseRefusesWhatIsNotAVersion(t *testing.T) {
	t.Parallel()

	for _, location := range []string{
		"",
		"https://github.com/o/r/releases",
		"https://github.com/o/r/releases/tag/v0.21.0;rm",
		"https://github.com/o/r/releases/tag/v0.21",
	} {
		if got, err := latestRelease(t.Context(), releases(t, location)); err == nil {
			t.Errorf("latestRelease for %q = %q, want an error", location, got)
		}
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"0.21.0", "0.20.0", true},
		{"0.20.10", "0.20.9", true},
		{"1.0.0", "0.99.99", true},
		{"0.20.0", "0.20.0", false},
		{"0.19.0", "0.20.0", false},
		{"", "0.20.0", false},
		{"0.21.0", "dev", false},
	} {
		if got := newer(tc.a, tc.b); got != tc.want {
			t.Errorf("newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func watcher(t *testing.T, current string, show func(context.Context) error) Watcher {
	t.Helper()

	return Watcher{
		Current:  current,
		Releases: releases(t, "https://github.com/o/r/releases/tag/v0.21.0"),
		Path:     filepath.Join(t.TempDir(), "state", "latest-release"),
		Show:     show,
		Log:      slog.New(slog.DiscardHandler),
	}
}

// A release is shown once, not at every check and every login after it.
func TestCheckShowsANewerReleaseOnce(t *testing.T) {
	t.Parallel()

	var shown []string

	var w Watcher

	w = watcher(t, "0.20.0", func(context.Context) error {
		// What is shown asks for the release, so it is kept by then.
		shown = append(shown, Pending(w.Path, w.Current))

		return nil
	})

	for range 3 {
		if err := w.Check(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	if len(shown) != 1 || shown[0] != "0.21.0" {
		t.Fatalf("shown %q, want one 0.21.0", shown)
	}
}

// A window closed at the check still gets the release at the next one.
func TestCheckShowsAgainAfterAFailedShow(t *testing.T) {
	t.Parallel()

	fail := true
	calls := 0
	w := watcher(t, "0.20.0", func(context.Context) error {
		calls++

		if fail {
			return errors.New("no window")
		}

		return nil
	})

	if err := w.Check(t.Context()); err == nil {
		t.Fatal("a failed show was not reported")
	}

	if got := Pending(w.Path, w.Current); got != "" {
		t.Fatalf("Pending = %q, want the release not counted as offered", got)
	}

	fail = false

	if err := w.Check(t.Context()); err != nil {
		t.Fatal(err)
	}

	if calls != 2 {
		t.Fatalf("shown %d times, want 2", calls)
	}
}

func TestCheckLeavesTheSameOrAnOlderReleaseAlone(t *testing.T) {
	t.Parallel()

	for _, current := range []string{"0.21.0", "0.22.0"} {
		w := watcher(t, current, func(context.Context) error {
			t.Errorf("running %s, a release shown", current)

			return nil
		})

		if err := w.Check(t.Context()); err != nil {
			t.Fatal(err)
		}

		if got := Pending(w.Path, current); got != "" {
			t.Errorf("running %s, Pending = %q, want nothing kept", current, got)
		}
	}
}
