package query

import "testing"

func TestShouldIncludeSupplierPaymentCounterpartyExcludesConfiguredPseudoNames(t *testing.T) {
	engine := &Engine{Company: "南京优集数据科技有限公司"}

	cases := []struct {
		name string
		role CounterpartyRole
	}{
		{name: "暂收款", role: CounterpartyMixed},
		{name: "网上电子汇划收入", role: CounterpartyMixed},
		{name: "对公中间业务收入-网上其他收入", role: CounterpartySupplier},
	}

	for _, tc := range cases {
		include, reason := engine.shouldIncludeSupplierPaymentCounterparty(tc.name, CounterpartyClassification{Role: tc.role})
		if include {
			t.Fatalf("%s should be excluded from supplier payments, got include=true", tc.name)
		}
		if reason != "non_counterparty_flow" {
			t.Fatalf("%s should use non_counterparty_flow reason, got %s", tc.name, reason)
		}
	}
}
