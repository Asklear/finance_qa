package ocr_test

import (
	"testing"

	"financeqa/internal/ocr"
)

func floatPtr(v float64) *float64 {
	return &v
}

func TestValidateContractPassesCoreFields(t *testing.T) {
	report := ocr.Validate(ocr.Result{
		DocumentType:   ocr.DocumentTypeContract,
		OCRTextExcerpt: "合同总金额为人民币300000元",
		Contract: ocr.ContractResult{
			ContractTitle: "数据服务合同",
			PartyA:        "南京优集数据科技有限公司",
			PartyB:        "深圳市五块石科技有限公司",
			StartDate:     "2026-02-01",
			EndDate:       "2027-01-31",
			TotalAmount:   floatPtr(300000),
			Currency:      "CNY",
			TaxRate:       floatPtr(0.06),
		},
	})
	if report.Status != ocr.QualityPass {
		t.Fatalf("status = %s flags=%v", report.Status, report.Flags)
	}
}

func TestValidateContractFlagsDateRange(t *testing.T) {
	report := ocr.Validate(ocr.Result{
		DocumentType:   ocr.DocumentTypeContract,
		OCRTextExcerpt: "合同期限",
		Contract: ocr.ContractResult{
			ContractTitle: "合同",
			PartyA:        "甲方公司",
			PartyB:        "乙方公司",
			StartDate:     "2027-01-31",
			EndDate:       "2026-02-01",
		},
	})
	if report.Status != ocr.QualityNeedsReview || !hasFlag(report.Flags, "date_range_invalid") {
		t.Fatalf("report = %#v", report)
	}
}

func TestValidateInvoiceFlagsAmountMismatch(t *testing.T) {
	report := ocr.Validate(ocr.Result{
		DocumentType:   ocr.DocumentTypeInvoice,
		OCRTextExcerpt: "价税合计",
		Invoice: ocr.InvoiceResult{
			InvoiceNumber: "123",
			IssueDate:     "2026-02-28",
			BuyerName:     "南京优集数据科技有限公司",
			SellerName:    "深圳市五块石科技有限公司",
			PreTaxAmount:  floatPtr(100),
			TaxAmount:     floatPtr(6),
			TotalAmount:   floatPtr(105),
		},
	})
	if report.Status != ocr.QualityNeedsReview || !hasFlag(report.Flags, "amount_sum_mismatch") {
		t.Fatalf("report = %#v", report)
	}
}

func hasFlag(flags []string, want string) bool {
	for _, flag := range flags {
		if flag == want {
			return true
		}
	}
	return false
}
