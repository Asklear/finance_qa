package query

import "strings"

func (e *Engine) detectContractRole(entity, from, to string) string {
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
	nq := normalizeEntityText(question)
	if nq == "" {
		return ""
	}
	rows, err := e.db.Query(`
SELECT name
FROM (
  SELECT customer_name AS name
  FROM fin_contracts
  WHERE COALESCE(TRIM(customer_name), '') <> ''
  UNION
  SELECT contract_content AS name
  FROM fin_contracts
  WHERE COALESCE(TRIM(contract_content), '') <> ''
) contract_candidates
ORDER BY LENGTH(name) DESC, name
`)
	if err != nil {
		return ""
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		if len([]rune(normalizeEntityText(name))) < 2 {
			continue
		}
		if strings.Contains(nq, normalizeEntityText(name)) {
			return name
		}
	}
	return ""
}
