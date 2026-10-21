// Package hermes reads native session titles without reading conversations.
package hermes

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	Agent         = "hermes"
	maxHomes      = 64
	maxTitleRunes = 4096
	readTimeout   = 100 * time.Millisecond
)

var sessionIDPattern = regexp.MustCompile(`^[0-9]{8}_[0-9]{6}_[0-9a-fA-F]{6,8}$`)

var errNotRegular = errors.New("session database is not a regular file")

// Title selects by bound session ID, never by directory or recent activity.
// A process home is authoritative; otherwise duplicate session IDs are refused.
func Title(
	ctx context.Context,
	roots []string,
	sessionID, processHome string,
	settledOnly ...bool,
) string {
	if !sessionIDPattern.MatchString(sessionID) || ctx.Err() != nil {
		return ""
	}

	settled := len(settledOnly) > 0 && settledOnly[0]

	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if processHome != "" {
		if !filepath.IsAbs(processHome) {
			return ""
		}

		title, _, _ := titleAt(ctx, processHome, sessionID, settled)

		return title
	}

	homes, ok := searchHomes(roots)
	if !ok {
		return ""
	}

	var title string

	found := false

	for _, dir := range homes {
		value, exists, err := titleAt(ctx, dir, sessionID, settled)
		if err != nil || ctx.Err() != nil {
			return ""
		}

		if !exists {
			continue
		}

		if found {
			return ""
		}

		title, found = value, true
	}

	return title
}

func searchHomes(roots []string) ([]string, bool) {
	homes := make([]string, 0, len(roots))
	seen := make(map[string]bool)

	for _, home := range roots {
		if !filepath.IsAbs(home) {
			continue
		}

		home = filepath.Clean(home)
		if seen[home] {
			continue
		}

		seen[home] = true
		homes = append(homes, home)

		entries, ok := profiles(home)
		if !ok {
			return nil, false
		}

		for _, entry := range entries {
			dir := filepath.Join(home, "profiles", entry.Name())
			if entry.IsDir() && !seen[dir] {
				homes = append(homes, dir)
				seen[dir] = true
			}
		}

		if len(homes) > maxHomes {
			return nil, false
		}
	}

	return homes, true
}

func profiles(home string) ([]os.DirEntry, bool) {
	// ReadDir on a file caps the directory read too, not just the subsequent SQL.
	//nolint:gosec // the home is configured or read from a process environment
	file, err := os.Open(filepath.Join(home, "profiles"))
	if os.IsNotExist(err) {
		return nil, true
	}

	if err != nil {
		return nil, false
	}

	defer func() { _ = file.Close() }()

	entries, err := file.ReadDir(maxHomes + 1)

	return entries, (err == nil || errors.Is(err, io.EOF)) && len(entries) <= maxHomes
}

func titleAt(ctx context.Context, home, sessionID string, settled bool) (string, bool, error) {
	path := filepath.Join(home, "state.db")
	// Refuse absent files and special devices before SQLite opens anything.
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}

	if err != nil {
		return "", false, err
	}

	if !info.Mode().IsRegular() {
		return "", false, errNotRegular
	}

	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	uri := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&_pragma=trusted_schema(0)"}

	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return "", false, err
	}

	defer func() { _ = db.Close() }()

	db.SetMaxOpenConns(1)

	var title sql.NullString

	query := `SELECT substr(title,1,?) FROM sessions WHERE id = ? LIMIT 1`
	if settled {
		query = `SELECT CASE WHEN title_source IN ('llm','user') THEN substr(title,1,?) ELSE NULL END FROM sessions WHERE id = ? LIMIT 1`
	}

	err = db.QueryRowContext(ctx, query, maxTitleRunes, sessionID).
		Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}

	if err != nil {
		return "", false, err
	}

	return title.String, true, nil
}
