package query

import (
	"fmt"
	"strings"
)

func tryCounterpartyClassificationAnswer(ctx counterpartyAuditContext) (Result, bool) {
	reasonParts := make([]string, 0, 4)
	if ctx.snap.BookCost+ctx.snap.BookExpense > 0 {
		reasonParts = append(reasonParts, fmt.Sprintf("账上成本/费用 %.2f 元", round2(ctx.snap.BookCost+ctx.snap.BookExpense)))
	}
	if ctx.snap.RevenueNet > 0 {
		reasonParts = append(reasonParts, fmt.Sprintf("账上收入 %.2f 元", round2(ctx.snap.RevenueNet)))
	}
	if ctx.snap.BankOut > 0 {
		reasonParts = append(reasonParts, fmt.Sprintf("银行付款 %.2f 元", round2(ctx.snap.BankOut)))
	}
	if ctx.snap.BankIn > 0 {
		reasonParts = append(reasonParts, fmt.Sprintf("银行收款 %.2f 元", round2(ctx.snap.BankIn)))
	}
	reason := strings.Join(reasonParts, "；")
	switch ctx.role {
	case "supplier":
		data := ctx.cloneResultData()
		data["basis"] = reasonParts
		return Result{
			Success:         true,
			Message:         fmt.Sprintf("[%s]%s %s 判断为供应商/成本侧往来。%s。", ctx.entity, ctx.roleLabel, ctx.periodLabel, reason),
			Data:            data,
			ExecutedSQL:     ctx.cloneSQLs(),
			CalculationLogs: ctx.cloneLogs(),
		}, true
	case "customer":
		data := ctx.cloneResultData()
		data["basis"] = reasonParts
		return Result{
			Success:         true,
			Message:         fmt.Sprintf("[%s]%s %s 判断为客户/收入侧往来。%s。", ctx.entity, ctx.roleLabel, ctx.periodLabel, reason),
			Data:            data,
			ExecutedSQL:     ctx.cloneSQLs(),
			CalculationLogs: ctx.cloneLogs(),
		}, true
	case "mixed":
		data := ctx.cloneResultData()
		data["basis"] = reasonParts
		return Result{
			Success:         true,
			Message:         fmt.Sprintf("[%s]%s %s 判断为混合往来。%s。", ctx.entity, ctx.roleLabel, ctx.periodLabel, reason),
			Data:            data,
			ExecutedSQL:     ctx.cloneSQLs(),
			CalculationLogs: ctx.cloneLogs(),
		}, true
	}
	return Result{}, false
}
