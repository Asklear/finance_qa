package feishusync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	dbpkg "financeqa/internal/db"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) UpsertSource(ctx context.Context, src SyncSource) error {
	src.SourceType = strings.TrimSpace(src.SourceType)
	src.SourceToken = strings.TrimSpace(src.SourceToken)
	if src.SyncMode == "" {
		src.SyncMode = "active_scan"
	}
	if src.SyncStatus == "" {
		src.SyncStatus = SyncStatusActive
	}

	_, err := r.db.ExecContext(ctx, `
INSERT INTO feishu_sync_sources(
	source_type, source_token, source_url, display_name, parent_token,
	sync_mode, sync_status, last_revision, last_content_hash, error_message, metadata_json, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(source_type, source_token) DO UPDATE SET
	source_url = excluded.source_url,
	display_name = excluded.display_name,
	parent_token = excluded.parent_token,
	sync_mode = excluded.sync_mode,
	sync_status = excluded.sync_status,
	last_revision = excluded.last_revision,
	last_content_hash = excluded.last_content_hash,
	error_message = excluded.error_message,
	metadata_json = excluded.metadata_json,
	updated_at = CURRENT_TIMESTAMP
`, src.SourceType, src.SourceToken, nullableString(src.SourceURL), nullableString(src.DisplayName), nullableString(src.ParentToken),
		src.SyncMode, src.SyncStatus, nullableString(src.LastRevision), nullableString(src.LastContentHash), nullableString(src.ErrorMessage), nullableString(src.MetadataJSON))
	return err
}

func (r *Repository) ListSources(ctx context.Context, filter SourceFilter) ([]SyncSource, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if strings.TrimSpace(filter.SourceType) != "" {
		where = append(where, "source_type = ?")
		args = append(args, strings.TrimSpace(filter.SourceType))
	}
	if !filter.IncludeDisabled {
		where = append(where, "sync_status <> ?")
		args = append(args, SyncStatusDisabled)
	}
	if filter.DueOnly {
		where = append(where, "(sync_status = ? OR next_scan_at IS NULL OR next_scan_at <= CURRENT_TIMESTAMP)")
		args = append(args, SyncStatusPending)
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT
	id,
	COALESCE(source_type, ''),
	COALESCE(source_token, ''),
	COALESCE(source_url, ''),
	COALESCE(display_name, ''),
	COALESCE(parent_token, ''),
	COALESCE(sync_mode, ''),
	COALESCE(sync_status, ''),
	COALESCE(last_revision, ''),
	COALESCE(last_content_hash, ''),
	COALESCE(error_message, ''),
	COALESCE(CAST(metadata_json AS TEXT), ''),
	COALESCE(CAST(last_sync_at AS TEXT), ''),
	COALESCE(CAST(last_success_at AS TEXT), ''),
	COALESCE(CAST(next_scan_at AS TEXT), '')
FROM feishu_sync_sources
WHERE `+strings.Join(where, " AND ")+`
ORDER BY source_type, source_token
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SyncSource
	for rows.Next() {
		var src SyncSource
		var lastSync, lastSuccess, nextScan string
		if err := rows.Scan(
			&src.ID,
			&src.SourceType,
			&src.SourceToken,
			&src.SourceURL,
			&src.DisplayName,
			&src.ParentToken,
			&src.SyncMode,
			&src.SyncStatus,
			&src.LastRevision,
			&src.LastContentHash,
			&src.ErrorMessage,
			&src.MetadataJSON,
			&lastSync,
			&lastSuccess,
			&nextScan,
		); err != nil {
			return nil, err
		}
		src.LastSyncAt = parseDBTime(lastSync)
		src.LastSuccessAt = parseDBTime(lastSuccess)
		src.NextScanAt = parseDBTime(nextScan)
		out = append(out, src)
	}
	return out, rows.Err()
}

func (r *Repository) MarkSourcePending(ctx context.Context, sourceID int64, nextScan time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE feishu_sync_sources
SET sync_status = ?, next_scan_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, SyncStatusPending, nullableTime(nextScan), sourceID)
	return err
}

func (r *Repository) MarkSourceSuccess(ctx context.Context, sourceID int64, hash, revision, metadata string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE feishu_sync_sources
SET sync_status = ?,
    last_content_hash = ?,
    last_revision = ?,
    metadata_json = ?,
    error_message = NULL,
    last_sync_at = CURRENT_TIMESTAMP,
    last_success_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, SyncStatusActive, nullableString(hash), nullableString(revision), nullableString(metadata), sourceID)
	return err
}

func (r *Repository) MarkSourceError(ctx context.Context, sourceID int64, errMsg string, nextScan time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE feishu_sync_sources
SET sync_status = ?,
    error_message = ?,
    next_scan_at = ?,
    last_sync_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, SyncStatusError, strings.TrimSpace(errMsg), nullableTime(nextScan), sourceID)
	return err
}

func (r *Repository) FindContractByFileHash(ctx context.Context, hash string) (ContractPDFState, bool, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ContractPDFState{}, false, nil
	}
	return r.findContractPDFState(ctx, `file_hash = ?`, hash)
}

func (r *Repository) FindContractByAnyFileHash(ctx context.Context, hashes ...string) (ContractPDFState, bool, error) {
	cleaned := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash == "" || containsHash(cleaned, hash) {
			continue
		}
		cleaned = append(cleaned, hash)
	}
	if len(cleaned) == 0 {
		return ContractPDFState{}, false, nil
	}
	if len(cleaned) == 1 {
		return r.FindContractByFileHash(ctx, cleaned[0])
	}
	placeholders := make([]string, len(cleaned))
	args := make([]any, len(cleaned))
	for i, hash := range cleaned {
		placeholders[i] = "?"
		args[i] = hash
	}
	return r.findContractPDFState(ctx, `file_hash IN (`+strings.Join(placeholders, ",")+`)`, args...)
}

func (r *Repository) DeleteDuplicateContractRows(ctx context.Context, keepID int64, hashes []string, storageKey string) (int64, error) {
	return r.deleteDuplicateContractRows(ctx, r.db, keepID, hashes, storageKey)
}

func (r *Repository) deleteDuplicateContractRows(ctx context.Context, exec sqlExecer, keepID int64, hashes []string, storageKey string) (int64, error) {
	if keepID == 0 {
		return 0, nil
	}
	cleaned := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash == "" || containsHash(cleaned, hash) {
			continue
		}
		cleaned = append(cleaned, hash)
	}
	storageKey = strings.TrimSpace(storageKey)
	if len(cleaned) == 0 || storageKey == "" {
		return 0, nil
	}
	placeholders := make([]string, len(cleaned))
	args := make([]any, 0, len(cleaned)+2)
	args = append(args, keepID, storageKey)
	for i, hash := range cleaned {
		placeholders[i] = "?"
		args = append(args, hash)
	}
	res, err := exec.ExecContext(ctx, `
DELETE FROM contract_main
WHERE id <> ?
  AND storage_key = ?
  AND file_hash IN (`+strings.Join(placeholders, ",")+`)
`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func containsHash(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func (r *Repository) FindActiveContractBySlot(ctx context.Context, slotKey string) (ContractPDFState, bool, error) {
	slotKey = strings.TrimSpace(slotKey)
	if slotKey == "" {
		return ContractPDFState{}, false, nil
	}
	return r.findContractPDFState(ctx, `feishu_slot_key = ? AND (sync_status IS NULL OR sync_status <> ?)`, slotKey, SyncStatusDeleted)
}

func (r *Repository) FindContractRelationTarget(ctx context.Context, rootToken, relationKey string) (ContractPDFState, bool, error) {
	rootToken = strings.TrimSpace(rootToken)
	relationKey = strings.TrimSpace(relationKey)
	if rootToken == "" || relationKey == "" {
		return ContractPDFState{}, false, nil
	}
	query := `
SELECT
	id,
	COALESCE(file_name, ''),
	COALESCE(file_hash, ''),
	COALESCE(storage_key, ''),
	COALESCE(feishu_file_token, ''),
	COALESCE(feishu_root_token, ''),
	COALESCE(feishu_parent_token, ''),
	COALESCE(feishu_relative_path, ''),
	COALESCE(feishu_folder_path, ''),
	COALESCE(feishu_slot_key, ''),
	COALESCE(feishu_relation_key, ''),
	COALESCE(file_size, 0),
	COALESCE(sync_status, ''),
	COALESCE(ocr_status, '')
FROM contract_main
WHERE feishu_root_token = ?
  AND feishu_relation_key = ?
  AND (sync_status IS NULL OR sync_status <> ?)
ORDER BY id DESC
LIMIT 1`
	var state ContractPDFState
	err := r.db.QueryRowContext(ctx, query, rootToken, relationKey, SyncStatusDeleted).Scan(
		&state.ID,
		&state.FileName,
		&state.FileHash,
		&state.StorageKey,
		&state.FeishuFileToken,
		&state.FeishuRootToken,
		&state.FeishuParentToken,
		&state.FeishuRelativePath,
		&state.FeishuFolderPath,
		&state.FeishuSlotKey,
		&state.RelationKey,
		&state.FileSize,
		&state.SyncStatus,
		&state.OCRStatus,
	)
	if err == sql.ErrNoRows {
		return ContractPDFState{}, false, nil
	}
	if err != nil {
		return ContractPDFState{}, false, err
	}
	return state, true, nil
}

func (r *Repository) LinkPendingInvoicesToContract(ctx context.Context, rootToken, relationKey string, contractID int64) (int64, error) {
	rootToken = strings.TrimSpace(rootToken)
	relationKey = strings.TrimSpace(relationKey)
	if rootToken == "" || relationKey == "" || contractID == 0 {
		return 0, nil
	}
	if ok, err := dbpkg.TableExists(ctx, r.db, "contract_invoices"); err != nil || !ok {
		return 0, err
	}
	if ok, err := dbpkg.ColumnsExist(ctx, r.db, "contract_invoices", []string{"contract_id", "feishu_root_token", "feishu_relation_key", "last_seen_at"}); err != nil || !ok {
		return 0, err
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE contract_invoices
SET contract_id = ?,
    last_seen_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE feishu_root_token = ?
  AND feishu_relation_key = ?
  AND contract_id = ?
  AND (sync_status IS NULL OR sync_status <> ?)
`, contractID, rootToken, relationKey, contractID, SyncStatusDeleted)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) UpsertContractPDFState(ctx context.Context, state ContractPDFState) (int64, error) {
	if state.SyncStatus == "" {
		state.SyncStatus = SyncStatusActive
	}
	if state.OCRStatus == "" {
		state.OCRStatus = OCRStatusNone
	}
	if strings.TrimSpace(state.FeishuRootToken) == "" {
		state.FeishuRootToken = state.FeishuParentToken
	}
	if state.ID == 0 && strings.TrimSpace(state.FeishuFileToken) != "" {
		if existing, ok, err := r.findContractPDFState(ctx, `feishu_file_token = ?`, strings.TrimSpace(state.FeishuFileToken)); err != nil {
			return 0, err
		} else if ok {
			state.ID = existing.ID
		}
	}
	if state.ID == 0 && strings.TrimSpace(state.FeishuSlotKey) != "" {
		if existing, ok, err := r.findContractPDFState(ctx, `feishu_slot_key = ?`, strings.TrimSpace(state.FeishuSlotKey)); err != nil {
			return 0, err
		} else if ok {
			state.ID = existing.ID
		}
	}

	if state.ID != 0 {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("begin contract state upsert: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := r.deleteDuplicateContractRows(ctx, tx, state.ID, []string{state.FileHash}, state.StorageKey); err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `
UPDATE contract_main
SET file_name = ?,
    file_hash = ?,
    storage_key = ?,
    feishu_file_token = ?,
    feishu_root_token = ?,
    feishu_parent_token = ?,
    feishu_relative_path = ?,
    feishu_folder_path = ?,
    feishu_slot_key = ?,
    feishu_file_name = ?,
    feishu_modified_time = ?,
    feishu_relation_key = ?,
    file_size = ?,
    sync_status = ?,
    ocr_status = ?,
    feishu_deleted_at = NULL,
    last_seen_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableString(state.FileName), nullableString(state.FileHash), nullableString(state.StorageKey), nullableString(state.FeishuFileToken), nullableString(state.FeishuRootToken), nullableString(state.FeishuParentToken), nullableString(state.FeishuRelativePath), nullableString(state.FeishuFolderPath), nullableString(state.FeishuSlotKey), nullableString(state.FileName), nullableString(state.FeishuModifiedTime), nullableString(state.RelationKey), state.FileSize, nullableString(state.SyncStatus), nullableString(state.OCRStatus), state.ID)
		if err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("commit contract state upsert: %w", err)
		}
		return state.ID, nil
	}

	var id int64
	err := r.db.QueryRowContext(ctx, `
	INSERT INTO contract_main(
	job_id, file_name, file_hash, storage_key, feishu_file_token, feishu_root_token,
	feishu_parent_token, feishu_relative_path, feishu_folder_path,
	feishu_slot_key, feishu_file_name, feishu_modified_time, feishu_relation_key,
	category_id, file_size, sync_status, ocr_status, last_seen_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING id
`, generatedJobID(state), nullableString(state.FileName), nullableString(state.FileHash), nullableString(state.StorageKey), nullableString(state.FeishuFileToken), nullableString(state.FeishuRootToken), nullableString(state.FeishuParentToken), nullableString(state.FeishuRelativePath), nullableString(state.FeishuFolderPath), nullableString(state.FeishuSlotKey), nullableString(state.FileName), nullableString(state.FeishuModifiedTime), nullableString(state.RelationKey), r.defaultContractCategoryID(ctx), state.FileSize, nullableString(state.SyncStatus), nullableString(state.OCRStatus)).Scan(&id)
	return id, err
}

func (r *Repository) defaultContractCategoryID(ctx context.Context) int64 {
	if ok, err := dbpkg.TableExists(ctx, r.db, "contract_categories"); err != nil || !ok {
		return 1
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT id
FROM contract_categories
ORDER BY COALESCE(sort_order, id), id
LIMIT 1
`).Scan(&id)
	if err != nil || id == 0 {
		return 1
	}
	return id
}

func generatedJobID(state ContractPDFState) string {
	if token := strings.TrimSpace(state.FeishuFileToken); token != "" {
		return "feishu:" + token
	}
	if hash := strings.TrimSpace(state.FileHash); hash != "" {
		return "feishu:sha256:" + hash
	}
	return fmt.Sprintf("feishu:manual:%d", time.Now().UnixNano())
}

func (r *Repository) FindInvoiceByAnyFileHash(ctx context.Context, hashes ...string) (InvoicePDFState, bool, error) {
	cleaned := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash == "" || containsHash(cleaned, hash) {
			continue
		}
		cleaned = append(cleaned, hash)
	}
	if len(cleaned) == 0 {
		return InvoicePDFState{}, false, nil
	}
	placeholders := make([]string, len(cleaned))
	args := make([]any, len(cleaned))
	for i, hash := range cleaned {
		placeholders[i] = "?"
		args[i] = hash
	}
	return r.findInvoicePDFState(ctx, `file_hash IN (`+strings.Join(placeholders, ",")+`)`, args...)
}

func (r *Repository) FindActiveInvoiceBySlot(ctx context.Context, slotKey string) (InvoicePDFState, bool, error) {
	slotKey = strings.TrimSpace(slotKey)
	if slotKey == "" {
		return InvoicePDFState{}, false, nil
	}
	return r.findInvoicePDFState(ctx, `feishu_slot_key = ? AND (sync_status IS NULL OR sync_status <> ?)`, slotKey, SyncStatusDeleted)
}

func (r *Repository) UpsertInvoicePDFState(ctx context.Context, state InvoicePDFState) (int64, error) {
	if state.ContractID == 0 {
		return 0, errors.New("invoice PDF state requires a linked contract_id")
	}
	if ok, err := dbpkg.ColumnsExist(ctx, r.db, "contract_invoices", []string{
		"contract_id",
		"invoice_number",
		"file_name",
		"storage_key",
		"file_hash",
		"feishu_file_token",
		"feishu_root_token",
		"feishu_parent_token",
		"feishu_relative_path",
		"feishu_folder_path",
		"feishu_slot_key",
		"feishu_file_name",
		"feishu_relation_key",
		"feishu_modified_time",
		"file_size",
		"sync_status",
		"ocr_status",
		"feishu_deleted_at",
		"last_seen_at",
		"created_at",
		"updated_at",
	}); err != nil {
		return 0, err
	} else if !ok {
		return 0, errors.New("contract_invoices is missing feishu invoice sync columns; run database bootstrap")
	}
	if state.SyncStatus == "" {
		state.SyncStatus = SyncStatusActive
	}
	if state.OCRStatus == "" {
		state.OCRStatus = OCRStatusNone
	}
	if strings.TrimSpace(state.FeishuRootToken) == "" {
		state.FeishuRootToken = state.FeishuParentToken
	}
	if strings.TrimSpace(state.InvoiceNumber) == "" {
		state.InvoiceNumber = placeholderInvoiceNumber(state)
	}
	if state.ID == 0 && strings.TrimSpace(state.FeishuFileToken) != "" {
		if existing, ok, err := r.findInvoicePDFState(ctx, `feishu_file_token = ?`, strings.TrimSpace(state.FeishuFileToken)); err != nil {
			return 0, err
		} else if ok {
			state.ID = existing.ID
			if state.ContractID == 0 {
				state.ContractID = existing.ContractID
			}
			if strings.TrimSpace(state.InvoiceNumber) == "" {
				state.InvoiceNumber = existing.InvoiceNumber
			}
		}
	}
	if state.ID == 0 && strings.TrimSpace(state.FeishuSlotKey) != "" {
		if existing, ok, err := r.findInvoicePDFState(ctx, `feishu_slot_key = ?`, strings.TrimSpace(state.FeishuSlotKey)); err != nil {
			return 0, err
		} else if ok {
			state.ID = existing.ID
			if state.ContractID == 0 {
				state.ContractID = existing.ContractID
			}
			if strings.TrimSpace(state.InvoiceNumber) == "" {
				state.InvoiceNumber = existing.InvoiceNumber
			}
		}
	}

	if state.ID != 0 {
		_, err := r.db.ExecContext(ctx, `
UPDATE contract_invoices
SET contract_id = ?,
    invoice_number = ?,
    file_name = ?,
    storage_key = ?,
    file_hash = ?,
    feishu_file_token = ?,
    feishu_root_token = ?,
    feishu_parent_token = ?,
    feishu_relative_path = ?,
    feishu_folder_path = ?,
    feishu_slot_key = ?,
    feishu_file_name = ?,
    feishu_relation_key = ?,
    feishu_modified_time = ?,
    file_size = ?,
    sync_status = ?,
    ocr_status = ?,
    feishu_deleted_at = NULL,
    last_seen_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, state.ContractID, state.InvoiceNumber, nullableString(state.FileName), nullableString(state.StorageKey), nullableString(state.FileHash), nullableString(state.FeishuFileToken), nullableString(state.FeishuRootToken), nullableString(state.FeishuParentToken), nullableString(state.FeishuRelativePath), nullableString(state.FeishuFolderPath), nullableString(state.FeishuSlotKey), nullableString(state.FileName), nullableString(state.RelationKey), nullableString(state.FeishuModifiedTime), state.FileSize, nullableString(state.SyncStatus), nullableString(state.OCRStatus), state.ID)
		return state.ID, err
	}

	var id int64
	err := r.db.QueryRowContext(ctx, `
INSERT INTO contract_invoices(
	contract_id, invoice_number, file_name, storage_key, file_hash,
	feishu_file_token, feishu_root_token, feishu_parent_token,
	feishu_relative_path, feishu_folder_path, feishu_slot_key,
	feishu_file_name, feishu_relation_key, feishu_modified_time, file_size, sync_status, ocr_status,
	last_seen_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING id
`, state.ContractID, state.InvoiceNumber, nullableString(state.FileName), nullableString(state.StorageKey), nullableString(state.FileHash), nullableString(state.FeishuFileToken), nullableString(state.FeishuRootToken), nullableString(state.FeishuParentToken), nullableString(state.FeishuRelativePath), nullableString(state.FeishuFolderPath), nullableString(state.FeishuSlotKey), nullableString(state.FileName), nullableString(state.RelationKey), nullableString(state.FeishuModifiedTime), state.FileSize, nullableString(state.SyncStatus), nullableString(state.OCRStatus)).Scan(&id)
	return id, err
}

func placeholderInvoiceNumber(state InvoicePDFState) string {
	if hash := strings.TrimSpace(state.FileHash); hash != "" {
		return pendingInvoiceNumber(hash)
	}
	if token := strings.TrimSpace(state.FeishuFileToken); token != "" {
		return pendingInvoiceNumber(token)
	}
	return pendingInvoiceNumber(fmt.Sprintf("%x", time.Now().UnixNano()))
}

func pendingInvoiceNumber(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return -1
	}, value)
	if value == "" {
		value = "unknown"
	}
	if len(value) > 12 {
		value = value[:12]
	}
	return "pending:" + value
}

func (r *Repository) MarkDeletedByMissingTokens(ctx context.Context, rootToken string, activeTokens []string) (int64, error) {
	rootToken = strings.TrimSpace(rootToken)
	if rootToken == "" {
		return 0, nil
	}
	contractDeleted, err := r.markContractDeletedByMissingTokens(ctx, rootToken, activeTokens)
	if err != nil {
		return 0, err
	}
	invoiceDeleted, err := r.markInvoiceDeletedByMissingTokens(ctx, rootToken, activeTokens)
	if err != nil {
		return 0, err
	}
	return contractDeleted + invoiceDeleted, nil
}

func (r *Repository) MarkContractDeletedByMissingTokens(ctx context.Context, rootToken string, activeTokens []string) (int64, error) {
	return r.markContractDeletedByMissingTokens(ctx, rootToken, activeTokens)
}

func (r *Repository) markContractDeletedByMissingTokens(ctx context.Context, rootToken string, activeTokens []string) (int64, error) {
	rootToken = strings.TrimSpace(rootToken)
	if rootToken == "" {
		return 0, nil
	}
	args := []any{SyncStatusDeleted, rootToken, rootToken}
	filter := `(
		feishu_root_token = ?
		OR ((feishu_root_token IS NULL OR TRIM(feishu_root_token) = '') AND feishu_parent_token = ?)
	) AND feishu_file_token IS NOT NULL AND TRIM(feishu_file_token) <> '' AND (sync_status IS NULL OR sync_status <> 'deleted')`
	if len(activeTokens) > 0 {
		placeholders := make([]string, 0, len(activeTokens))
		for _, token := range activeTokens {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, token)
		}
		if len(placeholders) > 0 {
			filter += ` AND feishu_file_token NOT IN (` + strings.Join(placeholders, ",") + `)`
		}
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE contract_main
SET sync_status = ?,
    feishu_deleted_at = CURRENT_TIMESTAMP,
    last_seen_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE `+filter, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) markInvoiceDeletedByMissingTokens(ctx context.Context, rootToken string, activeTokens []string) (int64, error) {
	rootToken = strings.TrimSpace(rootToken)
	if rootToken == "" {
		return 0, nil
	}
	if ok, err := dbpkg.TableExists(ctx, r.db, "contract_invoices"); err != nil || !ok {
		return 0, err
	}
	if ok, err := dbpkg.ColumnsExist(ctx, r.db, "contract_invoices", []string{"sync_status", "feishu_deleted_at", "last_seen_at", "feishu_root_token", "feishu_parent_token", "feishu_file_token"}); err != nil || !ok {
		return 0, err
	}
	args := []any{SyncStatusDeleted, rootToken, rootToken}
	filter := `(
		feishu_root_token = ?
		OR ((feishu_root_token IS NULL OR TRIM(feishu_root_token) = '') AND feishu_parent_token = ?)
	) AND feishu_file_token IS NOT NULL AND TRIM(feishu_file_token) <> '' AND (sync_status IS NULL OR sync_status <> 'deleted')`
	if len(activeTokens) > 0 {
		placeholders := make([]string, 0, len(activeTokens))
		for _, token := range activeTokens {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, token)
		}
		if len(placeholders) > 0 {
			filter += ` AND feishu_file_token NOT IN (` + strings.Join(placeholders, ",") + `)`
		}
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE contract_invoices
SET sync_status = ?,
    feishu_deleted_at = CURRENT_TIMESTAMP,
    last_seen_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE `+filter, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) InsertDuplicateLog(ctx context.Context, log DuplicateLog) error {
	if ok, err := dbpkg.TableExists(ctx, r.db, "contract_duplicate_logs"); err != nil || !ok {
		return nil
	}
	if ok, err := dbpkg.ColumnsExist(ctx, r.db, "contract_duplicate_logs", []string{
		"event_type",
		"source_file_token",
		"existing_contract_id",
		"target_contract_id",
		"file_hash",
		"old_file_hash",
		"slot_key",
		"message",
		"metadata_json",
	}); err != nil || !ok {
		return nil
	}

	_, err := r.db.ExecContext(ctx, `
INSERT INTO contract_duplicate_logs(
	event_type, source_file_token, existing_contract_id, target_contract_id,
	file_hash, old_file_hash, slot_key, message, metadata_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, nullableString(log.EventType), nullableString(log.SourceFileToken), nullableInt64(log.ExistingContractID), nullableInt64(log.TargetContractID), nullableString(log.FileHash), nullableString(log.OldFileHash), nullableString(log.SlotKey), nullableString(log.Message), nullableString(log.MetadataJSON))
	return err
}

func (r *Repository) findContractPDFState(ctx context.Context, filter string, args ...any) (ContractPDFState, bool, error) {
	query := `
SELECT
	id,
	COALESCE(file_name, ''),
	COALESCE(file_hash, ''),
	COALESCE(storage_key, ''),
	COALESCE(feishu_file_token, ''),
	COALESCE(feishu_root_token, ''),
	COALESCE(feishu_parent_token, ''),
	COALESCE(feishu_relative_path, ''),
	COALESCE(feishu_folder_path, ''),
	COALESCE(feishu_slot_key, ''),
	COALESCE(feishu_modified_time, ''),
	COALESCE(feishu_relation_key, ''),
	COALESCE(file_size, 0),
	COALESCE(sync_status, ''),
	COALESCE(ocr_status, '')
FROM contract_main
WHERE ` + filter + `
ORDER BY
	CASE WHEN ocr_status = 'done' THEN 0 ELSE 1 END,
	CASE WHEN feishu_file_token IS NULL OR TRIM(feishu_file_token) = '' THEN 0 ELSE 1 END,
	id DESC
LIMIT 1`
	var state ContractPDFState
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&state.ID,
		&state.FileName,
		&state.FileHash,
		&state.StorageKey,
		&state.FeishuFileToken,
		&state.FeishuRootToken,
		&state.FeishuParentToken,
		&state.FeishuRelativePath,
		&state.FeishuFolderPath,
		&state.FeishuSlotKey,
		&state.FeishuModifiedTime,
		&state.RelationKey,
		&state.FileSize,
		&state.SyncStatus,
		&state.OCRStatus,
	)
	if err == sql.ErrNoRows {
		return ContractPDFState{}, false, nil
	}
	if err != nil {
		return ContractPDFState{}, false, err
	}
	return state, true, nil
}

func (r *Repository) findInvoicePDFState(ctx context.Context, filter string, args ...any) (InvoicePDFState, bool, error) {
	if ok, err := dbpkg.TableExists(ctx, r.db, "contract_invoices"); err != nil || !ok {
		return InvoicePDFState{}, false, err
	}
	if ok, err := dbpkg.ColumnsExist(ctx, r.db, "contract_invoices", []string{
		"contract_id",
		"invoice_number",
		"file_name",
		"storage_key",
		"file_hash",
		"feishu_file_token",
		"feishu_root_token",
		"feishu_parent_token",
		"feishu_relative_path",
		"feishu_folder_path",
		"feishu_slot_key",
		"feishu_relation_key",
		"feishu_modified_time",
		"file_size",
		"sync_status",
		"ocr_status",
	}); err != nil || !ok {
		return InvoicePDFState{}, false, err
	}
	query := `
SELECT
	id,
	COALESCE(contract_id, 0),
	COALESCE(invoice_number, ''),
	COALESCE(file_name, ''),
	COALESCE(file_hash, ''),
	COALESCE(storage_key, ''),
	COALESCE(feishu_file_token, ''),
	COALESCE(feishu_root_token, ''),
	COALESCE(feishu_parent_token, ''),
	COALESCE(feishu_relative_path, ''),
	COALESCE(feishu_folder_path, ''),
	COALESCE(feishu_slot_key, ''),
	COALESCE(feishu_modified_time, ''),
	COALESCE(feishu_relation_key, ''),
	COALESCE(file_size, 0),
	COALESCE(sync_status, ''),
	COALESCE(ocr_status, '')
FROM contract_invoices
WHERE ` + filter + `
ORDER BY
	CASE WHEN ocr_status = 'done' THEN 0 ELSE 1 END,
	CASE WHEN feishu_file_token IS NULL OR TRIM(feishu_file_token) = '' THEN 0 ELSE 1 END,
	id DESC
LIMIT 1`
	var state InvoicePDFState
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&state.ID,
		&state.ContractID,
		&state.InvoiceNumber,
		&state.FileName,
		&state.FileHash,
		&state.StorageKey,
		&state.FeishuFileToken,
		&state.FeishuRootToken,
		&state.FeishuParentToken,
		&state.FeishuRelativePath,
		&state.FeishuFolderPath,
		&state.FeishuSlotKey,
		&state.FeishuModifiedTime,
		&state.RelationKey,
		&state.FileSize,
		&state.SyncStatus,
		&state.OCRStatus,
	)
	if err == sql.ErrNoRows {
		return InvoicePDFState{}, false, nil
	}
	if err != nil {
		return InvoicePDFState{}, false, err
	}
	return state, true, nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func parseDBTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
