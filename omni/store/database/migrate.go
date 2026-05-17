package database

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

func runMigrations(db *sql.DB) error {
	if err := ensureVersionTable(db); err != nil {
		return err
	}

	entries, err := fs.ReadDir(schemaFS, "schema/migrations")
	if err != nil {
		return nil
	}

	var versions []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}
	sort.Ints(versions)

	for _, v := range versions {
		dirPath := fmt.Sprintf("schema/migrations/%d", v)
		files, err := fs.ReadDir(schemaFS, dirPath)
		if err != nil {
			return err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
				continue
			}
			tableName := strings.TrimSuffix(f.Name(), ".sql")
			current, err := getTableVersion(db, tableName)
			if err != nil {
				return err
			}
			if current >= v {
				continue
			}
			data, err := schemaFS.ReadFile(dirPath + "/" + f.Name())
			if err != nil {
				return err
			}
			if _, err := db.Exec(string(data)); err != nil {
				return fmt.Errorf("migration v%d %s: %w", v, tableName, err)
			}
			if err := setTableVersion(db, tableName, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureVersionTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_versions (
		table_name TEXT PRIMARY KEY,
		version    INTEGER NOT NULL DEFAULT 0
	)`)
	return err
}

func getTableVersion(db *sql.DB, tableName string) (int, error) {
	var v int
	err := db.QueryRow(`SELECT version FROM schema_versions WHERE table_name = ?`, tableName).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}

func setTableVersion(db *sql.DB, tableName string, version int) error {
	_, err := db.Exec(
		`INSERT INTO schema_versions (table_name, version) VALUES (?, ?)
		 ON CONFLICT(table_name) DO UPDATE SET version = excluded.version`,
		tableName, version,
	)
	return err
}
