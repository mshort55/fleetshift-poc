package sqlite

import (
	"database/sql"
	"fmt"
	"testing"
)

// OpenTestDB opens an in-memory SQLite database with all migrations applied.
// The database is closed when the test finishes. Shared-cache mode is used
// so that all connections from the pool share the same in-memory database,
// which is necessary when the database is accessed from multiple goroutines.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
