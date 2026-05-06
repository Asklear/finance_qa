package query

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

func (e *Engine) ProbeContractDetailSources(spec QuerySpec) ContractDetailProbeResult {
	result := ContractDetailProbeResult{
		Intent:          inferContractDetailIntent(spec.NormalizedQuestion),
		CandidateTables: contractDetailCandidateTables(inferContractDetailIntent(spec.NormalizedQuestion)),
		Confidence:      0.2,
	}
	if e == nil || e.db == nil {
		result.Reason = "数据库连接不可用"
		return result
	}
	if spec.QueryFamily != QueryFamilyContractDetail {
		result.Intent = ContractDetailIntentUnknown
		result.CandidateTables = nil
		result.Reason = "非合同明细问题"
		return result
	}
	if len(e.tableColumns("contract_main")) == 0 {
		result.Reason = "缺少合同明细主表"
		return result
	}

	candidates, err := e.matchContractDetailCandidates(context.Background(), spec)
	if err != nil {
		result.Reason = "合同明细探测失败：" + err.Error()
		return result
	}
	result.MatchedContractRows = len(candidates)
	if len(candidates) == 0 {
		result.Reason = "没有匹配到合同明细记录"
		return result
	}
	if candidates[0].matchScore >= 20 {
		result.Confidence = 0.9
	} else if candidates[0].matchScore >= 10 {
		result.Confidence = 0.7
	} else {
		result.Confidence = 0.45
	}
	result.HasStructuredAnswer = contractDetailHasStructuredAnswer(e, result.Intent, candidates)
	result.NeedsPageText = contractDetailNeedsPageText(result.Intent, result.HasStructuredAnswer)
	if result.NeedsPageText {
		result.CandidateTables = appendUniqueStrings(result.CandidateTables, "contract_pages")
	}
	result.CandidateTables = dedupeStrings(result.CandidateTables)
	result.Reason = "已按合同名称、编号、双方主体和文件名探测合同明细"
	return result
}

func inferContractDetailIntent(question string) ContractDetailIntent {
	q := strings.TrimSpace(question)
	if containsAny(q, []string{"发票金额", "发票明细", "发票内容", "发票号码", "发票号", "发票里", "这张发票", "发票写了什么", "开票日期", "票面金额", "不含税", "含税", "税额", "购买方", "销售方"}) {
		return ContractDetailIntentInvoice
	}
	if containsAny(q, []string{"第几页", "哪一页", "第几条", "第几款", "原文", "正文", "全文", "写了什么", "合同里", "协议里", "合同内容", "具体内容", "内容是什么"}) {
		return ContractDetailIntentPage
	}
	if containsAny(q, []string{"条款", "付款方式", "付款条", "结算周期", "结算方式", "服务范围", "服务内容", "交付", "验收", "保密", "违约"}) {
		return ContractDetailIntentClause
	}
	if containsAny(q, []string{"签署", "签约", "起止", "到期", "续约", "税率", "合同金额"}) {
		return ContractDetailIntentField
	}
	return ContractDetailIntentUnknown
}

func contractDetailCandidateTables(intent ContractDetailIntent) []string {
	switch intent {
	case ContractDetailIntentInvoice:
		return []string{"contract_main", "contract_invoices"}
	case ContractDetailIntentPage:
		return []string{"contract_main", "contract_pages"}
	case ContractDetailIntentClause:
		return []string{"contract_main", "contract_pages"}
	case ContractDetailIntentField:
		return []string{"contract_main"}
	default:
		return []string{"contract_main"}
	}
}

func contractDetailHasStructuredAnswer(e *Engine, intent ContractDetailIntent, candidates []contractDetailCandidate) bool {
	if len(candidates) == 0 {
		return false
	}
	switch intent {
	case ContractDetailIntentInvoice:
		return e.hasContractDetailInvoiceRows(candidates)
	case ContractDetailIntentClause:
		for _, candidate := range candidates {
			if strings.TrimSpace(candidate.PaymentTerms) != "" ||
				strings.TrimSpace(candidate.PaymentMethod) != "" ||
				strings.TrimSpace(candidate.SettlementCycle) != "" ||
				strings.TrimSpace(candidate.ServiceScope) != "" {
				return true
			}
		}
	case ContractDetailIntentField:
		for _, candidate := range candidates {
			if candidate.structuredFields > 0 {
				return true
			}
		}
	}
	return false
}

func contractDetailNeedsPageText(intent ContractDetailIntent, hasStructuredAnswer bool) bool {
	switch intent {
	case ContractDetailIntentPage:
		return true
	case ContractDetailIntentClause:
		return !hasStructuredAnswer
	case ContractDetailIntentField:
		return !hasStructuredAnswer
	default:
		return false
	}
}

func (e *Engine) hasContractDetailInvoiceRows(candidates []contractDetailCandidate) bool {
	ids := contractDetailInternalIDs(candidates)
	if len(ids) == 0 {
		return false
	}
	return e.countContractDetailRowsByContractIDs("contract_invoices", ids) > 0
}

func (e *Engine) countContractDetailRowsByContractIDs(tableName string, ids []string) int {
	if len(ids) == 0 || len(e.tableColumns(tableName)) == 0 || !e.tableColumns(tableName)["contract_id"] {
		return 0
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	sqlText := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE CAST(contract_id AS TEXT) IN (%s)", tableName, strings.Join(placeholders, ","))
	var count int
	if err := e.db.QueryRow(sqlText, args...).Scan(&count); err != nil {
		return 0
	}
	return count
}

func (e *Engine) matchContractDetailCandidates(ctx context.Context, spec QuerySpec) ([]contractDetailCandidate, error) {
	cols := e.tableColumns("contract_main")
	if len(cols) == 0 || !cols["id"] {
		return nil, nil
	}
	fields := []string{
		"contract_number", "contract_title", "party_a", "party_b", "sign_date",
		"start_date", "end_date", "contract_amount", "amount_currency", "settlement_cycle",
		"settlement_unit_price", "payment_terms", "payment_method", "tax_rate", "service_scope",
		"file_name", "feishu_file_name", "storage_key", "updated_at",
	}
	selects := []string{"CAST(id AS TEXT)"}
	for _, field := range fields {
		selects = append(selects, contractDetailSelectExpr(cols, field))
	}
	sqlText := "SELECT " + strings.Join(selects, ", ") + " FROM contract_main"
	if filter := contractDetailActiveMainFilter(cols); filter != "" {
		sqlText += " WHERE " + filter
	}
	sqlText += " ORDER BY " + contractDetailOrderExpr(cols) + " LIMIT 200"

	args := []any{}
	rows, err := e.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	question := strings.TrimSpace(spec.NormalizedQuestion)
	invoiceMatchedIDs := map[string]struct{}{}
	if inferContractDetailIntent(question) == ContractDetailIntentInvoice {
		if invoiceIDs := e.matchContractIDsFromInvoices(ctx, question); len(invoiceIDs) > 0 {
			for _, id := range invoiceIDs {
				invoiceMatchedIDs[id] = struct{}{}
			}
			sqlText = "SELECT " + strings.Join(selects, ", ") + " FROM contract_main WHERE CAST(id AS TEXT) IN (" + contractDetailPlaceholders(len(invoiceIDs)) + ") ORDER BY " + contractDetailOrderExpr(cols)
			rows.Close()
			rows, err = e.db.QueryContext(ctx, sqlText, stringsToAny(invoiceIDs)...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
		}
	}
	candidates := make([]contractDetailCandidate, 0, 8)
	for rows.Next() {
		candidate, err := scanContractDetailCandidate(rows)
		if err != nil {
			return nil, err
		}
		candidate.matchScore = scoreContractDetailCandidate(question, candidate)
		if _, ok := invoiceMatchedIDs[candidate.internalID]; ok && candidate.matchScore < 50 {
			candidate.matchScore = 50
		}
		candidate.structuredFields = countContractDetailStructuredFields(candidate)
		if candidate.matchScore == 0 {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].matchScore == candidates[j].matchScore {
			return candidates[i].SignDate > candidates[j].SignDate
		}
		return candidates[i].matchScore > candidates[j].matchScore
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	candidates = filterContractDetailCandidatesByScore(candidates)
	return candidates, nil
}

func (e *Engine) matchContractIDsFromInvoices(ctx context.Context, question string) []string {
	cols := e.tableColumns("contract_invoices")
	if len(cols) == 0 || !cols["contract_id"] {
		return nil
	}
	selects := []string{
		"CAST(contract_id AS TEXT)",
		contractDetailTextSelectExpr(cols, "invoice_number"),
		contractDetailTextSelectExpr(cols, "file_name"),
		contractDetailTextSelectExpr(cols, "feishu_file_name"),
		contractDetailTextSelectExpr(cols, "buyer_name"),
		contractDetailTextSelectExpr(cols, "seller_name"),
		contractDetailTextSelectExpr(cols, "remarks"),
		contractDetailTextSelectExpr(cols, "items_json"),
	}
	sqlText := "SELECT " + strings.Join(selects, ", ") + " FROM contract_invoices"
	if cols["sync_status"] {
		sqlText += " WHERE (sync_status IS NULL OR sync_status <> 'deleted')"
	}
	rows, err := e.db.QueryContext(ctx, sqlText)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type scoredID struct {
		id    string
		score int
	}
	scored := make([]scoredID, 0, 8)
	for rows.Next() {
		var id, invoiceNumber, fileName, feishuFileName, buyer, seller, remarks, items string
		if err := rows.Scan(&id, &invoiceNumber, &fileName, &feishuFileName, &buyer, &seller, &remarks, &items); err != nil {
			continue
		}
		score := scoreContractInvoiceQuestionMatch(question, ContractInvoiceDetail{
			ContractID:    id,
			InvoiceNumber: invoiceNumber,
			BuyerName:     buyer,
			SellerName:    seller,
			Remarks:       remarks,
			ItemsJSON:     items,
			FileName:      preferredSourceFileName(fileName, feishuFileName, ""),
		})
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredID{id: strings.TrimSpace(id), score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	if len(scored) == 0 || scored[0].score < 12 {
		return nil
	}
	top := scored[0].score
	out := make([]string, 0, len(scored))
	for _, item := range scored {
		if item.id == "" {
			continue
		}
		if item.score == top || item.score*2 >= top {
			out = append(out, item.id)
		}
	}
	return dedupeStrings(out)
}

func scoreContractInvoiceQuestionMatch(question string, invoice ContractInvoiceDetail) int {
	score := 0
	fields := []struct {
		text   string
		weight int
	}{
		{invoice.InvoiceNumber, 30},
		{invoice.FileName, 16},
		{invoice.BuyerName, 10},
		{invoice.SellerName, 10},
		{invoice.Remarks, 8},
		{invoice.ItemsJSON, 6},
	}
	for _, field := range fields {
		text := strings.TrimSpace(field.text)
		if text == "" {
			continue
		}
		if strings.Contains(question, text) {
			score += field.weight * 2
			continue
		}
		if strings.Contains(text, question) {
			score += field.weight
			continue
		}
		for _, token := range contractDetailMatchTokens(text) {
			if looksLikeTemporalMetricEntity(token) || looksLikePeriodOnlyEntity(token) {
				continue
			}
			if strings.Contains(question, token) {
				score += field.weight
				break
			}
		}
	}
	return score
}

func filterContractDetailCandidatesByScore(candidates []contractDetailCandidate) []contractDetailCandidate {
	if len(candidates) <= 1 {
		return candidates
	}
	topScore := candidates[0].matchScore
	if topScore <= 0 {
		return nil
	}
	out := make([]contractDetailCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.matchScore == topScore {
			out = append(out, candidate)
			continue
		}
		if topScore < 20 && candidate.matchScore >= 10 {
			out = append(out, candidate)
			continue
		}
		if topScore >= 20 && candidate.matchScore*2 >= topScore {
			out = append(out, candidate)
		}
	}
	return out
}

func contractDetailSelectExpr(cols map[string]bool, field string) string {
	if field == "contract_amount" || field == "tax_rate" {
		if !cols[field] {
			return "0"
		}
		return "COALESCE(" + field + ", 0)"
	}
	if !cols[field] {
		return "''"
	}
	return "COALESCE(CAST(" + field + " AS TEXT), '')"
}

func contractDetailOrderExpr(cols map[string]bool) string {
	if cols["sign_date"] {
		return "sign_date DESC, contract_title"
	}
	if cols["contract_title"] {
		return "contract_title"
	}
	return "id"
}

func contractDetailActiveMainFilter(cols map[string]bool) string {
	if cols["sync_status"] {
		return "(sync_status IS NULL OR sync_status <> 'deleted')"
	}
	return ""
}

func scanContractDetailCandidate(rows *sql.Rows) (contractDetailCandidate, error) {
	var candidate contractDetailCandidate
	var amount, taxRate float64
	var fileName, feishuFileName, storageKey string
	err := rows.Scan(
		&candidate.internalID,
		&candidate.ContractNumber,
		&candidate.ContractTitle,
		&candidate.PartyA,
		&candidate.PartyB,
		&candidate.SignDate,
		&candidate.StartDate,
		&candidate.EndDate,
		&amount,
		&candidate.Currency,
		&candidate.SettlementCycle,
		&candidate.UnitPrice,
		&candidate.PaymentTerms,
		&candidate.PaymentMethod,
		&taxRate,
		&candidate.ServiceScope,
		&fileName,
		&feishuFileName,
		&storageKey,
		&candidate.UpdatedAt,
	)
	candidate.ContractAmount = amount
	candidate.TaxRate = taxRate
	candidate.FileName = preferredSourceFileName(fileName, feishuFileName, storageKey)
	return candidate, err
}

func scoreContractDetailCandidate(question string, candidate contractDetailCandidate) int {
	score := 0
	fields := []struct {
		text   string
		weight int
	}{
		{candidate.ContractTitle, 20},
		{candidate.ContractNumber, 18},
		{candidate.FileName, 12},
		{candidate.PartyA, 8},
		{candidate.PartyB, 8},
		{candidate.ServiceScope, 5},
	}
	for _, field := range fields {
		text := strings.TrimSpace(field.text)
		if text == "" {
			continue
		}
		if strings.Contains(question, text) {
			score += field.weight * 2
			continue
		}
		if strings.Contains(text, question) {
			score += field.weight
			continue
		}
		for _, token := range contractDetailMatchTokens(text) {
			if strings.Contains(question, token) {
				score += field.weight
				break
			}
		}
	}
	return score
}

func contractDetailMatchTokens(text string) []string {
	text = strings.TrimSpace(text)
	replacer := strings.NewReplacer("（", "|", "）", "|", "(", "|", ")", "|", "-", "|", "_", "|", " ", "|", "：", "|", ":", "|", "，", "|", ",", "|", "。", "|", ".", "|")
	parts := strings.Split(replacer.Replace(text), "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len([]rune(part)) < 2 {
			continue
		}
		out = append(out, part)
		if len([]rune(part)) > 4 {
			out = append(out, string([]rune(part)[:4]))
		}
	}
	return dedupeStrings(out)
}

func countContractDetailStructuredFields(candidate contractDetailCandidate) int {
	count := 0
	for _, value := range []string{
		candidate.ContractNumber,
		candidate.ContractTitle,
		candidate.PartyA,
		candidate.PartyB,
		candidate.SignDate,
		candidate.StartDate,
		candidate.EndDate,
		candidate.Currency,
		candidate.SettlementCycle,
		candidate.UnitPrice,
		candidate.PaymentTerms,
		candidate.PaymentMethod,
		candidate.ServiceScope,
		candidate.FileName,
	} {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	if candidate.ContractAmount != 0 {
		count++
	}
	if candidate.TaxRate != 0 {
		count++
	}
	return count
}

func contractDetailInternalIDs(candidates []contractDetailCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.internalID) != "" {
			out = append(out, candidate.internalID)
		}
	}
	return dedupeStrings(out)
}
