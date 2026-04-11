package parser

type FileMetadata struct {
	Filename     string `json:"filename"`
	FilePath     string `json:"file_path"`
	FileSize     int64  `json:"file_size"`
	ModifiedTime string `json:"modified_time"`
	ReportType   string `json:"report_type"`
	Company      string `json:"company"`
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`
}

type Record map[string]any

type ParseResult struct {
	Metadata FileMetadata `json:"metadata"`
	Data     []Record     `json:"data"`
}
