package feishusync

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"financeqa/internal/feishu"
)

type PDFScanner struct {
	client      feishu.Client
	repo        *Repository
	ocr         OCRDispatcher
	snapshotDir string
	store       ObjectStore
}

type ObjectStore interface {
	PutFile(ctx context.Context, localPath, key, contentType string) (string, error)
}

type ObjectStoreProbe interface {
	ObjectSHA256(ctx context.Context, key string) (string, bool, error)
	ObjectURI(key string) string
}

type ObjectStoreHashFinder interface {
	FindObjectBySHA256(ctx context.Context, prefix, hash string) (string, bool, error)
}

func NewPDFScanner(client feishu.Client, repo *Repository, ocr OCRDispatcher, snapshotDir string, stores ...ObjectStore) *PDFScanner {
	if ocr == nil {
		ocr = NoopOCRDispatcher{}
	}
	var store ObjectStore
	if len(stores) > 0 {
		store = stores[0]
	}
	return &PDFScanner{
		client:      client,
		repo:        repo,
		ocr:         ocr,
		snapshotDir: strings.TrimSpace(snapshotDir),
		store:       store,
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

	files, err := s.listPDFFiles(ctx, folderToken, &result)
	if err != nil {
		_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
		return result, err
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].DocumentKind == "contract" && files[j].DocumentKind != "contract"
	})

	activeTokens := make([]string, 0, len(files))
	var revisions []string
	for _, file := range files {
		activeTokens = append(activeTokens, file.File.Token)
		if strings.TrimSpace(file.File.Revision) != "" {
			revisions = append(revisions, strings.TrimSpace(file.File.Revision))
		}
		if err := s.scanPDFFile(ctx, src, folderToken, snapshotDir, file, &result); err != nil {
			_ = s.repo.MarkSourceError(ctx, src.ID, err.Error(), time.Time{})
			return result, err
		}
	}

	deleted, err := s.repo.MarkDeletedByMissingTokens(ctx, folderToken, activeTokens)
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
		OSSPrefix: sourceOSSPrefix(src, "OSS_CONTRACT_PREFIX", defaultContractOSSPrefix),
	}
	metadataJSON, _ := json.Marshal(metadata)
	if err := s.repo.MarkSourceSuccess(ctx, src.ID, "", strings.Join(revisions, ","), string(metadataJSON)); err != nil {
		return result, err
	}
	return result, nil
}

type pdfScanFile struct {
	File         feishu.DriveFile
	ParentToken  string
	RelativePath string
	FolderPath   string
	DocumentKind string
	RelationKey  string
}

type folderScanNode struct {
	Token string
	Path  string
}

func (s *PDFScanner) listPDFFiles(ctx context.Context, rootToken string, result *ScanResult) ([]pdfScanFile, error) {
	var out []pdfScanFile
	visited := map[string]bool{}
	queue := []folderScanNode{{Token: rootToken}}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		folderToken := strings.TrimSpace(node.Token)
		if folderToken == "" || visited[folderToken] {
			continue
		}
		visited[folderToken] = true

		files, err := s.client.ListFolderFiles(ctx, folderToken)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			file.Token = strings.TrimSpace(file.Token)
			file.Name = strings.TrimSpace(file.Name)
			if file.Token == "" {
				result.Skipped++
				continue
			}
			relativePath := joinRelativePath(node.Path, file.Name)
			if isFolderDriveFile(file) {
				queue = append(queue, folderScanNode{Token: file.Token, Path: relativePath})
				continue
			}
			if !isPDFDriveFile(file) {
				result.Skipped++
				continue
			}
			if strings.TrimSpace(file.ParentToken) == "" {
				file.ParentToken = folderToken
			}
			folderPath := cleanRelativePath(node.Path)
			kind := classifyPDFDocumentKind(relativePath)
			out = append(out, pdfScanFile{
				File:         file,
				ParentToken:  folderToken,
				RelativePath: relativePath,
				FolderPath:   folderPath,
				DocumentKind: kind,
				RelationKey:  relationKeyForPath(relativePath, kind),
			})
		}
	}
	return out, nil
}

func (s *PDFScanner) scanPDFFile(ctx context.Context, src SyncSource, rootToken, snapshotDir string, scanFile pdfScanFile, result *ScanResult) error {
	result.Scanned++
	if scanFile.DocumentKind == "invoice" {
		return s.scanInvoicePDFFile(ctx, src, rootToken, snapshotDir, scanFile, result)
	}

	file := scanFile.File
	fileName := strings.TrimSpace(file.Name)
	slotKey := SlotKey(rootToken, scanFile.RelativePath)
	filePath := filepath.Join(snapshotDir, rootToken, file.Token+".pdf")
	if err := s.client.DownloadFile(ctx, file.Token, filePath); err != nil {
		return err
	}
	fileSize := downloadedFileSize(filePath, file.Size)
	hash, err := FileSHA256(filePath)
	if err != nil {
		return err
	}
	legacyMD5, err := FileMD5(filePath)
	if err != nil {
		return err
	}

	if existing, ok, err := s.repo.FindContractByAnyFileHash(ctx, hash, legacyMD5); err != nil {
		return err
	} else if ok {
		storageKey := strings.TrimSpace(existing.StorageKey)
		if s.store != nil {
			resolvedKey, resolved, err := s.resolvePDFStorageKey(ctx, src, file, scanFile.RelativePath, filePath, storageKey, hash)
			if err != nil {
				return err
			}
			if resolved {
				storageKey = resolvedKey
			}
		}
		state := ContractPDFState{
			FileName:           fileName,
			FileHash:           hash,
			StorageKey:         storageKey,
			FeishuFileToken:    file.Token,
			FeishuRootToken:    rootToken,
			FeishuParentToken:  scanFile.ParentToken,
			FeishuRelativePath: scanFile.RelativePath,
			FeishuFolderPath:   scanFile.FolderPath,
			FeishuSlotKey:      slotKey,
			RelationKey:        scanFile.RelationKey,
			FileSize:           fileSize,
			SyncStatus:         SyncStatusActive,
			OCRStatus:          OCRStatusPending,
		}
		state.ID = existing.ID
		state.OCRStatus = existing.OCRStatus
		if state.OCRStatus == "" || state.OCRStatus == OCRStatusNone {
			state.OCRStatus = OCRStatusPending
		}
		contractID, err := s.repo.UpsertContractPDFState(ctx, state)
		if err != nil {
			return err
		}
		if _, err := s.repo.DeleteDuplicateContractRows(ctx, contractID, []string{hash, legacyMD5}, state.StorageKey); err != nil {
			return err
		}
		if _, err := s.repo.LinkPendingInvoicesToContract(ctx, rootToken, state.RelationKey, contractID); err != nil {
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
			MetadataJSON:       duplicateMetadata(file, rootToken, scanFile),
		})
	}

	storageKey, err := s.storePDFSnapshot(ctx, src, file, scanFile.RelativePath, filePath, hash)
	if err != nil {
		return err
	}
	state := ContractPDFState{
		FileName:           fileName,
		FileHash:           hash,
		StorageKey:         storageKey,
		FeishuFileToken:    file.Token,
		FeishuRootToken:    rootToken,
		FeishuParentToken:  scanFile.ParentToken,
		FeishuRelativePath: scanFile.RelativePath,
		FeishuFolderPath:   scanFile.FolderPath,
		FeishuSlotKey:      slotKey,
		RelationKey:        scanFile.RelationKey,
		FileSize:           fileSize,
		SyncStatus:         SyncStatusActive,
		OCRStatus:          OCRStatusPending,
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
			MetadataJSON:       duplicateMetadata(file, rootToken, scanFile),
		}); err != nil {
			return err
		}
		if _, err := s.repo.LinkPendingInvoicesToContract(ctx, rootToken, state.RelationKey, contractID); err != nil {
			return err
		}
		if err := s.ocr.EnqueueOCR(ctx, contractID, state.StorageKey, hash); err != nil {
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
	if err := s.ocr.EnqueueOCR(ctx, contractID, state.StorageKey, hash); err != nil {
		return err
	}
	if _, err := s.repo.LinkPendingInvoicesToContract(ctx, rootToken, state.RelationKey, contractID); err != nil {
		return err
	}
	result.Created++
	result.OCRQueued++
	return nil
}

func (s *PDFScanner) scanInvoicePDFFile(ctx context.Context, src SyncSource, rootToken, snapshotDir string, scanFile pdfScanFile, result *ScanResult) error {
	file := scanFile.File
	fileName := strings.TrimSpace(file.Name)
	slotKey := SlotKey(rootToken, scanFile.RelativePath)
	target, ok, err := s.repo.FindContractRelationTarget(ctx, rootToken, scanFile.RelationKey)
	if err != nil {
		return err
	}
	if !ok {
		result.Skipped++
		return nil
	}

	filePath := filepath.Join(snapshotDir, rootToken, file.Token+".pdf")
	if err := s.client.DownloadFile(ctx, file.Token, filePath); err != nil {
		return err
	}
	fileSize := downloadedFileSize(filePath, file.Size)
	hash, err := FileSHA256(filePath)
	if err != nil {
		return err
	}
	legacyMD5, err := FileMD5(filePath)
	if err != nil {
		return err
	}

	state := InvoicePDFState{
		FileName:           fileName,
		FileHash:           hash,
		FeishuFileToken:    file.Token,
		FeishuRootToken:    rootToken,
		FeishuParentToken:  scanFile.ParentToken,
		FeishuRelativePath: scanFile.RelativePath,
		FeishuFolderPath:   scanFile.FolderPath,
		FeishuSlotKey:      slotKey,
		RelationKey:        scanFile.RelationKey,
		FileSize:           fileSize,
		SyncStatus:         SyncStatusActive,
		OCRStatus:          OCRStatusPending,
		ContractID:         target.ID,
	}

	if existing, ok, err := s.repo.FindInvoiceByAnyFileHash(ctx, hash, legacyMD5); err != nil {
		return err
	} else if ok {
		storageKey := strings.TrimSpace(existing.StorageKey)
		if s.store != nil {
			resolvedKey, resolved, err := s.resolvePDFStorageKey(ctx, src, file, scanFile.RelativePath, filePath, storageKey, hash)
			if err != nil {
				return err
			}
			if resolved {
				storageKey = resolvedKey
			}
		}
		state.ID = existing.ID
		state.StorageKey = storageKey
		state.OCRStatus = existing.OCRStatus
		state.InvoiceNumber = existing.InvoiceNumber
		if state.ContractID == 0 {
			state.ContractID = existing.ContractID
		}
		if state.OCRStatus == "" || state.OCRStatus == OCRStatusNone {
			state.OCRStatus = OCRStatusPending
		}
		if _, err := s.repo.UpsertInvoicePDFState(ctx, state); err != nil {
			return err
		}
		result.Reused++
		return s.repo.InsertDuplicateLog(ctx, DuplicateLog{
			EventType:          DuplicateEventSameHash,
			SourceFileToken:    file.Token,
			ExistingContractID: existing.ContractID,
			TargetContractID:   state.ContractID,
			FileHash:           hash,
			SlotKey:            slotKey,
			Message:            "same invoice hash duplicate from feishu folder scan",
			MetadataJSON:       duplicateMetadata(file, rootToken, scanFile),
		})
	}

	storageKey, err := s.storePDFSnapshot(ctx, src, file, scanFile.RelativePath, filePath, hash)
	if err != nil {
		return err
	}
	state.StorageKey = storageKey

	if existing, ok, err := s.repo.FindActiveInvoiceBySlot(ctx, slotKey); err != nil {
		return err
	} else if ok {
		state.ID = existing.ID
		state.InvoiceNumber = existing.InvoiceNumber
		if state.ContractID == 0 {
			state.ContractID = existing.ContractID
		}
		invoiceID, err := s.repo.UpsertInvoicePDFState(ctx, state)
		if err != nil {
			return err
		}
		if err := s.repo.InsertDuplicateLog(ctx, DuplicateLog{
			EventType:          DuplicateEventSameSlotReplace,
			SourceFileToken:    file.Token,
			ExistingContractID: existing.ContractID,
			TargetContractID:   state.ContractID,
			FileHash:           hash,
			OldFileHash:        existing.FileHash,
			SlotKey:            slotKey,
			Message:            "same invoice slot replaced by feishu folder scan",
			MetadataJSON:       duplicateMetadata(file, rootToken, scanFile),
		}); err != nil {
			return err
		}
		if err := s.ocr.EnqueueOCR(ctx, invoiceID, state.StorageKey, hash); err != nil {
			return err
		}
		result.Replaced++
		result.OCRQueued++
		return nil
	}

	invoiceID, err := s.repo.UpsertInvoicePDFState(ctx, state)
	if err != nil {
		return err
	}
	if err := s.ocr.EnqueueOCR(ctx, invoiceID, state.StorageKey, hash); err != nil {
		return err
	}
	result.Created++
	result.OCRQueued++
	return nil
}

func downloadedFileSize(filePath string, fallback int64) int64 {
	info, err := os.Stat(filePath)
	if err == nil && !info.IsDir() && info.Size() > 0 {
		return info.Size()
	}
	return fallback
}

func (s *PDFScanner) storePDFSnapshot(ctx context.Context, src SyncSource, file feishu.DriveFile, relativePath, filePath, hash string) (string, error) {
	if s.store == nil {
		return filePath, nil
	}
	key := pdfObjectKey(src, file, relativePath)
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
			if uri, found, err := s.findExistingObjectByHash(ctx, src, hash); err != nil || found {
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
	if uri, exists, err := s.findExistingObjectByHash(ctx, src, hash); err != nil {
		return "", err
	} else if exists {
		return uri, nil
	}
	return s.store.PutFile(ctx, filePath, key, "application/pdf")
}

func (s *PDFScanner) resolvePDFStorageKey(ctx context.Context, src SyncSource, file feishu.DriveFile, relativePath, filePath, storageKey, hash string) (string, bool, error) {
	if resolvedKey, resolved, err := s.resolveExistingObject(ctx, storageKey, hash); err != nil || resolved {
		return resolvedKey, resolved, err
	}
	if resolvedKey, resolved, err := s.findExistingObjectByHash(ctx, src, hash); err != nil || resolved {
		return resolvedKey, resolved, err
	}
	if strings.TrimSpace(storageKey) == "" || !strings.HasPrefix(strings.TrimSpace(storageKey), "s3://") {
		uploadedKey, err := s.storePDFSnapshot(ctx, src, file, relativePath, filePath, hash)
		if err != nil {
			return "", false, err
		}
		return uploadedKey, true, nil
	}
	uploadedKey, err := s.storePDFSnapshot(ctx, src, file, relativePath, filePath, hash)
	if err != nil {
		return "", false, err
	}
	return uploadedKey, true, nil
}

func (s *PDFScanner) findExistingObjectByHash(ctx context.Context, src SyncSource, hash string) (string, bool, error) {
	if s.store == nil {
		return "", false, nil
	}
	finder, ok := s.store.(ObjectStoreHashFinder)
	if !ok {
		return "", false, nil
	}
	key, exists, err := finder.FindObjectBySHA256(ctx, sourceOSSPrefix(src, "OSS_CONTRACT_PREFIX", defaultContractOSSPrefix), hash)
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

func (s *PDFScanner) resolveExistingObject(ctx context.Context, storageKey, hash string) (string, bool, error) {
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" || s.store == nil {
		return "", false, nil
	}
	probe, ok := s.store.(ObjectStoreProbe)
	if !ok {
		return "", false, nil
	}
	remoteHash, exists, err := probe.ObjectSHA256(ctx, storageKey)
	if err != nil {
		return "", false, err
	}
	if !exists {
		return "", false, nil
	}
	if strings.TrimSpace(remoteHash) == "" {
		if uri := strings.TrimSpace(probe.ObjectURI(objectKeyFromStorageKey(storageKey))); uri != "" {
			return uri, true, nil
		}
		return storageKey, true, nil
	}
	if !sameContentHash(remoteHash, hash) {
		return "", false, nil
	}
	uri := strings.TrimSpace(probe.ObjectURI(objectKeyFromStorageKey(storageKey)))
	if uri == "" {
		return "", false, nil
	}
	return uri, true, nil
}

func safeObjectKeyPart(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, ":", "_")
	return value
}

func isPDFDriveFile(file feishu.DriveFile) bool {
	if strings.EqualFold(strings.TrimSpace(file.MimeType), "application/pdf") {
		return true
	}
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(file.Name)), ".pdf")
}

func isFolderDriveFile(file feishu.DriveFile) bool {
	fileType := strings.ToLower(strings.TrimSpace(file.Type))
	if fileType == "folder" || fileType == "dir" {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(file.MimeType)), "folder")
}

func duplicateMetadata(file feishu.DriveFile, rootToken string, scanFile pdfScanFile) string {
	data, err := json.Marshal(map[string]any{
		"source":        "feishu_active_scan",
		"root_token":    rootToken,
		"parent_token":  scanFile.ParentToken,
		"relative_path": scanFile.RelativePath,
		"folder_path":   scanFile.FolderPath,
		"document_kind": scanFile.DocumentKind,
		"relation_key":  scanFile.RelationKey,
		"file_token":    file.Token,
		"file_name":     file.Name,
		"revision":      file.Revision,
	})
	if err != nil {
		return ""
	}
	return string(data)
}

type scanMetadata struct {
	Scanned   int    `json:"scanned"`
	Skipped   int    `json:"skipped"`
	Created   int    `json:"created"`
	Reused    int    `json:"reused"`
	Replaced  int    `json:"replaced"`
	Deleted   int64  `json:"deleted"`
	OCRQueued int    `json:"ocr_queued"`
	OSSPrefix string `json:"oss_prefix,omitempty"`
}

func joinRelativePath(parentPath, name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	parentPath = cleanRelativePath(parentPath)
	if parentPath == "" {
		return cleanRelativePath(name)
	}
	if name == "" {
		return parentPath
	}
	return cleanRelativePath(path.Join(parentPath, name))
}

func cleanRelativePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+value), "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func classifyPDFDocumentKind(relativePath string) string {
	for _, segment := range splitRelativePath(relativePath) {
		if isInvoicePathSegment(segment) {
			return "invoice"
		}
	}
	return "contract"
}

func relationKeyForPath(relativePath, documentKind string) string {
	folderPath := cleanRelativePath(path.Dir(cleanRelativePath(relativePath)))
	if folderPath == "." {
		folderPath = ""
	}
	segments := splitRelativePath(folderPath)
	for len(segments) > 0 && isDocumentKindFolder(segments[len(segments)-1]) {
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 0 {
		if strings.EqualFold(documentKind, "contract") {
			base := strings.TrimSuffix(path.Base(cleanRelativePath(relativePath)), path.Ext(cleanRelativePath(relativePath)))
			return normalizeRelationKey(base)
		}
		return ""
	}
	return normalizeRelationKey(strings.Join(segments, "/"))
}

func splitRelativePath(value string) []string {
	value = cleanRelativePath(value)
	if value == "" {
		return nil
	}
	raw := strings.Split(value, "/")
	out := make([]string, 0, len(raw))
	for _, segment := range raw {
		segment = strings.TrimSpace(segment)
		if segment != "" && segment != "." {
			out = append(out, segment)
		}
	}
	return out
}

func isDocumentKindFolder(value string) bool {
	normalized := normalizeRelationSegment(value)
	return normalized == "合同" ||
		normalized == "合同文件" ||
		normalized == "合同扫描件" ||
		normalized == "contract" ||
		normalized == "contracts" ||
		isInvoicePathSegment(value)
}

func isInvoicePathSegment(value string) bool {
	normalized := normalizeRelationSegment(value)
	return strings.Contains(normalized, "发票") ||
		strings.Contains(normalized, "开票") ||
		normalized == "invoice" ||
		normalized == "invoices"
}

func normalizeRelationKey(value string) string {
	segments := splitRelativePath(value)
	normalized := make([]string, 0, len(segments))
	for _, segment := range segments {
		if item := normalizeRelationSegment(segment); item != "" {
			normalized = append(normalized, item)
		}
	}
	return strings.Join(normalized, "/")
}

func normalizeRelationSegment(value string) string {
	value = strings.TrimSpace(value)
	value = whitespacePattern.ReplaceAllString(value, " ")
	return strings.ToLower(value)
}
