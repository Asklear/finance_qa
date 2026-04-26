package query

import (
	"fmt"
	"regexp"
	"strings"
)

func buildContractDetailFactSet(spec QuerySpec, detail ContractDetailResult) FactSet {
	data := contractDetailResultData(detail)
	return FactSet{
		Source: "contract_detail",
		Facts: []Fact{
			{
				Source:         "contract_detail",
				MetricKey:      "contract_detail",
				Entity:         firstContractDetailTitle(detail),
				PeriodFrom:     spec.PeriodFrom,
				PeriodTo:       spec.PeriodTo,
				Value:          float64(len(detail.MatchedContracts)),
				AuthorityLevel: AuthoritySupporting,
				CoverageStatus: contractDetailCoverage(detail),
				Confidence:     detail.Probe.Confidence,
				TracePayload: map[string]any{
					"data":   sanitizeContractDetailPayload(data),
					"tables": append([]string{}, detail.SourceTables...),
					"logs":   []string{"[合同明细] 使用 contract_* 表探测并采集合约/发票细节"},
				},
			},
		},
	}
}

func composeContractDetailResult(frame AnswerFrame) (Result, error) {
	factSet, ok := findFactSetBySource(frame.FactSets, "contract_detail")
	if !ok || len(factSet.Facts) == 0 {
		return Result{}, fmt.Errorf("contract detail fact set missing")
	}
	trace := factSet.Facts[0].TracePayload
	data, _ := trace["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	data = cloneAnyMap(data)
	data["fact_sets"] = frame.FactSets
	data["query_pipeline"] = "orchestrator"
	data["source_plan"] = sourceCapabilitiesToStrings(frame.Plan.Capabilities)
	data = sanitizeContractDetailPayload(data).(map[string]any)
	return Result{
		Success:         true,
		Message:         buildContractDetailMessage(frame.Spec, data),
		Data:            data,
		ExecutedSQL:     nil,
		CalculationLogs: extractFrameTraceStrings(frame.FactSets, "logs"),
	}, nil
}

func contractDetailCoverage(detail ContractDetailResult) CoverageStatus {
	if len(detail.MatchedContracts) == 0 {
		return CoverageMissing
	}
	switch detail.Probe.Intent {
	case ContractDetailIntentInvoice:
		if len(detail.Invoices) == 0 && detail.InvoiceSummary.TotalInvoicedAmount == 0 {
			return CoveragePartial
		}
	case ContractDetailIntentClause, ContractDetailIntentField, ContractDetailIntentPage:
		if len(detail.PageSnippets) == 0 && !contractDetailHasMainDetail(detail.MatchedContracts[0]) {
			return CoveragePartial
		}
	}
	return CoverageFull
}

func contractDetailHasMainDetail(match ContractDetailMatch) bool {
	return strings.TrimSpace(match.PaymentTerms) != "" ||
		strings.TrimSpace(match.PaymentMethod) != "" ||
		strings.TrimSpace(match.SettlementCycle) != "" ||
		strings.TrimSpace(match.ServiceScope) != "" ||
		match.ContractAmount != 0
}

func contractDetailResultData(detail ContractDetailResult) map[string]any {
	data := map[string]any{
		"intent":        string(detail.Probe.Intent),
		"source_tables": append([]string{}, detail.SourceTables...),
		"source_note":   contractDetailSourceNote(detail.SourceTables),
		"contracts":     contractDetailMatchesData(detail.MatchedContracts),
		"probe": map[string]any{
			"intent":                   string(detail.Probe.Intent),
			"candidate_tables":         append([]string{}, detail.Probe.CandidateTables...),
			"matched_contract_rows":    detail.Probe.MatchedContractRows,
			"has_structured_answer":    detail.Probe.HasStructuredAnswer,
			"needs_page_text":          detail.Probe.NeedsPageText,
			"confidence":               detail.Probe.Confidence,
			"business_matching_reason": detail.Probe.Reason,
		},
	}
	if detail.InvoiceSummary.InvoiceCount > 0 || detail.InvoiceSummary.TotalInvoicedAmount != 0 {
		data["invoice_summary"] = contractInvoiceSummaryData(detail.InvoiceSummary)
	}
	if len(detail.Invoices) > 0 {
		data["invoices"] = contractInvoicesData(detail.Invoices)
	}
	if len(detail.PageSnippets) > 0 {
		data["page_snippets"] = contractPageSnippetsData(detail.PageSnippets)
	}
	return data
}

func contractDetailMatchesData(matches []ContractDetailMatch) []map[string]any {
	out := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		row := map[string]any{
			"contract_title":   match.ContractTitle,
			"party_a":          match.PartyA,
			"party_b":          match.PartyB,
			"sign_date":        match.SignDate,
			"start_date":       match.StartDate,
			"end_date":         match.EndDate,
			"contract_amount":  match.ContractAmount,
			"currency":         match.Currency,
			"settlement_cycle": match.SettlementCycle,
			"unit_price":       match.UnitPrice,
			"payment_terms":    match.PaymentTerms,
			"payment_method":   match.PaymentMethod,
			"tax_rate":         match.TaxRate,
			"service_scope":    match.ServiceScope,
			"file_name":        match.FileName,
		}
		out = append(out, row)
	}
	return out
}

func contractInvoiceSummaryData(summary ContractInvoiceSummaryDetail) map[string]any {
	return map[string]any{
		"invoice_count":         summary.InvoiceCount,
		"total_invoiced_amount": summary.TotalInvoicedAmount,
		"total_tax_amount":      summary.TotalTaxAmount,
		"contract_amount":       summary.ContractAmount,
		"invoiced_ratio":        summary.InvoicedRatio,
		"latest_invoice_date":   summary.LatestInvoiceDate,
		"latest_invoice_number": summary.LatestInvoiceNumber,
	}
}

func contractInvoicesData(invoices []ContractInvoiceDetail) []map[string]any {
	out := make([]map[string]any, 0, len(invoices))
	for _, invoice := range invoices {
		out = append(out, map[string]any{
			"invoice_number":           invoice.InvoiceNumber,
			"issue_date":               invoice.IssueDate,
			"buyer_name":               invoice.BuyerName,
			"seller_name":              invoice.SellerName,
			"total_amount_without_tax": invoice.TotalAmountWithoutTax,
			"total_tax_amount":         invoice.TotalTaxAmount,
			"total_amount":             invoice.TotalAmount,
			"remarks":                  invoice.Remarks,
		})
	}
	return out
}

func contractPageSnippetsData(snippets []ContractPageSnippet) []map[string]any {
	out := make([]map[string]any, 0, len(snippets))
	for _, snippet := range snippets {
		out = append(out, map[string]any{
			"page_number": snippet.PageNumber,
			"text":        snippet.Text,
		})
	}
	return out
}

func buildContractDetailMessage(spec QuerySpec, data map[string]any) string {
	intent := ContractDetailIntent(strings.TrimSpace(anyToString(data["intent"])))
	contracts := anyToMapSlice(data["contracts"])
	if len(contracts) == 0 {
		return "当前没有在合同明细库里匹配到这份合同。可以再给我合同名称、签约方或文件名中的关键词。"
	}
	title := strings.TrimSpace(anyToString(contracts[0]["contract_title"]))
	if title == "" {
		title = "匹配合同"
	}
	switch intent {
	case ContractDetailIntentInvoice:
		return buildContractDetailInvoiceMessage(title, data)
	case ContractDetailIntentClause, ContractDetailIntentField, ContractDetailIntentPage:
		return buildContractDetailClauseMessage(title, contracts[0], data)
	default:
		if strings.Contains(spec.NormalizedQuestion, "发票") {
			return buildContractDetailInvoiceMessage(title, data)
		}
		return buildContractDetailClauseMessage(title, contracts[0], data)
	}
}

func buildContractDetailClauseMessage(title string, contract map[string]any, data map[string]any) string {
	parts := []string{"【" + title + "】合同明细："}
	if value := strings.TrimSpace(anyToString(contract["payment_terms"])); value != "" {
		parts = append(parts, "付款条款："+value)
	}
	if value := strings.TrimSpace(anyToString(contract["payment_method"])); value != "" {
		parts = append(parts, "付款方式："+value)
	}
	if value := strings.TrimSpace(anyToString(contract["settlement_cycle"])); value != "" {
		parts = append(parts, "结算周期："+value)
	}
	if value := strings.TrimSpace(anyToString(contract["service_scope"])); value != "" {
		parts = append(parts, "服务范围："+value)
	}
	snippets := anyToMapSlice(data["page_snippets"])
	for _, snippet := range snippets {
		text := strings.TrimSpace(anyToString(snippet["text"]))
		if text == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("正文摘录：第%d页 %s", int(anyToFloat64(snippet["page_number"])), text))
		break
	}
	if len(parts) == 1 {
		parts = append(parts, "结构化字段里没有找到对应条款，分页正文里也没有匹配到可摘录内容。")
	}
	if note := strings.TrimSpace(anyToString(data["source_note"])); note != "" {
		parts = append(parts, note)
	}
	return sanitizeContractDetailText(strings.Join(parts, "\n"))
}

func buildContractDetailInvoiceMessage(title string, data map[string]any) string {
	parts := []string{"【" + title + "】发票情况："}
	if summary, ok := data["invoice_summary"].(map[string]any); ok {
		summaryParts := []string{}
		if count := int(anyToFloat64(summary["invoice_count"])); count > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("发票%d张", count))
		}
		if amount := anyToFloat64(summary["total_invoiced_amount"]); amount != 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("累计开票%.2f元", amount))
		}
		if tax := anyToFloat64(summary["total_tax_amount"]); tax != 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("税额%.2f元", tax))
		}
		if date := strings.TrimSpace(anyToString(summary["latest_invoice_date"])); date != "" {
			summaryParts = append(summaryParts, "最近开票日期"+date)
		}
		if len(summaryParts) > 0 {
			parts = append(parts, strings.Join(summaryParts, "，")+"。")
		}
	}
	invoices := anyToMapSlice(data["invoices"])
	for _, invoice := range invoices {
		parts = append(parts, fmt.Sprintf(
			"明细：%s，发票号%s，含税%.2f元，税额%.2f元。",
			anyToString(invoice["issue_date"]),
			anyToString(invoice["invoice_number"]),
			anyToFloat64(invoice["total_amount"]),
			anyToFloat64(invoice["total_tax_amount"]),
		))
	}
	if len(parts) == 1 {
		parts = append(parts, "合同明细库里没有找到这份合同对应的发票记录。")
	}
	if note := strings.TrimSpace(anyToString(data["source_note"])); note != "" {
		parts = append(parts, note)
	}
	return sanitizeContractDetailText(strings.Join(parts, "\n"))
}

func contractDetailSourceNote(tables []string) string {
	labels := make([]string, 0, len(tables))
	for _, table := range tables {
		switch baseSourceTableName(table) {
		case "contract_main":
			labels = append(labels, "合同主表")
		case "contract_invoices":
			labels = append(labels, "合同发票明细")
		case "contract_invoice_summaries":
			labels = append(labels, "合同发票汇总")
		case "contract_pages":
			labels = append(labels, "合同分页正文")
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return "来源：" + strings.Join(dedupeStrings(labels), "、")
}

func firstContractDetailTitle(detail ContractDetailResult) string {
	if len(detail.MatchedContracts) == 0 {
		return ""
	}
	return detail.MatchedContracts[0].ContractTitle
}

func sanitizeContractDetailPayload(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if isContractDetailSensitiveKey(normalized) {
				continue
			}
			out[key] = sanitizeContractDetailPayload(item)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if sanitized, ok := sanitizeContractDetailPayload(item).(map[string]any); ok {
				out = append(out, sanitized)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeContractDetailPayload(item))
		}
		return out
	case []FactSet:
		out := make([]FactSet, 0, len(typed))
		for _, factSet := range typed {
			factSet.Facts = sanitizeContractDetailFacts(factSet.Facts)
			out = append(out, factSet)
		}
		return out
	case string:
		return sanitizeContractDetailText(typed)
	default:
		return typed
	}
}

func sanitizeContractDetailFacts(facts []Fact) []Fact {
	out := make([]Fact, 0, len(facts))
	for _, fact := range facts {
		fact.TracePayload = sanitizeContractDetailPayload(fact.TracePayload).(map[string]any)
		out = append(out, fact)
	}
	return out
}

func isContractDetailSensitiveKey(key string) bool {
	switch key {
	case "id", "contract_id", "contract_ids", "page_id", "existing_contract_id", "storage_key", "file_hash", "job_id", "raw_ocr_json", "items_json":
		return true
	default:
		return false
	}
}

func sanitizeContractDetailText(text string) string {
	out := text
	for _, token := range []string{"contract_id", "contract_ids", "page_id", "storage_key", "file_hash", "job_id", "raw_ocr_json", "executed_sql"} {
		out = strings.ReplaceAll(out, token, "")
	}
	out = regexp.MustCompile(`C\d{3,}`).ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}
