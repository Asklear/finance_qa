package query

import "strings"

func contractSubjectCandidatesQuery(column string) string {
	return `
SELECT ` + column + ` AS name
FROM fin_contracts
WHERE COALESCE(TRIM(` + column + `), '') <> ''
ORDER BY LENGTH(` + column + `) DESC, ` + column + `
`
}

func (e *Engine) contractSubjectCandidates(column string) []string {
	rows, err := e.db.Query(contractSubjectCandidatesQuery(column))
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]string, 0, 64)
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func (e *Engine) contractCustomerCandidates() []string {
	return e.contractSubjectCandidates("customer_name")
}

func (e *Engine) contractContentCandidates() []string {
	return e.contractSubjectCandidates("contract_content")
}

func (e *Engine) resolveContractSubjectCandidates(alias string) []string {
	alias = trimEntityNoiseSuffixes(stripTemporalNoise(strings.TrimSpace(alias)))
	if looksLikeBusinessDimensionLabel(alias) {
		return nil
	}
	matches := rankCounterpartyAliasMatches(alias, e.contractCustomerCandidates())
	if len(matches) > 0 {
		return matches
	}
	return rankCounterpartyAliasMatches(alias, e.contractContentCandidates())
}

func (e *Engine) resolveContractSubject(question, entity string) string {
	if strings.TrimSpace(entity) == "" && shouldUseCompanyScopeContractAggregate(question) {
		return ""
	}
	nq := normalizeEntityText(question)
	if nq != "" {
		directCandidates := append(e.contractCustomerCandidates(), e.contractContentCandidates()...)
		for _, name := range directCandidates {
			nm := normalizeEntityText(name)
			if len([]rune(nm)) < 2 {
				continue
			}
			if strings.Contains(nq, nm) {
				return name
			}
		}
	}

	terms := []string{
		strings.TrimSpace(entity),
		extractNamedEntityFromQuestion(question),
		extractOrganizationEntityMatch(question),
	}
	seen := map[string]struct{}{}
	for _, term := range terms {
		term = trimEntityNoiseSuffixes(stripTemporalNoise(strings.TrimSpace(term)))
		if len([]rune(term)) < 2 || looksLikeBusinessDimensionLabel(term) {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		if matches := e.resolveContractSubjectCandidates(term); len(matches) > 0 {
			return matches[0]
		}
	}

	if len(seen) > 0 {
		return ""
	}

	for _, seg := range chineseEntitySegmentPattern.FindAllString(strings.TrimSpace(question), -1) {
		runes := []rune(seg)
		for length := len(runes); length >= 2; length-- {
			for i := 0; i <= len(runes)-length; i++ {
				sub := trimEntityNoiseSuffixes(stripTemporalNoise(string(runes[i : i+length])))
				if shouldSkipEntityFragment(sub, 2) || looksLikeBusinessDimensionLabel(sub) {
					continue
				}
				if matches := e.resolveContractSubjectCandidates(sub); len(matches) > 0 {
					return matches[0]
				}
			}
		}
	}
	return ""
}

func (e *Engine) detectContractRole(entity, from, to string) string {
	if resolved := e.resolveContractSubject("", entity); resolved != "" {
		entity = resolved
	}
	like := "%" + entity + "%"
	var costRows, fundRows int
	e.db.QueryRow(`
SELECT COUNT(1)
FROM fin_cost_settlements cs
JOIN fin_contracts c ON c.contract_id = cs.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND cs.year_month BETWEEN ? AND ?
`, like, like, from, to).Scan(&costRows)
	e.db.QueryRow(`
SELECT COUNT(1)
FROM fin_fund_income f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE (c.customer_name LIKE ? OR c.contract_content LIKE ?)
  AND f.year_month BETWEEN ? AND ?
`, like, like, from, to).Scan(&fundRows)

	hasCustomer := fundRows > 0
	hasSupplier := costRows > 0
	switch {
	case hasCustomer && hasSupplier:
		return "mixed_contract"
	case hasCustomer:
		return "customer_contract"
	case hasSupplier:
		return "supplier_contract"
	default:
		return "unknown"
	}
}

func (e *Engine) queryMatchingContracts(entity string) []contractDimensionRow {
	if resolved := e.resolveContractSubject("", entity); resolved != "" {
		entity = resolved
	}
	rows, err := e.db.Query(`
SELECT contract_id, customer_name, contract_content
FROM fin_contracts
WHERE customer_name LIKE ? OR contract_content LIKE ?
ORDER BY contract_id
`, "%"+entity+"%", "%"+entity+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]contractDimensionRow, 0)
	for rows.Next() {
		var row contractDimensionRow
		if err := rows.Scan(&row.ContractID, &row.CustomerName, &row.ContractContent); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out
}

func (e *Engine) matchContractSubjectByName(question string) string {
	return e.resolveContractSubject(question, "")
}
