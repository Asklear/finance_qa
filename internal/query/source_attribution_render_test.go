package query

import (
	"strings"
	"testing"

	dbpkg "financeqa/internal/db"
)

func TestSourceDisplaysForTablesOmitsMergedGroupHelperTables(t *testing.T) {
	tables := []string{
		"tenant_uhub.fin_contracts",
		"tenant_uhub.fin_cost_settlements",
		"tenant_uhub.fin_cost_settlement_groups",
		"tenant_uhub.fin_cost_settlement_group_members",
	}
	metadata := map[string]dbpkg.TableSourceMetadata{
		"tenant_uhub.fin_contracts": {
			Display: "《合同信息表》",
		},
		"tenant_uhub.fin_cost_settlements": {
			Display: "《优集成本计算表-4.23-池.xlsx》的【成本-月度结算】",
		},
		"tenant_uhub.fin_cost_settlement_groups": {
			Display: "合同成本结算合并金额组，记录 Excel 合并单元格代表的供应商级成本事实，不拆分到单个合同。",
		},
		"tenant_uhub.fin_cost_settlement_group_members": {
			Display: "合同成本结算合并金额组成员表，关联合并金额组与其覆盖的真实合同。",
		},
	}

	displays := sourceDisplaysForTables(tables, metadata)
	joined := strings.Join(displays, "；")
	if strings.Contains(joined, "合并金额组") {
		t.Fatalf("boss-facing source displays should omit helper tables, got %#v", displays)
	}
	if !strings.Contains(joined, "合同信息表") || !strings.Contains(joined, "优集成本计算表") {
		t.Fatalf("business source displays should remain, got %#v", displays)
	}
}
