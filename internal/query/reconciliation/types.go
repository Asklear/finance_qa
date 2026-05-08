package reconciliation

type EvidenceLevel string

const (
	EvidenceDirect  EvidenceLevel = "direct"
	EvidenceDerived EvidenceLevel = "derived"
	EvidenceUnknown EvidenceLevel = "unknown"
)

type CounterpartySnapshot struct {
	Name                    string        `json:"name"`
	Role                    string        `json:"role"`
	BankIn                  float64       `json:"bank_in"`
	BankOut                 float64       `json:"bank_out"`
	ARDecrease              float64       `json:"ar_decrease"`
	ARIncrease              float64       `json:"ar_increase"`
	APDecrease              float64       `json:"ap_decrease"`
	APIncrease              float64       `json:"ap_increase"`
	PrepaymentIncrease      float64       `json:"prepayment_increase"`
	PrepaymentCleared       float64       `json:"prepayment_cleared"`
	RevenueNet              float64       `json:"revenue_net"`
	OutputVAT               float64       `json:"output_vat"`
	InputVAT                float64       `json:"input_vat"`
	BookCost                float64       `json:"book_cost"`
	BookExpense             float64       `json:"book_expense"`
	ComparisonBasis         string        `json:"comparison_basis"`
	DifferenceReason        string        `json:"difference_reason"`
	EvidenceLevel           EvidenceLevel `json:"evidence_level"`
	RequiresMonthDisclosure bool          `json:"requires_month_disclosure"`
	Support                 []string      `json:"support"`
}

type VoucherContext struct {
	Period      string
	VoucherDate string
	VoucherNo   string
}

type MonthlyBookView struct {
	Revenue             float64 `json:"revenue"`
	Cost                float64 `json:"cost"`
	TaxSurcharge        float64 `json:"tax_surcharge"`
	SellingExpense      float64 `json:"selling_expense"`
	AdminExpense        float64 `json:"admin_expense"`
	FinanceExpense      float64 `json:"finance_expense"`
	NonOperatingIncome  float64 `json:"non_operating_income"`
	NonOperatingExpense float64 `json:"non_operating_expense"`
	OperatingProfit     float64 `json:"operating_profit"`
	Profit              float64 `json:"profit"`
	NetProfit           float64 `json:"net_profit"`
	IncomeTax           float64 `json:"income_tax"`
	TotalCost           float64 `json:"total_cost"`
}
