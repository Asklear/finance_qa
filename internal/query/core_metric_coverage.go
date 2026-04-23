package query

import "strings"

type coreMetricCoverage struct {
	RequestedFrom string
	RequestedTo   string
	ActualFrom    string
	ActualTo      string
	AvailableTo   string
	Truncated     bool
	HasData       bool
}

func buildCoreMetricCoverage(from, to, availableTo string) coreMetricCoverage {
	coverage := coreMetricCoverage{
		RequestedFrom: from,
		RequestedTo:   to,
		ActualFrom:    from,
		ActualTo:      to,
		AvailableTo:   strings.TrimSpace(availableTo),
		HasData:       true,
	}
	if coverage.AvailableTo == "" {
		return coverage
	}
	if strings.TrimSpace(from) == "" {
		coverage.ActualFrom = coverage.AvailableTo
		coverage.RequestedFrom = coverage.AvailableTo
	}
	if strings.TrimSpace(to) == "" {
		coverage.ActualTo = coverage.AvailableTo
		coverage.RequestedTo = coverage.AvailableTo
		return coverage
	}
	if coverage.ActualFrom > coverage.AvailableTo {
		coverage.HasData = false
		coverage.ActualFrom = ""
		coverage.ActualTo = ""
		return coverage
	}
	if coverage.ActualTo > coverage.AvailableTo {
		coverage.ActualTo = coverage.AvailableTo
		coverage.Truncated = true
	}
	if coverage.ActualFrom != "" && coverage.ActualTo != "" && coverage.ActualFrom > coverage.ActualTo {
		coverage.HasData = false
	}
	return coverage
}

func (e *Engine) resolveCoreMetricCoverage(from, to string) coreMetricCoverage {
	return buildCoreMetricCoverage(from, to, e.latestAvailableFinancialPeriod())
}
