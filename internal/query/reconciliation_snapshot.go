package query

import (
	"fmt"
	"math"
	"strings"
)

func (e *Engine) buildCounterpartySnapshot(name, from, to string) counterpartySnapshot {
	snap := counterpartySnapshot{Name: name, Role: "unknown", EvidenceLevel: evidenceDerived}
	evidence := e.collectCounterpartyEvidence(name, from, to)
	classification := ClassifyCounterparty(name, evidence)
	taxReport := NormalizeTax(name, evidence)
	snap.BankIn, snap.BankOut = summarizeCounterpartyCashEvidence(evidence)
	support := make([]string, 0, 8)
	for _, ev := range evidence {
		code := ev.AccountCode
		direction := ev.Direction
		amount := ev.CreditAmount
		if amount == 0 {
			amount = ev.DebitAmount
		}
		switch {
		case strings.HasPrefix(code, "1122"):
			if direction == "贷" {
				snap.ARDecrease += amount
			} else {
				snap.ARIncrease += amount
			}
		case strings.HasPrefix(code, "2202"):
			if direction == "借" {
				snap.APDecrease += amount
			} else {
				snap.APIncrease += amount
			}
		case strings.HasPrefix(code, "1123"):
			if direction == "借" {
				snap.PrepaymentIncrease += amount
			} else {
				snap.PrepaymentCleared += amount
			}
		case strings.HasPrefix(code, "6001"), strings.HasPrefix(code, "6051"):
			if direction == "贷" {
				snap.RevenueNet += amount
			} else {
				snap.RevenueNet -= amount
			}
		case strings.HasPrefix(code, "22210106"):
			if direction == "贷" {
				snap.OutputVAT += amount
			} else {
				snap.OutputVAT -= amount
			}
		case strings.HasPrefix(code, "22210101"):
			if direction == "借" {
				snap.InputVAT += amount
			} else {
				snap.InputVAT -= amount
			}
		case strings.HasPrefix(code, "6401"):
			if direction == "借" {
				snap.BookCost += amount
			} else {
				snap.BookCost -= amount
			}
		case strings.HasPrefix(code, "660"):
			if direction == "借" {
				snap.BookExpense += amount
			} else {
				snap.BookExpense -= amount
			}
		}

		if len(support) < 8 {
			brief := ev.Summary
			if brief == "" {
				brief = ev.Counterparty
			}
			support = append(support, fmt.Sprintf("%s %s %.2f %s", code, direction, amount, brief))
		}
	}
	snap.Support = support
	snap.Role = string(classification.Role)
	if snap.Role == "" {
		snap.Role = "unknown"
	}

	switch {
	case taxReport.Output.Included && snap.BankIn > 0 && approxEqual(snap.BankIn, snap.RevenueNet+snap.OutputVAT):
		snap.ComparisonBasis = "vat_gap_only"
		snap.DifferenceReason = "银行回款是含税口径，账上销售额是不含税口径，两者差额主要是销项税。"
		snap.EvidenceLevel = evidenceDirect
	case snap.BankIn > 0 && snap.ARDecrease > 0 && snap.RevenueNet > 0:
		snap.ComparisonBasis = "historical_receipt_and_current_revenue"
		snap.DifferenceReason = "同一对手方本月同时出现历史应收回款和当月确认收入，到账金额不能直接当成当月收入。"
		snap.EvidenceLevel = evidenceDerived
		snap.RequiresMonthDisclosure = true
	case taxReport.Input.Included && snap.BankOut > 0 && (snap.BookCost > 0 || snap.BookExpense > 0 || snap.InputVAT > 0 || snap.PrepaymentIncrease > 0 || snap.APDecrease > 0):
		snap.ComparisonBasis = "supplier_payment_or_cost"
		snap.DifferenceReason = taxReport.Input.DifferenceReason
		snap.EvidenceLevel = evidenceDirect
	case strings.Contains(taxReport.Output.DifferenceReason, "历史应收回款") || (snap.BankIn > 0 && snap.ARDecrease > 0):
		snap.ComparisonBasis = "historical_receipt"
		snap.DifferenceReason = "数据库能确认这是一笔冲减历史应收的回款，但没有字段直接说明对应哪一个结算月份。"
		snap.EvidenceLevel = evidenceUnknown
		snap.RequiresMonthDisclosure = true
	case snap.RevenueNet > 0:
		snap.ComparisonBasis = "recognized_revenue"
		snap.DifferenceReason = "本月有账上确认收入。"
	}

	return snap
}

func summarizeCounterpartyCashEvidence(evidence []LedgerEvidence) (float64, float64) {
	bankIn := 0.0
	bankOut := 0.0
	seenBankEvents := map[string]int{}
	for _, ev := range evidence {
		if ev.Source != "bank_statement" {
			continue
		}
		key, direction, amount, ok := counterpartyCashEventSignature(ev)
		if !ok {
			continue
		}
		seenBankEvents[key]++
		if direction == "in" {
			bankIn += amount
		} else {
			bankOut += amount
		}
	}
	for _, ev := range evidence {
		if ev.Source != "journal" || !looksLikeBankAccountCode(ev.AccountCode) {
			continue
		}
		key, direction, amount, ok := counterpartyCashEventSignature(ev)
		if !ok {
			continue
		}
		if remaining := seenBankEvents[key]; remaining > 0 {
			seenBankEvents[key] = remaining - 1
			continue
		}
		if direction == "in" {
			bankIn += amount
		} else {
			bankOut += amount
		}
	}
	return round2(bankIn), round2(bankOut)
}

func counterpartyCashEventKey(ev LedgerEvidence) string {
	key, _, _, _ := counterpartyCashEventSignature(ev)
	return key
}

func counterpartyCashEventSignature(ev LedgerEvidence) (string, string, float64, bool) {
	amount := 0.0
	direction := ""
	switch ev.Source {
	case "bank_statement":
		if ev.CreditAmount > 0 {
			direction = "in"
			amount = round2(ev.CreditAmount)
		} else if ev.DebitAmount > 0 {
			direction = "out"
			amount = round2(ev.DebitAmount)
		}
	default:
		if ev.DebitAmount > 0 {
			direction = "in"
			amount = round2(ev.DebitAmount)
		} else if ev.CreditAmount > 0 {
			direction = "out"
			amount = round2(ev.CreditAmount)
		}
	}
	if amount == 0 || direction == "" {
		return "", "", 0, false
	}
	date := strings.TrimSpace(ev.VoucherDate)
	if len(date) >= 10 {
		date = date[:10]
	}
	return strings.Join([]string{
		date,
		direction,
		fmt.Sprintf("%.2f", amount),
	}, "\x1f"), direction, amount, true
}

func looksLikeBankAccountCode(code string) bool {
	return strings.HasPrefix(strings.TrimSpace(code), "1001") || strings.HasPrefix(strings.TrimSpace(code), "1002")
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) <= 0.02
}
