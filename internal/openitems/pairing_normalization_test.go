package openitems

import "testing"

func TestNormalizeCounterpartyStripsPurchaseVerbPrefix(t *testing.T) {
	got, source := normalizeCounterparty("", "单位", "购买南京林悦智能科技有限公司服务_南京林悦智能科技有限公司_2025.12.30")
	if got != "南京林悦智能科技有限公司" {
		t.Fatalf("normalizeCounterparty() = %q, want 南京林悦智能科技有限公司", got)
	}
	if source != CounterpartyEvidenceSummary {
		t.Fatalf("source = %s, want %s", source, CounterpartyEvidenceSummary)
	}
}

func TestSameCounterpartyNormalizesParenthesesVariants(t *testing.T) {
	if !sameCounterparty("北京市中闻（南京）律师事务所", "北京市中闻(南京)律师事务所") {
		t.Fatalf("sameCounterparty() should treat full-width and half-width parentheses as the same entity")
	}
}

func TestExtractCompanyNamePrefersCleanSegmentOverDuplicatedVerbNoise(t *testing.T) {
	got := extractCompanyName("转账南京众信数通智转账南京众信数通智能科技有限公司_南京众信数通智能科技有限公司_2026.03.12")
	if got != "南京众信数通智能科技有限公司" {
		t.Fatalf("extractCompanyName() = %q, want 南京众信数通智能科技有限公司", got)
	}
}

func TestExtractCompanyNameDoesNotCollapseToGenericFirmSuffix(t *testing.T) {
	got := extractCompanyName("收到北京市中闻(南京)律师事务所发票_北京市中闻（南京）律师事务所_2026.02.28")
	if got != "北京市中闻(南京)律师事务所" && got != "北京市中闻（南京）律师事务所" {
		t.Fatalf("extractCompanyName() = %q, want 北京市中闻(南京)律师事务所 variant", got)
	}
}
