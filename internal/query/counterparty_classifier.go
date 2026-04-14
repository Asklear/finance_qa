package query

import (
	"math"
	"sort"
	"strings"
)

// CounterpartyRole 表示交易对手的业务角色。
type CounterpartyRole string

const (
	CounterpartyCustomer CounterpartyRole = "customer"
	CounterpartySupplier CounterpartyRole = "supplier"
	CounterpartyEmployee CounterpartyRole = "employee"
	CounterpartyMixed    CounterpartyRole = "mixed"
	CounterpartyUnknown  CounterpartyRole = "unknown"
)

// LedgerEvidence 用于承载分录、流水或明细行证据。
// 主线程可以把 journal / bank_statement 的行映射到这个结构，再做分类和税额归因。
type LedgerEvidence struct {
	Source          string  `json:"source,omitempty"`
	Counterparty    string  `json:"counterparty,omitempty"`
	AccountCode     string  `json:"account_code,omitempty"`
	AccountName     string  `json:"account_name,omitempty"`
	Summary         string  `json:"summary,omitempty"`
	Direction       string  `json:"direction,omitempty"`
	TransactionType string  `json:"transaction_type,omitempty"`
	DebitAmount     float64 `json:"debit_amount,omitempty"`
	CreditAmount    float64 `json:"credit_amount,omitempty"`
}

// CounterpartyClassification 是交易对手识别结果。
type CounterpartyClassification struct {
	Counterparty string                       `json:"counterparty,omitempty"`
	Role         CounterpartyRole             `json:"role"`
	Confidence   float64                      `json:"confidence"`
	Scores       map[CounterpartyRole]float64 `json:"scores,omitempty"`
	Signals      []string                     `json:"signals,omitempty"`
}

var (
	customerKeywords = []string{
		"应收", "回款", "收款", "结算款", "销售", "收入", "主营业务收入", "营业收入", "预收", "合同资产", "客户", "1122", "1121",
	}
	supplierKeywords = []string{
		"应付", "付款", "采购", "成本", "材料", "供应商", "外包", "2202",
		"预付账款", "1123", "112301",
	}
	employeeKeywords = []string{
		"工资", "薪酬", "社保", "公积金", "报销", "差旅", "福利", "餐补", "伙食", "应付职工薪酬", "2211",
	}
	outputTaxKeywords = []string{"销项税", "222101", "销项"}
	inputTaxKeywords  = []string{"进项税", "222102", "进项"}
)

// ClassifyCounterparty 基于分录证据识别交易对手角色。
// 规则会综合流水方向、科目、摘要、税种关键词，不会只看净流向。
func ClassifyCounterparty(counterparty string, evidence []LedgerEvidence) CounterpartyClassification {
	cfg := getRuleConfig()
	scores := map[CounterpartyRole]float64{
		CounterpartyCustomer: 0,
		CounterpartySupplier: 0,
		CounterpartyEmployee: 0,
	}
	signals := make([]string, 0, len(evidence)*2)

	for _, ev := range evidence {
		text := normalizeEntityText(strings.Join([]string{
			ev.Source, ev.Counterparty, ev.AccountCode, ev.AccountName, ev.Summary, ev.Direction, ev.TransactionType,
		}, " "))
		if text == "" {
			continue
		}

		// 账务科目和摘要是主证据，流水方向只是弱证据。
		switch {
		case hasAny(text, employeeKeywords):
			scores[CounterpartyEmployee] += 3.0
			signals = append(signals, "employee:"+pickFirstHit(text, employeeKeywords))
		case hasSupplierStrongEvidence(ev, text):
			scores[CounterpartySupplier] += 2.8
			signals = append(signals, "supplier_strong:"+pickSupplierSignal(ev, text))
		case hasAny(text, supplierKeywords):
			scores[CounterpartySupplier] += 2.6
			signals = append(signals, "supplier:"+pickFirstHit(text, supplierKeywords))
		case hasAny(text, customerKeywords):
			scores[CounterpartyCustomer] += 2.6
			signals = append(signals, "customer:"+pickFirstHit(text, customerKeywords))
		}

		if ev.Source == "bank_statement" {
			if hasAny(text, employeeKeywords) {
				scores[CounterpartyEmployee] += 0.6
				signals = append(signals, "bank_employee")
				continue
			}
			net := ev.CreditAmount - ev.DebitAmount
			switch {
			case net > 0:
				scores[CounterpartyCustomer] += 0.4
				signals = append(signals, "bank_credit")
			case net < 0:
				scores[CounterpartySupplier] += 0.4
				signals = append(signals, "bank_debit")
			}
		}

		// 付款摘要中若出现“报销/工资/社保”等，单独给员工证据加权。
		if hasAny(text, employeeKeywords) {
			scores[CounterpartyEmployee] += 0.4
		}
	}

	primaryRole, primaryScore, secondScore := topTwoScores(scores)
	if primaryScore <= cfg.RoleMinPrimaryScore {
		return CounterpartyClassification{
			Counterparty: counterparty,
			Role:         CounterpartyUnknown,
			Confidence:   0,
			Scores:       scores,
			Signals:      dedupeSignals(signals),
		}
	}

	role := primaryRole
	if secondScore > 0 && secondaryIndicatesMixed(primaryScore, secondScore, scores, cfg) {
		role = CounterpartyMixed
	}

	total := 0.0
	for _, v := range scores {
		total += v
	}
	confidence := 0.0
	if total > 0 {
		confidence = math.Round((primaryScore/total)*1000) / 1000
	}
	if confidence < cfg.RoleMinConfidence {
		role = CounterpartyUnknown
	}

	return CounterpartyClassification{
		Counterparty: counterparty,
		Role:         role,
		Confidence:   confidence,
		Scores:       scores,
		Signals:      dedupeSignals(signals),
	}
}

func topTwoScores(scores map[CounterpartyRole]float64) (CounterpartyRole, float64, float64) {
	type kv struct {
		role  CounterpartyRole
		score float64
	}
	items := make([]kv, 0, len(scores))
	for role, score := range scores {
		items = append(items, kv{role: role, score: score})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})
	if len(items) == 0 {
		return CounterpartyUnknown, 0, 0
	}
	if len(items) == 1 {
		return items[0].role, items[0].score, 0
	}
	return items[0].role, items[0].score, items[1].score
}

func secondaryIndicatesMixed(primaryScore, secondScore float64, scores map[CounterpartyRole]float64, cfg RuleConfig) bool {
	if primaryScore <= 0 || secondScore <= 0 {
		return false
	}
	positiveRoles := 0
	for _, score := range scores {
		if score >= cfg.RoleMixedMinPositiveScore {
			positiveRoles++
		}
	}
	if positiveRoles >= cfg.RoleMixedMinPositiveRoles {
		return true
	}
	return secondScore/primaryScore >= cfg.RoleMixedMinRatio
}

func hasAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, normalizeEntityText(kw)) {
			return true
		}
	}
	return false
}

func pickFirstHit(text string, keywords []string) string {
	for _, kw := range keywords {
		nk := normalizeEntityText(kw)
		if nk != "" && strings.Contains(text, nk) {
			return nk
		}
	}
	return ""
}

func hasSupplierStrongEvidence(ev LedgerEvidence, text string) bool {
	if hasAny(text, []string{"2202", "应付账款"}) {
		return true
	}
	if hasAny(text, []string{"1123", "112301", "预付账款"}) {
		return true
	}
	if (strings.HasPrefix(ev.AccountCode, "6602") || strings.HasPrefix(ev.AccountCode, "6401")) && (ev.DebitAmount > 0 || ev.Direction == "借") {
		return true
	}
	if hasAny(text, []string{"服务费", "技术服务费", "外包服务"}) && (ev.DebitAmount > 0 || ev.Direction == "借") {
		return true
	}
	if hasAny(text, []string{"22210101", "222102", "进项税"}) && (ev.DebitAmount > 0 || ev.Direction == "借") {
		return true
	}
	return false
}

func pickSupplierSignal(ev LedgerEvidence, text string) string {
	switch {
	case hasAny(text, []string{"2202", "应付账款"}):
		return "2202"
	case hasAny(text, []string{"1123", "112301", "预付账款"}):
		return "1123"
	case strings.HasPrefix(ev.AccountCode, "6602"):
		return "6602"
	case strings.HasPrefix(ev.AccountCode, "6401"):
		return "6401"
	case hasAny(text, []string{"22210101", "222102", "进项税"}):
		return "input_tax"
	case hasAny(text, []string{"服务费", "技术服务费", "外包服务"}):
		return "service_fee"
	default:
		return "supplier_evidence"
	}
}

func dedupeSignals(signals []string) []string {
	seen := make(map[string]struct{}, len(signals))
	out := make([]string, 0, len(signals))
	for _, s := range signals {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
