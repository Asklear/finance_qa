package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type TableColumnComments map[string]map[string]string

type columnCommentQueryer interface {
	SourceMetadataExecutor
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func LoadTableColumnComments(ctx context.Context, runner SourceMetadataExecutor, dbPath string, tableNames []string) (TableColumnComments, error) {
	out := make(TableColumnComments, len(tableNames))
	seen := map[string]struct{}{}
	for _, tableName := range tableNames {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		if _, ok := seen[tableName]; ok {
			continue
		}
		seen[tableName] = struct{}{}
		if defaults := defaultColumnComments(tableName); len(defaults) > 0 {
			out[tableName] = cloneColumnComments(defaults)
		}
	}

	queryer, ok := runner.(columnCommentQueryer)
	if !ok {
		return out, nil
	}
	if looksLikeSQLitePath(strings.TrimSpace(dbPath)) {
		return loadSQLiteColumnComments(ctx, queryer, tableNames, out)
	}
	return loadPostgresColumnComments(ctx, queryer, dbPath, tableNames, out)
}

func loadSQLiteColumnComments(ctx context.Context, runner columnCommentQueryer, tableNames []string, out TableColumnComments) (TableColumnComments, error) {
	for _, tableName := range tableNames {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		base := baseTableName(tableName)
		if base == "" {
			continue
		}
		rows, err := runner.QueryContext(ctx, `
SELECT column_name, comment
FROM meta_column_comments
WHERE table_name = ?
`, base)
		if err != nil {
			if isMissingSQLiteMetadataError(err) {
				continue
			}
			return nil, fmt.Errorf("load sqlite column comments for %s: %w", base, err)
		}
		if err := mergeColumnCommentRows(rows, tableName, out); err != nil {
			return nil, fmt.Errorf("scan sqlite column comments for %s: %w", base, err)
		}
	}
	return out, nil
}

func loadPostgresColumnComments(ctx context.Context, runner columnCommentQueryer, dbPath string, tableNames []string, out TableColumnComments) (TableColumnComments, error) {
	for _, tableName := range tableNames {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		schema, rel := splitQualifiedTableName(tableName)
		if schema == "" {
			schema = effectiveSchemaWithRunner(ctx, runner, dbPath)
		}
		rows, err := runner.QueryContext(ctx, `
SELECT a.attname AS column_name,
       COALESCE(pgd.description, '') AS comment
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid
LEFT JOIN pg_catalog.pg_description pgd ON pgd.objoid = c.oid AND pgd.objsubid = a.attnum
WHERE n.nspname = ?
  AND c.relname = ?
  AND a.attnum > 0
  AND NOT a.attisdropped
ORDER BY a.attnum
`, schema, rel)
		if err != nil {
			return nil, fmt.Errorf("load pg column comments for %s.%s: %w", schema, rel, err)
		}
		if err := mergeColumnCommentRows(rows, tableName, out); err != nil {
			return nil, fmt.Errorf("scan pg column comments for %s.%s: %w", schema, rel, err)
		}
	}
	return out, nil
}

func mergeColumnCommentRows(rows *sql.Rows, tableName string, out TableColumnComments) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var columnName string
		var comment sql.NullString
		if err := rows.Scan(&columnName, &comment); err != nil {
			return err
		}
		columnName = strings.TrimSpace(columnName)
		text := strings.TrimSpace(comment.String)
		if columnName == "" || text == "" {
			continue
		}
		if _, ok := out[tableName]; !ok {
			out[tableName] = map[string]string{}
		}
		out[tableName][columnName] = text
	}
	return rows.Err()
}

func cloneColumnComments(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func isMissingSQLiteMetadataError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") && strings.Contains(msg, "meta_column_comments")
}
