package query

import querycompany "financeqa/internal/query/company"

func ResolveCompany(req string, companies []string) string {
	return querycompany.Resolve(req, companies)
}

func ResolveCompanyMention(question string, companies []string) string {
	return querycompany.ResolveMention(question, companies)
}

func bestCompanyMatch(query string, companies []string) (string, int) {
	return querycompany.BestMatch(query, companies)
}

func companyAliases(company string) []string {
	return querycompany.Aliases(company)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
