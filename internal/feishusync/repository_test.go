package feishusync_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"financeqa/internal/db"
	"financeqa/internal/feishusync"
)

func TestRepositoryUpsertAndListSources(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := feishusync.SyncSource{
		SourceType:  feishusync.SourceTypeFinanceWorkbook,
		SourceToken: "Iel5bFZWSoGF7hxjyPpcn5Elnqd",
		SourceURL:   "https://ucngfmhi7qmy.feishu.cn/file/Iel5bFZWSoGF7hxjyPpcn5Elnqd",
		DisplayName: "飞书财务表格",
		SyncStatus:  feishusync.SyncStatusActive,
	}

	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.ListSources(context.Background(), feishusync.SourceFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("source count = %d, want 1: %#v", len(got), got)
	}
	if got[0].SourceToken != src.SourceToken {
		t.Fatalf("source token = %q, want %q", got[0].SourceToken, src.SourceToken)
	}
	if got[0].SourceType != feishusync.SourceTypeFinanceWorkbook {
		t.Fatalf("source type = %q", got[0].SourceType)
	}
}

func TestRepositoryUpdatesSourceState(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	repo := feishusync.NewRepository(sqlDB)
	src := feishusync.SyncSource{
		SourceType:  feishusync.SourceTypePDFFolder,
		SourceToken: "JeTEfS3qQly8RJd0CJNcASumnCg",
		SyncStatus:  feishusync.SyncStatusActive,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	listed, err := repo.ListSources(context.Background(), feishusync.SourceFilter{})
	if err != nil {
		t.Fatalf("list after upsert: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("source count = %d, want 1", len(listed))
	}
	id := listed[0].ID

	if err := repo.MarkSourcePending(context.Background(), id, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("mark pending: %v", err)
	}
	pending := mustSingleSource(t, repo)
	if pending.SyncStatus != feishusync.SyncStatusPending {
		t.Fatalf("sync status = %q, want pending", pending.SyncStatus)
	}

	if err := repo.MarkSourceSuccess(context.Background(), id, "hash-1", "rev-1", `{"ok":true}`); err != nil {
		t.Fatalf("mark success: %v", err)
	}
	success := mustSingleSource(t, repo)
	if success.SyncStatus != feishusync.SyncStatusActive {
		t.Fatalf("sync status = %q, want active", success.SyncStatus)
	}
	if success.LastContentHash != "hash-1" || success.LastRevision != "rev-1" {
		t.Fatalf("success cursor = hash %q revision %q", success.LastContentHash, success.LastRevision)
	}

	if err := repo.MarkSourceError(context.Background(), id, "download failed", time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("mark error: %v", err)
	}
	failed := mustSingleSource(t, repo)
	if failed.SyncStatus != feishusync.SyncStatusError {
		t.Fatalf("sync status = %q, want error", failed.SyncStatus)
	}
	if failed.ErrorMessage != "download failed" {
		t.Fatalf("error message = %q", failed.ErrorMessage)
	}
}

func openFeishuSyncTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "feishu-sync.sqlite")
	if err := db.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	sqlDB, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func mustSingleSource(t *testing.T, repo *feishusync.Repository) feishusync.SyncSource {
	t.Helper()

	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("source count = %d, want 1: %#v", len(sources), sources)
	}
	return sources[0]
}
