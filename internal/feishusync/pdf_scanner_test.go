package feishusync_test

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
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
			Size:        0,
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
	if state.FileSize != int64(len("contract-a")) {
		t.Fatalf("file_size = %d, want downloaded size %d", state.FileSize, len("contract-a"))
	}
	if len(ocr.calls) != 1 || ocr.calls[0].documentID != state.ID || ocr.calls[0].fileHash != state.FileHash {
		t.Fatalf("ocr calls = %#v, state = %#v", ocr.calls, state)
	}
	if _, err := os.Stat(state.StorageKey); err != nil {
		t.Fatalf("storage key should point to downloaded file: %v", err)
	}
}

func TestPDFScannerSkipsDownloadWhenContractMetadataUnchanged(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	hash := writeSnapshotAndHash(t, "contract-a")
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:           "合同A.pdf",
		FileHash:           hash,
		StorageKey:         "tenant/uhub/contract/合同A.pdf",
		FeishuFileToken:    "file-a",
		FeishuRootToken:    "folder-a",
		FeishuParentToken:  "folder-a",
		FeishuRelativePath: "合同A.pdf",
		FeishuFolderPath:   "",
		FeishuSlotKey:      "folder-a:合同a.pdf",
		RelationKey:        "合同a",
		FileSize:           int64(len("contract-a")),
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	if _, err := sqlDB.Exec(`UPDATE contract_main SET feishu_modified_time = ? WHERE id = ?`, "1714972020", existingID); err != nil {
		t.Fatalf("seed modified time: %v", err)
	}
	client := &fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:        "file-a",
			Name:         "合同A.pdf",
			MimeType:     "application/pdf",
			ParentToken:  "folder-a",
			ModifiedTime: "1714972020",
		}},
		downloads: map[string][]byte{"file-a": []byte("contract-a")},
	}
	ocr := &recordingOCRDispatcher{}
	scanner := feishusync.NewPDFScanner(client, repo, ocr, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan unchanged pdf: %v", err)
	}
	if result.Reused != 1 || result.Scanned != 1 || result.Created != 0 || result.OCRQueued != 0 {
		t.Fatalf("result=%#v, want reused=1 scanned=1 created=0 ocr=0", result)
	}
	if len(client.downloadedTokens) != 0 {
		t.Fatalf("unchanged PDF should not be downloaded: %#v", client.downloadedTokens)
	}
	if len(ocr.calls) != 0 {
		t.Fatalf("unchanged PDF should not enqueue OCR: %#v", ocr.calls)
	}
	state, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-a:合同a.pdf")
	if err != nil {
		t.Fatalf("find active contract: %v", err)
	}
	if !ok || state.ID != existingID || state.FileHash != hash || state.OCRStatus != feishusync.OCRStatusDone {
		t.Fatalf("state=%#v ok=%v", state, ok)
	}
}

func TestPDFScannerDownloadsWhenContractMetadataUnchangedButStoredArtifactMissing(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	hash := writeSnapshotAndHash(t, "contract-a")
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:           "合同A.pdf",
		FileHash:           hash,
		FeishuFileToken:    "file-a",
		FeishuRootToken:    "folder-a",
		FeishuParentToken:  "folder-a",
		FeishuRelativePath: "合同A.pdf",
		FeishuFolderPath:   "",
		FeishuSlotKey:      "folder-a:合同a.pdf",
		RelationKey:        "合同a",
		FeishuModifiedTime: "1714972020",
		FileSize:           int64(len("contract-a")),
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	client := &fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:        "file-a",
			Name:         "合同A.pdf",
			MimeType:     "application/pdf",
			ParentToken:  "folder-a",
			ModifiedTime: "1714972020",
		}},
		downloads: map[string][]byte{"file-a": []byte("contract-a")},
	}
	ocr := &recordingOCRDispatcher{}
	scanner := feishusync.NewPDFScanner(client, repo, ocr, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan unchanged pdf with missing storage: %v", err)
	}
	if result.Reused != 1 || result.Created != 0 || result.OCRQueued != 0 {
		t.Fatalf("result=%#v, want reused=1 created=0 ocr=0", result)
	}
	if len(client.downloadedTokens) != 1 || client.downloadedTokens[0] != "file-a" {
		t.Fatalf("missing stored artifact should force download: %#v", client.downloadedTokens)
	}
	state, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-a:合同a.pdf")
	if err != nil {
		t.Fatalf("find active contract: %v", err)
	}
	if !ok || state.ID != existingID || state.FileHash != hash || state.OCRStatus != feishusync.OCRStatusDone {
		t.Fatalf("state=%#v ok=%v", state, ok)
	}
}

func TestPDFScannerDownloadsWhenInvoiceMetadataUnchangedButStoredArtifactMissing(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-root")
	contractID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:           "服务合同.pdf",
		FileHash:           writeSnapshotAndHash(t, "contract-a"),
		StorageKey:         "tenant/uhub/contract/客户A/服务合同.pdf",
		FeishuFileToken:    "contract-file",
		FeishuRootToken:    "folder-root",
		FeishuParentToken:  "folder-customer",
		FeishuRelativePath: "客户A/服务合同.pdf",
		FeishuFolderPath:   "客户A",
		FeishuSlotKey:      "folder-root:客户a/服务合同.pdf",
		RelationKey:        "客户a",
		FeishuModifiedTime: "1714972000",
		FileSize:           int64(len("contract-a")),
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed contract: %v", err)
	}
	invoiceHash := writeSnapshotAndHash(t, "invoice-a")
	invoiceID, err := repo.UpsertInvoicePDFState(context.Background(), feishusync.InvoicePDFState{
		ContractID:         contractID,
		InvoiceNumber:      "pending:invoice",
		FileName:           "2026-03发票.pdf",
		FileHash:           invoiceHash,
		FeishuFileToken:    "invoice-file",
		FeishuRootToken:    "folder-root",
		FeishuParentToken:  "folder-invoices",
		FeishuRelativePath: "客户A/发票/2026-03发票.pdf",
		FeishuFolderPath:   "客户A/发票",
		FeishuSlotKey:      "folder-root:客户a/发票/2026-03发票.pdf",
		RelationKey:        "客户a",
		FeishuModifiedTime: "1714972020",
		FileSize:           int64(len("invoice-a")),
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	client := &fakeFeishuClient{
		filesByFolder: map[string][]feishu.DriveFile{
			"folder-root": {{
				Token: "folder-customer",
				Name:  "客户A",
				Type:  "folder",
			}},
			"folder-customer": {{
				Token: "folder-invoices",
				Name:  "发票",
				Type:  "folder",
			}},
			"folder-invoices": {{
				Token:        "invoice-file",
				Name:         "2026-03发票.pdf",
				MimeType:     "application/pdf",
				ParentToken:  "folder-invoices",
				ModifiedTime: "1714972020",
			}},
		},
		downloads: map[string][]byte{"invoice-file": []byte("invoice-a")},
	}
	ocr := &recordingOCRDispatcher{}
	scanner := feishusync.NewPDFScanner(client, repo, ocr, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan unchanged invoice with missing storage: %v", err)
	}
	if result.Reused != 1 || result.Created != 0 || result.OCRQueued != 0 {
		t.Fatalf("result=%#v, want reused=1 created=0 ocr=0", result)
	}
	if len(client.downloadedTokens) != 1 || client.downloadedTokens[0] != "invoice-file" {
		t.Fatalf("missing stored invoice artifact should force download: %#v", client.downloadedTokens)
	}
	gotID := assertFeishuInvoicePDFMetadata(t, sqlDB, "invoice-file", feishuPDFMetadata{
		RootToken:    "folder-root",
		ParentToken:  "folder-invoices",
		RelativePath: "客户A/发票/2026-03发票.pdf",
		FolderPath:   "客户A/发票",
		RelationKey:  "客户a",
		LinkedID:     contractID,
	})
	if gotID != invoiceID {
		t.Fatalf("invoice id = %d, want existing %d", gotID, invoiceID)
	}
}

func TestPDFScannerUpdatesContractPathWithoutDownloadWhenTokenAndModifiedTimeUnchanged(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-root")
	hash := writeSnapshotAndHash(t, "contract-a")
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:           "服务合同.pdf",
		FileHash:           hash,
		StorageKey:         "tenant/uhub/contract/客户A/服务合同.pdf",
		FeishuFileToken:    "file-a",
		FeishuRootToken:    "folder-root",
		FeishuParentToken:  "folder-old",
		FeishuRelativePath: "客户A/服务合同.pdf",
		FeishuFolderPath:   "客户A",
		FeishuSlotKey:      "folder-root:客户a/服务合同.pdf",
		RelationKey:        "客户a",
		FeishuModifiedTime: "1714972020",
		FileSize:           int64(len("contract-a")),
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	client := &fakeFeishuClient{
		filesByFolder: map[string][]feishu.DriveFile{
			"folder-root": {{
				Token: "folder-new",
				Name:  "客户B",
				Type:  "folder",
			}},
			"folder-new": {{
				Token:        "file-a",
				Name:         "服务合同.pdf",
				MimeType:     "application/pdf",
				ParentToken:  "folder-new",
				ModifiedTime: "1714972020",
			}},
		},
		downloads: map[string][]byte{"file-a": []byte("contract-a")},
	}
	scanner := feishusync.NewPDFScanner(client, repo, &recordingOCRDispatcher{}, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan moved pdf: %v", err)
	}
	if result.Reused != 1 || result.Created != 0 || result.OCRQueued != 0 {
		t.Fatalf("result=%#v, want reused=1 created=0 ocr=0", result)
	}
	if len(client.downloadedTokens) != 0 {
		t.Fatalf("moved unchanged PDF should not be downloaded: %#v", client.downloadedTokens)
	}
	state, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-root:客户b/服务合同.pdf")
	if err != nil {
		t.Fatalf("find moved contract: %v", err)
	}
	if !ok || state.ID != existingID || state.FeishuRelativePath != "客户B/服务合同.pdf" || state.RelationKey != "客户b" || state.OCRStatus != feishusync.OCRStatusDone {
		t.Fatalf("state=%#v ok=%v", state, ok)
	}
}

func TestPDFScannerForceDownloadsWhenMetadataUnchanged(t *testing.T) {
	t.Setenv("FINANCEQA_FEISHU_FORCE_DOWNLOAD", "1")

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	hash := writeSnapshotAndHash(t, "contract-a")
	if _, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:           "合同A.pdf",
		FileHash:           hash,
		StorageKey:         "tenant/uhub/contract/合同A.pdf",
		FeishuFileToken:    "file-a",
		FeishuRootToken:    "folder-a",
		FeishuParentToken:  "folder-a",
		FeishuRelativePath: "合同A.pdf",
		FeishuFolderPath:   "",
		FeishuSlotKey:      "folder-a:合同a.pdf",
		RelationKey:        "合同a",
		FeishuModifiedTime: "1714972020",
		FileSize:           int64(len("contract-a")),
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusDone,
	}); err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	client := &fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:        "file-a",
			Name:         "合同A.pdf",
			MimeType:     "application/pdf",
			ParentToken:  "folder-a",
			ModifiedTime: "1714972020",
		}},
		downloads: map[string][]byte{"file-a": []byte("contract-a")},
	}
	scanner := feishusync.NewPDFScanner(client, repo, &recordingOCRDispatcher{}, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan forced pdf: %v", err)
	}
	if result.Reused != 1 || len(client.downloadedTokens) != 1 || client.downloadedTokens[0] != "file-a" {
		t.Fatalf("result=%#v downloaded=%#v, want forced download", result, client.downloadedTokens)
	}
}

func TestPDFScannerMetadataShortcutDisabledDownloadsWhenMetadataUnchanged(t *testing.T) {
	t.Setenv("FINANCEQA_FEISHU_METADATA_SHORTCUT", "0")

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-a")
	hash := writeSnapshotAndHash(t, "contract-a")
	if _, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:           "合同A.pdf",
		FileHash:           hash,
		StorageKey:         "tenant/uhub/contract/合同A.pdf",
		FeishuFileToken:    "file-a",
		FeishuRootToken:    "folder-a",
		FeishuParentToken:  "folder-a",
		FeishuRelativePath: "合同A.pdf",
		FeishuFolderPath:   "",
		FeishuSlotKey:      "folder-a:合同a.pdf",
		RelationKey:        "合同a",
		FeishuModifiedTime: "1714972020",
		FileSize:           int64(len("contract-a")),
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusDone,
	}); err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	client := &fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:        "file-a",
			Name:         "合同A.pdf",
			MimeType:     "application/pdf",
			ParentToken:  "folder-a",
			ModifiedTime: "1714972020",
		}},
		downloads: map[string][]byte{"file-a": []byte("contract-a")},
	}
	scanner := feishusync.NewPDFScanner(client, repo, &recordingOCRDispatcher{}, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan metadata-disabled pdf: %v", err)
	}
	if result.Reused != 1 || len(client.downloadedTokens) != 1 || client.downloadedTokens[0] != "file-a" {
		t.Fatalf("result=%#v downloaded=%#v, want shortcut disabled download", result, client.downloadedTokens)
	}
}

func TestPDFScannerRecursivelyScansNestedPDFsAndLinksByFolderRelation(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-root", `{"oss_prefix":"tenant/uhub/contract"}`)
	store := &recordingObjectStore{}
	ocr := &recordingOCRDispatcher{}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		filesByFolder: map[string][]feishu.DriveFile{
			"folder-root": {{
				Token: "folder-customer",
				Name:  "客户A",
				Type:  "folder",
			}},
			"folder-customer": {{
				Token:       "contract-file",
				Name:        "服务合同.pdf",
				MimeType:    "application/pdf",
				ParentToken: "folder-customer",
				Size:        0,
				Revision:    "rev-contract",
			}, {
				Token: "folder-invoices",
				Name:  "发票",
				Type:  "folder",
			}},
			"folder-invoices": {{
				Token:       "invoice-file",
				Name:        "2026-03发票.pdf",
				MimeType:    "application/pdf",
				ParentToken: "folder-invoices",
				Size:        0,
				Revision:    "rev-invoice",
			}},
		},
		downloads: map[string][]byte{
			"contract-file": []byte("contract-a"),
			"invoice-file":  []byte("invoice-a"),
		},
	}, repo, ocr, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Created != 2 || result.OCRQueued != 2 || result.Scanned != 2 {
		t.Fatalf("result = %#v, want created=2 ocr=2 scanned=2", result)
	}
	assertObjectKeys(t, store.puts,
		"tenant/uhub/contract/客户A/服务合同.pdf",
		"tenant/uhub/contract/客户A/发票/2026-03发票.pdf",
	)

	contractID := assertFeishuPDFMetadata(t, sqlDB, "contract-file", feishuPDFMetadata{
		RootToken:    "folder-root",
		ParentToken:  "folder-customer",
		RelativePath: "客户A/服务合同.pdf",
		FolderPath:   "客户A",
		RelationKey:  "客户a",
		LinkedID:     0,
	})
	assertPDFFileSize(t, sqlDB, "contract_main", "contract-file", int64(len("contract-a")))
	assertFeishuInvoicePDFMetadata(t, sqlDB, "invoice-file", feishuPDFMetadata{
		RootToken:    "folder-root",
		ParentToken:  "folder-invoices",
		RelativePath: "客户A/发票/2026-03发票.pdf",
		FolderPath:   "客户A/发票",
		RelationKey:  "客户a",
		LinkedID:     contractID,
	})
	assertPDFFileSize(t, sqlDB, "contract_invoices", "invoice-file", int64(len("invoice-a")))
	var invoiceContractMainRows int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM contract_main WHERE feishu_file_token = ?`, "invoice-file").Scan(&invoiceContractMainRows); err != nil {
		t.Fatalf("count invoice contract_main rows: %v", err)
	}
	if invoiceContractMainRows != 0 {
		t.Fatalf("invoice PDFs should not be stored in contract_main, got %d rows", invoiceContractMainRows)
	}
}

func TestPDFScannerLinksInvoiceArrivingAfterContract(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-root")
	client := &fakeFeishuClient{
		filesByFolder: map[string][]feishu.DriveFile{
			"folder-root": {{
				Token: "folder-customer",
				Name:  "客户A",
				Type:  "folder",
			}},
			"folder-customer": {{
				Token:       "contract-file",
				Name:        "服务合同.pdf",
				MimeType:    "application/pdf",
				ParentToken: "folder-customer",
			}},
		},
		downloads: map[string][]byte{"contract-file": []byte("contract-a")},
	}
	scanner := feishusync.NewPDFScanner(client, repo, &recordingOCRDispatcher{}, t.TempDir())

	if _, err := scanner.ScanFolder(context.Background(), src); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	contractID := assertFeishuPDFMetadata(t, sqlDB, "contract-file", feishuPDFMetadata{
		RootToken:    "folder-root",
		ParentToken:  "folder-customer",
		RelativePath: "客户A/服务合同.pdf",
		FolderPath:   "客户A",
		RelationKey:  "客户a",
	})

	client.filesByFolder["folder-customer"] = append(client.filesByFolder["folder-customer"], feishu.DriveFile{
		Token: "folder-invoices",
		Name:  "发票",
		Type:  "folder",
	})
	client.filesByFolder["folder-invoices"] = []feishu.DriveFile{{
		Token:       "invoice-file",
		Name:        "2026-03发票.pdf",
		MimeType:    "application/pdf",
		ParentToken: "folder-invoices",
	}}
	client.downloads["invoice-file"] = []byte("invoice-a")

	if _, err := scanner.ScanFolder(context.Background(), src); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	assertFeishuInvoicePDFMetadata(t, sqlDB, "invoice-file", feishuPDFMetadata{
		RootToken:    "folder-root",
		ParentToken:  "folder-invoices",
		RelativePath: "客户A/发票/2026-03发票.pdf",
		FolderPath:   "客户A/发票",
		RelationKey:  "客户a",
		LinkedID:     contractID,
	})
}

func TestPDFScannerStoresOSSKeyWhenObjectStoreConfigured(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract/优集客户合同"}`)
	store := &recordingObjectStore{uri: "tenant/uhub/contract/优集客户合同/合同A.pdf"}
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
	}, repo, ocr, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Created != 1 || len(store.puts) != 1 {
		t.Fatalf("result=%#v puts=%#v", result, store.puts)
	}
	state, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-a:合同a.pdf")
	if err != nil {
		t.Fatalf("find active contract: %v", err)
	}
	if !ok {
		t.Fatalf("new pdf should create active contract")
	}
	if state.StorageKey != store.uri {
		t.Fatalf("storage key = %q, want %q", state.StorageKey, store.uri)
	}
	if len(ocr.calls) != 1 || ocr.calls[0].filePath != store.uri {
		t.Fatalf("ocr calls = %#v", ocr.calls)
	}
	if store.puts[0].key != "tenant/uhub/contract/优集客户合同/合同A.pdf" {
		t.Fatalf("object key should follow historical OSS path: %#v", store.puts[0])
	}
}

func TestPDFScannerReusesSameHashWithoutUploadingDuplicate(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract/优集客户合同"}`)
	hash := writeSnapshotAndHash(t, "contract-a")
	existingStorageKey := "tenant/uhub/contract/优集客户合同/旧合同.pdf"
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "旧合同.pdf",
		FileHash:          hash,
		StorageKey:        existingStorageKey,
		FeishuFileToken:   "old-file",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:旧合同.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	store := &recordingObjectStore{uri: "tenant/uhub/contract/优集客户合同/新合同.pdf"}
	store.hashes = map[string]string{existingStorageKey: hash}
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
	}, repo, ocr, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Reused != 1 || result.Created != 0 || result.OCRQueued != 0 {
		t.Fatalf("result = %#v, want reused=1 created=0 ocr=0", result)
	}
	if len(store.puts) != 0 {
		t.Fatalf("same hash should not upload duplicate object: %#v", store.puts)
	}
	state, ok, err := repo.FindContractByFileHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("find by hash: %v", err)
	}
	if !ok || state.ID != existingID || state.StorageKey != existingStorageKey {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
}

func TestPDFScannerReusesExistingOSSKeyWhenRemoteHashMetadataMissing(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract/优集客户合同"}`)
	hash := writeSnapshotAndHash(t, "contract-a")
	existingStorageKey := "tenant/uhub/contract/优集客户合同/旧合同.pdf"
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "旧合同.pdf",
		FileHash:          hash,
		StorageKey:        existingStorageKey,
		FeishuFileToken:   "old-file",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:旧合同.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	store := &recordingObjectStore{existingWithoutHash: map[string]bool{existingStorageKey: true}}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "new-file",
			Name:        "新合同.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"new-file": []byte("contract-a")},
	}, repo, &recordingOCRDispatcher{}, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Reused != 1 || result.Created != 0 || result.OCRQueued != 0 {
		t.Fatalf("result = %#v, want reused=1 created=0 ocr=0", result)
	}
	if len(store.hashCalls) != 1 || store.hashCalls[0] != existingStorageKey || len(store.puts) != 0 {
		t.Fatalf("same hash with missing OSS metadata should only HEAD existing key: hashCalls=%#v puts=%#v", store.hashCalls, store.puts)
	}
	state, ok, err := repo.FindContractByFileHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("find by hash: %v", err)
	}
	if !ok || state.ID != existingID || state.StorageKey != existingStorageKey {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
}

func TestPDFScannerReusesLegacyMD5RowAndUpgradesToSHA256(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract"}`)
	content := []byte("contract-a")
	md5Bytes := md5.Sum(content)
	legacyMD5 := hex.EncodeToString(md5Bytes[:])
	sha256Hash := writeSnapshotAndHash(t, string(content))
	existingStorageKey := "tenant/uhub/contract/客户A/合同A.pdf"
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:   "合同A.pdf",
		FileHash:   legacyMD5,
		StorageKey: existingStorageKey,
		SyncStatus: feishusync.SyncStatusActive,
		OCRStatus:  feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	store := &recordingObjectStore{
		hashes: map[string]string{existingStorageKey: sha256Hash},
	}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "file-a",
			Name:        "合同A.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"file-a": content},
	}, repo, &recordingOCRDispatcher{}, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Reused != 1 || result.Created != 0 || result.OCRQueued != 0 || len(store.puts) != 0 {
		t.Fatalf("result=%#v puts=%#v", result, store.puts)
	}
	state, ok, err := repo.FindContractByFileHash(context.Background(), sha256Hash)
	if err != nil {
		t.Fatalf("find by sha256: %v", err)
	}
	if !ok || state.ID != existingID || state.FileHash != sha256Hash || state.FeishuFileToken != "file-a" {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
	if _, ok, err := repo.FindContractByFileHash(context.Background(), legacyMD5); err != nil {
		t.Fatalf("find by legacy md5: %v", err)
	} else if ok {
		t.Fatalf("legacy md5 should be upgraded away from primary file_hash")
	}
}

func TestPDFScannerMergesExistingSHA256DuplicateIntoLegacyOCRRow(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract"}`)
	content := []byte("contract-a")
	md5Bytes := md5.Sum(content)
	legacyMD5 := hex.EncodeToString(md5Bytes[:])
	sha256Hash := writeSnapshotAndHash(t, string(content))
	storageKey := "tenant/uhub/contract/客户A/合同A.pdf"

	legacyID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:   "合同A.pdf",
		FileHash:   legacyMD5,
		StorageKey: storageKey,
		SyncStatus: feishusync.SyncStatusActive,
		OCRStatus:  feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	duplicateID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "合同A.pdf",
		FileHash:          sha256Hash,
		StorageKey:        storageKey,
		FeishuFileToken:   "file-a",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:合同a.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusPending,
	})
	if err != nil {
		t.Fatalf("seed duplicate row: %v", err)
	}
	if duplicateID == legacyID {
		t.Fatalf("fixture should create two rows")
	}

	store := &recordingObjectStore{
		hashes: map[string]string{storageKey: sha256Hash},
	}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "file-a",
			Name:        "合同A.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"file-a": content},
	}, repo, &recordingOCRDispatcher{}, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Reused != 1 || result.Created != 0 || result.OCRQueued != 0 || len(store.puts) != 0 {
		t.Fatalf("result=%#v puts=%#v", result, store.puts)
	}
	state, ok, err := repo.FindContractByFileHash(context.Background(), sha256Hash)
	if err != nil {
		t.Fatalf("find by sha256: %v", err)
	}
	if !ok || state.ID != legacyID || state.FileHash != sha256Hash || state.FeishuFileToken != "file-a" || state.OCRStatus != feishusync.OCRStatusDone {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
	var duplicateCount int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM contract_main WHERE id = ?`, duplicateID).Scan(&duplicateCount); err != nil {
		t.Fatalf("count duplicate row: %v", err)
	}
	if duplicateCount != 0 {
		t.Fatalf("duplicate row %d should be deleted", duplicateID)
	}
}

func TestPDFScannerNormalizesHistoricalOSSKeyWithoutUploading(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract/优集客户合同"}`)
	hash := writeSnapshotAndHash(t, "contract-a")
	existingStorageKey := "tenant/uhub/contract/优集客户合同/旧合同.pdf"
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "旧合同.pdf",
		FileHash:          hash,
		StorageKey:        existingStorageKey,
		FeishuFileToken:   "old-file",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:旧合同.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	store := &recordingObjectStore{
		uri:    "tenant/uhub/contract/优集客户合同/新合同.pdf",
		hashes: map[string]string{existingStorageKey: hash},
	}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "new-file",
			Name:        "新合同.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"new-file": []byte("contract-a")},
	}, repo, &recordingOCRDispatcher{}, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Reused != 1 || len(store.puts) != 0 {
		t.Fatalf("result=%#v puts=%#v", result, store.puts)
	}
	state, ok, err := repo.FindContractByFileHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("find by hash: %v", err)
	}
	if !ok || state.ID != existingID || state.StorageKey != existingStorageKey {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
}

func TestPDFScannerFindsExistingOSSObjectByHashBeforeUploading(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract"}`)
	hash := writeSnapshotAndHash(t, "contract-a")
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "合同A.pdf",
		FileHash:          hash,
		StorageKey:        "tmp/feishu-snapshots/folder-a/new-file.pdf",
		FeishuFileToken:   "old-file",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:合同a.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	existingOSSKey := "tenant/uhub/contract/优集客户合同/合同A.pdf"
	store := &recordingObjectStore{
		hashKeys: map[string]string{hash: existingOSSKey},
	}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "new-file",
			Name:        "合同A.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"new-file": []byte("contract-a")},
	}, repo, &recordingOCRDispatcher{}, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Reused != 1 || len(store.puts) != 0 {
		t.Fatalf("result=%#v puts=%#v", result, store.puts)
	}
	state, ok, err := repo.FindContractByFileHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("find by hash: %v", err)
	}
	if !ok || state.ID != existingID || state.StorageKey != existingOSSKey {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
}

func TestPDFScannerAvoidsOverwritingOccupiedOSSPathWithDifferentHash(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract"}`)
	hash := writeSnapshotAndHash(t, "contract-v2")
	targetKey := "tenant/uhub/contract/合同A.pdf"
	store := &recordingObjectStore{
		hashes: map[string]string{targetKey: "existing-different-hash"},
	}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "file-a",
			Name:        "合同A.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"file-a": []byte("contract-v2")},
	}, repo, &recordingOCRDispatcher{}, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	wantKey := "tenant/uhub/contract/合同A.sha256-" + hash[:12] + ".pdf"
	if result.Created != 1 || len(store.puts) != 1 || store.puts[0].key != wantKey {
		t.Fatalf("result=%#v puts=%#v want key %q", result, store.puts, wantKey)
	}
	state, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-a:合同a.pdf")
	if err != nil {
		t.Fatalf("find active contract: %v", err)
	}
	if !ok || state.StorageKey != wantKey {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
}

func TestPDFScannerRepairsMismatchedS3StorageKeyForSameHash(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSourceWithMetadata(t, repo, "folder-a", `{"oss_prefix":"tenant/uhub/contract"}`)
	hash := writeSnapshotAndHash(t, "contract-a")
	existingID, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:          "合同A.pdf",
		FileHash:          hash,
		StorageKey:        "tenant/uhub/contract/合同A.pdf",
		FeishuFileToken:   "old-file",
		FeishuParentToken: "folder-a",
		FeishuSlotKey:     "folder-a:合同a.pdf",
		SyncStatus:        feishusync.SyncStatusActive,
		OCRStatus:         feishusync.OCRStatusDone,
	})
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	existingOSSKey := "tenant/uhub/contract/优集客户合同/合同A.pdf"
	store := &recordingObjectStore{
		hashes: map[string]string{
			"tenant/uhub/contract/合同A.pdf": "existing-different-hash",
		},
		hashKeys: map[string]string{hash: existingOSSKey},
	}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		files: []feishu.DriveFile{{
			Token:       "new-file",
			Name:        "合同A.pdf",
			MimeType:    "application/pdf",
			ParentToken: "folder-a",
			Size:        12,
		}},
		downloads: map[string][]byte{"new-file": []byte("contract-a")},
	}, repo, &recordingOCRDispatcher{}, t.TempDir(), store)

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Reused != 1 || len(store.puts) != 0 {
		t.Fatalf("result=%#v puts=%#v", result, store.puts)
	}
	state, ok, err := repo.FindContractByFileHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("find by hash: %v", err)
	}
	if !ok || state.ID != existingID || state.StorageKey != existingOSSKey {
		t.Fatalf("state = %#v ok=%v", state, ok)
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
	if len(ocr.calls) != 1 || ocr.calls[0].documentID != existingID {
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

func TestPDFScannerMarksMissingNestedPDFsDeleted(t *testing.T) {
	t.Parallel()

	sqlDB := openFeishuSyncTestDB(t)
	createContractPDFStateFixture(t, sqlDB)
	repo := feishusync.NewRepository(sqlDB)
	src := mustSeedPDFSource(t, repo, "folder-root")
	if _, err := repo.UpsertContractPDFState(context.Background(), feishusync.ContractPDFState{
		FileName:           "已删除合同.pdf",
		FileHash:           "old-hash",
		StorageKey:         "/tmp/old.pdf",
		FeishuFileToken:    "missing-file",
		FeishuRootToken:    "folder-root",
		FeishuParentToken:  "folder-customer",
		FeishuRelativePath: "客户A/已删除合同.pdf",
		FeishuFolderPath:   "客户A",
		FeishuSlotKey:      "folder-root:客户a/已删除合同.pdf",
		RelationKey:        "客户a",
		SyncStatus:         feishusync.SyncStatusActive,
		OCRStatus:          feishusync.OCRStatusDone,
	}); err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	scanner := feishusync.NewPDFScanner(&fakeFeishuClient{
		filesByFolder: map[string][]feishu.DriveFile{
			"folder-root": {{
				Token: "folder-customer",
				Name:  "客户A",
				Type:  "folder",
			}},
		},
	}, repo, NoopOCRForTest{}, t.TempDir())

	result, err := scanner.ScanFolder(context.Background(), src)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("result = %#v, want deleted=1", result)
	}
	if _, ok, err := repo.FindActiveContractBySlot(context.Background(), "folder-root:客户a/已删除合同.pdf"); err != nil {
		t.Fatalf("find by slot: %v", err)
	} else if ok {
		t.Fatalf("missing nested pdf should be marked deleted")
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
	filesByFolder    map[string][]feishu.DriveFile
	downloads        map[string][]byte
	exported         map[string][]byte
	downloadedTokens []string
	exportedTokens   []string
}

func (c *fakeFeishuClient) ListFolderFiles(_ context.Context, folderToken string) ([]feishu.DriveFile, error) {
	if c.filesByFolder != nil {
		return c.filesByFolder[folderToken], nil
	}
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
	c.exportedTokens = append(c.exportedTokens, fileToken)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, c.exported[fileToken], 0o644)
}

type recordingOCRDispatcher struct {
	calls []ocrCall
}

type ocrCall struct {
	documentID int64
	filePath   string
	fileHash   string
}

func (d *recordingOCRDispatcher) EnqueueOCR(_ context.Context, documentID int64, filePath string, fileHash string) error {
	d.calls = append(d.calls, ocrCall{documentID: documentID, filePath: filePath, fileHash: fileHash})
	return nil
}

type NoopOCRForTest struct{}

func (NoopOCRForTest) EnqueueOCR(context.Context, int64, string, string) error {
	return nil
}

type recordingObjectStore struct {
	uri                 string
	hashes              map[string]string
	hashKeys            map[string]string
	existingWithoutHash map[string]bool
	puts                []objectStorePut
	hashCalls           []string
}

type objectStorePut struct {
	localPath   string
	key         string
	contentType string
}

func (s *recordingObjectStore) PutFile(_ context.Context, localPath, key, contentType string) (string, error) {
	s.puts = append(s.puts, objectStorePut{localPath: localPath, key: key, contentType: contentType})
	if s.uri == "" {
		return key, nil
	}
	return s.uri, nil
}

func (s *recordingObjectStore) ObjectSHA256(_ context.Context, key string) (string, bool, error) {
	s.hashCalls = append(s.hashCalls, key)
	if s.existingWithoutHash != nil && s.existingWithoutHash[key] {
		return "", true, nil
	}
	if s.hashes == nil {
		return "", false, nil
	}
	hash, ok := s.hashes[key]
	return hash, ok, nil
}

func (s *recordingObjectStore) ObjectURI(key string) string {
	return key
}

func (s *recordingObjectStore) FindObjectBySHA256(_ context.Context, _ string, hash string) (string, bool, error) {
	if s.hashKeys == nil {
		return "", false, nil
	}
	key, ok := s.hashKeys[hash]
	return key, ok, nil
}

type feishuPDFMetadata struct {
	RootToken    string
	ParentToken  string
	RelativePath string
	FolderPath   string
	RelationKey  string
	LinkedID     int64
}

func assertFeishuPDFMetadata(t *testing.T, db *sql.DB, fileToken string, want feishuPDFMetadata) int64 {
	t.Helper()

	var id int64
	var got feishuPDFMetadata
	if err := db.QueryRow(`
SELECT
	id,
	COALESCE(feishu_root_token, ''),
	COALESCE(feishu_parent_token, ''),
	COALESCE(feishu_relative_path, ''),
	COALESCE(feishu_folder_path, ''),
	COALESCE(feishu_relation_key, '')
FROM contract_main
WHERE feishu_file_token = ?
`, fileToken).Scan(&id, &got.RootToken, &got.ParentToken, &got.RelativePath, &got.FolderPath, &got.RelationKey); err != nil {
		t.Fatalf("load contract_main metadata for %s: %v", fileToken, err)
	}
	if got != want {
		t.Fatalf("metadata for %s = %#v, want %#v", fileToken, got, want)
	}
	return id
}

func assertFeishuInvoicePDFMetadata(t *testing.T, db *sql.DB, fileToken string, want feishuPDFMetadata) int64 {
	t.Helper()

	var id int64
	var got feishuPDFMetadata
	if err := db.QueryRow(`
SELECT
	id,
	COALESCE(feishu_root_token, ''),
	COALESCE(feishu_parent_token, ''),
	COALESCE(feishu_relative_path, ''),
	COALESCE(feishu_folder_path, ''),
	COALESCE(feishu_relation_key, ''),
	COALESCE(contract_id, 0)
FROM contract_invoices
WHERE feishu_file_token = ?
`, fileToken).Scan(&id, &got.RootToken, &got.ParentToken, &got.RelativePath, &got.FolderPath, &got.RelationKey, &got.LinkedID); err != nil {
		t.Fatalf("load contract_invoices metadata for %s: %v", fileToken, err)
	}
	if got != want {
		t.Fatalf("invoice metadata for %s = %#v, want %#v", fileToken, got, want)
	}
	return id
}

func assertObjectKeys(t *testing.T, puts []objectStorePut, want ...string) {
	t.Helper()

	if len(puts) != len(want) {
		t.Fatalf("object puts = %#v, want keys %v", puts, want)
	}
	got := make(map[string]bool, len(puts))
	for _, put := range puts {
		got[put.key] = true
	}
	for _, key := range want {
		if !got[key] {
			t.Fatalf("object puts = %#v, missing key %q", puts, key)
		}
	}
}

func assertPDFFileSize(t *testing.T, db *sql.DB, tableName, fileToken string, want int64) {
	t.Helper()

	var got int64
	query := `SELECT COALESCE(file_size, 0) FROM ` + tableName + ` WHERE feishu_file_token = ?`
	if err := db.QueryRow(query, fileToken).Scan(&got); err != nil {
		t.Fatalf("load %s file_size for %s: %v", tableName, fileToken, err)
	}
	if got != want {
		t.Fatalf("%s file_size for %s = %d, want %d", tableName, fileToken, got, want)
	}
}

func mustSeedPDFSource(t *testing.T, repo *feishusync.Repository, folderToken string) feishusync.SyncSource {
	return mustSeedPDFSourceWithMetadata(t, repo, folderToken, "")
}

func mustSeedPDFSourceWithMetadata(t *testing.T, repo *feishusync.Repository, folderToken, metadata string) feishusync.SyncSource {
	t.Helper()

	src := feishusync.SyncSource{
		SourceType:   feishusync.SourceTypePDFFolder,
		SourceToken:  folderToken,
		DisplayName:  "PDF Folder",
		SyncStatus:   feishusync.SyncStatusActive,
		MetadataJSON: metadata,
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
