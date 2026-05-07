package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

func buildSupplierPaymentSummary(data map[string]any) map[string]any {
	suppliers, _ := data["suppliers"].([]any)
	if len(suppliers) == 0 {
		return nil
	}
	summary := map[string]any{
		"kind":                     "supplier_payments_period_summary",
		"period":                   data["period"],
		"count":                    data["count"],
		"total":                    data["total"],
		"suppliers":                suppliers,
		"excluded_counterparties":  data["excluded_counterparties"],
		"supporting_evidence_used": true,
	}
	if first, ok := suppliers[0].(map[string]any); ok {
		summary["top_supplier"] = first
	}
	return summary
}

func buildBossReply(payload, data map[string]any) map[string]any {
	if summary := mapValue(payload["host_summary_supplier_payments"]); summary != nil {
		return map[string]any{
			"结论": fmt.Sprintf("%s 发生付款的外部供应商共 %s 家，合计 %s 元。", stringValue(summary["period"]), countString(summary["count"]), moneyString(summary["total"])),
			"原因": "供应商识别结合对手方证据与过滤规则复核。",
		}
	}
	if summary := mapValue(payload["host_summary_contract"]); summary != nil {
		return bossReplyFromContractSummary(summary)
	}
	if note := firstString(data["tax_inclusion_note"]); note != "" {
		return map[string]any{
			"结论": firstString(payload["message"], payload["final_answer"]),
			"原因": note,
		}
	}
	return nil
}

func bossReplyFromContractSummary(summary map[string]any) map[string]any {
	kind := firstString(summary["kind"])
	switch kind {
	case "counterparty_receipts_with_subperiod":
		return map[string]any{
			"结论": fmt.Sprintf("%s累计到账 %s 元；其中 %s 到账 %s 元。", entityPrefix(summary), moneyString(summary["total_amount"]), stringValue(summary["sub_period"]), moneyString(summary["sub_period_amount"])),
		}
	case "contract_dimension":
		return map[string]any{
			"结论": contractDimensionConclusion(summary),
			"原因": firstString(summary["source_note"], summary["source_summary"]),
		}
	case "contract_aggregate":
		return map[string]any{
			"结论": contractAggregateConclusion(summary),
			"原因": firstString(summary["source_summary"], summary["source_note"], strings.Join(stringSlice(summary["source_tables"]), "、")),
		}
	case "contract_strict_missing":
		reply := map[string]any{
			"结论": "合同口径当前不能直接回答：" + strings.TrimSuffix(firstString(summary["reason"], "合同/项目台账在请求期间没有足够记录"), "。") + "。",
			"原因": firstString(summary["source_note"], summary["continuity_note"]),
			"建议": "如需兜底口径，请明确要求改按账上、财务账或银行流水查询。",
		}
		if inference := continuityInference(summary); inference != "" {
			reply["结论"] = firstString(reply["结论"]) + "\n" + inference
		}
		return reply
	default:
		return nil
	}
}

func restoreHumanSourceNotes(summary, data map[string]any) {
	for _, key := range []string{"source_summary", "source_note"} {
		note := firstString(data[key])
		if note != "" && !containsTechnicalSource(note) {
			summary[key] = note
		}
	}
}

func contractDimensionConclusion(summary map[string]any) string {
	role := firstString(summary["role"])
	askedTopic := firstString(summary["asked_topic"])
	cashView := mapValue(summary["cash_view"])
	bookView := mapValue(summary["book_view"])
	switch role {
	case "supplier_contract":
		prefix := ""
		if askedTopic == "revenue" {
			prefix = "这是供应商合同，"
		}
		return prefix + fmt.Sprintf("合同付款 %s 元，合同成本 %s 元。", moneyFromMap(cashView, "cash_paid_amount", "付款"), moneyFromMap(bookView, "contract_cost", "cost_settlement", "合同成本"))
	case "mixed_contract":
		return fmt.Sprintf("合同到账 %s 元，合同付款 %s 元；收入结算 %s 元，合同成本 %s 元。",
			moneyFromMap(cashView, "received_amount", "到账"),
			moneyFromMap(cashView, "cash_paid_amount", "付款"),
			moneyFromMap(bookView, "revenue_settlement", "收入结算"),
			moneyFromMap(bookView, "cost_settlement", "contract_cost", "合同成本"),
		)
	default:
		caution := ""
		if askedTopic == "profit" && moneyFromMap(bookView, "contract_cost", "cost_settlement") == "0.00" {
			caution = "暂不能直接给完整合同利润；"
		}
		return caution + fmt.Sprintf("合同到账 %s 元，合同结算 %s 元，开票 %s 元。",
			moneyFromMap(cashView, "received_amount", "到账"),
			moneyFromMap(bookView, "settlement_amount", "revenue_settlement", "合同结算"),
			moneyFromMap(bookView, "invoice_amount", "开票"),
		)
	}
}

func contractAggregateConclusion(summary map[string]any) string {
	cashView := mapValue(summary["cash_view"])
	bookView := mapValue(summary["book_view"])
	parts := []string{}
	if v := moneyFromMap(cashView, "净现金", "net_cash"); v != "0.00" {
		parts = append(parts, "净现金 "+v+" 元")
	}
	if v := moneyFromMap(cashView, "到账", "回款", "received_amount"); v != "0.00" {
		parts = append(parts, "合同到账 "+v+" 元")
	}
	if v := moneyFromMap(cashView, "付款", "paid_amount", "cash_paid_amount"); v != "0.00" {
		parts = append(parts, "合同付款 "+v+" 元")
	}
	for _, key := range []string{"营收", "收入", "合同成本", "利润", "已开票未回款", "已收票未付款"} {
		if v := moneyFromMap(bookView, key); v != "0.00" {
			label := key
			if key == "营收" || key == "收入" {
				label = "合同营收"
			}
			if key == "利润" {
				label = "合同利润"
			}
			parts = append(parts, label+" "+v+" 元")
		}
	}
	if len(parts) == 0 {
		return firstString(summary["reason"], "合同/项目汇总结果已生成。")
	}
	if details := invoiceOpenItemsDetails(summary); details != "" {
		parts = append(parts, details)
	}
	return strings.Join(parts, "，") + "。"
}

func invoiceOpenItemsDetails(summary map[string]any) string {
	contractSummary := mapValue(summary["contract_summary"])
	if contractSummary == nil {
		return ""
	}
	items := mapSliceValue(contractSummary["invoice_open_items"])
	if len(items) == 0 {
		return ""
	}
	details := make([]string, 0, len(items))
	for _, item := range items {
		customer := firstString(item["customer_name"], item["counterparty"], item["name"])
		content := firstString(item["contract_content"], item["contract_title"], item["project_name"])
		open := moneyString(firstNonNilAny(item["open_amount"], item["unreceived_amount"], item["invoiced_unreceived_amount"]))
		if customer == "" && content == "" {
			continue
		}
		label := strings.Trim(strings.TrimSpace(customer+"-"+content), "-")
		details = append(details, fmt.Sprintf("%s 未回款 %s 元", label, open))
	}
	if len(details) == 0 {
		return ""
	}
	return "明细：" + strings.Join(details, "；")
}

func formatBossReply(reply map[string]any) string {
	parts := []string{}
	for _, key := range []string{"结论", "原因", "建议", "动作建议"} {
		if value := firstString(reply[key]); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n")
}

func appendSourceNotes(answer string, data map[string]any) string {
	parts := []string{strings.TrimSpace(answer)}
	for _, note := range []string{firstString(data["source_note"], data["source_summary"]), firstString(data["source_update_note"])} {
		if note == "" || containsTechnicalSource(note) || strings.Contains(answer, note) {
			continue
		}
		parts = append(parts, note)
	}
	return strings.Join(parts, "\n")
}

func containsTechnicalSource(value string) bool {
	return technicalTablePattern.MatchString(value) || strings.Contains(value, "SELECT ")
}

func entityPrefix(summary map[string]any) string {
	if entity := firstString(summary["entity"]); entity != "" {
		return entity + " "
	}
	return ""
}

func moneyFromMap(m map[string]any, keys ...string) string {
	if m == nil {
		return "0.00"
	}
	for _, key := range keys {
		if value, ok := m[key]; ok {
			return moneyString(value)
		}
	}
	return "0.00"
}

func moneyString(value any) string {
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%.2f", v)
	case int:
		return fmt.Sprintf("%.2f", float64(v))
	case json.Number:
		f, _ := v.Float64()
		return fmt.Sprintf("%.2f", f)
	case string:
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return "0.00"
}

func countString(value any) string {
	switch v := value.(type) {
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%.2f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return fmt.Sprintf("%d", i)
		}
		if f, err := v.Float64(); err == nil {
			return fmt.Sprintf("%.2f", f)
		}
	case string:
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return "0"
}

func continuityInference(summary map[string]any) string {
	candidates := mapSliceValue(summary["continuity_candidates"])
	if len(candidates) == 0 {
		return ""
	}
	total := 0.0
	for _, candidate := range candidates {
		total += floatValue(firstNonNilAny(candidate["candidate_received_amount"], candidate["received_amount"], candidate["amount"]))
	}
	if total == 0 {
		return "存在疑似同项目连续性候选；这是供宿主推断的参考，不是固定映射表。"
	}
	return fmt.Sprintf("基于连续性候选的疑似推断：同项目候选当前回款合计 %s 元；这是供宿主推断的参考，不是固定映射表。", moneyString(total))
}
