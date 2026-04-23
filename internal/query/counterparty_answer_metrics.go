package query

import (
	"fmt"
	"strings"
)

func tryCounterpartyRevenueAnswer(ctx counterpartyAuditContext) (Result, bool) {
	if !containsAny(ctx.q, []string{"销售额", "收入", "营收"}) {
		return Result{}, false
	}
	resultData := ctx.cloneResultData()
	if ctx.snap.RevenueNet > 0 {
		amount := round2(ctx.snap.RevenueNet)
		msg := fmt.Sprintf("[%s]%s %s 账上确认收入 %.2f 元（不含税）", ctx.entity, ctx.roleLabel, ctx.periodLabel, amount)
		if ctx.snap.ComparisonBasis == "vat_gap_only" {
			msg = fmt.Sprintf("[%s]%s %s 账上确认收入 %.2f 元（不含税）；银行累计到账 %.2f 元（含税）。两者差额 %.2f 元为增值税销项税，不应解释为未回款。", ctx.entity, ctx.roleLabel, ctx.periodLabel, amount, round2(ctx.snap.BankIn), round2(ctx.snap.BankIn-amount))
		} else if ctx.snap.ComparisonBasis == "historical_receipt_and_current_revenue" {
			msg = fmt.Sprintf("[%s]%s %s 账上确认收入 %.2f 元（不含税）；银行累计到账 %.2f 元（含税回款口径），其中包含历史应收回款，不能直接并成当期销售额。", ctx.entity, ctx.roleLabel, ctx.periodLabel, amount, round2(ctx.snap.BankIn))
		} else if ctx.taxReport.Output.Included && ctx.taxReport.Output.TaxAmount > 0 && approxEqual(ctx.snap.BankIn, ctx.snap.RevenueNet+ctx.snap.OutputVAT) {
			msg = fmt.Sprintf("[%s]%s %s 账上确认收入 %.2f 元（不含税）；银行累计到账 %.2f 元（含税）。两者差额 %.2f 元主要是增值税销项税。", ctx.entity, ctx.roleLabel, ctx.periodLabel, amount, round2(ctx.snap.BankIn), round2(ctx.taxReport.Output.TaxAmount))
		}
		resultData["amount"] = amount
		resultData["total"] = amount
		return annotateJournalTaxDisclosure(Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: ctx.cloneSQLs(), CalculationLogs: ctx.cloneLogs()}, true), true
	}
	if ctx.snap.BankIn > 0 {
		amount := round2(ctx.snap.BankIn)
		msg := fmt.Sprintf("[%s]%s %s 仅看到到账 %.2f 元，暂未看到同期间收入确认分录。", ctx.entity, ctx.roleLabel, ctx.periodLabel, amount)
		resultData["amount"] = amount
		resultData["total"] = amount
		return Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: ctx.cloneSQLs(), CalculationLogs: ctx.cloneLogs()}, true
	}
	return Result{}, false
}

func tryCounterpartyEmployeeExpenseAnswer(ctx counterpartyAuditContext) (Result, bool) {
	if ctx.role != "employee" && !strings.Contains(ctx.q, "报销") {
		return Result{}, false
	}
	amount := round2(ctx.snap.BankOut)
	if amount == 0 {
		amount = round2(ctx.snap.BookExpense + ctx.snap.BookCost)
	}
	if amount <= 0 {
		return Result{}, false
	}
	resultData := ctx.cloneResultData()
	resultData["amount"] = amount
	resultData["total"] = amount
	msg := fmt.Sprintf("[%s]%s %s 报销/费用 %.2f 元", ctx.entity, ctx.roleLabel, ctx.periodLabel, amount)
	return Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: ctx.cloneSQLs(), CalculationLogs: ctx.cloneLogs()}, true
}

func tryCounterpartyCostAnswer(ctx counterpartyAuditContext) (Result, bool) {
	if !containsAny(ctx.q, []string{"成本", "费用", "支出", "付款"}) {
		return Result{}, false
	}
	amount := round2(ctx.snap.BookCost + ctx.snap.BookExpense)
	label := "账上成本/费用"
	if containsAny(ctx.q, []string{"付款", "付了", "支付"}) || amount == 0 {
		amount = round2(ctx.snap.BankOut)
		label = "付款"
	}
	if amount <= 0 {
		return Result{}, false
	}
	resultData := ctx.cloneResultData()
	msg := fmt.Sprintf("[%s]%s %s %s %.2f 元", ctx.entity, ctx.roleLabel, ctx.periodLabel, label, amount)
	if ctx.role == "supplier" || ctx.role == "mixed" {
		if label == "付款" && containsAny(ctx.q, []string{"成本", "费用"}) {
			msg = fmt.Sprintf("[%s]%s %s 属于供应商相关。当前账上未匹配到同期间成本/费用分录，先按付款口径识别到 %.2f 元，不应归到收入差异里。", ctx.entity, ctx.roleLabel, ctx.periodLabel, amount)
		} else {
			msg = fmt.Sprintf("[%s]%s %s 属于供应商相关，%s %.2f 元，不应归到收入差异里。", ctx.entity, ctx.roleLabel, ctx.periodLabel, label, amount)
		}
	}
	resultData["amount"] = amount
	resultData["total"] = amount
	if label == "付款" {
		resultData["payment"] = amount
	} else {
		resultData["cost"] = amount
	}
	return annotateJournalTaxDisclosure(Result{Success: true, Message: msg, Data: resultData, ExecutedSQL: ctx.cloneSQLs(), CalculationLogs: ctx.cloneLogs()}, label != "付款"), true
}
