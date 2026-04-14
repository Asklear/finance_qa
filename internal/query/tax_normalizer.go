package query

import (
	"fmt"
	"math"
	"strings"
)

// TaxSide 表示税额归因的方向。
type TaxSide string

const (
	TaxSideOutput TaxSide = "output"
	TaxSideInput  TaxSide = "input"
)

// TaxBreakdown 描述某一侧税额的现金、权责与税额拆分。
type TaxBreakdown struct {
	Side             TaxSide          `json:"side"`
	Counterparty     string           `json:"counterparty,omitempty"`
	Role             CounterpartyRole `json:"role"`
	Included         bool             `json:"included"`
	CashAmount       float64          `json:"cash_amount"`
	AccrualAmount    float64          `json:"accrual_amount"`
	TaxAmount        float64          `json:"tax_amount"`
	DifferenceReason string           `json:"difference_reason,omitempty"`
	Signals          []string         `json:"signals,omitempty"`
}

// TaxNormalizationReport 同时给出销项和进项的拆分。
type TaxNormalizationReport struct {
	Counterparty string           `json:"counterparty,omitempty"`
	Role         CounterpartyRole `json:"role"`
	Output       TaxBreakdown     `json:"output"`
	Input        TaxBreakdown     `json:"input"`
}

// NormalizeTax 会同时产出销项和进项口径，便于主线程在 engine 层复用。
func NormalizeTax(counterparty string, evidence []LedgerEvidence) TaxNormalizationReport {
	classification := ClassifyCounterparty(counterparty, evidence)
	return TaxNormalizationReport{
		Counterparty: counterparty,
		Role:         classification.Role,
		Output:       normalizeTaxSide(TaxSideOutput, counterparty, classification.Role, evidence),
		Input:        normalizeTaxSide(TaxSideInput, counterparty, classification.Role, evidence),
	}
}

func NormalizeOutputTax(counterparty string, evidence []LedgerEvidence) TaxBreakdown {
	return normalizeTaxSide(TaxSideOutput, counterparty, ClassifyCounterparty(counterparty, evidence).Role, evidence)
}

func NormalizeInputTax(counterparty string, evidence []LedgerEvidence) TaxBreakdown {
	return normalizeTaxSide(TaxSideInput, counterparty, ClassifyCounterparty(counterparty, evidence).Role, evidence)
}

func normalizeTaxSide(side TaxSide, counterparty string, role CounterpartyRole, evidence []LedgerEvidence) TaxBreakdown {
	var cashIn, cashOut, accrual, tax float64
	signals := make([]string, 0, len(evidence)*2)

	for _, ev := range evidence {
		text := normalizeEntityText(strings.Join([]string{
			ev.Source, ev.Counterparty, ev.AccountCode, ev.AccountName, ev.Summary, ev.Direction, ev.TransactionType,
		}, " "))
		if text == "" {
			continue
		}

		if ev.Source == "bank_statement" {
			cashIn += math.Max(ev.CreditAmount, 0)
			cashOut += math.Max(ev.DebitAmount, 0)
			continue
		}

		if ev.Source != "journal" {
			continue
		}

		switch side {
		case TaxSideOutput:
			if isRevenueEvidence(text, ev) {
				accrual += chooseAmount(ev)
				signals = append(signals, "revenue:"+pickFirstHit(text, customerKeywords))
			}
			if isOutputTaxEvidence(text, ev) {
				tax += chooseAmount(ev)
				signals = append(signals, "output_tax:"+pickFirstHit(text, outputTaxKeywords))
			}
		case TaxSideInput:
			if isCostEvidence(text, ev) {
				accrual += chooseAmount(ev)
				signals = append(signals, "cost:"+pickFirstHit(text, supplierKeywords))
			}
			if isInputTaxEvidence(text, ev) {
				tax += chooseAmount(ev)
				signals = append(signals, "input_tax:"+pickFirstHit(text, inputTaxKeywords))
			}
		}
	}

	breakdown := TaxBreakdown{
		Side:          side,
		Counterparty:  counterparty,
		Role:          role,
		CashAmount:    round2(selectCashAmount(side, cashIn, cashOut)),
		AccrualAmount: round2(accrual),
		TaxAmount:     round2(tax),
		Signals:       dedupeSignals(signals),
	}
	breakdown.Included = sideInclusionAllowed(side, role)
	breakdown.DifferenceReason = explainDifference(breakdown, evidence)
	return breakdown
}

func selectCashAmount(side TaxSide, cashIn, cashOut float64) float64 {
	if side == TaxSideOutput {
		return cashIn - cashOut
	}
	return cashOut - cashIn
}

func sideInclusionAllowed(side TaxSide, role CounterpartyRole) bool {
	switch side {
	case TaxSideOutput:
		return role == CounterpartyCustomer || role == CounterpartyMixed || role == CounterpartyUnknown
	case TaxSideInput:
		return role == CounterpartySupplier || role == CounterpartyEmployee || role == CounterpartyMixed || role == CounterpartyUnknown
	default:
		return true
	}
}

func explainDifference(b TaxBreakdown, evidence []LedgerEvidence) string {
	if b.Side == TaxSideOutput && !b.Included {
		return "供应商付款或成本相关，不纳入收入差异列表"
	}
	if b.Side == TaxSideInput && !b.Included {
		return "客户回款不属于进项税差异"
	}

	if hasARCollectionSignals(evidence) && b.Side == TaxSideOutput {
		return "历史应收回款，现金收款包含往年应收，不是本月确认收入"
	}
	if b.Side == TaxSideOutput && b.TaxAmount > 0 {
		return "差额主要由销项税额构成"
	}
	if b.Role == CounterpartySupplier && b.Side == TaxSideInput {
		return "供应商付款或成本确认，差额主要由进项税额构成"
	}
	if b.Role == CounterpartyEmployee && b.Side == TaxSideInput {
		return "员工报销或薪酬相关，差额主要由进项税额或费用构成"
	}
	if b.Side == TaxSideInput && b.TaxAmount > 0 {
		return "差额主要由进项税额构成"
	}
	if len(evidence) == 0 {
		return "证据不足"
	}
	return "现金、权责和税额口径存在差异"
}

func hasARCollectionSignals(evidence []LedgerEvidence) bool {
	for _, ev := range evidence {
		text := normalizeEntityText(strings.Join([]string{ev.AccountCode, ev.AccountName, ev.Summary, ev.Direction, ev.TransactionType}, " "))
		if hasAny(text, []string{"1122", "应收账款", "历史应收", "往年应收"}) {
			return true
		}
	}
	return false
}

func isRevenueEvidence(text string, ev LedgerEvidence) bool {
	if hasAny(text, []string{"营业收入", "主营业务收入", "销售收入", "收入", "销售", "6001", "4001"}) {
		return true
	}
	return ev.CreditAmount > 0 && hasAny(text, customerKeywords)
}

func isCostEvidence(text string, ev LedgerEvidence) bool {
	if hasAny(text, []string{"营业成本", "成本", "采购", "费用", "支出", "服务费", "技术服务费", "6601", "6602", "6401", "5001", "存货", "材料"}) {
		return true
	}
	return ev.DebitAmount > 0 && hasAny(text, supplierKeywords)
}

func isOutputTaxEvidence(text string, ev LedgerEvidence) bool {
	if hasAny(text, outputTaxKeywords) {
		return true
	}
	return ev.CreditAmount > 0 && hasAny(text, []string{"应交税费"})
}

func isInputTaxEvidence(text string, ev LedgerEvidence) bool {
	if hasAny(text, inputTaxKeywords) {
		return true
	}
	return ev.DebitAmount > 0 && hasAny(text, []string{"应交税费"})
}

func chooseAmount(ev LedgerEvidence) float64 {
	if ev.CreditAmount > 0 {
		return ev.CreditAmount
	}
	return ev.DebitAmount
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// TraceTaxNormalization 可以直接喂给主线程日志或调试输出。
func TraceTaxNormalization(report TaxNormalizationReport) string {
	return fmt.Sprintf(
		"counterparty=%s role=%s output(cash=%.2f accrual=%.2f tax=%.2f reason=%s included=%v) input(cash=%.2f accrual=%.2f tax=%.2f reason=%s included=%v)",
		report.Counterparty,
		report.Role,
		report.Output.CashAmount, report.Output.AccrualAmount, report.Output.TaxAmount, report.Output.DifferenceReason, report.Output.Included,
		report.Input.CashAmount, report.Input.AccrualAmount, report.Input.TaxAmount, report.Input.DifferenceReason, report.Input.Included,
	)
}
