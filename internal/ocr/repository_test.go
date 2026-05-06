package ocr_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"financeqa/internal/ocr"

	_ "modernc.org/sqlite"
)

func TestRepositoryClaimsPendingActiveRows(t *testing.T) {
	db := openOCRTestDB(t)
	execOCRFixture(t, db)
	mustExec(t, db, `INSERT INTO contract_main(file_name, storage_key, file_hash, sync_status, ocr_status, last_seen_at) VALUES
		('active.pdf', '/tmp/active.pdf', 'hash-a', 'active', 'pending', '2026-05-01 10:00:00'),
		('deleted.pdf', '/tmp/deleted.pdf', 'hash-d', 'deleted', 'pending', '2026-05-01 10:00:00'),
		('done.pdf', '/tmp/done.pdf', 'hash-x', 'active', 'done', '2026-05-01 10:00:00')`)
	mustExec(t, db, `INSERT INTO contract_invoices(contract_id, invoice_number, file_name, storage_key, file_hash, sync_status, ocr_status, last_seen_at) VALUES
		(10, 'pending:invoice-a', 'invoice-active.pdf', '/tmp/invoice-active.pdf', 'hash-ia', 'active', 'pending', '2026-05-01 10:01:00'),
		(10, 'pending:invoice-d', 'invoice-deleted.pdf', '/tmp/invoice-deleted.pdf', 'hash-id', 'deleted', 'pending', '2026-05-01 10:01:00')`)

	repo := ocr.NewRepository(db)
	docs, err := repo.ClaimPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClaimPending: %v", err)
	}
	if len(docs) != 2 || docs[0].FileName != "active.pdf" || docs[0].Table != "contract_main" || docs[1].FileName != "invoice-active.pdf" || docs[1].Table != "contract_invoices" || docs[1].ContractID != 10 {
		t.Fatalf("docs = %#v", docs)
	}
	var status string
	if err := db.QueryRow(`SELECT ocr_status FROM contract_main WHERE file_name='active.pdf'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("status = %q", status)
	}
	if err := db.QueryRow(`SELECT ocr_status FROM contract_invoices WHERE file_name='invoice-active.pdf'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("invoice status = %q", status)
	}
}

func TestRepositorySavesContractResult(t *testing.T) {
	db := openOCRTestDB(t)
	execOCRFixture(t, db)
	mustExec(t, db, `INSERT INTO contract_main(id, file_name, storage_key, file_hash, sync_status, ocr_status) VALUES
		(10, 'contract.pdf', '/tmp/contract.pdf', 'hash-c', 'active', 'running')`)

	repo := ocr.NewRepository(db)
	amount := 300000.0
	taxRate := 0.06
	result := ocr.Result{
		DocumentType:    ocr.DocumentTypeContract,
		OCRTextExcerpt:  "合同总金额为人民币300000元",
		ConfidenceNotes: "ok",
		Contract: ocr.ContractResult{
			ContractTitle:   "数据服务合同",
			SubCategory:     "数据服务",
			PartyA:          "南京优集数据科技有限公司",
			PartyB:          "深圳市五块石科技有限公司",
			StartDate:       "2026-02-01",
			EndDate:         "2027-01-31",
			TotalAmount:     &amount,
			Currency:        "CNY",
			TaxRate:         &taxRate,
			PaymentSchedule: []ocr.PaymentSchedule{{Amount: &amount, Condition: "一次性支付"}},
		},
	}
	quality := ocr.Validate(result)
	err := repo.SaveResult(context.Background(), ocr.PendingDocument{ID: 10, FileName: "contract.pdf", StorageKey: "/tmp/contract.pdf", FileHash: "hash-c"}, result, quality, testRunMeta())
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	var title, subCategory, partyA, status, processedAt string
	var storedAmount float64
	if err := db.QueryRow(`
SELECT contract_title, sub_category, party_a, contract_amount, ocr_status, COALESCE(CAST(processed_at AS TEXT), '')
FROM contract_main
WHERE id=10
`).Scan(&title, &subCategory, &partyA, &storedAmount, &status, &processedAt); err != nil {
		t.Fatal(err)
	}
	if title != "数据服务合同" || subCategory != "数据服务" || partyA != "南京优集数据科技有限公司" || storedAmount != 300000 || status != "done" {
		t.Fatalf("contract row mismatch title=%q subCategory=%q partyA=%q amount=%v status=%q", title, subCategory, partyA, storedAmount, status)
	}
	if !strings.HasPrefix(processedAt, "2026-05-01 20:00:00") {
		t.Fatalf("processed_at = %q, want Asia/Shanghai wall time 2026-05-01 20:00:00", processedAt)
	}
	var pageText string
	if err := db.QueryRow(`SELECT plain_text FROM contract_pages WHERE contract_id=10 AND page_num=0`).Scan(&pageText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pageText, "300000") {
		t.Fatalf("pageText = %q", pageText)
	}
	var extension string
	if err := db.QueryRow(`SELECT extension_data FROM contract_main WHERE id=10`).Scan(&extension); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(extension, "payment_schedule") {
		t.Fatalf("extension_data = %s", extension)
	}
}

func TestRepositorySavesFullTextPages(t *testing.T) {
	db := openOCRTestDB(t)
	execOCRFixture(t, db)
	mustExec(t, db, `INSERT INTO contract_main(id, file_name, storage_key, file_hash, sync_status, ocr_status) VALUES
		(12, 'contract-pages.pdf', '/tmp/contract-pages.pdf', 'hash-p', 'active', 'running')`)

	repo := ocr.NewRepository(db)
	result := ocr.Result{
		DocumentType:   ocr.DocumentTypeContract,
		OCRTextExcerpt: "第一页摘要",
		Contract: ocr.ContractResult{
			ContractTitle: "多页合同",
			PartyA:        "甲方公司",
			PartyB:        "乙方公司",
		},
		Pages: []ocr.PageResult{
			{PageNumber: 1, PlainText: "第一页完整正文", MarkdownText: "# 第一页"},
			{PageNumber: 2, PlainText: "第二页付款条款", MarkdownText: "# 第二页"},
		},
	}
	quality := ocr.Validate(result)
	err := repo.SaveResult(context.Background(), ocr.PendingDocument{ID: 12, FileName: "contract-pages.pdf", StorageKey: "/tmp/contract-pages.pdf", FileHash: "hash-p"}, result, quality, testRunMeta())
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	var totalPages int
	if err := db.QueryRow(`SELECT total_pages FROM contract_main WHERE id=12`).Scan(&totalPages); err != nil {
		t.Fatal(err)
	}
	if totalPages != 2 {
		t.Fatalf("total_pages = %d, want 2", totalPages)
	}
	rows, err := db.Query(`SELECT page_num, page_number, plain_text FROM contract_pages WHERE contract_id=12 ORDER BY page_num`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var pageNum, pageNumber int
		var text string
		if err := rows.Scan(&pageNum, &pageNumber, &text); err != nil {
			t.Fatal(err)
		}
		got = append(got, text)
		if pageNum != pageNumber-1 {
			t.Fatalf("page_num=%d page_number=%d", pageNum, pageNumber)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "第一页完整正文" || got[1] != "第二页付款条款" {
		t.Fatalf("pages = %#v", got)
	}
}

func TestRepositorySavesPageMetadata(t *testing.T) {
	db := openOCRTestDB(t)
	execOCRFixture(t, db)
	mustExec(t, db, `INSERT INTO contract_main(id, file_name, storage_key, file_hash, sync_status, ocr_status) VALUES
		(13, 'contract-page-meta.pdf', '/tmp/contract-page-meta.pdf', 'hash-meta', 'active', 'running')`)

	repo := ocr.NewRepository(db)
	result := ocr.Result{
		DocumentType:   ocr.DocumentTypeContract,
		OCRTextExcerpt: "含表格和签章",
		Contract: ocr.ContractResult{
			ContractTitle: "页面元数据合同",
			PartyA:        "甲方公司",
			PartyB:        "乙方公司",
		},
		Pages: []ocr.PageResult{{
			PageNumber:   1,
			PlainText:    "付款计划表和盖章页",
			MarkdownText: "| 项目 | 金额 |",
			HasTable:     true,
			HasSignature: true,
			Confidence:   0.92,
		}},
	}
	quality := ocr.Validate(result)
	if err := repo.SaveResult(context.Background(), ocr.PendingDocument{ID: 13, FileName: "contract-page-meta.pdf", StorageKey: "/tmp/contract-page-meta.pdf", FileHash: "hash-meta"}, result, quality, testRunMeta()); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	var hasTable, hasSignature bool
	var confidence float64
	var createdAt, updatedAt string
	if err := db.QueryRow(`
SELECT has_table, has_signature, ocr_confidence, COALESCE(CAST(created_at AS TEXT), ''), COALESCE(CAST(updated_at AS TEXT), '')
FROM contract_pages
WHERE contract_id=13 AND page_num=0
`).Scan(&hasTable, &hasSignature, &confidence, &createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if !hasTable || !hasSignature || confidence != 0.92 || strings.TrimSpace(createdAt) == "" || strings.TrimSpace(updatedAt) == "" {
		t.Fatalf("page metadata hasTable=%v hasSignature=%v confidence=%v createdAt=%q updatedAt=%q", hasTable, hasSignature, confidence, createdAt, updatedAt)
	}
}

func TestRepositoryStoresUnmatchedInvoiceCandidate(t *testing.T) {
	db := openOCRTestDB(t)
	execOCRFixture(t, db)
	mustExec(t, db, `INSERT INTO contract_main(id, file_name, storage_key, file_hash, sync_status, ocr_status) VALUES
		(11, 'invoice.pdf', '/tmp/invoice.pdf', 'hash-i', 'active', 'running')`)

	repo := ocr.NewRepository(db)
	preTax := 100.0
	tax := 6.0
	total := 106.0
	result := ocr.Result{
		DocumentType:   ocr.DocumentTypeInvoice,
		OCRTextExcerpt: "价税合计106元",
		Invoice: ocr.InvoiceResult{
			InvoiceNumber: "12345678901234567890",
			IssueDate:     "2026-02-28",
			BuyerName:     "南京优集数据科技有限公司",
			SellerName:    "深圳市五块石科技有限公司",
			PreTaxAmount:  &preTax,
			TaxAmount:     &tax,
			TotalAmount:   &total,
		},
	}
	quality := ocr.Validate(result)
	err := repo.SaveResult(context.Background(), ocr.PendingDocument{ID: 11, FileName: "invoice.pdf", StorageKey: "/tmp/invoice.pdf", FileHash: "hash-i"}, result, quality, testRunMeta())
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}
	var invoiceCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM contract_invoices`).Scan(&invoiceCount); err != nil {
		t.Fatal(err)
	}
	if invoiceCount != 0 {
		t.Fatalf("invoiceCount = %d, want 0", invoiceCount)
	}
	var extension string
	if err := db.QueryRow(`SELECT extension_data FROM contract_main WHERE id=11`).Scan(&extension); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(extension, "invoice_candidate") {
		t.Fatalf("extension_data = %s", extension)
	}
}

func TestRepositorySavesLinkedInvoiceToContractInvoices(t *testing.T) {
	db := openOCRTestDB(t)
	execOCRFixture(t, db)
	mustExec(t, db, `INSERT INTO contract_main(id, file_name, storage_key, file_hash, sync_status, ocr_status) VALUES
		(20, 'contract.pdf', '/tmp/contract.pdf', 'hash-c', 'active', 'done')`)
	mustExec(t, db, `INSERT INTO contract_invoices(id, contract_id, invoice_number, file_name, storage_key, file_hash, sync_status, ocr_status) VALUES
		(21, 20, 'pending:hash-i', 'invoice.pdf', '/tmp/invoice.pdf', 'hash-i', 'active', 'running')`)

	repo := ocr.NewRepository(db)
	preTax := 100.0
	tax := 6.0
	total := 106.0
	result := ocr.Result{
		DocumentType:   ocr.DocumentTypeInvoice,
		OCRTextExcerpt: "价税合计106元",
		Invoice: ocr.InvoiceResult{
			InvoiceNumber: "12345678901234567890",
			IssueDate:     "2026-02-28",
			BuyerName:     "南京优集数据科技有限公司",
			SellerName:    "深圳市五块石科技有限公司",
			PreTaxAmount:  &preTax,
			TaxAmount:     &tax,
			TotalAmount:   &total,
			Items:         []ocr.InvoiceItem{{Name: "服务费", Quantity: "1", Amount: &preTax}},
		},
	}
	quality := ocr.Validate(result)
	err := repo.SaveResult(context.Background(), ocr.PendingDocument{ID: 21, Table: "contract_invoices", ContractID: 20, FileName: "invoice.pdf", StorageKey: "/tmp/invoice.pdf", FileHash: "hash-i"}, result, quality, testRunMeta())
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	var contractID int64
	var invoiceNumber string
	var amount float64
	var storageKey string
	if err := db.QueryRow(`
SELECT contract_id, invoice_number, total_amount, storage_key
FROM contract_invoices
WHERE invoice_number = '12345678901234567890'
`).Scan(&contractID, &invoiceNumber, &amount, &storageKey); err != nil {
		t.Fatal(err)
	}
	if contractID != 20 || invoiceNumber != "12345678901234567890" || amount != 106 || storageKey != "/tmp/invoice.pdf" {
		t.Fatalf("invoice row mismatch contractID=%d invoice=%q amount=%v storage=%q", contractID, invoiceNumber, amount, storageKey)
	}
	var invoiceStatus string
	if err := db.QueryRow(`SELECT ocr_status FROM contract_invoices WHERE id=21`).Scan(&invoiceStatus); err != nil {
		t.Fatal(err)
	}
	if invoiceStatus != "done" {
		t.Fatalf("invoice ocr_status = %q, want done", invoiceStatus)
	}
}

func TestRepositoryMergesInvoiceOCRIntoExistingInvoiceNumber(t *testing.T) {
	db := openOCRTestDB(t)
	execOCRFixture(t, db)
	mustExec(t, db, `INSERT INTO contract_main(id, file_name, storage_key, file_hash, sync_status, ocr_status) VALUES
		(20, 'contract.pdf', '/tmp/contract.pdf', 'hash-c', 'active', 'done')`)
	mustExec(t, db, `INSERT INTO contract_invoices(id, contract_id, invoice_number, total_amount, file_name, storage_key, file_hash, sync_status, ocr_status) VALUES
		(30, 20, '12345678901234567890', 88, 'old-invoice.pdf', '/tmp/old-invoice.pdf', '', 'active', 'done'),
		(31, 20, 'pending:hash-i', NULL, 'invoice.pdf', '/tmp/invoice.pdf', 'hash-i', 'active', 'running')`)

	repo := ocr.NewRepository(db)
	preTax := 100.0
	tax := 6.0
	total := 106.0
	result := ocr.Result{
		DocumentType: ocr.DocumentTypeInvoice,
		Invoice: ocr.InvoiceResult{
			InvoiceNumber: "12345678901234567890",
			IssueDate:     "2026-02-28",
			BuyerName:     "南京优集数据科技有限公司",
			SellerName:    "深圳市五块石科技有限公司",
			PreTaxAmount:  &preTax,
			TaxAmount:     &tax,
			TotalAmount:   &total,
		},
	}
	quality := ocr.Validate(result)
	err := repo.SaveResult(context.Background(), ocr.PendingDocument{ID: 31, Table: "contract_invoices", ContractID: 20, FileName: "invoice.pdf", StorageKey: "/tmp/invoice.pdf", FileHash: "hash-i"}, result, quality, testRunMeta())
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	var rowCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM contract_invoices WHERE contract_id=20`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 1 {
		t.Fatalf("invoice rows = %d, want 1", rowCount)
	}
	var id int64
	var amount float64
	var storageKey, fileHash, status string
	if err := db.QueryRow(`SELECT id, total_amount, storage_key, file_hash, ocr_status FROM contract_invoices WHERE invoice_number='12345678901234567890'`).Scan(&id, &amount, &storageKey, &fileHash, &status); err != nil {
		t.Fatal(err)
	}
	if id != 30 || amount != 106 || storageKey != "/tmp/invoice.pdf" || fileHash != "hash-i" || status != "done" {
		t.Fatalf("merged row id=%d amount=%v storage=%q hash=%q status=%q", id, amount, storageKey, fileHash, status)
	}
}

func openOCRTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func execOCRFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `CREATE TABLE contract_main (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_name TEXT,
		storage_key TEXT,
		file_hash TEXT,
		sync_status TEXT,
		ocr_status TEXT,
		ocr_engine TEXT,
		total_pages INTEGER,
		processed_at TIMESTAMP,
		contract_title TEXT,
		contract_number TEXT,
		party_a TEXT,
		party_a_credit_code TEXT,
		party_b TEXT,
		party_b_credit_code TEXT,
		sign_date TEXT,
		start_date TEXT,
		end_date TEXT,
		contract_amount REAL,
		amount_currency TEXT,
		settlement_cycle TEXT,
		settlement_unit_price REAL,
		price_unit TEXT,
		payment_terms TEXT,
		payment_method TEXT,
		service_scope TEXT,
		tax_rate REAL,
		sub_category TEXT,
		extension_data TEXT,
		custom_metrics TEXT,
		last_seen_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	mustExec(t, db, `CREATE TABLE contract_pages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER,
		page_num INTEGER,
		page_number INTEGER,
		markdown_text TEXT,
		plain_text TEXT,
		raw_ocr_json TEXT,
		has_images INTEGER,
		has_table INTEGER,
		has_signature INTEGER,
		word_count INTEGER,
		char_count INTEGER,
		ocr_confidence REAL,
		created_at TIMESTAMP,
		updated_at TIMESTAMP,
		UNIQUE(contract_id, page_num)
	)`)
	mustExec(t, db, `CREATE TABLE contract_invoices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER NOT NULL,
		invoice_code TEXT,
		invoice_number TEXT NOT NULL,
		invoice_type TEXT,
		issue_date TEXT,
		check_code TEXT,
		machine_number TEXT,
		tax_bureau_code TEXT,
		tax_bureau_name TEXT,
		buyer_name TEXT,
		buyer_tax_id TEXT,
		seller_name TEXT,
		seller_tax_id TEXT,
		total_amount_without_tax REAL,
		total_tax_amount REAL,
		total_amount REAL,
		total_amount_cn TEXT,
		items_json TEXT,
		remarks TEXT,
		payee TEXT,
		reviewer TEXT,
		drawer TEXT,
		file_name TEXT,
		storage_key TEXT,
		file_hash TEXT,
		feishu_file_token TEXT,
		feishu_root_token TEXT,
		feishu_parent_token TEXT,
		feishu_relative_path TEXT,
		feishu_folder_path TEXT,
		feishu_slot_key TEXT,
		feishu_file_name TEXT,
		feishu_relation_key TEXT,
		file_size INTEGER,
		sync_status TEXT,
		ocr_status TEXT,
		ocr_engine TEXT,
		processed_at TIMESTAMP,
		total_pages INTEGER,
		feishu_deleted_at TIMESTAMP,
		last_seen_at TIMESTAMP,
		created_at TIMESTAMP,
		updated_at TIMESTAMP,
		match_method TEXT,
		match_confidence TEXT,
		extension_data TEXT,
		UNIQUE(contract_id, invoice_number)
	)`)
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}

func testRunMeta() ocr.RunMetadata {
	return ocr.RunMetadata{
		Model:          "test-model",
		ElapsedSeconds: 1.2,
		Usage:          ocr.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		ProcessedAt:    time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
