package query

import (
	"fmt"
	"strings"

	"financeqa/internal/accounting"
)

func (e *Engine) composeBossReconciliationMessage(period string, book monthlyBookView, bookSource string, cash *accounting.CashPerspective, highlights []counterpartySnapshot) string {
	lines := []string{
		fmt.Sprintf("%s 我拆成两层给你看：账上看收入 %.2f 元、成本及费用 %.2f 元、净利润 %.2f 元；银行卡上看收款 %.2f 元、付款 %.2f 元，净流入 %.2f 元。", period, book.Revenue, book.TotalCost, book.NetProfit, cash.Income, cash.Expense, cash.Net),
	}
	if bookSource == "income_statement" {
		lines = append(lines, "账上这组数优先取利润表当月发生额，银行卡这组数取当月真实收付。两边不是同一个口径，所以不能直接拿来做差。")
	}
	if len(highlights) > 0 {
		lines = append(lines, "差异主要来自这几类：")
	}
	for _, snap := range highlights {
		switch snap.ComparisonBasis {
		case "historical_receipt_and_current_revenue":
			lines = append(lines, fmt.Sprintf("1. %s：本月既有到账 %.2f 元，也有账上确认收入 %.2f 元和销项税 %.2f 元。库里能确认这是“历史应收回款 + 当月新确认收入”同时出现，不能把两笔直接相减；现有库里也看不出到账对应的是哪一个结算月份。", snap.Name, snap.BankIn, snap.RevenueNet, snap.OutputVAT))
		case "vat_gap_only":
			lines = append(lines, fmt.Sprintf("1. %s：到账 %.2f 元，账上收入 %.2f 元，差额 %.2f 元主要就是销项税，不是业务多赚少赚。", snap.Name, snap.BankIn, snap.RevenueNet, snap.OutputVAT))
		case "supplier_payment_or_cost":
			lines = append(lines, fmt.Sprintf("1. %s：这是供应商相关付款和成本确认。2 月付款 %.2f 元，账上成本/费用 %.2f 元，进项税 %.2f 元，不该放进收入差异里。", snap.Name, snap.BankOut, snap.BookCost+snap.BookExpense, snap.InputVAT))
		case "historical_receipt":
			lines = append(lines, fmt.Sprintf("1. %s：这笔到账 %.2f 元能确认是在冲历史应收，但数据库没有字段直接说明对应哪一个结算月份，所以最多只能说是历史回款。", snap.Name, snap.BankIn))
		case "recognized_revenue":
			lines = append(lines, fmt.Sprintf("1. %s：本月账上确认收入 %.2f 元。", snap.Name, snap.RevenueNet))
		}
	}
	if len(highlights) > 0 {
		lines = append(lines, "如果你要继续追问“这笔到底对应哪一月结算”，下一步得补结算单、开票记录或合同台账，单靠当前数据库里的财务表不能硬判。")
	}
	return strings.Join(lines, "\n")
}
