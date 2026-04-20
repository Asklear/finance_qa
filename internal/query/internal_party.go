package query

import (
	"fmt"
	"regexp"
	"strings"
)

type voucherLedgerRow struct {
	VoucherDate  string
	VoucherNo    string
	AccountCode  string
	AccountName  string
	Direction    string
	Summary      string
	Counterparty string
	DebitAmount  float64
	CreditAmount float64
}

func (e *Engine) detectInternalBranchTransferCash(start, end string) (float64, string, []string) {
	cfg := getRuleConfig()
	query := `
SELECT
  IFNULL(TRIM(voucher_date), ''),
  IFNULL(TRIM(voucher_no), ''),
  IFNULL(TRIM(account_code), ''),
  IFNULL(TRIM(account_name), ''),
  IFNULL(TRIM(direction), ''),
  IFNULL(TRIM(summary), ''),
  IFNULL(TRIM(counterparty), ''),
  COALESCE(debit_amount, 0),
  COALESCE(credit_amount, 0)
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
ORDER BY ` + ledgerVoucherOrderByClause() + `
`
	rows, err := e.db.Query(query, e.Company, e.Company, start, end)
	if err != nil {
		return 0, query, []string{fmt.Sprintf("[分公司内部转账] query error=%v", err)}
	}
	defer rows.Close()

	groups := make(map[string][]voucherLedgerRow)
	index := 0
	for rows.Next() {
		var row voucherLedgerRow
		if err := rows.Scan(
			&row.VoucherDate,
			&row.VoucherNo,
			&row.AccountCode,
			&row.AccountName,
			&row.Direction,
			&row.Summary,
			&row.Counterparty,
			&row.DebitAmount,
			&row.CreditAmount,
		); err != nil {
			return 0, query, []string{fmt.Sprintf("[分公司内部转账] scan error=%v", err)}
		}
		key := ledgerVoucherGroupKey(row, index)
		groups[key] = append(groups[key], row)
		index++
	}
	if err := rows.Err(); err != nil {
		return 0, query, []string{fmt.Sprintf("[分公司内部转账] iterate error=%v", err)}
	}

	total := 0.0
	logs := make([]string, 0)
	for key, group := range groups {
		if !voucherHasInternalSettlementDebit(group, cfg) {
			continue
		}
		internalParty, basis := inferInternalPartyFromVoucher(e.Company, group, cfg)
		if internalParty == "" {
			continue
		}
		amount := 0.0
		for _, row := range group {
			if isBankCreditVoucherRow(row) {
				amount += row.CreditAmount
			}
		}
		amount = round2(amount)
		if amount <= 0 {
			continue
		}
		total += amount
		logs = append(logs, fmt.Sprintf("[分公司内部转账] voucher=%s party=%s basis=%s amount=%.2f", displayVoucherGroupKey(key, group), internalParty, basis, amount))
	}

	if len(logs) == 0 {
		logs = append(logs, "[分公司内部转账] no matched internal transfer vouchers")
	}
	return round2(total), query, logs
}

func ledgerVoucherOrderByClause() string {
	return strings.Join([]string{
		"voucher_date",
		"COALESCE(NULLIF(TRIM(voucher_no), ''), '')",
		"account_code",
		"COALESCE(NULLIF(TRIM(account_name), ''), '')",
		"COALESCE(NULLIF(TRIM(summary), ''), '')",
		"COALESCE(NULLIF(TRIM(counterparty), ''), '')",
		"COALESCE(debit_amount, 0)",
		"COALESCE(credit_amount, 0)",
	}, ", ")
}

func ledgerVoucherGroupKey(row voucherLedgerRow, index int) string {
	if strings.TrimSpace(row.VoucherNo) != "" {
		return row.VoucherDate + "|" + row.VoucherNo
	}
	return fmt.Sprintf("%s|row-%d", row.VoucherDate, index)
}

func displayVoucherGroupKey(key string, group []voucherLedgerRow) string {
	if len(group) == 0 {
		return key
	}
	if strings.TrimSpace(group[0].VoucherNo) != "" {
		return group[0].VoucherDate + "/" + group[0].VoucherNo
	}
	return group[0].VoucherDate + "/(no-voucher-no)"
}

func isBankCreditVoucherRow(row voucherLedgerRow) bool {
	if !(strings.HasPrefix(row.AccountCode, "1001") || strings.HasPrefix(row.AccountCode, "1002")) {
		return false
	}
	if row.CreditAmount <= 0 {
		return false
	}
	direction := strings.TrimSpace(row.Direction)
	return direction == "" || direction == "贷"
}

func voucherHasInternalSettlementDebit(rows []voucherLedgerRow, cfg RuleConfig) bool {
	for _, row := range rows {
		if row.DebitAmount <= 0 {
			continue
		}
		if strings.TrimSpace(row.Direction) != "" && strings.TrimSpace(row.Direction) != "借" {
			continue
		}
		switch {
		case strings.HasPrefix(row.AccountCode, "2211"):
			return true
		case strings.HasPrefix(row.AccountCode, "1221"):
			return true
		case strings.HasPrefix(row.AccountCode, "2241"):
			return true
		}
		text := normalizeEntityText(row.AccountName + row.Summary)
		if hasAny(text, cfg.InternalPartyAccountContextKeywords()) {
			return true
		}
	}
	return false
}

func inferInternalPartyFromVoucher(company string, rows []voucherLedgerRow, cfg RuleConfig) (string, string) {
	bestName := ""
	bestScore := 0
	bestBasis := ""
	contextScore := 0
	if voucherHasInternalSettlementDebit(rows, cfg) {
		contextScore = 1
	}
	for _, row := range rows {
		candidates := extractInternalPartyCandidates(row.Counterparty, cfg)
		candidates = append(candidates, extractInternalPartyCandidates(row.Summary, cfg)...)
		candidates = append(candidates, extractInternalPartyCandidates(row.AccountName, cfg)...)
		for _, candidate := range candidates {
			score, basis := internalPartyScore(company, candidate, contextScore, cfg)
			if score > bestScore || (score == bestScore && len([]rune(candidate)) > len([]rune(bestName))) {
				bestScore = score
				bestName = candidate
				bestBasis = basis
			}
		}
	}
	if bestScore < 2 {
		return "", ""
	}
	return bestName, bestBasis
}

func extractInternalPartyCandidates(text string, cfg RuleConfig) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	candidates := []string{text}
	pattern := internalOrgPattern(cfg)
	candidates = append(candidates, pattern.FindAllString(text, -1)...)

	fields := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '_', '-', '－', '—', ':', '：', '/', '／', ',', '，', ';', '；':
			return true
		default:
			return false
		}
	})
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if pattern.MatchString(field) {
			candidates = append(candidates, pattern.FindAllString(field, -1)...)
		}
		if looksLikeInternalOrgUnit(field, cfg) {
			candidates = append(candidates, field)
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func internalPartyScore(company, candidate string, contextScore int, cfg RuleConfig) (int, string) {
	candidate = strings.TrimSpace(candidate)
	if !looksLikeInternalOrgUnit(candidate, cfg) {
		return 0, ""
	}
	score := 1 + contextScore
	basis := []string{"org_unit"}
	if internalPartyMatchesCompany(company, candidate) {
		score += 2
		basis = append(basis, "shared_brand")
	}
	if isGenericBranchLabel(candidate, cfg) {
		score++
		basis = append(basis, "generic_branch_label")
	}
	if contextScore > 0 {
		basis = append(basis, "internal_account_context")
	}
	return score, strings.Join(basis, "+")
}

func internalPartyMatchesCompany(company, candidate string) bool {
	nCandidate := normalizeEntityText(candidate)
	if nCandidate == "" {
		return false
	}
	for _, alias := range companyAliases(company) {
		nAlias := normalizeEntityText(alias)
		if len([]rune(nAlias)) < 2 {
			continue
		}
		if strings.Contains(nCandidate, nAlias) || strings.Contains(nAlias, nCandidate) {
			return true
		}
	}
	return false
}

func looksLikeInternalOrgUnit(candidate string, cfg RuleConfig) bool {
	n := normalizeEntityText(candidate)
	for _, suffix := range cfg.InternalPartyOrgSuffixes() {
		if strings.Contains(n, normalizeEntityText(suffix)) {
			return true
		}
	}
	return false
}

func isGenericBranchLabel(candidate string, cfg RuleConfig) bool {
	n := normalizeEntityText(candidate)
	for _, suffix := range cfg.InternalPartyOrgSuffixes() {
		if strings.HasSuffix(n, normalizeEntityText(suffix)) &&
			!strings.Contains(n, normalizeEntityText("有限公司")) &&
			!strings.Contains(n, normalizeEntityText("有限责任公司")) {
			return true
		}
	}
	return false
}

func internalOrgPattern(cfg RuleConfig) *regexp.Regexp {
	suffixes := cfg.InternalPartyOrgSuffixes()
	if len(suffixes) == 0 {
		return regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]+?(?:分公司|子公司|事业部|办事处|分部|总部|总公司)`)
	}
	quoted := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		suffix = strings.TrimSpace(suffix)
		if suffix == "" {
			continue
		}
		quoted = append(quoted, regexp.QuoteMeta(suffix))
	}
	if len(quoted) == 0 {
		return regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]+?(?:分公司|子公司|事业部|办事处|分部|总部|总公司)`)
	}
	return regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]+?(?:` + strings.Join(quoted, "|") + `)`)
}
