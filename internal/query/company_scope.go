package query

import (
	"fmt"
	"strings"
)

func (e *Engine) scopedCompanyClause(column string) (string, []any) {
	column = strings.TrimSpace(column)
	if column == "" {
		column = "company"
	}
	company := strings.TrimSpace(e.Company)
	if company == "" {
		return "1=1", nil
	}
	return fmt.Sprintf("%s = ?", column), []any{company}
}
