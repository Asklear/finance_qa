package feishusync

import "time"

const (
	SourceTypePDFFolder       = "pdf_folder"
	SourceTypeFinanceWorkbook = "finance_workbook"

	SyncStatusActive   = "active"
	SyncStatusPending  = "pending"
	SyncStatusMissing  = "missing"
	SyncStatusError    = "error"
	SyncStatusDisabled = "disabled"
)

type SyncSource struct {
	ID              int64     `json:"id"`
	SourceType      string    `json:"source_type"`
	SourceToken     string    `json:"source_token"`
	SourceURL       string    `json:"source_url,omitempty"`
	DisplayName     string    `json:"display_name,omitempty"`
	ParentToken     string    `json:"parent_token,omitempty"`
	SyncMode        string    `json:"sync_mode,omitempty"`
	SyncStatus      string    `json:"sync_status"`
	LastRevision    string    `json:"last_revision,omitempty"`
	LastContentHash string    `json:"last_content_hash,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	MetadataJSON    string    `json:"metadata_json,omitempty"`
	LastSyncAt      time.Time `json:"last_sync_at,omitempty"`
	LastSuccessAt   time.Time `json:"last_success_at,omitempty"`
	NextScanAt      time.Time `json:"next_scan_at,omitempty"`
}

type SourceFilter struct {
	SourceType      string
	DueOnly         bool
	IncludeDisabled bool
}
