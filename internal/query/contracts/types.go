package contracts

type DimensionRow struct {
	ContractID      string
	CustomerName    string
	ContractContent string
}

type DimensionSummary struct {
	Entity         string
	Role           string
	Period         string
	PeriodFrom     string
	PeriodTo       string
	SubPeriod      string
	Contracts      []map[string]any
	Data           map[string]any
	ExecutedSQL    []string
	CalculationLog []string
}
