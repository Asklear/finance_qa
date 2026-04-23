package query

import (
	"fmt"
	"strings"
)

func (e *Engine) tryCounterpartyReceiptsAnswer(ctx counterpartyAuditContext) (Result, bool) {
	if !containsAny(ctx.q, []string{"回款", "到账", "收款"}) || containsAny(ctx.q, []string{"预收款", "应收款"}) {
		return Result{}, false
	}
	amount := round2(ctx.snap.BankIn)
	if amount == 0 {
		return Result{Success: false, Message: fmt.Sprintf("[%s] 在 %s 未找到回款/到账记录", ctx.entity, ctx.periodLabel)}, true
	}
	resultData := ctx.cloneResultData()
	logs := ctx.cloneLogs()
	msg := buildCounterpartyReceiptsMessage(ctx, amount, "", 0)
	if subPeriod, ok := extractReceiptSubPeriod(ctx.q, ctx.from, ctx.to); ok {
		subAmount := round2(e.counterpartyBankReceipts(ctx.entity, subPeriod, subPeriod))
		msg = buildCounterpartyReceiptsMessage(ctx, amount, subPeriod, subAmount)
		resultData["sub_period"] = subPeriod
		resultData["sub_period_receipts"] = subAmount
		logs = append(logs, fmt.Sprintf("[回款拆分] cumulative=%s amount=%.2f sub_period=%s sub_amount=%.2f", ctx.periodLabel, amount, subPeriod, subAmount))
	}
	resultData["amount"] = amount
	resultData["total"] = amount
	return Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: ctx.cloneSQLs(), CalculationLogs: logs}, true
}

func buildCounterpartyReceiptsMessage(ctx counterpartyAuditContext, amount float64, subPeriod string, subAmount float64) string {
	periodLead := normalizeCounterpartyReceiptPeriodLead(ctx.receiptPeriodLabel)
	msg := fmt.Sprintf("[%s]%s%s到账（银行含税口径） %.2f 元", ctx.entity, ctx.roleLabel, periodLead, amount)
	if ctx.snap.ComparisonBasis == "historical_receipt" || ctx.snap.ComparisonBasis == "historical_receipt_and_current_revenue" {
		msg = fmt.Sprintf("[%s]%s%s到账（银行含税口径） %.2f 元。数据库能确认这类到账包含历史应收回款因素，不能直接当成当期新收入。", ctx.entity, ctx.roleLabel, periodLead, amount)
	}
	if subPeriod == "" {
		return msg
	}
	msg = fmt.Sprintf("[%s]%s%s到账（银行含税口径） %.2f 元；其中%s到账 %.2f 元", ctx.entity, ctx.roleLabel, periodLead, amount, displaySubPeriodLabel(subPeriod), subAmount)
	if ctx.snap.ComparisonBasis == "historical_receipt" || ctx.snap.ComparisonBasis == "historical_receipt_and_current_revenue" {
		msg = fmt.Sprintf("[%s]%s%s到账（银行含税口径） %.2f 元；其中%s到账 %.2f 元。数据库能确认这类到账包含历史应收回款因素，不能直接当成当期新收入。", ctx.entity, ctx.roleLabel, periodLead, amount, displaySubPeriodLabel(subPeriod), subAmount)
	}
	return msg
}

func normalizeCounterpartyReceiptPeriodLead(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if label == "今年" || label == "本年" {
		return label
	}
	return " " + label + " "
}
