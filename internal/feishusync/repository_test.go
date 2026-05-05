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
		SourceToken: "workbook-token",
		SourceURL:   "https://example.feishu.cn/file/workbook-token",
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
		SourceToken: "folder-token",
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

func TestRepositoryContractPDFState(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	ctx := context.Background()

	id, err := repo.UpsertContractPDFState(ctx, feishusync.ContractPDFState{
		FileName:          "合同A.pdf",
		FileHash:          "hash-a",
		StorageKey:        "/tmp/hash-a.pdf",
		FeishuFileToken:   "token-a",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:合同a.pdf",
		FileSize:          12,
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusPending,
	})
	if err != nil {
		t.Fatalf("upsert contract state: %v", err)
	}
	if id == 0 {
		t.Fatalf("contract id should be set")
	}

	byHash, ok, err := repo.FindContractByFileHash(ctx, "hash-a")
	if err != nil {
		t.Fatalf("find by hash: %v", err)
	}
	if !ok || byHash.ID != id || byHash.FileName != "合同A.pdf" {
		t.Fatalf("find by hash = %#v ok=%v", byHash, ok)
	}

	bySlot, ok, err := repo.FindActiveContractBySlot(ctx, "folder-a:合同a.pdf")
	if err != nil {
		t.Fatalf("find by slot: %v", err)
	}
	if !ok || bySlot.ID != id || bySlot.FeishuFileToken != "token-a" {
		t.Fatalf("find by slot = %#v ok=%v", bySlot, ok)
	}

	deleted, err := repo.MarkContractDeletedByMissingTokens(ctx, "folder-a", []string{"other-token"})
	if err != nil {
		t.Fatalf("mark deleted: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted count = %d, want 1", deleted)
	}
	afterDelete, ok, err := repo.FindActiveContractBySlot(ctx, "folder-a:合同a.pdf")
	if err != nil {
		t.Fatalf("find after delete: %v", err)
	}
	if ok {
		t.Fatalf("deleted contract should not be active: %#v", afterDelete)
	}
}

func TestRepositoryInvoicePDFState(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	ctx := context.Background()

	contractID, err := repo.UpsertContractPDFState(ctx, feishusync.ContractPDFState{
		FileName:          "合同A.pdf",
		FileHash:          "hash-contract-a",
		StorageKey:        "/tmp/contract-a.pdf",
		FeishuFileToken:   "contract-token-a",
		FeishuRootToken:   "folder-root",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-root:客户a/合同a.pdf",
		RelationKey:       "客户a",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed contract state: %v", err)
	}

	invoiceID, err := repo.UpsertInvoicePDFState(ctx, feishusync.InvoicePDFState{
		ContractID:         contractID,
		FileName:           "发票A.pdf",
		FileHash:           "hash-invoice-a",
		StorageKey:         "/tmp/invoice-a.pdf",
		FeishuFileToken:    "invoice-token-a",
		FeishuRootToken:    "folder-root",
		FeishuParentToken:  "folder-invoice",
		FeishuRelativePath: "客户A/发票/发票A.pdf",
		FeishuFolderPath:   "客户A/发票",
		FeishuSlotKey:      "folder-root:客户a/发票/发票a.pdf",
		RelationKey:        "客户a",
		FileSize:           22,
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusPending,
	})
	if err != nil {
		t.Fatalf("upsert invoice state: %v", err)
	}
	if invoiceID == 0 {
		t.Fatalf("invoice id should be set")
	}

	byHash, ok, err := repo.FindInvoiceByAnyFileHash(ctx, "hash-invoice-a")
	if err != nil {
		t.Fatalf("find invoice by hash: %v", err)
	}
	if !ok || byHash.ID != invoiceID || byHash.ContractID != contractID {
		t.Fatalf("find invoice by hash = %#v ok=%v", byHash, ok)
	}
	if len(byHash.InvoiceNumber) > 20 || byHash.InvoiceNumber != "pending:hashinvoicea" {
		t.Fatalf("placeholder invoice number = %q", byHash.InvoiceNumber)
	}
	bySlot, ok, err := repo.FindActiveInvoiceBySlot(ctx, "folder-root:客户a/发票/发票a.pdf")
	if err != nil {
		t.Fatalf("find invoice by slot: %v", err)
	}
	if !ok || bySlot.ID != invoiceID || bySlot.RelationKey != "客户a" {
		t.Fatalf("find invoice by slot = %#v ok=%v", bySlot, ok)
	}

	deleted, err := repo.MarkDeletedByMissingTokens(ctx, "folder-root", []string{"contract-token-a"})
	if err != nil {
		t.Fatalf("mark deleted: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted count = %d, want 1", deleted)
	}
	if _, ok, err := repo.FindActiveInvoiceBySlot(ctx, "folder-root:客户a/发票/发票a.pdf"); err != nil {
		t.Fatalf("find invoice after delete: %v", err)
	} else if ok {
		t.Fatalf("deleted invoice should not be active")
	}
}

func TestRepositoryContractPDFStateSatisfiesProductionRequiredColumns(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	if _, err := sqlDB.Exec(`CREATE TABLE contract_categories (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		code TEXT NOT NULL,
		sort_order INTEGER
	)`); err != nil {
		t.Fatalf("create categories: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO contract_categories(id, name, code, sort_order) VALUES (7, '客户合同', 'customer', 1)`); err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	repo := feishusync.NewRepository(sqlDB)

	id, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "合同A.pdf",
		FileHash:          "hash-a",
		StorageKey:        "/tmp/hash-a.pdf",
		FeishuFileToken:   "token-a",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:合同a.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusPending,
	})
	if err != nil {
		t.Fatalf("upsert contract state: %v", err)
	}

	var jobID, createdAt, updatedAt string
	var categoryID int64
	if err := sqlDB.QueryRow(`SELECT job_id, category_id, COALESCE(CAST(created_at AS TEXT), ''), COALESCE(CAST(updated_at AS TEXT), '') FROM contract_main WHERE id = ?`, id).Scan(&jobID, &categoryID, &createdAt, &updatedAt); err != nil {
		t.Fatalf("load required columns: %v", err)
	}
	if jobID != "feishu:token-a" || categoryID != 7 {
		t.Fatalf("job_id=%q category_id=%d, want generated job and default category", jobID, categoryID)
	}
	if createdAt == "" || updatedAt == "" {
		t.Fatalf("created_at=%q updated_at=%q, want both populated", createdAt, updatedAt)
	}
}

func TestRepositoryContractPDFStateDoesNotRequireDeprecatedLinkedColumn(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixtureWithoutLinkedColumn(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)

	id, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "合同A.pdf",
		FileHash:          "hash-a",
		StorageKey:        "/tmp/hash-a.pdf",
		FeishuFileToken:   "token-a",
		FeishuRootToken:   "folder-root",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-root:客户a/合同a.pdf",
		RelationKey:       "客户a",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusPending,
	})
	if err != nil {
		t.Fatalf("upsert contract state without linked column: %v", err)
	}

	found, ok, err := repo.FindContractRelationTarget(context.Background(), "folder-root", "客户a")
	if err != nil {
		t.Fatalf("find relation target without linked column: %v", err)
	}
	if !ok || found.ID != id {
		t.Fatalf("relation target = %#v ok=%v, want id %d", found, ok, id)
	}
}

func TestRepositoryInsertDuplicateLogIsBestEffort(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)

	err := repo.InsertDuplicateLog(context.Background(), feishusync.DuplicateLog{
		EventType:          feishusync.DuplicateEventSameHash,
		SourceFileToken:    "token-new",
		ExistingContractID: 1,
		FileHash:           "hash-a",
		SlotKey:            "folder-a:合同a.pdf",
		Message:            "same hash duplicate",
		MetadataJSON:       `{"source":"test"}`,
	})
	if err != nil {
		t.Fatalf("insert duplicate log: %v", err)
	}

	var count int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM contract_duplicate_logs WHERE event_type = ?`, feishusync.DuplicateEventSameHash).Scan(&count); err != nil {
		t.Fatalf("count duplicate logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("duplicate log count = %d, want 1", count)
	}

	if _, err := sqlDB.Exec(`DROP TABLE contract_duplicate_logs`); err != nil {
		t.Fatalf("drop duplicate log table: %v", err)
	}
	if err := repo.InsertDuplicateLog(context.Background(), feishusync.DuplicateLog{EventType: feishusync.DuplicateEventSameHash}); err != nil {
		t.Fatalf("missing duplicate log table should be ignored: %v", err)
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

func createContractPDFStateFixture(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := []string{
		`CREATE TABLE contract_main (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL UNIQUE,
			file_name TEXT,
			file_hash TEXT,
			storage_key TEXT,
			feishu_file_token TEXT,
			feishu_root_token TEXT,
			feishu_parent_token TEXT,
			feishu_relative_path TEXT,
			feishu_folder_path TEXT,
			feishu_slot_key TEXT,
			feishu_file_name TEXT,
			feishu_relation_key TEXT,
			category_id INTEGER NOT NULL,
			file_size INTEGER,
			sync_status TEXT,
			ocr_status TEXT,
			feishu_deleted_at TIMESTAMP,
			last_seen_at TIMESTAMP,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)`,
		`CREATE TABLE contract_invoices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id INTEGER NOT NULL,
			invoice_number TEXT NOT NULL,
			file_name TEXT,
			storage_key TEXT,
			file_hash TEXT,
			feishu_file_token TEXT,
			feishu_root_token TEXT,
			feishu_parent_token TEXT,
			feishu_relative_path TEXT,
			feishu_folder_path TEXT,
			feishu_slot_key TEXT,
			feishu_file_name TEXT,
			feishu_relation_key TEXT,
			file_size INTEGER,
			sync_status TEXT,
			ocr_status TEXT,
			feishu_deleted_at TIMESTAMP,
			last_seen_at TIMESTAMP,
			created_at TIMESTAMP,
			updated_at TIMESTAMP,
			extension_data TEXT,
			UNIQUE(contract_id, invoice_number)
		)`,
		`CREATE TABLE contract_duplicate_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT,
			source_file_token TEXT,
			existing_contract_id INTEGER,
			target_contract_id INTEGER,
			file_hash TEXT,
			old_file_hash TEXT,
			slot_key TEXT,
			message TEXT,
			metadata_json TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec fixture sql: %v\n%s", err, stmt)
		}
	}
}

func createContractPDFStateFixtureWithoutLinkedColumn(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := []string{
		`CREATE TABLE contract_main (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL UNIQUE,
			file_name TEXT,
			file_hash TEXT,
			storage_key TEXT,
			feishu_file_token TEXT,
			feishu_root_token TEXT,
			feishu_parent_token TEXT,
			feishu_relative_path TEXT,
			feishu_folder_path TEXT,
			feishu_slot_key TEXT,
			feishu_file_name TEXT,
			feishu_relation_key TEXT,
			category_id INTEGER NOT NULL,
			file_size INTEGER,
			sync_status TEXT,
			ocr_status TEXT,
			feishu_deleted_at TIMESTAMP,
			last_seen_at TIMESTAMP,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)`,
		`CREATE TABLE contract_invoices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id INTEGER NOT NULL,
			invoice_number TEXT NOT NULL,
			file_name TEXT,
			storage_key TEXT,
			file_hash TEXT,
			feishu_file_token TEXT,
			feishu_root_token TEXT,
			feishu_parent_token TEXT,
			feishu_relative_path TEXT,
			feishu_folder_path TEXT,
			feishu_slot_key TEXT,
			feishu_file_name TEXT,
			feishu_relation_key TEXT,
			file_size INTEGER,
			sync_status TEXT,
			ocr_status TEXT,
			feishu_deleted_at TIMESTAMP,
			last_seen_at TIMESTAMP,
			created_at TIMESTAMP,
			updated_at TIMESTAMP,
			extension_data TEXT,
			UNIQUE(contract_id, invoice_number)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec fixture sql: %v\n%s", err, stmt)
		}
	}
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
