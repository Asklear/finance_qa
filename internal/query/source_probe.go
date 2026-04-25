package query

import (
	"context"
	"fmt"
	"strings"
)

const BossSourceContractAggregate = "contract_aggregate"

type SourceProbeResult struct {
	Source           string
	SemanticMatch    bool
	CanAnswer        bool
	CoverageStatus   CoverageStatus
	Metric           BossMetric
	PeriodFrom       string
	PeriodTo         string
	RowCount         int
	MissingReason    string
	PrimaryTables    []string
	SupportingTables []string
	SourceDocuments  []string
}

func (e *Engine) ProbeBossSources(ctx context.Context, rewrite BossQueryRewrite) []SourceProbeResult {
	if e == nil || e.db == nil {
		return nil
	}
	if rewrite.SourceConstraint == BossSourceBankStatement || rewrite.Perspective == BossPerspectiveExplicitCash {
		return []SourceProbeResult{e.probeBankStatement(ctx, rewrite)}
	}
	if rewrite.RequiresSourceProbe || rewrite.Perspective == BossPerspectiveContractFirst {
		return []SourceProbeResult{e.probeContractAggregate(ctx, rewrite)}
	}
	return nil
}

func (e *Engine) probeBankStatement(ctx context.Context, rewrite BossQueryRewrite) SourceProbeResult {
	result := SourceProbeResult{
		Source:         BossSourceBankStatement,
		SemanticMatch:  true,
		Metric:         rewrite.Metric,
		PeriodFrom:     rewrite.PeriodFrom,
		PeriodTo:       rewrite.PeriodTo,
		PrimaryTables:  []string{"bank_statement"},
		CoverageStatus: CoverageMissing,
	}
	result.SourceDocuments = e.sourceDocumentsForTables(ctx, result.PrimaryTables)

	cols := e.tableColumns("bank_statement")
	required := []string{"transaction_date"}
	amountPredicate := ""
	switch rewrite.Metric {
	case BossMetricPayments:
		required = append(required, "debit_amount")
		amountPredicate = "COALESCE(debit_amount, 0) <> 0"
	case BossMetricCashFlow:
		required = append(required, "credit_amount", "debit_amount")
		amountPredicate = "(COALESCE(credit_amount, 0) <> 0 OR COALESCE(debit_amount, 0) <> 0)"
	default:
		required = append(required, "credit_amount")
		amountPredicate = "COALESCE(credit_amount, 0) <> 0"
	}
	if missing := missingColumns(cols, required); len(missing) > 0 {
		result.MissingReason = "银行流水缺少字段：" + strings.Join(missing, "、")
		return result
	}

	sqlText := fmt.Sprintf(`
SELECT COUNT(*)
FROM bank_statement
WHERE transaction_date BETWEEN ? AND ?
  AND %s
`, amountPredicate)
	args := []any{rewrite.PeriodFrom + "-01", monthEndDay(rewrite.PeriodTo)}
	if strings.TrimSpace(rewrite.Entity) != "" {
		sqlText += "  AND counterparty_name LIKE ?\n"
		args = append(args, "%"+strings.TrimSpace(rewrite.Entity)+"%")
	}

	var rowCount int
	if err := e.db.QueryRowContext(ctx, sqlText, args...).Scan(&rowCount); err != nil {
		result.MissingReason = "银行流水探测失败：" + err.Error()
		return result
	}
	result.RowCount = rowCount
	if rowCount == 0 {
		result.MissingReason = "银行流水在请求期间 " + displayPeriod(rewrite.PeriodFrom, rewrite.PeriodTo) + " 没有匹配记录"
		return result
	}
	result.CanAnswer = true
	result.CoverageStatus = CoverageFull
	return result
}

func (e *Engine) sourceDocumentsForTables(ctx context.Context, tables []string) []string {
	catalog := e.BuildSemanticCatalog(ctx, tables)
	docs := make([]string, 0, len(tables))
	for _, tableName := range tables {
		profile, ok := catalog.Profiles[tableName]
		if !ok {
			continue
		}
		docs = append(docs, profile.SourceDocuments...)
	}
	return dedupeStrings(docs)
}

func missingColumns(cols map[string]bool, required []string) []string {
	if len(cols) == 0 {
		return required
	}
	missing := make([]string, 0, len(required))
	for _, col := range required {
		if !cols[strings.ToLower(strings.TrimSpace(col))] {
			missing = append(missing, col)
		}
	}
	return missing
}

func combineProbeDocuments(probes ...SourceProbeResult) []string {
	docs := []string{}
	for _, probe := range probes {
		docs = append(docs, probe.SourceDocuments...)
	}
	return dedupeStrings(docs)
}
