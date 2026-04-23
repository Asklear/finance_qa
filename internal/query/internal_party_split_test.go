package query

import (
	"strings"
	"testing"
)

func TestResolveInternalPartyFromTextsPrefersSharedBrandBranch(t *testing.T) {
	cfg := getRuleConfig()

	party, basis := resolveInternalPartyFromTexts(
		"南京优集数据科技有限公司",
		[]string{
			"支付南京优集杭州分公司代发薪酬",
			"上海分公司",
			"招商银行",
		},
		1,
		cfg,
	)

	if party != "南京优集杭州分公司" {
		t.Fatalf("party = %q, want %q", party, "南京优集杭州分公司")
	}
	if !strings.Contains(basis, "shared_brand") {
		t.Fatalf("basis should include shared_brand, got: %s", basis)
	}
	if !strings.Contains(basis, "internal_account_context") {
		t.Fatalf("basis should include internal_account_context, got: %s", basis)
	}
}
