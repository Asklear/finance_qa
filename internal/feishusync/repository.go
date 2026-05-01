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
