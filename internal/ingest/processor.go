package ingest

type Processor struct {
	importer *Importer
}

func NewProcessor() *Processor {
	return &Processor{importer: NewImporter()}
}

func (p *Processor) ProcessFile(path string) (ImportSummary, error) {
	result, err := p.importer.ParseFile(path)
	if err != nil {
		return ImportSummary{}, err
	}
	return ImportSummary{
		FilePath:    path,
		ReportType:  result.Metadata.ReportType,
		Company:     result.Metadata.Company,
		PeriodStart: result.Metadata.PeriodStart,
		PeriodEnd:   result.Metadata.PeriodEnd,
		RecordCount: len(result.Data),
	}, nil
}
