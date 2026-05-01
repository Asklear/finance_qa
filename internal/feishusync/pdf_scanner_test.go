package feishusync_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/feishu"
	"financeqa/internal/feishusync"
)

func TestPDFScannerCreatesPendingOCRForNewPDF(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	ocr := &recordingOCRDispatcher{}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "file-a",
			Name:        "合同A.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
			Revision:    "rev-a",
		}},
		downloads: map[string][]byte{"file-a": []byte("contract-a")},
	}, repo, ocr, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Created != 1 || result.OCRQueued != 1 || result.Scanned != 1 {
		t.Fatalf("result = %#v, want created=1 ocr=1 scanned=1", result)
	}
	state, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-a:合同a.pdf")
	if err != nil {
		t.Fatalf("find active contract: %v", err)
	}
	if !ok {
		t.Fatalf("new pdf should create active contract")
	}
	if state.OCRStatus != feishusync.OCRStatusPending || state.FeishuFileToken != "file-a" {
		t.Fatalf("state = %#v", state)
	}
	if len(ocr.calls) != 1 || ocr.calls[0].contractID != state.ID || ocr.calls[0].fileHash != state.FileHash {
		t.Fatalf("ocr calls = %#v, state = %#v", ocr.calls, state)
	}
	if _, err := os.Stat(state.StorageKey); err != nil {
		t.Fatalf("storage key should point to downloaded file: %v", err)
	}
}

func TestPDFScannerReusesSameHashAndLogsDuplicate(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	hash := writeSnapshotAndHash(t, "contract-a")
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "旧合同.pdf",
		FileHash:          hash,
		StorageKey:        "/tmp/old.pdf",
		FeishuFileToken:   "old-file",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:旧合同.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	ocr := &recordingOCRDispatcher{}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "new-file",
			Name:        "新合同.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"new-file": []byte("contract-a")},
	}, repo, ocr, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Reused != 1 || result.Created != 0 || result.OCRQueued != 0 {
		t.Fatalf("result = %#v, want reused=1 created=0 ocr=0", result)
	}
	state, ok, err := repo.FindContractByFileHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("find by hash: %v", err)
	}
	if !ok || state.ID != existingID || state.FeishuFileToken != "new-file" || state.OCRStatus != feishusync.OCRStatusDone {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
	assertDuplicateLogCount(t, sqlDB, feishusync.DuplicateEventSameHash, 1)
	if len(ocr.calls) != 0 {
		t.Fatalf("same hash should not enqueue OCR: %#v", ocr.calls)
	}
}

func TestPDFScannerReplacesSameSlotWithNewHash(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "合同A.pdf",
		FileHash:          "old-hash",
		StorageKey:        "/tmp/old.pdf",
		FeishuFileToken:   "old-file",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:合同a.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	ocr := &recordingOCRDispatcher{}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "new-file",
			Name:        "合同A.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"new-file": []byte("contract-a-v2")},
	}, repo, ocr, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Replaced != 1 || result.OCRQueued != 1 || result.Created != 0 {
		t.Fatalf("result = %#v, want replaced=1 ocr=1 created=0", result)
	}
	state, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-a:合同a.pdf")
	if err != nil {
		t.Fatalf("find by slot: %v", err)
	}
	if !ok || state.ID != existingID || state.FeishuFileToken != "new-file" || state.FileHash == "old-hash" || state.OCRStatus != feishusync.OCRStatusPending {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
	assertDuplicateLogCount(t, sqlDB, feishusync.DuplicateEventSameSlotReplace, 1)
	if len(ocr.calls) != 1 || ocr.calls[0].contractID != existingID {
		t.Fatalf("ocr calls = %#v", ocr.calls)
	}
}

func TestPDFScannerMarksMissingPDFsDeleted(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	if _, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "已删除合同.pdf",
		FileHash:          "old-hash",
		StorageKey:        "/tmp/old.pdf",
		FeishuFileToken:   "missing-file",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:已删除合同.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	}); err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{}, repo, NoopOCRForTest{}, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("result = %#v, want deleted=1", result)
	}
	_, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-a:已删除合同.pdf")
	if err != nil {
		t.Fatalf("find by slot: %v", err)
	}
	if ok {
		t.Fatalf("missing pdf should be marked deleted")
	}
}

func TestPDFScannerSkipsNonPDF(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	client := &fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "text-file",
			Name:        "说明.txt",
			MimeType:    "text/plain",
			ParentToken: "folder-a",
		}},
		downloads: map[string][]byte{"text-file": []byte("ignore")},
	}
	scanner := feishusync.NewPDFScanner(client, repo, NoopOCRForTest{}, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Skipped != 1 || result.Scanned != 0 {
		t.Fatalf("result = %#v, want skipped=1 scanned=0", result)
	}
	if len(client.downloadedTokens) != 0 {
		t.Fatalf("non-pdf should not be downloaded: %#v", client.downloadedTokens)
	}
}

type fakeFeishuClient struct {
	files            []feishu.DriveFile
	downloads        map[string][]byte
	exported         map[string][]byte
	downloadedTokens []string
}

func (c *fakeFeishuClient) ListFolderFiles(_ context.Context, _ string) ([]feishu.DriveFile, error) {
	return c.files, nil
}

func (c *fakeFeishuClient) GetFileMetadata(_ context.Context, fileToken string) (feishu.DriveFile, error) {
	for _, file := range c.files {
		if file.Token == fileToken {
			return file, nil
		}
	}
	return feishu.DriveFile{}, nil
}

func (c *fakeFeishuClient) DownloadFile(_ context.Context, fileToken, destPath string) error {
	c.downloadedTokens = append(c.downloadedTokens, fileToken)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, c.downloads[fileToken], 0o644)
}

func (c *fakeFeishuClient) ExportToXLSX(_ context.Context, fileToken, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, c.exported[fileToken], 0o644)
}

type recordingOCRDispatcher struct {
	calls []ocrCall
}

type ocrCall struct {
	contractID int64
	filePath   string
	fileHash   string
}

func (d *recordingOCRDispatcher) EnqueueOCR(_ context.Context, contractID int64, filePath string, fileHash string) error {
	d.calls = append(d.calls, ocrCall{contractID: contractID, filePath: filePath, fileHash: fileHash})
	return nil
}

type NoopOCRForTest struct{}

func (NoopOCRForTest) EnqueueOCR(context.Context, int64, string, string) error {
	return nil
}

func mustSeedPDFSource(t *testing.T, repo *feishusync.Repository, folderToken string) feishusync.SyncSource {
	t.Helper()

	src := feishusync.SyncSource{
		SourceType:  feishusync.SourceTypePDFFolder,
		SourceToken: folderToken,
		DisplayName: "PDF Folder",
		SyncStatus:  feishusync.SyncStatusActive,
	}
	if err := repo.UpsertSource(context.Background(), src); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	sources, err := repo.ListSources(context.Background(), feishusync.SourceFilter{SourceType: feishusync.SourceTypePDFFolder})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("source count = %d, want 1", len(sources))
	}
	return sources[0]
}

func writeSnapshotAndHash(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "snapshot.pdf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	hash, err := feishusync.FileSHA256(path)
	if err != nil {
		t.Fatalf("hash snapshot: %v", err)
	}
	return hash
}

func assertDuplicateLogCount(t *testing.T, db *sql.DB, eventType string, want int) {
	t.Helper()

	var got int
	if err := db.QueryRow(`SELECT COUNT(1) FROM contract_duplicate_logs WHERE event_type = ?`, eventType).Scan(&got); err != nil {
		t.Fatalf("count duplicate logs: %v", err)
	}
	if got != want {
		t.Fatalf("duplicate log count for %s = %d, want %d", eventType, got, want)
	}
}
