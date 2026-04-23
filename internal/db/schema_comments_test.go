package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBootstrapSeedsTableAndColumnCommentsForAllSQLiteTables(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "schema-comments.sqlite")
	if err := Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.Query(`
SELECT name
FROM sqlite_master
WHERE type = 'table'
  AND name NOT LIKE 'sqlite_%'
ORDER BY name
`)
	if err != nil {
		t.Fatalf("list sqlite tables: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, tableName)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite tables: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("expected bootstrap to create sqlite tables")
	}

	for _, tableName := range tables {
		tableName := tableName
		t.Run(tableName, func(t *testing.T) {
			t.Parallel()

			var tableComment string
			if err := db.QueryRow(`SELECT comment FROM meta_table_comments WHERE table_name = ?`, tableName).Scan(&tableComment); err != nil {
				t.Fatalf("load table comment for %s: %v", tableName, err)
			}
			if strings.TrimSpace(tableComment) == "" {
				t.Fatalf("table %s should have a non-empty comment", tableName)
			}

			pragmaSQL := fmt.Sprintf(`PRAGMA table_info("%s")`, strings.ReplaceAll(tableName, `"`, `""`))
			columnRows, err := db.Query(pragmaSQL)
			if err != nil {
				t.Fatalf("load columns for %s: %v", tableName, err)
			}
			defer func() { _ = columnRows.Close() }()

			var columns []string
			for columnRows.Next() {
				var cid int
				var columnName string
				var columnType string
				var notNull int
				var defaultValue sql.NullString
				var pk int
				if err := columnRows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &pk); err != nil {
					t.Fatalf("scan column info for %s: %v", tableName, err)
				}
				columns = append(columns, columnName)
			}
			if err := columnRows.Err(); err != nil {
				t.Fatalf("iterate columns for %s: %v", tableName, err)
			}
			if len(columns) == 0 {
				t.Fatalf("expected table %s to have columns", tableName)
			}

			for _, columnName := range columns {
				var columnComment string
				if err := db.QueryRow(`
SELECT comment
FROM meta_column_comments
WHERE table_name = ?
  AND column_name = ?
`, tableName, columnName).Scan(&columnComment); err != nil {
					t.Fatalf("load column comment for %s.%s: %v", tableName, columnName, err)
				}
				if strings.TrimSpace(columnComment) == "" {
					t.Fatalf("column %s.%s should have a non-empty comment", tableName, columnName)
				}
			}
		})
	}
}

func TestBootstrapStructuredSourceCommentsCarryFunctionalDescription(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "source-comment-description.sqlite")
	if err := Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var comment string
	if err := db.QueryRow(`SELECT comment FROM meta_table_comments WHERE table_name = 'fin_fund_income'`).Scan(&comment); err != nil {
		t.Fatalf("load fin_fund_income comment: %v", err)
	}
	if !strings.Contains(comment, `"display":"《合同资金收入表》"`) {
		t.Fatalf("fin_fund_income comment should retain display metadata, got %q", comment)
	}
	if !strings.Contains(comment, `"description":"合同资金收入与回款明细`) {
		t.Fatalf("fin_fund_income comment should include functional description, got %q", comment)
	}
}

func TestFormatPostgresColumnCommentSQLQuotesPayloadSafely(t *testing.T) {
	t.Parallel()

	sql := formatPostgresColumnCommentSQL("tenant_uhub", "fin_fund_income", "received_amount", `回款口径's`)
	want := `COMMENT ON COLUMN "tenant_uhub"."fin_fund_income"."received_amount" IS '回款口径''s'`
	if sql != want {
		t.Fatalf("unexpected column comment sql:\nwant: %s\ngot:  %s", want, sql)
	}
}
