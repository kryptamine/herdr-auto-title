package hermes_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"path/filepath"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"

	"github.com/kryptamine/herdr-auto-title/internal/hermes"
)

//nolint:paralleltest // registering a SQL function changes driver-global state
func TestTitleDoesNotExecuteUntrustedSchemaFunction(t *testing.T) {
	var calls atomic.Int32

	if err := sqlite.RegisterScalarFunction(
		"herdr_untrusted_title",
		0,
		func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			calls.Add(1)
			return "A title from untrusted schema", nil
		},
	); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	if _, err := db.Exec(
		`CREATE VIEW sessions AS SELECT '20261021_133849_f560a2' AS id, herdr_untrusted_title() AS title`,
	); err != nil {
		t.Fatal(err)
	}

	got := hermes.Title(context.Background(), []string{home}, sessionID, "")
	if calls.Load() != 0 || got != "" {
		t.Fatalf("untrusted schema executed: calls=%d title=%q", calls.Load(), got)
	}
}
