package query

func buildContractAggregateResultSnapshot(spec QuerySpec, summary contractAggregateSummary) (string, map[string]any) {
	selection := resolveContractAggregateSelection(spec, summary)
	return buildContractAggregateResultMessage(selection, summary), buildContractAggregateResultData(selection, summary)
}
