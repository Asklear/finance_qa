package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type rawContractPage struct {
	PageNumber int
	Text       string
}

func (e *Engine) collectContractDetail(ctx context.Context, spec QuerySpec, probe ContractDetailProbeResult) (ContractDetailResult, error) {
	result := ContractDetailResult{Probe: probe}
	candidates, err := e.matchContractDetailCandidates(ctx, spec)
	if err != nil {
		return result, err
	}
	result.MatchedContracts = contractDetailMatchesFromCandidates(candidates)
	if len(result.MatchedContracts) == 0 {
		result.SourceTables = dedupeSourceTables(probe.CandidateTables...)
		return result, nil
	}
	result.SourceTables = appendUniqueStrings(result.SourceTables, "contract_main")

	primaryCandidates := candidates
	if len(primaryCandidates) > 1 {
		primaryCandidates = primaryCandidates[:1]
	}
	ids := contractDetailInternalIDs(primaryCandidates)
	if probe.Intent == ContractDetailIntentInvoice {
		invoices, err := e.collectContractInvoicesForQuestion(ids, spec.NormalizedQuestion)
		if err != nil {
			return result, err
		}
		if len(invoices) > 0 {
			result.Invoices = invoices
			result.InvoiceSummary = summarizeContractInvoices(primaryCandidates, invoices)
			result.SourceTables = appendUniqueStrings(result.SourceTables, "contract_invoices")
		}
	}

	if probe.NeedsPageText || shouldCollectContractPagesForDetail(spec.NormalizedQuestion, probe.Intent, result) {
		pages, err := e.collectContractPages(ids)
		if err != nil {
			return result, err
		}
		snippets := extractContractPageSnippets(spec.NormalizedQuestion, pages)
		if len(snippets) > 0 {
			result.PageSnippets = snippets
			result.SourceTables = appendUniqueStrings(result.SourceTables, "contract_pages")
		}
	}

	result.SourceTables = dedupeSourceTables(result.SourceTables...)
	return result, nil
}

func shouldCollectContractPagesForDetail(question string, intent ContractDetailIntent, result ContractDetailResult) bool {
	if intent != ContractDetailIntentClause && intent != ContractDetailIntentField {
		return false
	}
	if len(result.MatchedContracts) == 0 {
		return false
	}
	first := result.MatchedContracts[0]
	if intent == ContractDetailIntentClause {
		return !contractDetailMainAnswersQuestion(question, first)
	}
	return !contractDetailMainAnswersQuestion(question, first)
}

func contractDetailMainAnswersQuestion(question string, match ContractDetailMatch) bool {
	q := strings.TrimSpace(question)
	switch {
	case containsAny(q, []string{"付款条款"}):
		return strings.TrimSpace(match.PaymentTerms) != ""
	case containsAny(q, []string{"付款方式"}):
		return strings.TrimSpace(match.PaymentMethod) != "" || strings.TrimSpace(match.PaymentTerms) != ""
	case containsAny(q, []string{"结算周期", "结算方式", "结算"}):
		return strings.TrimSpace(match.SettlementCycle) != "" || strings.TrimSpace(match.UnitPrice) != ""
	case containsAny(q, []string{"服务范围", "服务内容", "交付"}):
		return strings.TrimSpace(match.ServiceScope) != ""
	case containsAny(q, []string{"税率"}):
		return match.TaxRate != 0
	case containsAny(q, []string{"签署", "签约"}):
		return strings.TrimSpace(match.SignDate) != ""
	case containsAny(q, []string{"起止", "到期", "续约"}):
		return strings.TrimSpace(match.StartDate) != "" || strings.TrimSpace(match.EndDate) != ""
	default:
		return contractDetailHasMainDetail(match)
	}
}

func contractDetailMatchesFromCandidates(candidates []contractDetailCandidate) []ContractDetailMatch {
	out := make([]ContractDetailMatch, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, ContractDetailMatch{
			ContractNumber:  strings.TrimSpace(candidate.ContractNumber),
			ContractTitle:   strings.TrimSpace(candidate.ContractTitle),
			PartyA:          strings.TrimSpace(candidate.PartyA),
			PartyB:          strings.TrimSpace(candidate.PartyB),
			SignDate:        strings.TrimSpace(candidate.SignDate),
			StartDate:       strings.TrimSpace(candidate.StartDate),
			EndDate:         strings.TrimSpace(candidate.EndDate),
			ContractAmount:  candidate.ContractAmount,
			Currency:        strings.TrimSpace(candidate.Currency),
			SettlementCycle: strings.TrimSpace(candidate.SettlementCycle),
			UnitPrice:       strings.TrimSpace(candidate.UnitPrice),
			PaymentTerms:    strings.TrimSpace(candidate.PaymentTerms),
			PaymentMethod:   strings.TrimSpace(candidate.PaymentMethod),
			TaxRate:         candidate.TaxRate,
			ServiceScope:    strings.TrimSpace(candidate.ServiceScope),
			FileName:        strings.TrimSpace(candidate.FileName),
			UpdatedAt:       normalizeSourceUpdatedAt(candidate.UpdatedAt),
		})
	}
	return out
}

func (e *Engine) collectContractInvoicesForQuestion(ids []string, question string) ([]ContractInvoiceDetail, error) {
	invoices, err := e.collectContractInvoices(ids)
	if err != nil || len(invoices) == 0 {
		return invoices, err
	}
	exact := make([]ContractInvoiceDetail, 0, 1)
	for _, invoice := range invoices {
		number := strings.TrimSpace(invoice.InvoiceNumber)
		if number != "" && strings.Contains(question, number) {
			exact = append(exact, invoice)
		}
	}
	if len(exact) > 0 {
		return exact, nil
	}
	type scoredInvoice struct {
		invoice ContractInvoiceDetail
		score   int
	}
	scored := make([]scoredInvoice, 0, len(invoices))
	for _, invoice := range invoices {
		score := scoreContractInvoiceQuestionMatch(question, invoice)
		if score > 0 {
			scored = append(scored, scoredInvoice{invoice: invoice, score: score})
		}
	}
	if len(scored) == 0 {
		return invoices, nil
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].invoice.IssueDate > scored[j].invoice.IssueDate
		}
		return scored[i].score > scored[j].score
	})
	top := scored[0].score
	out := make([]ContractInvoiceDetail, 0, len(scored))
	for _, item := range scored {
		if item.score == top || item.score*2 >= top {
			out = append(out, item.invoice)
		}
	}
	return out, nil
}

func (e *Engine) collectContractInvoices(ids []string) ([]ContractInvoiceDetail, error) {
	if len(ids) == 0 || len(e.tableColumns("contract_invoices")) == 0 {
		return nil, nil
	}
	cols := e.tableColumns("contract_invoices")
	if !cols["contract_id"] {
		return nil, nil
	}
	selects := []string{
		"CAST(contract_id AS TEXT)",
		contractDetailTextSelectExpr(cols, "invoice_number"),
		contractDetailTextSelectExpr(cols, "issue_date"),
		contractDetailTextSelectExpr(cols, "buyer_name"),
		contractDetailTextSelectExpr(cols, "seller_name"),
		contractDetailNumericSelectExpr(cols, "total_amount_without_tax"),
		contractDetailNumericSelectExpr(cols, "total_tax_amount"),
		contractDetailNumericSelectExpr(cols, "total_amount"),
		contractDetailTextSelectExpr(cols, "remarks"),
		contractDetailTextSelectExpr(cols, "items_json"),
		contractDetailTextSelectExpr(cols, "file_name"),
		contractDetailTextSelectExpr(cols, "feishu_file_name"),
		contractDetailTextSelectExpr(cols, "storage_key"),
		contractDetailTextSelectExpr(cols, "updated_at"),
	}
	orderBy := "invoice_number"
	if cols["issue_date"] {
		orderBy = "issue_date DESC, invoice_number"
	}
	sqlText := fmt.Sprintf(
		"SELECT %s FROM contract_invoices WHERE CAST(contract_id AS TEXT) IN (%s) ORDER BY %s LIMIT 10",
		strings.Join(selects, ", "),
		contractDetailPlaceholders(len(ids)),
		orderBy,
	)
	rows, err := e.db.Query(sqlText, stringsToAny(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ContractInvoiceDetail, 0, 10)
	for rows.Next() {
		var item ContractInvoiceDetail
		var fileName, feishuFileName, storageKey string
		if err := rows.Scan(
			&item.ContractID,
			&item.InvoiceNumber,
			&item.IssueDate,
			&item.BuyerName,
			&item.SellerName,
			&item.TotalAmountWithoutTax,
			&item.TotalTaxAmount,
			&item.TotalAmount,
			&item.Remarks,
			&item.ItemsJSON,
			&fileName,
			&feishuFileName,
			&storageKey,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.FileName = preferredSourceFileName(fileName, feishuFileName, storageKey)
		item.UpdatedAt = normalizeSourceUpdatedAt(item.UpdatedAt)
		item.ItemsSummary = summarizeInvoiceItemsJSON(item.ItemsJSON)
		out = append(out, item)
	}
	return out, rows.Err()
}

func summarizeInvoiceItemsJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "{}" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		runes := []rune(raw)
		if len(runes) > 120 {
			return string(runes[:120])
		}
		return raw
	}
	items := make([]string, 0, 4)
	var walk func(any)
	walk = func(value any) {
		if len(items) >= 4 {
			return
		}
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				walk(item)
				if len(items) >= 4 {
					return
				}
			}
		case map[string]any:
			name := firstStringValue(typed, "name", "item", "item_name", "goods_name", "project_name", "货物或应税劳务名称", "项目名称", "名称")
			amount := firstNumberValue(typed, "amount", "total", "金额", "价税合计", "不含税金额")
			tax := firstNumberValue(typed, "tax", "tax_amount", "税额")
			if name == "" {
				for _, value := range typed {
					if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
						name = strings.TrimSpace(s)
						break
					}
				}
			}
			if name != "" {
				part := name
				if amount != 0 {
					part += fmt.Sprintf(" 金额%.2f", amount)
				}
				if tax != 0 {
					part += fmt.Sprintf(" 税额%.2f", tax)
				}
				items = append(items, part)
			}
		}
	}
	walk(decoded)
	return strings.Join(items, "；")
}

func firstStringValue(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstNumberValue(row map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			if number := anyToFloat64(value); number != 0 {
				return number
			}
		}
	}
	return 0
}

func summarizeContractInvoices(candidates []contractDetailCandidate, invoices []ContractInvoiceDetail) ContractInvoiceSummaryDetail {
	summary := ContractInvoiceSummaryDetail{InvoiceCount: len(invoices)}
	for _, invoice := range invoices {
		summary.TotalInvoicedAmount += invoice.TotalAmount
		summary.TotalTaxAmount += invoice.TotalTaxAmount
		if invoice.IssueDate > summary.LatestInvoiceDate {
			summary.LatestInvoiceDate = invoice.IssueDate
			summary.LatestInvoiceNumber = invoice.InvoiceNumber
		}
	}
	for _, candidate := range candidates {
		summary.ContractAmount += candidate.ContractAmount
	}
	if summary.ContractAmount != 0 {
		summary.InvoicedRatio = summary.TotalInvoicedAmount / summary.ContractAmount
	}
	return summary
}

func (e *Engine) collectContractPages(ids []string) ([]rawContractPage, error) {
	if len(ids) == 0 || len(e.tableColumns("contract_pages")) == 0 {
		return nil, nil
	}
	cols := e.tableColumns("contract_pages")
	if !cols["contract_id"] {
		return nil, nil
	}
	pageExpr := "0"
	switch {
	case cols["page_number"]:
		pageExpr = "COALESCE(page_number, 0)"
	case cols["page_num"]:
		pageExpr = "COALESCE(page_num, 0)"
	}
	selects := []string{
		pageExpr,
		contractDetailTextSelectExpr(cols, "markdown_text"),
		contractDetailTextSelectExpr(cols, "plain_text"),
	}
	sqlText := fmt.Sprintf(
		"SELECT %s FROM contract_pages WHERE CAST(contract_id AS TEXT) IN (%s) ORDER BY %s",
		strings.Join(selects, ", "),
		contractDetailPlaceholders(len(ids)),
		pageExpr,
	)
	rows, err := e.db.Query(sqlText, stringsToAny(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]rawContractPage, 0, 16)
	for rows.Next() {
		var pageNumber int
		var markdownText, plainText string
		if err := rows.Scan(&pageNumber, &markdownText, &plainText); err != nil {
			return nil, err
		}
		text := strings.TrimSpace(markdownText)
		if text == "" {
			text = strings.TrimSpace(plainText)
		}
		if text == "" {
			continue
		}
		out = append(out, rawContractPage{PageNumber: pageNumber, Text: text})
	}
	return out, rows.Err()
}

func extractContractPageSnippets(question string, pages []rawContractPage) []ContractPageSnippet {
	type scoredPage struct {
		page  rawContractPage
		score int
	}
	scored := make([]scoredPage, 0, len(pages))
	keywords := contractDetailQuestionKeywords(question)
	for _, page := range pages {
		score := 0
		for _, keyword := range keywords {
			if strings.Contains(page.Text, keyword) {
				score += contractDetailSnippetKeywordWeight(keyword)
			}
		}
		if score == 0 && len(scored) == 0 {
			score = 1
		}
		if score == 0 {
			continue
		}
		scored = append(scored, scoredPage{page: page, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].page.PageNumber < scored[j].page.PageNumber
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > 3 {
		scored = scored[:3]
	}
	out := make([]ContractPageSnippet, 0, len(scored))
	for _, item := range scored {
		out = append(out, ContractPageSnippet{
			PageNumber: item.page.PageNumber,
			Text:       trimContractPageSnippet(item.page.Text, keywords, 200),
		})
	}
	return out
}

func contractDetailSnippetKeywordWeight(keyword string) int {
	switch keyword {
	case "付款", "付款条款", "发票", "开票", "金额", "税额", "验收", "结算":
		return 25
	default:
		return 10
	}
}

func contractDetailQuestionKeywords(question string) []string {
	known := []string{"付款", "付款条款", "发票", "开票", "金额", "税额", "验收", "服务", "范围", "结算", "周期", "方式", "保密", "违约", "交付", "到期", "续约"}
	out := make([]string, 0, len(known)+4)
	for _, keyword := range known {
		if strings.Contains(question, keyword) {
			out = append(out, keyword)
		}
	}
	for _, token := range contractDetailMatchTokens(question) {
		if len([]rune(token)) >= 3 {
			out = append(out, token)
		}
	}
	return dedupeStrings(out)
}

func trimContractPageSnippet(text string, keywords []string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r", "\n"))
	for strings.Contains(text, "\n\n") {
		text = strings.ReplaceAll(text, "\n\n", "\n")
	}
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	start := 0
	for _, keyword := range keywords {
		if idx := strings.Index(text, keyword); idx >= 0 {
			start = idx
			break
		}
	}
	runes := []rune(text)
	runeStart := utf8.RuneCountInString(text[:start]) - 40
	if runeStart < 0 {
		runeStart = 0
	}
	end := runeStart + limit
	if end > len(runes) {
		end = len(runes)
	}
	snippet := strings.TrimSpace(string(runes[runeStart:end]))
	if runeStart > 0 {
		snippet = "..." + snippet
	}
	if end < len(runes) {
		snippet += "..."
	}
	return snippet
}

func contractDetailTextSelectExpr(cols map[string]bool, field string) string {
	if !cols[field] {
		return "''"
	}
	return "COALESCE(CAST(" + field + " AS TEXT), '')"
}

func contractDetailNumericSelectExpr(cols map[string]bool, field string) string {
	if !cols[field] {
		return "0"
	}
	return "COALESCE(" + field + ", 0)"
}

func contractDetailPlaceholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, "?")
	}
	return strings.Join(parts, ",")
}

func stringsToAny(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}

func scanNullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}
