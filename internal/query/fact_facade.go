package query

import queryfact "financeqa/internal/query/fact"

type AuthorityLevel = queryfact.AuthorityLevel

const (
	AuthorityOfficial   = queryfact.AuthorityOfficial
	AuthoritySupporting = queryfact.AuthoritySupporting
	AuthorityDerived    = queryfact.AuthorityDerived
)

type CoverageStatus = queryfact.CoverageStatus

const (
	CoverageFull    = queryfact.CoverageFull
	CoveragePartial = queryfact.CoveragePartial
	CoverageMissing = queryfact.CoverageMissing
)

type Fact = queryfact.Fact
type FactSet = queryfact.FactSet

type AnswerFrame struct {
	Spec     QuerySpec
	Plan     QueryPlan
	FactSets []FactSet
}
