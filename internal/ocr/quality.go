package ocr

import (
	"math"
	"strings"
	"time"
)

func Validate(result Result) QualityReport {
	flags := append([]string{}, result.QualityFlags...)
	switch strings.TrimSpace(result.DocumentType) {
	case DocumentTypeContract:
		flags = append(flags, validateContract(result)...)
	case DocumentTypeInvoice:
		flags = append(flags, validateInvoice(result)...)
	default:
		flags = append(flags, "unknown_document_type")
	}
	if strings.TrimSpace(result.OCRTextExcerpt) == "" {
		flags = append(flags, "low_evidence")
	}
	flags = dedupe(flags)
	if len(flags) > 0 {
		return QualityReport{Status: QualityNeedsReview, Flags: flags}
	}
	return QualityReport{Status: QualityPass}
}

func validateContract(result Result) []string {
	var flags []string
	contract := result.Contract
	filled := 0
	for _, value := range []string{contract.ContractTitle, contract.PartyA, contract.PartyB} {
		if strings.TrimSpace(value) != "" {
			filled++
		}
	}
	if filled < 2 {
		flags = append(flags, "missing_parties")
	}
	if strings.TrimSpace(contract.ContractTitle) == "" {
		flags = append(flags, "missing_title")
	}
	if contract.TotalAmount != nil && *contract.TotalAmount <= 0 {
		flags = append(flags, "missing_amount")
	}
	if !validTaxRate(contract.TaxRate) {
		flags = append(flags, "tax_rate_invalid")
	}
	if contract.StartDate != "" && contract.EndDate != "" {
		start, startOK := parseDate(contract.StartDate)
		end, endOK := parseDate(contract.EndDate)
		if startOK && endOK && start.After(end) {
			flags = append(flags, "date_range_invalid")
		}
	}
	return flags
}

func validateInvoice(result Result) []string {
	var flags []string
	invoice := result.Invoice
	if strings.TrimSpace(invoice.InvoiceNumber) == "" {
		flags = append(flags, "missing_invoice_number")
	}
	if _, ok := parseDate(invoice.IssueDate); !ok {
		flags = append(flags, "missing_issue_date")
	}
	if strings.TrimSpace(invoice.BuyerName) == "" || strings.TrimSpace(invoice.SellerName) == "" {
		flags = append(flags, "missing_buyer_or_seller")
	}
	if invoice.TotalAmount == nil || *invoice.TotalAmount <= 0 {
		flags = append(flags, "missing_total_amount")
	}
	if invoice.PreTaxAmount != nil && invoice.TaxAmount != nil && invoice.TotalAmount != nil {
		if math.Abs((*invoice.PreTaxAmount+*invoice.TaxAmount)-*invoice.TotalAmount) > 0.02 {
			flags = append(flags, "amount_sum_mismatch")
		}
	}
	for _, item := range invoice.Items {
		if !validTaxRate(item.TaxRate) {
			flags = append(flags, "tax_rate_invalid")
			break
		}
	}
	return flags
}

func validTaxRate(value *float64) bool {
	if value == nil {
		return true
	}
	return *value >= 0 && *value <= 1
}

func parseDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return parsed, err == nil
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
