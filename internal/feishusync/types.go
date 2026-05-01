package feishusync

import (
	"context"
	"time"
)

const (
	SourceTypePDFFolder       = "pdf_folder"
	SourceTypeFinanceWorkbook = "finance_workbook"

	SyncStatusActive   = "active"
	SyncStatusPending  = "pending"
	SyncStatusMissing  = "missing"
	SyncStatusError    = "error"
	SyncStatusDisabled = "disabled"
	SyncStatusDeleted  = "deleted"

	OCRStatusNone    = "none"
	OCRStatusPending = "pending"
	OCRStatusDone    = "done"
	OCRStatusFailed  = "failed"

	DuplicateEventSameHash        = "same_hash_duplicate"
	DuplicateEventSameSlotReplace = "same_slot_replaced"
	DuplicateEventDeletedReupload = "deleted_then_reuploaded"
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

type ContractPDFState struct {
	ID                int64  `json:"id"`
	FileName          string `json:"file_name"`
	FileHash          string `json:"file_hash"`
	StorageKey        string `json:"storage_key"`
	FeishuFileToken   string `json:"feishu_file_token"`
	FeishuParentToken string `json:"feishu_parent_token"`
	FeishuSlotKey     string `json:"feishu_slot_key"`
	FileSize          int64  `json:"file_size"`
	SyncStatus        string `json:"sync_status"`
	OCRStatus         string `json:"ocr_status"`
}

type DuplicateLog struct {
	EventType          string
	SourceFileToken    string
	ExistingContractID int64
	TargetContractID   int64
	FileHash           string
	OldFileHash        string
	SlotKey            string
	Message            string
	MetadataJSON       string
}

type ScanResult struct {
	SourceID  int64  `json:"source_id,omitempty"`
	Source    string `json:"source,omitempty"`
	Scanned   int    `json:"scanned"`
	Skipped   int    `json:"skipped"`
	Created   int    `json:"created"`
	Reused    int    `json:"reused"`
	Replaced  int    `json:"replaced"`
	Deleted   int64  `json:"deleted"`
	OCRQueued int    `json:"ocr_queued"`
}

type OCRDispatcher interface {
	EnqueueOCR(ctx context.Context, contractID int64, filePath string, fileHash string) error
}

type NoopOCRDispatcher struct{}

func (NoopOCRDispatcher) EnqueueOCR(context.Context, int64, string, string) error {
	return nil
}
