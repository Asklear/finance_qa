package ocr

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Repository struct {
	db *sql.DB
}

var repositoryBusinessLocation = loadRepositoryBusinessLocation()

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ClaimPending(ctx context.Context, limit int) ([]PendingDocument, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, 'contract_main', COALESCE(file_name, ''), COALESCE(storage_key, ''), COALESCE(file_hash, ''), 0, COALESCE(CAST(last_seen_at AS TEXT), '')
FROM contract_main
WHERE ocr_status = 'pending'
  AND storage_key IS NOT NULL
  AND TRIM(storage_key) <> ''
  AND (sync_status IS NULL OR sync_status = '' OR sync_status = 'active')
UNION ALL
SELECT id, 'contract_invoices', COALESCE(file_name, ''), COALESCE(storage_key, ''), COALESCE(file_hash, ''), COALESCE(contract_id, 0), COALESCE(CAST(last_seen_at AS TEXT), '')
FROM contract_invoices
WHERE ocr_status = 'pending'
  AND storage_key IS NOT NULL
  AND TRIM(storage_key) <> ''
  AND (sync_status IS NULL OR sync_status = '' OR sync_status = 'active')
ORDER BY 7 ASC, 2 ASC, 1 ASC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim pending select: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var docs []PendingDocument
	for rows.Next() {
		var doc PendingDocument
		var lastSeen string
		if err := rows.Scan(&doc.ID, &doc.Table, &doc.FileName, &doc.StorageKey, &doc.FileHash, &doc.ContractID, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan pending document: %w", err)
		}
		if strings.TrimSpace(doc.Table) == "" {
			doc.Table = "contract_main"
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending documents: %w", err)
	}

	for _, doc := range docs {
		table := pendingDocumentTable(doc)
		if _, err := r.db.ExecContext(ctx, `
UPDATE `+table+`
SET ocr_status = 'running'
WHERE id = ? AND ocr_status = 'pending'
`, doc.ID); err != nil {
			return nil, fmt.Errorf("mark pending document running: %w", err)
		}
	}
	return docs, nil
}

func (r *Repository) SaveResult(ctx context.Context, doc PendingDocument, result Result, quality QualityReport, run RunMetadata) error {
	extension, metrics, err := r.resultJSON(ctx, doc, result, quality, run)
	if err != nil {
		return err
	}

	switch strings.TrimSpace(result.DocumentType) {
	case DocumentTypeContract:
		if pendingDocumentTable(doc) != "contract_main" {
			return r.saveInvoiceFileResult(ctx, doc, result, extension, metrics, run)
		}
		if err := r.saveContractResult(ctx, doc, result, extension, metrics, run); err != nil {
			return err
		}
		return r.upsertOCRPages(ctx, doc.ID, result)
	case DocumentTypeInvoice:
		if pendingDocumentTable(doc) == "contract_invoices" {
			if err := r.saveInvoiceFileResult(ctx, doc, result, extension, metrics, run); err != nil {
				return err
			}
			return nil
		}
		if err := r.saveInvoiceCandidate(ctx, doc, result, extension, metrics, run); err != nil {
			return err
		}
		return r.upsertOCRPages(ctx, doc.ID, result)
	default:
		if pendingDocumentTable(doc) == "contract_invoices" {
			_, err := r.db.ExecContext(ctx, `
UPDATE contract_invoices
SET ocr_status = 'done',
    ocr_engine = ?,
    processed_at = ?,
    total_pages = ?,
    extension_data = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableText(run.Model), nullableTime(run.ProcessedAt), resultPageCount(result), extension, doc.ID)
			if err != nil {
				return fmt.Errorf("save unknown invoice ocr result: %w", err)
			}
			return nil
		}
		_, err := r.db.ExecContext(ctx, `
UPDATE `+pendingDocumentTable(doc)+`
SET ocr_status = 'done',
    ocr_engine = ?,
    processed_at = ?,
    total_pages = ?,
    extension_data = ?,
    custom_metrics = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableText(run.Model), nullableTime(run.ProcessedAt), resultPageCount(result), extension, metrics, doc.ID)
		if err != nil {
			return fmt.Errorf("save unknown ocr result: %w", err)
		}
		return r.upsertOCRPages(ctx, doc.ID, result)
	}
}

func (r *Repository) MarkFailed(ctx context.Context, doc PendingDocument, run RunMetadata, errMsg string) error {
	extension, metrics, err := r.loadJSONObjects(ctx, doc)
	if err != nil {
		return err
	}
	extension["gemini_ocr"] = map[string]any{
		"status":        QualityFailed,
		"error_message": strings.TrimSpace(errMsg),
		"run":           run,
	}
	metrics["gemini_ocr"] = map[string]any{
		"status":              QualityFailed,
		"error_message":       strings.TrimSpace(errMsg),
		"model":               run.Model,
		"elapsed_seconds":     run.ElapsedSeconds,
		"input_tokens":        run.Usage.InputTokens,
		"output_tokens":       run.Usage.OutputTokens,
		"estimated_cost_usd":  run.EstimatedCostUSD,
		"processed_at":        run.ProcessedAt,
		"quality_flags_count": 0,
	}
	extensionJSON, err := marshalJSONObject(extension)
	if err != nil {
		return err
	}
	metricsJSON, err := marshalJSONObject(metrics)
	if err != nil {
		return err
	}

	if pendingDocumentTable(doc) == "contract_invoices" {
		_, err = r.db.ExecContext(ctx, `
UPDATE contract_invoices
SET ocr_status = 'failed',
    ocr_engine = ?,
    processed_at = ?,
    extension_data = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableText(run.Model), nullableTime(run.ProcessedAt), extensionJSON, doc.ID)
		if err != nil {
			return fmt.Errorf("mark invoice ocr failed: %w", err)
		}
		return nil
	}

	_, err = r.db.ExecContext(ctx, `
UPDATE `+pendingDocumentTable(doc)+`
SET ocr_status = 'failed',
    ocr_engine = ?,
    processed_at = ?,
    extension_data = ?,
    custom_metrics = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableText(run.Model), nullableTime(run.ProcessedAt), extensionJSON, metricsJSON, doc.ID)
	if err != nil {
		return fmt.Errorf("mark ocr failed: %w", err)
	}
	return nil
}

func (r *Repository) RetryFailed(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, 'contract_main'
FROM contract_main
WHERE ocr_status = 'failed'
  AND storage_key IS NOT NULL
  AND TRIM(storage_key) <> ''
  AND (sync_status IS NULL OR sync_status = '' OR sync_status = 'active')
UNION ALL
SELECT id, 'contract_invoices'
FROM contract_invoices
WHERE ocr_status = 'failed'
  AND storage_key IS NOT NULL
  AND TRIM(storage_key) <> ''
  AND (sync_status IS NULL OR sync_status = '' OR sync_status = 'active')
ORDER BY 2 ASC, 1 ASC
LIMIT ?
`, limit)
	if err != nil {
		return 0, fmt.Errorf("select failed ocr rows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var docs []PendingDocument
	for rows.Next() {
		var doc PendingDocument
		if err := rows.Scan(&doc.ID, &doc.Table); err != nil {
			return 0, fmt.Errorf("scan failed ocr row: %w", err)
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate failed ocr rows: %w", err)
	}

	var updated int64
	for _, doc := range docs {
		res, err := r.db.ExecContext(ctx, `UPDATE `+pendingDocumentTable(doc)+` SET ocr_status = 'pending', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, doc.ID)
		if err != nil {
			return updated, fmt.Errorf("retry failed ocr row: %w", err)
		}
		n, _ := res.RowsAffected()
		updated += n
	}
	return updated, nil
}

func (r *Repository) saveContractResult(ctx context.Context, doc PendingDocument, result Result, extension, metrics string, run RunMetadata) error {
	contract := result.Contract
	_, err := r.db.ExecContext(ctx, `
UPDATE contract_main
SET contract_title = ?,
    contract_number = ?,
    party_a = ?,
    party_a_credit_code = ?,
    party_b = ?,
    party_b_credit_code = ?,
    sign_date = ?,
    start_date = ?,
    end_date = ?,
    contract_amount = ?,
    amount_currency = ?,
    settlement_cycle = ?,
    settlement_unit_price = ?,
    price_unit = ?,
    payment_terms = ?,
    payment_method = ?,
    service_scope = ?,
    tax_rate = ?,
    sub_category = ?,
    ocr_status = 'done',
    ocr_engine = ?,
    processed_at = ?,
    total_pages = ?,
    extension_data = ?,
    custom_metrics = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableText(contract.ContractTitle), nullableText(contract.ContractNumber), nullableText(contract.PartyA), nullableText(contract.PartyACreditCode),
		nullableText(contract.PartyB), nullableText(contract.PartyBCreditCode), nullableText(contract.SignDate), nullableText(contract.StartDate),
		nullableText(contract.EndDate), nullableFloat(contract.TotalAmount), nullableText(defaultCurrency(contract.Currency)),
		nullableText(contract.SettlementCycle), nullableFloat(contract.SettlementUnitPrice), nullableText(contract.PriceUnit),
		nullableText(contract.PaymentTerms), nullableText(contract.PaymentMethod), nullableText(contract.ServiceScopeSummary),
		nullableFloat(contract.TaxRate), nullableText(contractSubCategory(result)), nullableText(run.Model), nullableTime(run.ProcessedAt), resultPageCount(result), extension, metrics, doc.ID)
	if err != nil {
		return fmt.Errorf("save contract ocr result: %w", err)
	}
	return nil
}

func contractSubCategory(result Result) string {
	if value := normalizeContractSubCategory(result.Contract.SubCategory); value != "" {
		return value
	}
	text := strings.Join([]string{
		result.Contract.ContractTitle,
		result.Contract.ServiceScopeSummary,
		result.FileSummary,
		result.OCRTextExcerpt,
	}, "\n")
	for _, page := range result.Pages {
		text += "\n" + page.MarkdownText + "\n" + page.PlainText
	}
	return inferContractSubCategory(text)
}

func normalizeContractSubCategory(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "数据服务", "API服务", "市场调研", "推广服务", "云计算服务", "专项服务", "咨询服务":
		return value
	default:
		return ""
	}
}

func inferContractSubCategory(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	switch {
	case containsAnyText(text, "边缘计算", "云算力", "maas", "大模型", "kimi", "moonshot", "模型服务", "aggregated maas"):
		return "云计算服务"
	case containsAnyText(text, "法律", "律师", "代理记账", "财税", "iso", "管理体系"):
		return "专项服务"
	case containsAnyText(text, "api", "接口", "商指针", "价格和库存监测", "技术服务合同"):
		return "API服务"
	case containsAnyText(text, "推广", "渠道", "ip流量", "京东推广"):
		return "推广服务"
	case containsAnyText(text, "市场调研", "行业榜单", "商品监控", "定向商品"):
		return "市场调研"
	case containsAnyText(text, "数据采购", "数据服务", "data service", "data license", "data provider", "数据授权", "数据成品", "数据分析", "信息服务"):
		return "数据服务"
	case containsAnyText(text, "咨询", "服务框架", "项目咨询", "委托服务"):
		return "咨询服务"
	default:
		return ""
	}
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func (r *Repository) saveInvoiceCandidate(ctx context.Context, doc PendingDocument, result Result, extension, metrics string, run RunMetadata) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE contract_main
SET ocr_status = 'done',
    ocr_engine = ?,
    processed_at = ?,
    total_pages = ?,
    extension_data = ?,
    custom_metrics = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableText(run.Model), nullableTime(run.ProcessedAt), resultPageCount(result), extension, metrics, doc.ID)
	if err != nil {
		return fmt.Errorf("save invoice candidate: %w", err)
	}
	return nil
}

func (r *Repository) saveInvoiceFileResult(ctx context.Context, doc PendingDocument, result Result, extension, metrics string, run RunMetadata) error {
	invoiceID, err := r.updateInvoiceResult(ctx, doc, result, extension)
	if err != nil {
		return err
	}
	if invoiceID != 0 {
		doc.ID = invoiceID
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE contract_invoices
SET ocr_status = 'done',
    ocr_engine = ?,
    processed_at = ?,
    total_pages = ?,
    extension_data = ?,
    file_name = ?,
    storage_key = ?,
    file_hash = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableText(run.Model), nullableTime(run.ProcessedAt), resultPageCount(result), extension, nullableText(doc.FileName), nullableText(doc.StorageKey), nullableText(doc.FileHash), doc.ID)
	if err != nil {
		return fmt.Errorf("save invoice file ocr result: %w", err)
	}
	return nil
}

func (r *Repository) updateInvoiceResult(ctx context.Context, doc PendingDocument, result Result, extension string) (int64, error) {
	invoice := result.Invoice
	invoiceNumber := strings.TrimSpace(invoice.InvoiceNumber)
	if invoiceNumber == "" {
		invoiceNumber = strings.TrimSpace(invoice.InvoiceCode)
	}
	if invoiceNumber == "" {
		invoiceNumber = doc.FileHash
	}
	targetID, err := r.mergeInvoiceTarget(ctx, doc, invoiceNumber)
	if err != nil {
		return 0, err
	}
	if targetID != 0 {
		doc.ID = targetID
	}
	itemsJSON, err := json.Marshal(invoice.Items)
	if err != nil {
		return 0, fmt.Errorf("marshal invoice items: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE contract_invoices
SET invoice_code = ?,
    invoice_number = ?,
    invoice_type = ?,
    issue_date = ?,
    check_code = ?,
    machine_number = ?,
    tax_bureau_code = ?,
    tax_bureau_name = ?,
    buyer_name = ?,
    buyer_tax_id = ?,
    seller_name = ?,
    seller_tax_id = ?,
    total_amount_without_tax = ?,
    total_tax_amount = ?,
    total_amount = ?,
    total_amount_cn = ?,
    items_json = ?,
    remarks = ?,
    payee = ?,
    reviewer = ?,
    drawer = ?,
    match_method = ?,
    match_confidence = ?,
    extension_data = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableText(invoice.InvoiceCode), invoiceNumber, nullableText(invoice.InvoiceType), nullableText(invoice.IssueDate),
		nullableText(invoice.CheckCode), nullableText(invoice.MachineNumber), nullableText(invoice.TaxBureauCode), nullableText(invoice.TaxBureauName),
		nullableText(invoice.BuyerName), nullableText(invoice.BuyerTaxID), nullableText(invoice.SellerName), nullableText(invoice.SellerTaxID),
		nullableFloat(invoice.PreTaxAmount), nullableFloat(invoice.TaxAmount), nullableFloat(invoice.TotalAmount), nullableText(invoice.TotalAmountCN),
		string(itemsJSON), nullableText(invoice.Remarks), nullableText(invoice.Payee), nullableText(invoice.Reviewer), nullableText(invoice.Drawer),
		"feishu_folder_relation", "folder_path", extension, doc.ID)
	if err != nil {
		return 0, fmt.Errorf("update invoice result: %w", err)
	}
	return doc.ID, nil
}

func (r *Repository) mergeInvoiceTarget(ctx context.Context, doc PendingDocument, invoiceNumber string) (int64, error) {
	invoiceNumber = strings.TrimSpace(invoiceNumber)
	if doc.ID == 0 || doc.ContractID == 0 || invoiceNumber == "" || strings.HasPrefix(invoiceNumber, "pending:") {
		return doc.ID, nil
	}
	var existingID int64
	err := r.db.QueryRowContext(ctx, `
SELECT id
FROM contract_invoices
WHERE contract_id = ?
  AND invoice_number = ?
  AND id <> ?
LIMIT 1
`, doc.ContractID, invoiceNumber, doc.ID).Scan(&existingID)
	if err == sql.ErrNoRows {
		return doc.ID, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find existing invoice number: %w", err)
	}
	if existingID == 0 {
		return doc.ID, nil
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM contract_invoices WHERE id = ?`, doc.ID)
	if err != nil {
		return 0, fmt.Errorf("delete pending invoice duplicate: %w", err)
	}
	return existingID, nil
}

func (r *Repository) upsertOCRPages(ctx context.Context, contractID int64, result Result) error {
	if len(result.Pages) == 0 {
		return r.upsertPage(ctx, contractID, result, PageResult{
			PageNumber:   1,
			MarkdownText: strings.TrimSpace(result.OCRTextExcerpt),
			PlainText:    strings.TrimSpace(result.OCRTextExcerpt),
			Confidence:   0,
		}, 0)
	}
	for i, page := range result.Pages {
		if err := r.upsertPage(ctx, contractID, result, page, i); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) upsertPage(ctx context.Context, contractID int64, result Result, page PageResult, index int) error {
	rawJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal evidence raw json: %w", err)
	}
	pageNumber := page.PageNumber
	if pageNumber <= 0 {
		pageNumber = index + 1
	}
	pageNum := pageNumber - 1
	plainText := strings.TrimSpace(page.PlainText)
	markdownText := strings.TrimSpace(page.MarkdownText)
	if plainText == "" {
		plainText = markdownText
	}
	if markdownText == "" {
		markdownText = plainText
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO contract_pages(
	contract_id, page_num, page_number, markdown_text, plain_text,
	raw_ocr_json, has_images, has_table, has_signature, word_count, char_count,
	ocr_confidence, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(contract_id, page_num) DO UPDATE SET
	page_number = excluded.page_number,
	markdown_text = excluded.markdown_text,
	plain_text = excluded.plain_text,
	raw_ocr_json = excluded.raw_ocr_json,
	has_images = excluded.has_images,
	has_table = excluded.has_table,
	has_signature = excluded.has_signature,
	word_count = excluded.word_count,
	char_count = excluded.char_count,
	ocr_confidence = excluded.ocr_confidence,
	created_at = COALESCE(contract_pages.created_at, excluded.created_at),
	updated_at = excluded.updated_at
`, contractID, pageNum, pageNumber, markdownText, plainText, string(rawJSON), true, page.HasTable, page.HasSignature, wordCount(plainText), len([]rune(plainText)), page.Confidence)
	if err != nil {
		return fmt.Errorf("upsert contract page evidence: %w", err)
	}
	return nil
}

func (r *Repository) resultJSON(ctx context.Context, doc PendingDocument, result Result, quality QualityReport, run RunMetadata) (string, string, error) {
	extension, metrics, err := r.loadJSONObjects(ctx, doc)
	if err != nil {
		return "", "", err
	}
	payload := map[string]any{
		"document_type": strings.TrimSpace(result.DocumentType),
		"file_name":     doc.FileName,
		"storage_key":   doc.StorageKey,
		"file_hash":     doc.FileHash,
		"result":        result,
		"quality":       quality,
		"run":           run,
	}
	switch strings.TrimSpace(result.DocumentType) {
	case DocumentTypeContract:
		payload["payment_schedule"] = result.Contract.PaymentSchedule
	case DocumentTypeInvoice:
		payload["invoice_candidate"] = result.Invoice
	}
	extension["gemini_ocr"] = payload
	metrics["gemini_ocr"] = map[string]any{
		"status":              quality.Status,
		"flags":               quality.Flags,
		"quality_flags_count": len(quality.Flags),
		"model":               run.Model,
		"elapsed_seconds":     run.ElapsedSeconds,
		"input_tokens":        run.Usage.InputTokens,
		"output_tokens":       run.Usage.OutputTokens,
		"total_tokens":        run.Usage.TotalTokens,
		"estimated_cost_usd":  run.EstimatedCostUSD,
		"processed_at":        run.ProcessedAt,
	}
	extensionJSON, err := marshalJSONObject(extension)
	if err != nil {
		return "", "", err
	}
	metricsJSON, err := marshalJSONObject(metrics)
	if err != nil {
		return "", "", err
	}
	return extensionJSON, metricsJSON, nil
}

func (r *Repository) loadJSONObjects(ctx context.Context, doc PendingDocument) (map[string]any, map[string]any, error) {
	var extensionText, metricsText string
	table := pendingDocumentTable(doc)
	metricsExpr := "''"
	if table == "contract_main" {
		metricsExpr = "COALESCE(CAST(custom_metrics AS TEXT), '')"
	}
	err := r.db.QueryRowContext(ctx, `
SELECT COALESCE(CAST(extension_data AS TEXT), ''), `+metricsExpr+`
FROM `+table+`
WHERE id = ?
`, doc.ID).Scan(&extensionText, &metricsText)
	if err != nil {
		return nil, nil, fmt.Errorf("load ocr json columns: %w", err)
	}
	return parseJSONObject(extensionText), parseJSONObject(metricsText), nil
}

func pendingDocumentTable(doc PendingDocument) string {
	if strings.TrimSpace(doc.Table) == "contract_invoices" {
		return "contract_invoices"
	}
	return "contract_main"
}

func parseJSONObject(text string) map[string]any {
	text = strings.TrimSpace(text)
	if text == "" {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(text), &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}

func marshalJSONObject(value map[string]any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal ocr json object: %w", err)
	}
	return string(data), nil
}

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.In(repositoryBusinessLocation).Format("2006-01-02 15:04:05.999999")
}

func loadRepositoryBusinessLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		return loc
	}
	return time.FixedZone("Asia/Shanghai", 8*60*60)
}

func defaultCurrency(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "CNY"
	}
	return value
}

func resultPageCount(result Result) int {
	if len(result.Pages) > 0 {
		return len(result.Pages)
	}
	return 1
}

func wordCount(text string) int {
	return len(strings.Fields(text))
}

func (r *Repository) tableExists(ctx context.Context, tableName string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name = ?`, tableName).Scan(&count)
	if err == nil {
		return count > 0, nil
	}

	var regclass sql.NullString
	err = r.db.QueryRowContext(ctx, `SELECT to_regclass(?)`, tableName).Scan(&regclass)
	if err != nil {
		return false, err
	}
	return regclass.Valid && strings.TrimSpace(regclass.String) != "", nil
}

func (r *Repository) columnsExist(ctx context.Context, tableName string, columns []string) (bool, error) {
	existing := map[string]bool{}
	rows, err := r.db.QueryContext(ctx, `PRAGMA table_info(`+tableName+`)`)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var cid int
			var name string
			var typ string
			var notNull int
			var defaultValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				return false, err
			}
			existing[name] = true
		}
		if err := rows.Err(); err != nil {
			return false, err
		}
		return containsAllColumns(existing, columns), nil
	}

	rows, err = r.db.QueryContext(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_name = ?
`, tableName)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return containsAllColumns(existing, columns), nil
}

func containsAllColumns(existing map[string]bool, columns []string) bool {
	for _, column := range columns {
		if !existing[column] {
			return false
		}
	}
	return true
}
