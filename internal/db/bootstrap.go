package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"

	"financeqa/internal/support"
)

// Open returns a sqlite database handle for the provided path.
func Open(_ context.Context, dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, errors.New("db path is required")
	}

	if err := support.EnsureParentDir(dbPath); err != nil {
		return nil, fmt.Errorf("create db parent dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	return db, nil
}

// Bootstrap initializes the sqlite database with TypeScript-compatible schema.
func Bootstrap(ctx context.Context, dbPath string) error {
	db, err := Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite db: %w", err)
	}

	if _, err := db.ExecContext(ctx, TypeScriptCompatibleSchema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}


	return nil
}

