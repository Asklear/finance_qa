package integration

import (
	"strings"
	"testing"
)

func TestLiveDBRevenueAggregateSourceAttributionStaysRevenueScoped(t *testing.T) {
	engine := requireLiveDBEngine(t, "南京优集数据科技有限公司")

	res := engine.Query("2026年Q1收入是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	for _, tableName := range sourceTables {
		if tableName == "tenant_uhub.fin_cost_settlements" {
			t.Fatalf("source_tables should not include tenant_uhub.fin_cost_settlements for revenue-only aggregate, got %#v", sourceTables)
		}
	}

	sourceNote, _ := res.Data["source_note"].(string)
	if strings.Contains(sourceNote, "优集成本计算表") {
		t.Fatalf("source_note should stay revenue-scoped for revenue-only aggregate, got %q", sourceNote)
	}

	if res.Data["supporting_source_partitions"] != nil {
		t.Fatalf("supporting_source_partitions should be nil for revenue-only aggregate, got %#v", res.Data["supporting_source_partitions"])
	}
}

func TestLiveDBCostAggregatePayloadStaysCostScoped(t *testing.T) {
	engine := requireLiveDBEngine(t, "南京优集数据科技有限公司")

	res := engine.Query("2026年Q1成本是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	sourceTables, ok := res.Data["source_tables"].([]string)
	if !ok {
		t.Fatalf("source_tables missing or wrong type: %#v", res.Data["source_tables"])
	}
	for _, tableName := range sourceTables {
		if tableName == "tenant_uhub.fin_fund_income" {
			t.Fatalf("source_tables should not include tenant_uhub.fin_fund_income for cost-only aggregate, got %#v", sourceTables)
		}
	}

	moneyView, ok := res.Data["money_view"].(map[string]any)
	if !ok {
		t.Fatalf("money_view missing or wrong type: %#v", res.Data["money_view"])
	}
	if _, ok := moneyView["付款"]; !ok {
		t.Fatalf("money_view should include 付款 for cost-only aggregate, got %#v", moneyView)
	}
	if _, ok := moneyView["到账"]; ok {
		t.Fatalf("money_view should omit 到账 for cost-only aggregate, got %#v", moneyView)
	}

	accountView, ok := res.Data["account_view"].(map[string]any)
	if !ok {
		t.Fatalf("account_view missing or wrong type: %#v", res.Data["account_view"])
	}
	if _, ok := accountView["合同成本"]; !ok {
		t.Fatalf("account_view should include 合同成本 for cost-only aggregate, got %#v", accountView)
	}
	if _, ok := accountView["营收"]; ok {
		t.Fatalf("account_view should omit 营收 for cost-only aggregate, got %#v", accountView)
	}
}

func TestLiveDBProfitAggregatePayloadStaysProfitScoped(t *testing.T) {
	engine := requireLiveDBEngine(t, "南京优集数据科技有限公司")

	res := engine.Query("2026年Q1利润是多少？")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	moneyView, ok := res.Data["money_view"].(map[string]any)
	if !ok {
		t.Fatalf("money_view missing or wrong type: %#v", res.Data["money_view"])
	}
	if _, ok := moneyView["净现金"]; !ok {
		t.Fatalf("money_view should include 净现金 for profit-only aggregate, got %#v", moneyView)
	}
	if _, ok := moneyView["回款"]; !ok {
		t.Fatalf("money_view should include 回款 for profit-only aggregate, got %#v", moneyView)
	}
	if _, ok := moneyView["付款"]; !ok {
		t.Fatalf("money_view should include 付款 for profit-only aggregate, got %#v", moneyView)
	}

	accountView, ok := res.Data["account_view"].(map[string]any)
	if !ok {
		t.Fatalf("account_view missing or wrong type: %#v", res.Data["account_view"])
	}
	if _, ok := accountView["利润"]; !ok {
		t.Fatalf("account_view should include 利润 for profit-only aggregate, got %#v", accountView)
	}
	if _, ok := accountView["营收"]; ok {
		t.Fatalf("account_view should omit 营收 for profit-only aggregate, got %#v", accountView)
	}
	if _, ok := accountView["合同成本"]; ok {
		t.Fatalf("account_view should omit 合同成本 for profit-only aggregate, got %#v", accountView)
	}
}
