package ingest

import (
	"financeqa/internal/dimensions"
)

type Processor struct {
	importer *Importer
	dim      *dimensions.Manager
}

func NewProcessor(dim *dimensions.Manager) *Processor {
	return &Processor{
		importer: NewImporter(dim),
		dim:      dim,
	}
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
