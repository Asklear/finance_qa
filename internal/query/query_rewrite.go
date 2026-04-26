package query

import (
	"strings"
	"time"
)

type BossMetric string

const (
	BossMetricUnknown  BossMetric = "unknown"
	BossMetricRevenue  BossMetric = "revenue"
	BossMetricCost     BossMetric = "cost"
	BossMetricProfit   BossMetric = "profit"
	BossMetricReceipts BossMetric = "receipts"
	BossMetricPayments BossMetric = "payments"
	BossMetricInvoice  BossMetric = "invoice"
	BossMetricARAP     BossMetric = "arap"
	BossMetricTax      BossMetric = "tax"
	BossMetricHRCost   BossMetric = "hr_cost"
	BossMetricCashFlow BossMetric = "cash_flow"
	BossMetricHealth   BossMetric = "health"
)

type BossScope string

const (
	BossScopeUnknown  BossScope = "unknown"
	BossScopeCompany  BossScope = "company"
	BossScopeEntity   BossScope = "entity"
	BossScopeContract BossScope = "contract"
)

type BossGranularity string

const (
	BossGranularityUnknown   BossGranularity = "unknown"
	BossGranularityAggregate BossGranularity = "aggregate"
	BossGranularityDetail    BossGranularity = "detail"
	BossGranularityBreakdown BossGranularity = "breakdown"
	BossGranularityAnalysis  BossGranularity = "analysis"
	BossGranularityReconcile BossGranularity = "reconciliation"
	BossGranularityBalance   BossGranularity = "balance"
	BossGranularitySubperiod BossGranularity = "aggregate_with_subperiod"
)

type BossPerspective string

const (
	BossPerspectiveUnknown              BossPerspective = "unknown"
	BossPerspectiveContractFirst        BossPerspective = "boss_contract_first"
	BossPerspectiveExplicitCash         BossPerspective = "explicit_cash"
	BossPerspectiveOfficialThenEvidence BossPerspective = "official_then_evidence"
	BossPerspectiveFinancialAccount     BossPerspective = "financial_account"
)

const (
	BossSourceBankStatement = "bank_statement"
	BossSourceJournal       = "journal"
	BossSourceBalance       = "balance"
	BossSourceContract      = "contract"
)

type BossQueryRewrite struct {
	Metric              BossMetric
	Scope               BossScope
	Entity              string
	PeriodFrom          string
	PeriodTo            string
	SubPeriod           string
	Granularity         BossGranularity
	Perspective         BossPerspective
	SourceConstraint    string
	RequiresSourceProbe bool
}

func RewriteBossQuery(question string, anchor time.Time) BossQueryRewrite {
	q := NormalizeQuestion(question)
	from, to := ExtractPeriodWithNow(q, anchor)
	subPeriod, hasSubPeriod := extractReceiptSubPeriod(q, from, to)
	entity := extractNamedEntityFromQuestion(q)
	if looksLikeBossRewriteNonEntity(entity) {
		entity = ""
	}
	metric := detectBossMetric(q)
	perspective, sourceConstraint := detectBossPerspectiveAndSource(q, metric)
	scope := detectBossScope(q, entity)
	granularity := detectBossGranularity(q, metric, hasSubPeriod)

	return BossQueryRewrite{
		Metric:              metric,
		Scope:               scope,
		Entity:              entity,
		PeriodFrom:          from,
		PeriodTo:            to,
		SubPeriod:           subPeriod,
		Granularity:         granularity,
		Perspective:         perspective,
		SourceConstraint:    sourceConstraint,
		RequiresSourceProbe: shouldBossRewriteProbe(metric, perspective),
	}
}

func detectBossMetric(q string) BossMetric {
	cfg := getRuleConfig()
	switch {
	case isARAPQuestion(q):
		return BossMetricARAP
	case containsAny(q, []string{"回款", "到账", "收款"}):
		return BossMetricReceipts
	case containsAny(q, []string{"付款", "支付", "付了"}):
		return BossMetricPayments
	case containsAny(q, []string{"开票", "未开票", "发票"}):
		return BossMetricInvoice
	case containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) || shouldUseHRBreakdown(q, cfg):
		return BossMetricHRCost
	case containsAny(q, cfg.intentKeywordGroup(string(IntentTaxQuery))):
		return BossMetricTax
	case containsAny(q, cfg.intentKeywordGroup(string(IntentARAPQuery))) || isOpeningPeriodQuestion(q):
		return BossMetricARAP
	case containsAny(q, cfg.intentKeywordGroup(routerGroupHealth)):
		return BossMetricHealth
	case containsAny(q, cfg.MetricKeywords(metricKeyProfit)):
		return BossMetricProfit
	case containsAny(q, cfg.MetricKeywords(metricKeyCost)):
		return BossMetricCost
	case containsAny(q, cfg.MetricKeywords(metricKeyRevenue)):
		return BossMetricRevenue
	case containsAny(q, []string{"现金流", "净现金流", "净增加", "净流入", "净流出", "实际支出", "实际到账"}):
		return BossMetricCashFlow
	default:
		return BossMetricUnknown
	}
}

func detectBossPerspectiveAndSource(q string, metric BossMetric) (BossPerspective, string) {
	switch {
	case containsAny(q, []string{"银行", "银行卡", "流水", "现金流", "实际到账", "实际支出", "现金口径"}):
		return BossPerspectiveExplicitCash, BossSourceBankStatement
	case shouldUseExplicitFinancialAccountQuestion(q):
		return BossPerspectiveFinancialAccount, BossSourceJournal
	case metric == BossMetricARAP && shouldUseContractFirstARAP(q):
		return BossPerspectiveContractFirst, ""
	case metric == BossMetricARAP:
		return BossPerspectiveOfficialThenEvidence, BossSourceBalance
	case metric == BossMetricTax || metric == BossMetricHRCost:
		return BossPerspectiveFinancialAccount, BossSourceJournal
	case isBossContractFirstMetric(metric):
		return BossPerspectiveContractFirst, ""
	default:
		return BossPerspectiveUnknown, ""
	}
}

func shouldUseExplicitFinancialAccountQuestion(q string) bool {
	return containsAny(q, []string{
		"序时账", "序时帐", "凭证", "利润表", "财务账", "会计账", "报表口径", "账上",
		"科目余额", "发生额及余额", "余额表", "资产负债表",
	})
}

func detectBossScope(q, entity string) BossScope {
	if shouldUseExpenseBreakdown(q) {
		return BossScopeCompany
	}
	if shouldUseCompanyScopeContractAggregate(q) {
		return BossScopeCompany
	}
	if strings.Contains(q, "合同") || (strings.Contains(q, "项目") && strings.TrimSpace(entity) != "") {
		return BossScopeContract
	}
	if strings.TrimSpace(entity) != "" {
		return BossScopeEntity
	}
	return BossScopeCompany
}

func looksLikeBossRewriteNonEntity(entity string) bool {
	normalized := normalizeEntityText(entity)
	if normalized == "" {
		return false
	}
	if looksLikeBusinessDimensionLabel(entity) {
		return true
	}
	return containsAny(normalized, []string{
		"银行卡", "银行", "实际", "到账", "回款", "收款", "付款", "现金", "现金流",
		"应收", "应付", "账款", "开票", "收票", "发票", "未付", "未回", "未收",
		"整体", "大类", "构成", "分类", "类别", "支出", "费用", "开支",
	})
}

func detectBossGranularity(q string, metric BossMetric, hasSubPeriod bool) BossGranularity {
	switch {
	case hasSubPeriod:
		return BossGranularitySubperiod
	case shouldUseReconciliation(q):
		return BossGranularityReconcile
	case metric == BossMetricARAP || containsAny(q, []string{"期初", "期末", "余额"}):
		return BossGranularityBalance
	case containsAny(q, []string{"明细", "列表", "每笔", "逐笔"}):
		return BossGranularityDetail
	case containsAny(q, []string{"拆", "拆分", "拆开", "分别", "构成"}):
		return BossGranularityBreakdown
	case metric == BossMetricHealth || containsAny(q, []string{"分析", "建议", "风险", "健康"}):
		return BossGranularityAnalysis
	default:
		return BossGranularityAggregate
	}
}

func shouldBossRewriteProbe(metric BossMetric, perspective BossPerspective) bool {
	if perspective == BossPerspectiveExplicitCash {
		return false
	}
	return perspective == BossPerspectiveContractFirst || isBossContractFirstMetric(metric)
}

func isBossContractFirstMetric(metric BossMetric) bool {
	switch metric {
	case BossMetricRevenue, BossMetricCost, BossMetricProfit, BossMetricReceipts, BossMetricPayments, BossMetricInvoice, BossMetricARAP:
		return true
	default:
		return false
	}
}
