package ocr

import "time"

const (
	DocumentTypeContract = "contract"
	DocumentTypeInvoice  = "invoice"
	DocumentTypeUnknown  = "unknown"

	QualityPass        = "pass"
	QualityNeedsReview = "needs_review"
	QualityFailed      = "failed"
)

type Result struct {
	DocumentType    string         `json:"document_type"`
	FileSummary     string         `json:"file_summary,omitempty"`
	Contract        ContractResult `json:"contract"`
	Invoice         InvoiceResult  `json:"invoice"`
	Pages           []PageResult   `json:"pages"`
	OCRTextExcerpt  string         `json:"ocr_text_excerpt"`
	ConfidenceNotes string         `json:"confidence_notes"`
	QualityFlags    []string       `json:"quality_flags"`
}

type PageResult struct {
	PageNumber   int     `json:"page_number"`
	MarkdownText string  `json:"markdown_text"`
	PlainText    string  `json:"plain_text"`
	HasTable     bool    `json:"has_table"`
	HasSignature bool    `json:"has_signature"`
	Confidence   float64 `json:"confidence"`
}

type ContractResult struct {
	ContractTitle       string            `json:"contract_title"`
	SubCategory         string            `json:"sub_category"`
	ContractNumber      string            `json:"contract_number"`
	PartyA              string            `json:"party_a"`
	PartyACreditCode    string            `json:"party_a_credit_code"`
	PartyB              string            `json:"party_b"`
	PartyBCreditCode    string            `json:"party_b_credit_code"`
	SignDate            string            `json:"sign_date"`
	StartDate           string            `json:"start_date"`
	EndDate             string            `json:"end_date"`
	TotalAmount         *float64          `json:"total_contract_amount"`
	Currency            string            `json:"currency"`
	PaymentSchedule     []PaymentSchedule `json:"payment_schedule"`
	PaymentTerms        string            `json:"payment_terms"`
	ServiceScopeSummary string            `json:"service_scope_summary"`
	SettlementCycle     string            `json:"settlement_cycle"`
	SettlementUnitPrice *float64          `json:"settlement_unit_price"`
	PriceUnit           string            `json:"price_unit"`
	PaymentMethod       string            `json:"payment_method"`
	TaxRate             *float64          `json:"tax_rate"`
}

type PaymentSchedule struct {
	Amount    *float64 `json:"amount"`
	DueDate   string   `json:"due_date"`
	Condition string   `json:"condition"`
}

type InvoiceResult struct {
	InvoiceType   string        `json:"invoice_type"`
	InvoiceNumber string        `json:"invoice_number"`
	InvoiceCode   string        `json:"invoice_code"`
	IssueDate     string        `json:"issue_date"`
	CheckCode     string        `json:"check_code"`
	MachineNumber string        `json:"machine_number"`
	TaxBureauCode string        `json:"tax_bureau_code"`
	TaxBureauName string        `json:"tax_bureau_name"`
	BuyerName     string        `json:"buyer_name"`
	BuyerTaxID    string        `json:"buyer_tax_id"`
	SellerName    string        `json:"seller_name"`
	SellerTaxID   string        `json:"seller_tax_id"`
	PreTaxAmount  *float64      `json:"pre_tax_amount"`
	TaxAmount     *float64      `json:"tax_amount"`
	TotalAmount   *float64      `json:"total_amount"`
	TotalAmountCN string        `json:"total_amount_cn"`
	Currency      string        `json:"currency"`
	Items         []InvoiceItem `json:"items"`
	Remarks       string        `json:"remarks"`
	Payee         string        `json:"payee"`
	Reviewer      string        `json:"reviewer"`
	Drawer        string        `json:"drawer"`
}

type InvoiceItem struct {
	Name      string   `json:"name"`
	Spec      string   `json:"spec"`
	Unit      string   `json:"unit"`
	Quantity  string   `json:"quantity"`
	UnitPrice *float64 `json:"unit_price"`
	Amount    *float64 `json:"amount"`
	TaxRate   *float64 `json:"tax_rate"`
	TaxAmount *float64 `json:"tax_amount"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type RunMetadata struct {
	Model            string    `json:"model"`
	ElapsedSeconds   float64   `json:"elapsed_seconds"`
	Usage            Usage     `json:"usage"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	ProcessedAt      time.Time `json:"processed_at"`
}

type QualityReport struct {
	Status string   `json:"status"`
	Flags  []string `json:"flags"`
}

type PendingDocument struct {
	ID         int64  `json:"id"`
	Table      string `json:"table,omitempty"`
	FileName   string `json:"file_name"`
	StorageKey string `json:"storage_key"`
	FileHash   string `json:"file_hash"`
	ContractID int64  `json:"contract_id,omitempty"`
}
