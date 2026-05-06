package database

import (
	"database/sql"
	"embed"
	"io/fs"
	"sort"
	"sync"

	"github.com/adrg/xdg"
	_ "modernc.org/sqlite"
)

//go:embed schema/*.sql
var schemaFS embed.FS

var (
	once  sync.Once
	db    *sql.DB
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
		if err := applySchema(conn); err != nil {
			dbErr = err
			conn.Close()
			return
		}
		db = conn
	})
	return db, dbErr
}

// ApplySchema runs all embedded schema files against conn in sorted order.
func ApplySchema(conn *sql.DB) error { return applySchema(conn) }

func applySchema(conn *sql.DB) error {
	entries, err := fs.ReadDir(schemaFS, "schema")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := schemaFS.ReadFile("schema/" + e.Name())
		if err != nil {
			return err
		}
		if _, err = conn.Exec(string(data)); err != nil {
			return err
		}
	}
	return nil
}
