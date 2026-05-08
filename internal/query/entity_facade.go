package query

import queryentity "financeqa/internal/query/entity"

func looksLikeBusinessDimensionLabel(entity string) bool {
	return queryentity.LooksLikeBusinessDimensionLabel(entity)
}
