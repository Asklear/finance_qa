package feishusync

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"financeqa/internal/feishu"
	"financeqa/internal/ingest"
)

type ContractWorkbookImporter interface {
	ImportFileWithOptions(ctx context.Context, dbPath, filePath string, opts ingest.ImportOptions) (ingest.ImportSummary, error)
}

type WorkbookScanner struct {
	client          feishu.Client
	repo            *Repository
	importer        ContractWorkbookImporter
	dbPath          string
	snapshotDir     string
	companyOverride string
}

func NewWorkbookScanner(client feishu.Client, repo *Repository, importer ContractWorkbookImporter, dbPath, snapshotDir, company string) *WorkbookScanner {
	return &WorkbookScanner{
		client:          client,
		repo:            repo,
		importer:        importer,
		dbPath:          strings.TrimSpace(dbPath),
		snapshotDir:     strings.TrimSpace(snapshotDir),
		companyOverride: strings.TrimSpace(company),
	}
}

func (s *WorkbookScanner) ScanWorkbook(ctx context.Context, src SyncSource) (ScanResult, error) {
	result := ScanResult{SourceID: src.ID, Source: src.SourceToken}
	if s.client == nil {
		return result, errors.New("feishu client is required")
	}
	if s.repo == nil {
		return result, errors.New("feishu sync repository is required")
	}
	if s.importer == nil {
		return result, errors.New("contract workbook importer is required")
	}
	fileToken := strings.TrimSpace(src.SourceToken)
	if fileToken == "" {
		return result, errors.New("source token is required")
	}
	snapshotDir := s.snapshotDir
	if snapshotDir == "" {
		snapshotDir = filepath.Join("tmp", "feishu-snapshots")
	}

	meta, err := s.client.GetFileMetadata(ctx, fileToken)
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
	}
	if strings.TrimSpace(meta.Token) == "" {
		meta.Token = fileToken
	}

	snapshotPath := filepath.Join(snapshotDir, fileToken+".xlsx")
	if isXLSXDriveFile(meta) {
		err = s.client.DownloadFile(ctx, fileToken, snapshotPath)
	} else {
		err = s.client.ExportToXLSX(ctx, fileToken, snapshotPath)
	}
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
	}

	hash, err := FileSHA256(snapshotPath)
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
	}
	result.Scanned = 1

	if hash == strings.TrimSpace(src.LastContentHash) {
		result.Skipped = 1
		if err := s.repo.MarkSourceSuccess(ctx, src.ID, hash, strings.TrimSpace(meta.Revision), workbookMetadata(meta, ingest.ImportSummary{}, true)); err != nil {
			return result, err
		}
		return result, nil
	}

	summary, err := s.importer.ImportFileWithOptions(ctx, s.dbPath, snapshotPath, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: s.companyOverride,
	})
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
	}
	result.Created = 1

	if err := s.repo.MarkSourceSuccess(ctx, src.ID, hash, strings.TrimSpace(meta.Revision), workbookMetadata(meta, summary, false)); err != nil {
		return result, err
	}
	return result, nil
}

func isXLSXDriveFile(file feishu.DriveFile) bool {
	if strings.EqualFold(filepath.Ext(strings.TrimSpace(file.Name)), ".xlsx") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(file.MimeType), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
}

func workbookMetadata(file feishu.DriveFile, summary ingest.ImportSummary, skipped bool) string {
	data, err := json.Marshal(map[string]any{
		"source":       "feishu_active_scan",
		"file_token":   file.Token,
		"file_name":    file.Name,
		"revision":     file.Revision,
		"skipped":      skipped,
		"report_type":  summary.ReportType,
		"record_count": summary.RecordCount,
		"period_start": summary.PeriodStart,
		"period_end":   summary.PeriodEnd,
	})
	if err != nil {
		return ""
	}
	return string(data)
}
