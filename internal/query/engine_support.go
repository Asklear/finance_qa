package query

import "database/sql"

func availableCompanies(db *sql.DB) ([]string, error) {
	rows, _ := db.Query(`SELECT DISTINCT company FROM balance_sheet UNION SELECT DISTINCT company FROM bank_statement UNION SELECT DISTINCT company FROM journal`)
	if rows == nil {
		return nil, nil
	}
	defer rows.Close()
	var companies []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		if c != "" {
			companies = append(companies, c)
		}
	}
	return companies, nil
}
