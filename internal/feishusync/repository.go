package feishusync

import (
	"context"
	"database/sql"
	"strings"
	"time"
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

func (r *Repository) FindActiveContractBySlot(ctx context.Context, slotKey string) (ContractPDFState, bool, error) {
	slotKey = strings.TrimSpace(slotKey)
	if slotKey == "" {
		return ContractPDFState{}, false, nil
	}
	return r.findContractPDFState(ctx, `feishu_slot_key = ? AND (sync_status IS NULL OR sync_status <> ?)`, slotKey, SyncStatusDeleted)
}

func (r *Repository) UpsertContractPDFState(ctx context.Context, state ContractPDFState) (int64, error) {
	if state.SyncStatus == "" {
		state.SyncStatus = SyncStatusActive
	}
	if state.OCRStatus == "" {
		state.OCRStatus = OCRStatusNone
	}
	if strings.TrimSpace(state.FeishuFileToken) != "" {
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
		_, err := r.db.ExecContext(ctx, `
UPDATE contract_main
SET file_name = ?,
    file_hash = ?,
    storage_key = ?,
    feishu_file_token = ?,
    feishu_parent_token = ?,
    feishu_slot_key = ?,
    feishu_file_name = ?,
    file_size = ?,
    sync_status = ?,
    ocr_status = ?,
    feishu_deleted_at = NULL,
    last_seen_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableString(state.FileName), nullableString(state.FileHash), nullableString(state.StorageKey), nullableString(state.FeishuFileToken), nullableString(state.FeishuParentToken), nullableString(state.FeishuSlotKey), nullableString(state.FileName), state.FileSize, nullableString(state.SyncStatus), nullableString(state.OCRStatus), state.ID)
		return state.ID, err
	}

	var id int64
	err := r.db.QueryRowContext(ctx, `
INSERT INTO contract_main(
	file_name, file_hash, storage_key, feishu_file_token, feishu_parent_token,
	feishu_slot_key, feishu_file_name, file_size, sync_status, ocr_status, last_seen_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
RETURNING id
`, nullableString(state.FileName), nullableString(state.FileHash), nullableString(state.StorageKey), nullableString(state.FeishuFileToken), nullableString(state.FeishuParentToken), nullableString(state.FeishuSlotKey), nullableString(state.FileName), state.FileSize, nullableString(state.SyncStatus), nullableString(state.OCRStatus)).Scan(&id)
	return id, err
}

func (r *Repository) MarkContractDeletedByMissingTokens(ctx context.Context, folderToken string, activeTokens []string) (int64, error) {
	folderToken = strings.TrimSpace(folderToken)
	if folderToken == "" {
		return 0, nil
	}
	args := []any{SyncStatusDeleted, folderToken}
	filter := `feishu_parent_token = ? AND feishu_file_token IS NOT NULL AND TRIM(feishu_file_token) <> '' AND (sync_status IS NULL OR sync_status <> 'deleted')`
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
    last_seen_at = CURRENT_TIMESTAMP
WHERE `+filter, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) InsertDuplicateLog(ctx context.Context, log DuplicateLog) error {
	if ok, err := r.tableExists(ctx, "contract_duplicate_logs"); err != nil || !ok {
		return nil
	}
	if ok, err := r.columnsExist(ctx, "contract_duplicate_logs", []string{
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
	COALESCE(feishu_parent_token, ''),
	COALESCE(feishu_slot_key, ''),
	COALESCE(file_size, 0),
	COALESCE(sync_status, ''),
	COALESCE(ocr_status, '')
FROM contract_main
WHERE ` + filter + `
ORDER BY id DESC
LIMIT 1`
	var state ContractPDFState
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&state.ID,
		&state.FileName,
		&state.FileHash,
		&state.StorageKey,
		&state.FeishuFileToken,
		&state.FeishuParentToken,
		&state.FeishuSlotKey,
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

func (r *Repository) tableExists(ctx context.Context, tableName string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name = ?`, tableName).Scan(&count)
	if err == nil {
		return count > 0, nil
	}

	var regclass sql.NullString
	err = r.db.QueryRowContext(ctx, `SELECT to_regclass(?)`, tableName).Scan(&regclass)
	if err != nil {
		return false, err
	}
	return regclass.Valid && strings.TrimSpace(regclass.String) != "", nil
}

func (r *Repository) columnsExist(ctx context.Context, tableName string, columns []string) (bool, error) {
	existing := map[string]bool{}
	rows, err := r.db.QueryContext(ctx, `PRAGMA table_info(`+tableName+`)`)
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

	rows, err = r.db.QueryContext(ctx, `
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

func containsAllColumns(existing map[string]bool, columns []string) bool {
	for _, column := range columns {
		if !existing[column] {
			return false
		}
	}
	return true
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
