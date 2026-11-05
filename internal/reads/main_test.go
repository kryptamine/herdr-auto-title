package reads_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMain opens one SQLite connection before the parallel tests do. A
// temporary workaround: modernc.org/sqlite v1.36.3 races under -race on
// its first open; delete this once Go and that module are raised.
func TestMain(m *testing.M) {
	if err := warmSQLite(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func warmSQLite() error {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}

	defer func() { _ = db.Close() }()

	return db.Ping()
}
