package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type SchemaCommentExecutor interface {
	SourceMetadataExecutor
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func EnsureDefaultSchemaComments(ctx context.Context, runner SchemaCommentExecutor, dbPath string, tableNames []string) error {
	seen := map[string]struct{}{}
	for _, tableName := range tableNames {
		normalized := strings.TrimSpace(tableName)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}

		columns, err := listExistingTableColumns(ctx, runner, dbPath, normalized)
		if err != nil {
			return err
		}
		if len(columns) == 0 {
			continue
		}

		if !isSourceMetadataManagedTable(normalized) {
			comment := defaultTableDescription(normalized)
			if comment == "" {
				return fmt.Errorf("missing default table comment for %s", normalized)
			}
			if err := UpsertTableComment(ctx, runner, dbPath, normalized, comment); err != nil {
				return err
			}
		}

		defaults := defaultColumnComments(normalized)
		if len(defaults) == 0 {
			return fmt.Errorf("missing default column comments for %s", normalized)
		}
		for _, columnName := range columns {
			comment := strings.TrimSpace(defaults[columnName])
			if comment == "" {
				return fmt.Errorf("missing default column comment for %s.%s", normalized, columnName)
			}
			if err := UpsertColumnComment(ctx, runner, dbPath, normalized, columnName, comment); err != nil {
				return err
			}
		}
	}
	return nil
}

func UpsertTableComment(ctx context.Context, runner SourceMetadataExecutor, dbPath, tableName, comment string) error {
	base := baseTableName(tableName)
	comment = strings.TrimSpace(comment)
	if base == "" || comment == "" {
		return nil
	}
	if looksLikeSQLitePath(strings.TrimSpace(dbPath)) {
		if _, err := runner.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS meta_table_comments (
	table_name TEXT PRIMARY KEY,
	comment TEXT,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
			return fmt.Errorf("ensure sqlite meta_table_comments: %w", err)
		}
		if _, err := runner.ExecContext(ctx, `
INSERT INTO meta_table_comments(table_name, comment, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(table_name) DO UPDATE SET comment=excluded.comment, updated_at=CURRENT_TIMESTAMP
`, base, comment); err != nil {
			return fmt.Errorf("upsert sqlite table comment %s: %w", base, err)
		}
		return nil
	}
	schema, rel := splitQualifiedTableName(tableName)
	if schema == "" {
		schema = effectiveSchemaWithRunner(ctx, runner, dbPath)
	}
	if _, err := runner.ExecContext(ctx, formatPostgresTableCommentSQL(schema, rel, comment)); err != nil {
		return fmt.Errorf("comment on table %s.%s: %w", schema, rel, err)
	}
	return nil
}

func UpsertColumnComment(ctx context.Context, runner SourceMetadataExecutor, dbPath, tableName, columnName, comment string) error {
	base := baseTableName(tableName)
	columnName = strings.TrimSpace(columnName)
	comment = strings.TrimSpace(comment)
	if base == "" || columnName == "" || comment == "" {
		return nil
	}
	if looksLikeSQLitePath(strings.TrimSpace(dbPath)) {
		if _, err := runner.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS meta_column_comments (
	table_name TEXT NOT NULL,
	column_name TEXT NOT NULL,
	comment TEXT,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (table_name, column_name)
)`); err != nil {
			return fmt.Errorf("ensure sqlite meta_column_comments: %w", err)
		}
		if _, err := runner.ExecContext(ctx, `
INSERT INTO meta_column_comments(table_name, column_name, comment, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(table_name, column_name) DO UPDATE SET comment=excluded.comment, updated_at=CURRENT_TIMESTAMP
`, base, columnName, comment); err != nil {
			return fmt.Errorf("upsert sqlite column comment %s.%s: %w", base, columnName, err)
		}
		return nil
	}
	schema, rel := splitQualifiedTableName(tableName)
	if schema == "" {
		schema = effectiveSchemaWithRunner(ctx, runner, dbPath)
	}
	if _, err := runner.ExecContext(ctx, formatPostgresColumnCommentSQL(schema, rel, columnName, comment)); err != nil {
		return fmt.Errorf("comment on column %s.%s.%s: %w", schema, rel, columnName, err)
	}
	return nil
}

func formatPostgresColumnCommentSQL(schema, tableName, columnName, comment string) string {
	return fmt.Sprintf(
		`COMMENT ON COLUMN "%s"."%s"."%s" IS '%s'`,
		escapeIdentifier(schema),
		escapeIdentifier(tableName),
		escapeIdentifier(columnName),
		strings.ReplaceAll(comment, `'`, `''`),
	)
}

func isSourceMetadataManagedTable(tableName string) bool {
	meta := DefaultTableSourceMetadata(tableName)
	return strings.TrimSpace(meta.Display) != ""
}

func listExistingTableColumns(ctx context.Context, runner SchemaCommentExecutor, dbPath, tableName string) ([]string, error) {
	if looksLikeSQLitePath(strings.TrimSpace(dbPath)) {
		pragmaSQL := fmt.Sprintf(`PRAGMA table_info("%s")`, strings.ReplaceAll(baseTableName(tableName), `"`, `""`))
		rows, err := runner.QueryContext(ctx, pragmaSQL)
		if err != nil {
			return nil, fmt.Errorf("load sqlite columns for %s: %w", tableName, err)
		}
		defer func() { _ = rows.Close() }()

		columns := make([]string, 0, 16)
		for rows.Next() {
			var cid int
			var columnName string
			var columnType string
			var notNull int
			var defaultValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &pk); err != nil {
				return nil, fmt.Errorf("scan sqlite columns for %s: %w", tableName, err)
			}
			columns = append(columns, strings.TrimSpace(columnName))
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate sqlite columns for %s: %w", tableName, err)
		}
		return columns, nil
	}

	schema, rel := splitQualifiedTableName(tableName)
	if schema == "" {
		schema = effectiveSchemaWithRunner(ctx, runner, dbPath)
	}
	rows, err := runner.QueryContext(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = ?
  AND table_name = ?
ORDER BY ordinal_position
`, schema, rel)
	if err != nil {
		return nil, fmt.Errorf("load postgres columns for %s.%s: %w", schema, rel, err)
	}
	defer func() { _ = rows.Close() }()

	columns := make([]string, 0, 16)
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err != nil {
			return nil, fmt.Errorf("scan postgres columns for %s.%s: %w", schema, rel, err)
		}
		columns = append(columns, strings.TrimSpace(columnName))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres columns for %s.%s: %w", schema, rel, err)
	}
	return columns, nil
}
