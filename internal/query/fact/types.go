package fact

type AuthorityLevel string

const (
	AuthorityOfficial   AuthorityLevel = "official"
	AuthoritySupporting AuthorityLevel = "supporting"
	AuthorityDerived    AuthorityLevel = "derived"
)

type CoverageStatus string

const (
	CoverageFull    CoverageStatus = "full"
	CoveragePartial CoverageStatus = "partial"
	CoverageMissing CoverageStatus = "missing"
)

type Fact struct {
	Source         string
	MetricKey      string
	Entity         string
	PeriodFrom     string
	PeriodTo       string
	OpeningPeriod  string
	Value          float64
	AuthorityLevel AuthorityLevel
	CoverageStatus CoverageStatus
	Staleness      string
	Confidence     float64
	TracePayload   map[string]any
}

type FactSet struct {
	Source string
	Facts  []Fact
}
