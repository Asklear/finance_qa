package query

import (
	"fmt"
	"math"
)

func buildCounterpartyFallbackAnswer(ctx counterpartyAuditContext) Result {
	fallbackAmount := round2(math.Max(ctx.snap.BankIn, math.Max(ctx.snap.BankOut, math.Max(ctx.snap.RevenueNet, ctx.snap.BookCost+ctx.snap.BookExpense))))
	if fallbackAmount == 0 {
		return Result{Success: false, Message: fmt.Sprintf("穿透审计失败：[%s] 无发生额", ctx.entity)}
	}
	resultData := ctx.cloneResultData()
	if (ctx.role == "supplier" || ctx.role == "mixed") && containsAny(ctx.q, []string{"成本", "费用", "支出", "付款"}) {
		amount := round2(math.Max(ctx.snap.BankOut, fallbackAmount))
		msg := fmt.Sprintf("[%s]%s %s 属于供应商相关，当前可确认付款 %.2f 元。若问的是账面成本/费用，现有库里未匹配到同期间分录，所以先按付款口径回答。", ctx.entity, ctx.roleLabel, ctx.periodLabel, amount)
		resultData["amount"] = amount
		resultData["total"] = amount
		resultData["payment"] = amount
		resultData["comparison_basis"] = "payment_fallback_due_missing_book_cost"
		return Result{
			Success:         true,
			Message:         msg,
			Data:            resultData,
			ExecutedSQL:     ctx.cloneSQLs(),
			CalculationLogs: ctx.cloneLogs(),
		}
	}
	resultData["amount"] = fallbackAmount
	resultData["total"] = fallbackAmount
	return Result{
		Success:         true,
		Message:         fmt.Sprintf("[%s]%s 已提取相关发生额 %.2f 元", ctx.entity, ctx.roleLabel, fallbackAmount),
		Data:            resultData,
		ExecutedSQL:     ctx.cloneSQLs(),
		CalculationLogs: ctx.cloneLogs(),
	}
}
