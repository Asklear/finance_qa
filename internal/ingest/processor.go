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
	if kind, ok, err := detectContractWorkbookKind(path); err != nil {
		return ImportSummary{}, err
	} else if ok {
		bundle, err := parseContractWorkbook(path, kind)
		if err != nil {
			return ImportSummary{}, err
		}
		return ImportSummary{
			FilePath:    path,
			ReportType:  string(kind),
			PeriodStart: bundle.PeriodStart,
			PeriodEnd:   bundle.PeriodEnd,
			RecordCount: bundle.TotalRecordCount,
		}, nil
	}

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
