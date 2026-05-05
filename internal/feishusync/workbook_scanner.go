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
	store           ObjectStore
}

func NewWorkbookScanner(client feishu.Client, repo *Repository, importer ContractWorkbookImporter, dbPath, snapshotDir, company string, stores ...ObjectStore) *WorkbookScanner {
	var store ObjectStore
	if len(stores) > 0 {
		store = stores[0]
	}
	return &WorkbookScanner{
		client:          client,
		repo:            repo,
		importer:        importer,
		dbPath:          strings.TrimSpace(dbPath),
		snapshotDir:     strings.TrimSpace(snapshotDir),
		companyOverride: strings.TrimSpace(company),
		store:           store,
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
		storageKey := sourceStorageKey(src.MetadataJSON)
		if s.store != nil {
			if resolvedKey, resolved, err := s.resolveWorkbookSnapshot(ctx, src, meta, hash, storageKey); err != nil {
				_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
				return result, err
			} else if resolved {
				storageKey = resolvedKey
			} else if strings.TrimSpace(storageKey) == "" || strings.HasPrefix(strings.TrimSpace(storageKey), "s3://") {
				uploadedKey, err := s.storeWorkbookSnapshot(ctx, src, meta, snapshotPath, hash)
				if err != nil {
					_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
					return result, err
				}
				storageKey = uploadedKey
			}
		}
		if err := s.repo.MarkSourceSuccess(ctx, src.ID, hash, strings.TrimSpace(meta.Revision), workbookMetadata(src, meta, ingest.ImportSummary{}, true, storageKey)); err != nil {
			return result, err
		}
		return result, nil
	}

	storageKey, err := s.storeWorkbookSnapshot(ctx, src, meta, snapshotPath, hash)
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
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

	if err := s.repo.MarkSourceSuccess(ctx, src.ID, hash, strings.TrimSpace(meta.Revision), workbookMetadata(src, meta, summary, false, storageKey)); err != nil {
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

func (s *WorkbookScanner) storeWorkbookSnapshot(ctx context.Context, src SyncSource, file feishu.DriveFile, snapshotPath, hash string) (string, error) {
	if s.store == nil {
		return "", nil
	}
	key := workbookObjectKey(src, file)
	if probe, ok := s.store.(ObjectStoreProbe); ok {
		remoteHash, exists, err := probe.ObjectSHA256(ctx, key)
		if err != nil {
			return "", err
		}
		if exists && sameContentHash(remoteHash, hash) {
			if uri := strings.TrimSpace(probe.ObjectURI(key)); uri != "" {
				return uri, nil
			}
		}
		if exists {
			if uri, found, err := s.findExistingWorkbookObjectByHash(ctx, src, file, hash); err != nil || found {
				return uri, err
			}
			nextKey, same, err := unusedHashSuffixedObjectKey(ctx, probe, key, hash)
			if err != nil {
				return "", err
			}
			if same {
				if uri := strings.TrimSpace(probe.ObjectURI(nextKey)); uri != "" {
					return uri, nil
				}
			}
			key = nextKey
		}
	}
	if uri, exists, err := s.findExistingWorkbookObjectByHash(ctx, src, file, hash); err != nil {
		return "", err
	} else if exists {
		return uri, nil
	}
	return s.store.PutFile(ctx, snapshotPath, key, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
}

func (s *WorkbookScanner) findExistingWorkbookObjectByHash(ctx context.Context, src SyncSource, file feishu.DriveFile, hash string) (string, bool, error) {
	if s.store == nil {
		return "", false, nil
	}
	finder, ok := s.store.(ObjectStoreHashFinder)
	if !ok {
		return "", false, nil
	}
	key, exists, err := finder.FindObjectBySHA256(ctx, financeOSSPrefix(src, file), hash)
	if err != nil || !exists {
		return "", false, err
	}
	if probe, ok := s.store.(ObjectStoreProbe); ok {
		if uri := strings.TrimSpace(probe.ObjectURI(key)); uri != "" {
			return uri, true, nil
		}
	}
	return key, true, nil
}

func (s *WorkbookScanner) resolveWorkbookSnapshot(ctx context.Context, src SyncSource, file feishu.DriveFile, hash, existingStorageKey string) (string, bool, error) {
	if s.store == nil {
		return "", false, nil
	}
	probe, ok := s.store.(ObjectStoreProbe)
	if !ok {
		return "", false, nil
	}
	candidates := []string{strings.TrimSpace(existingStorageKey), workbookObjectKey(src, file)}
	for _, key := range candidates {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		remoteHash, exists, err := probe.ObjectSHA256(ctx, key)
		if err != nil {
			return "", false, err
		}
		if exists && sameContentHash(remoteHash, hash) {
			if uri := strings.TrimSpace(probe.ObjectURI(objectKeyFromStorageKey(key))); uri != "" {
				return uri, true, nil
			}
		}
	}
	if uri, exists, err := s.findExistingWorkbookObjectByHash(ctx, src, file, hash); err != nil || exists {
		return uri, exists, err
	}
	return "", false, nil
}

func workbookMetadata(src SyncSource, file feishu.DriveFile, summary ingest.ImportSummary, skipped bool, storageKey string) string {
	data, err := json.Marshal(map[string]any{
		"source":       "feishu_active_scan",
		"file_token":   file.Token,
		"file_name":    file.Name,
		"revision":     file.Revision,
		"oss_prefix":   financeOSSPrefix(src, file),
		"storage_key":  strings.TrimSpace(storageKey),
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
