package database

import (
	"context"
	"path/filepath"
)

// OpenSQLiteReadOnly opens an existing server schema without running its
// startup migrations or seeds. It is for diagnostics and content planners.
func OpenSQLiteReadOnly(path string) (*SQLiteDB, error) {
	db, err := OpenReadOnly(path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteDB{DB: db, path: path}, nil
}

// CopySQLiteFile takes a consistent copy of an existing database, including
// committed WAL pages. The destination must be new and is private.
func CopySQLiteFile(source, dest string) error {
	db, err := OpenReadOnly(source)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// VACUUM INTO needs to write its output. The source itself is still
	// opened mode=ro, and the connection belongs exclusively to this copy.
	if _, err := conn.ExecContext(ctx, "PRAGMA query_only=OFF"); err != nil {
		return err
	}
	dest, err = filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := createEmpty(dest); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		removeDB(dest)
		return err
	}
	return nil
}
