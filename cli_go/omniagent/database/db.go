package database

import (
	"database/sql"
	_ "embed"
	"sync"

	"github.com/adrg/xdg"
	_ "modernc.org/sqlite"
)

//go:embed schema/schema.sql
var schema string

var (
	once sync.Once
	db   *sql.DB
	dbErr error
)

// GetDB returns the singleton SQLite database connection, initializing it on first call.
// The database file is stored at an XDG-compliant data path.
func GetDB() (*sql.DB, error) {
	once.Do(func() {
		path, err := xdg.DataFile("memory/omniagent.db")
		if err != nil {
			dbErr = err
			return
		}
		conn, err := sql.Open("sqlite", path)
		if err != nil {
			dbErr = err
			return
		}
		if _, err = conn.Exec(schema); err != nil {
			dbErr = err
			conn.Close()
			return
		}
		db = conn
	})
	return db, dbErr
}
