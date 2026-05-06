package feishusync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	if src.SourceType == SourceTypeFinanceWorkbookFolder {
		file, err := s.resolveWorkbookFolderFile(ctx, fileToken)
		if err != nil {
			_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
			return result, err
		}
		fileToken = strings.TrimSpace(file.Token)
		result.Source = src.SourceToken
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

	snapshotPath := workbookSnapshotPath(snapshotDir, fileToken, meta)
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
		if workbookFileMappingsSynced(src.MetadataJSON, hash) {
			result.Skipped = 1
			if err := s.repo.MarkSourceSuccess(ctx, src.ID, hash, strings.TrimSpace(meta.Revision), workbookMetadata(src, meta, ingest.ImportSummary{}, true, storageKey, hash)); err != nil {
				return result, err
			}
			return result, nil
		}

		summary, err := s.importer.ImportFileWithOptions(ctx, s.dbPath, snapshotPath, ingest.ImportOptions{
			Incremental:      false,
			CompanyOverride:  s.companyOverride,
			SourceFileName:   strings.TrimSpace(meta.Name),
			SourceStorageKey: storageKey,
			SourceFileSize:   workbookSnapshotSize(snapshotPath, meta),
		})
		if err != nil {
			_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
			return result, err
		}
		result.Created = 1
		if err := s.repo.MarkSourceSuccess(ctx, src.ID, hash, strings.TrimSpace(meta.Revision), workbookMetadata(src, meta, summary, false, storageKey, hash)); err != nil {
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
		Incremental:      false,
		CompanyOverride:  s.companyOverride,
		SourceFileName:   strings.TrimSpace(meta.Name),
		SourceStorageKey: storageKey,
		SourceFileSize:   workbookSnapshotSize(snapshotPath, meta),
	})
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
	}
	result.Created = 1

	if err := s.repo.MarkSourceSuccess(ctx, src.ID, hash, strings.TrimSpace(meta.Revision), workbookMetadata(src, meta, summary, false, storageKey, hash)); err != nil {
		return result, err
	}
	return result, nil
}

func (s *WorkbookScanner) resolveWorkbookFolderFile(ctx context.Context, folderToken string) (feishu.DriveFile, error) {
	files, err := s.client.ListFolderFiles(ctx, folderToken)
	if err != nil {
		return feishu.DriveFile{}, err
	}
	candidates := make([]feishu.DriveFile, 0, len(files))
	for _, file := range files {
		if !isWorkbookDriveFile(file) {
			continue
		}
		candidates = append(candidates, file)
	}
	if len(candidates) == 0 {
		return feishu.DriveFile{}, fmt.Errorf("no finance workbook found in folder %s", folderToken)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := workbookModifiedTime(candidates[i])
		right := workbookModifiedTime(candidates[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		return strings.TrimSpace(candidates[i].Name) > strings.TrimSpace(candidates[j].Name)
	})
	if strings.TrimSpace(candidates[0].Token) == "" {
		return feishu.DriveFile{}, fmt.Errorf("selected finance workbook in folder %s has empty token", folderToken)
	}
	return candidates[0], nil
}

func isWorkbookDriveFile(file feishu.DriveFile) bool {
	if isFolderDriveFile(file) {
		return false
	}
	name := strings.TrimSpace(file.Name)
	if name == "" || strings.HasPrefix(name, "~$") || strings.HasPrefix(name, ".") {
		return false
	}
	if isXLSXDriveFile(file) {
		return true
	}
	fileType := strings.ToLower(strings.TrimSpace(file.Type))
	mime := strings.ToLower(strings.TrimSpace(file.MimeType))
	return fileType == "sheet" || fileType == "bitable" || strings.Contains(mime, "spreadsheet")
}

func workbookModifiedTime(file feishu.DriveFile) time.Time {
	value := strings.TrimSpace(file.ModifiedTime)
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
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil && unix > 0 {
		if unix > 1_000_000_000_000 {
			return time.UnixMilli(unix)
		}
		return time.Unix(unix, 0)
	}
	return time.Time{}
}

func isXLSXDriveFile(file feishu.DriveFile) bool {
	if strings.EqualFold(filepath.Ext(strings.TrimSpace(file.Name)), ".xlsx") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(file.MimeType), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
}

func workbookSnapshotPath(snapshotDir, fileToken string, file feishu.DriveFile) string {
	name := safeObjectKeyPart(strings.TrimSpace(file.Name))
	if name == "" || looksLikeFeishuTokenWorkbookName(name) {
		name = safeObjectKeyPart(strings.TrimSpace(fileToken))
	}
	if !strings.EqualFold(filepath.Ext(name), ".xlsx") {
		name += ".xlsx"
	}
	return filepath.Join(snapshotDir, name)
}

func workbookSnapshotSize(snapshotPath string, file feishu.DriveFile) int64 {
	if info, err := os.Stat(strings.TrimSpace(snapshotPath)); err == nil && info.Size() > 0 {
		return info.Size()
	}
	if file.Size > 0 {
		return file.Size
	}
	return 0
}

func looksLikeFeishuTokenWorkbookName(name string) bool {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" {
		return false
	}
	return regexp.MustCompile(`^[A-Za-z0-9]{20,}$`).MatchString(name)
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

func workbookMetadata(src SyncSource, file feishu.DriveFile, summary ingest.ImportSummary, skipped bool, storageKey, contentHash string) string {
	payload := map[string]any{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(src.MetadataJSON)), &payload)
	payload["source"] = "feishu_active_scan"
	payload["source_token"] = strings.TrimSpace(src.SourceToken)
	payload["source_type"] = strings.TrimSpace(src.SourceType)
	payload["file_token"] = file.Token
	payload["file_name"] = file.Name
	payload["revision"] = file.Revision
	payload["oss_prefix"] = financeOSSPrefix(src, file)
	if strings.TrimSpace(storageKey) != "" || payload["storage_key"] == nil {
		payload["storage_key"] = strings.TrimSpace(storageKey)
	}
	payload["skipped"] = skipped
	if !skipped || strings.TrimSpace(summary.ReportType) != "" || payload["report_type"] == nil {
		payload["report_type"] = summary.ReportType
	}
	if !skipped || summary.RecordCount > 0 || payload["record_count"] == nil {
		payload["record_count"] = summary.RecordCount
	}
	if !skipped || strings.TrimSpace(summary.PeriodStart) != "" || payload["period_start"] == nil {
		payload["period_start"] = summary.PeriodStart
	}
	if !skipped || strings.TrimSpace(summary.PeriodEnd) != "" || payload["period_end"] == nil {
		payload["period_end"] = summary.PeriodEnd
	}
	if !skipped && strings.TrimSpace(contentHash) != "" {
		payload["file_mappings_content_hash"] = strings.TrimSpace(contentHash)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func workbookFileMappingsSynced(metadataJSON, contentHash string) bool {
	contentHash = strings.TrimSpace(contentHash)
	if contentHash == "" {
		return false
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(metadataJSON)), &payload); err != nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprintf("%v", payload["file_mappings_content_hash"])) == contentHash
}
