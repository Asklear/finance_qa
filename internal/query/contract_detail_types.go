package query

type ContractDetailIntent string

const (
	ContractDetailIntentUnknown ContractDetailIntent = "unknown"
	ContractDetailIntentClause  ContractDetailIntent = "clause"
	ContractDetailIntentInvoice ContractDetailIntent = "invoice"
	ContractDetailIntentField   ContractDetailIntent = "field"
	ContractDetailIntentPage    ContractDetailIntent = "page_text"
)

type ContractDetailProbeResult struct {
	Intent              ContractDetailIntent
	CandidateTables     []string
	MatchedContractRows int
	HasStructuredAnswer bool
	NeedsPageText       bool
	Confidence          float64
	Reason              string
}

type ContractDetailResult struct {
	MatchedContracts []ContractDetailMatch
	InvoiceSummary   ContractInvoiceSummaryDetail
	Invoices         []ContractInvoiceDetail
	PageSnippets     []ContractPageSnippet
	SourceTables     []string
	Probe            ContractDetailProbeResult
}

type ContractDetailMatch struct {
	ContractNumber  string
	ContractTitle   string
	PartyA          string
	PartyB          string
	SignDate        string
	StartDate       string
	EndDate         string
	ContractAmount  float64
	Currency        string
	SettlementCycle string
	UnitPrice       string
	PaymentTerms    string
	PaymentMethod   string
	TaxRate         float64
	ServiceScope    string
	FileName        string
	UpdatedAt       string
}

type ContractInvoiceSummaryDetail struct {
	InvoiceCount        int
	TotalInvoicedAmount float64
	TotalTaxAmount      float64
	ContractAmount      float64
	InvoicedRatio       float64
	LatestInvoiceDate   string
	LatestInvoiceNumber string
}

type ContractInvoiceDetail struct {
	ContractID             string
	InvoiceNumber         string
	IssueDate             string
	BuyerName             string
	SellerName            string
	TotalAmountWithoutTax float64
	TotalTaxAmount        float64
	TotalAmount           float64
	Remarks               string
	ItemsJSON             string
	ItemsSummary          string
	FileName              string
	UpdatedAt             string
}

type ContractPageSnippet struct {
	PageNumber int
	Text       string
}

type contractDetailCandidate struct {
	internalID       string
	ContractNumber   string
	ContractTitle    string
	PartyA           string
	PartyB           string
	SignDate         string
	StartDate        string
	EndDate          string
	ContractAmount   float64
	Currency         string
	SettlementCycle  string
	UnitPrice        string
	PaymentTerms     string
	PaymentMethod    string
	TaxRate          float64
	ServiceScope     string
	FileName         string
	UpdatedAt        string
	matchScore       int
	structuredFields int
}
