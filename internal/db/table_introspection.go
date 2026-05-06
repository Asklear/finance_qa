package db

import (
	"context"
	"database/sql"
	"strings"
)

// TableExists reports whether a table exists in either SQLite or PostgreSQL.
func TableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name = ?`, tableName).Scan(&count)
	if err == nil {
		return count > 0, nil
	}

	var regclass sql.NullString
	err = db.QueryRowContext(ctx, `SELECT to_regclass(?)`, tableName).Scan(&regclass)
	if err != nil {
		return false, err
	}
	return regclass.Valid && strings.TrimSpace(regclass.String) != "", nil
}

// ColumnsExist reports whether all listed columns exist in the target table.
func ColumnsExist(ctx context.Context, db *sql.DB, tableName string, columns []string) (bool, error) {
	existing := map[string]bool{}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+tableName+`)`)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var cid int
			var name string
			var typ string
			var notNull int
			var defaultValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				return false, err
			}
			existing[name] = true
		}
		if err := rows.Err(); err != nil {
			return false, err
		}
		return containsAllColumns(existing, columns), nil
	}

	rows, err = db.QueryContext(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_name = ?
`, tableName)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return containsAllColumns(existing, columns), nil
}

func containsAllColumns(existing map[string]bool, columns []string) bool {
	for _, column := range columns {
		if !existing[column] {
			return false
		}
	}
	return true
}
