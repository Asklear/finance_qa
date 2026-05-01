package feishusync

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"financeqa/internal/feishu"
)

type PDFScanner struct {
	client      feishu.Client
	repo        *Repository
	ocr         OCRDispatcher
	snapshotDir string
}

func NewPDFScanner(client feishu.Client, repo *Repository, ocr OCRDispatcher, snapshotDir string) *PDFScanner {
	if ocr == nil {
		ocr = NoopOCRDispatcher{}
	}
	return &PDFScanner{
		client:      client,
		repo:        repo,
		ocr:         ocr,
		snapshotDir: strings.TrimSpace(snapshotDir),
	}
}

func (s *PDFScanner) ScanFolder(ctx context.Context, src SyncSource) (ScanResult, error) {
	result := ScanResult{SourceID: src.ID, Source: src.SourceToken}
	if s.client == nil {
		return result, errors.New("feishu client is required")
	}
	if s.repo == nil {
		return result, errors.New("feishu sync repository is required")
	}
	folderToken := strings.TrimSpace(src.SourceToken)
	if folderToken == "" {
		return result, errors.New("source token is required")
	}
	snapshotDir := s.snapshotDir
	if snapshotDir == "" {
		snapshotDir = filepath.Join("tmp", "feishu-snapshots")
	}

	files, err := s.client.ListFolderFiles(ctx, folderToken)
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
	}

	activeTokens := make([]string, 0, len(files))
	var revisions []string
	for _, file := range files {
		if !isPDFDriveFile(file) {
			result.Skipped++
			continue
		}
		file.Token = strings.TrimSpace(file.Token)
		if file.Token == "" {
			result.Skipped++
			continue
		}
		activeTokens = append(activeTokens, file.Token)
		if strings.TrimSpace(file.Revision) != "" {
			revisions = append(revisions, strings.TrimSpace(file.Revision))
		}
		if err := s.scanPDFFile(ctx, folderToken, snapshotDir, file, &result); err != nil {
			_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
			return result, err
		}
	}

	deleted, err := s.repo.MarkContractDeletedByMissingTokens(ctx, folderToken, activeTokens)
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
	}
	result.Deleted = deleted

	metadata := scanMetadata{
		Scanned:   result.Scanned,
		Skipped:   result.Skipped,
		Created:   result.Created,
		Reused:    result.Reused,
		Replaced:  result.Replaced,
		Deleted:   result.Deleted,
		OCRQueued: result.OCRQueued,
	}
	metadataJSON, _ := json.Marshal(metadata)
	if err := s.repo.MarkSourceSuccess(ctx, src.ID, "", strings.Join(revisions, ","), string(metadataJSON)); err != nil {
		return result, err
	}
	return result, nil
}

func (s *PDFScanner) scanPDFFile(ctx context.Context, folderToken, snapshotDir string, file feishu.DriveFile, result *ScanResult) error {
	result.Scanned++

	fileName := strings.TrimSpace(file.Name)
	slotKey := SlotKey(folderToken, fileName)
	filePath := filepath.Join(snapshotDir, folderToken, file.Token+".pdf")
	if err := s.client.DownloadFile(ctx, file.Token, filePath); err != nil {
		return err
	}
	hash, err := FileSHA256(filePath)
	if err != nil {
		return err
	}

	state := ContractPDFState{
		FileName:          fileName,
		FileHash:          hash,
		StorageKey:        filePath,
		FeishuFileToken:   file.Token,
		FeishuParentToken: folderToken,
		FeishuSlotKey:     slotKey,
		FileSize:          file.Size,
		SyncStatus:        SyncStatusActive,
		OCRStatus:         OCRStatusPending,
	}

	if existing, ok, err := s.repo.FindContractByFileHash(ctx, hash); err != nil {
		return err
	} else if ok {
		state.ID = existing.ID
		state.OCRStatus = existing.OCRStatus
		if state.OCRStatus == "" || state.OCRStatus == OCRStatusNone {
			state.OCRStatus = OCRStatusPending
		}
		contractID, err := s.repo.UpsertContractPDFState(ctx, state)
		if err != nil {
			return err
		}
		result.Reused++
		return s.repo.InsertDuplicateLog(ctx, DuplicateLog{
			EventType:          DuplicateEventSameHash,
			SourceFileToken:    file.Token,
			ExistingContractID: existing.ID,
			TargetContractID:   contractID,
			FileHash:           hash,
			SlotKey:            slotKey,
			Message:            "same hash duplicate from feishu folder scan",
			MetadataJSON:       duplicateMetadata(file, folderToken),
		})
	}

	if existing, ok, err := s.repo.FindActiveContractBySlot(ctx, slotKey); err != nil {
		return err
	} else if ok {
		state.ID = existing.ID
		contractID, err := s.repo.UpsertContractPDFState(ctx, state)
		if err != nil {
			return err
		}
		if err := s.repo.InsertDuplicateLog(ctx, DuplicateLog{
			EventType:          DuplicateEventSameSlotReplace,
			SourceFileToken:    file.Token,
			ExistingContractID: existing.ID,
			TargetContractID:   contractID,
			FileHash:           hash,
			OldFileHash:        existing.FileHash,
			SlotKey:            slotKey,
			Message:            "same slot replaced by feishu folder scan",
			MetadataJSON:       duplicateMetadata(file, folderToken),
		}); err != nil {
			return err
		}
		if err := s.ocr.EnqueueOCR(ctx, contractID, filePath, hash); err != nil {
			return err
		}
		result.Replaced++
		result.OCRQueued++
		return nil
	}

	contractID, err := s.repo.UpsertContractPDFState(ctx, state)
	if err != nil {
		return err
	}
	if err := s.ocr.EnqueueOCR(ctx, contractID, filePath, hash); err != nil {
		return err
	}
	result.Created++
	result.OCRQueued++
	return nil
}

func isPDFDriveFile(file feishu.DriveFile) bool {
	if strings.EqualFold(strings.TrimSpace(file.MimeType), "application/pdf") {
		return true
	}
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(file.Name)), ".pdf")
}

func duplicateMetadata(file feishu.DriveFile, folderToken string) string {
	data, err := json.Marshal(map[string]any{
		"source":       "feishu_active_scan",
		"folder_token": folderToken,
		"file_token":   file.Token,
		"file_name":    file.Name,
		"revision":     file.Revision,
	})
	if err != nil {
		return ""
	}
	return string(data)
}

type scanMetadata struct {
	Scanned   int   `json:"scanned"`
	Skipped   int   `json:"skipped"`
	Created   int   `json:"created"`
	Reused    int   `json:"reused"`
	Replaced  int   `json:"replaced"`
	Deleted   int64 `json:"deleted"`
	OCRQueued int   `json:"ocr_queued"`
}
