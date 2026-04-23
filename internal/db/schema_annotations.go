package db

import (
	"fmt"
	"sort"
	"strings"
)

type SchemaTableAnnotation struct {
	Description string
	Columns     map[string]string
}

var physicalToLogicalTableNames = reverseTableAliasMap(logicalTableNames)

var sqliteSchemaTables = []string{
	"balance_sheet",
	"income_statement",
	"balance_detail",
	"journal",
	"bank_statement",
	"fin_contracts",
	"fin_revenue_settlements",
	"fin_cost_settlements",
	"fin_fund_income",
	"table_idempotency_policies",
	"meta_table_comments",
	"meta_column_comments",
	"dimensions",
	"dimension_members",
	"mapping_rules",
}

var sqliteSourceMetadataTables = []string{
	"journal",
	"fin_journal",
	"bank_statement",
	"fin_bank_statement",
	"income_statement",
	"fin_income_statement",
	"balance_sheet",
	"fin_balance_sheet",
	"balance_detail",
	"fin_balance_detail",
	"fin_contracts",
	"fin_fund_income",
	"fin_cost_settlements",
	"fin_revenue_settlements",
}

var postgresSchemaTables = []string{
	"fin_journal",
	"fin_bank_statement",
	"fin_income_statement",
	"fin_balance_sheet",
	"fin_balance_detail",
	"fin_contracts",
	"fin_revenue_settlements",
	"fin_cost_settlements",
	"fin_fund_income",
	"fin_table_idempotency_policies",
	"fin_dimensions",
	"fin_dimension_members",
	"fin_mapping_rules",
}

var postgresSourceMetadataTables = []string{
	"fin_journal",
	"fin_bank_statement",
	"fin_income_statement",
	"fin_balance_sheet",
	"fin_balance_detail",
	"fin_contracts",
	"fin_fund_income",
	"fin_cost_settlements",
	"fin_revenue_settlements",
}

var commonColumnComments = map[string]string{
	"id":                    "自增主键ID",
	"company":               "公司主体名称",
	"period":                "会计期间（YYYY-MM）",
	"account_code":          "财务科目编码",
	"account_name":          "财务科目名称",
	"account_level":         "科目层级",
	"file_version":          "导入文件版本号",
	"imported_at":           "导入时间",
	"year":                  "会计年度",
	"opening_period":        "会计期间起始月份/期初月份（YYYYMM 或 YYYY-MM）",
	"opening_balance":       "期初余额",
	"closing_balance":       "期末余额",
	"opening_debit":         "期初借方余额",
	"opening_credit":        "期初贷方余额",
	"current_debit":         "本期借方发生额",
	"current_credit":        "本期贷方发生额",
	"closing_debit":         "期末借方余额",
	"closing_credit":        "期末贷方余额",
	"voucher_date":          "凭证日期",
	"voucher_no":            "凭证号",
	"summary":               "摘要",
	"direction":             "借贷方向",
	"debit_amount":          "借方金额",
	"credit_amount":         "贷方金额",
	"amount":                "分录金额",
	"counterparty":          "对手方名称",
	"account_no":            "银行账号",
	"currency":              "币种",
	"transaction_date":      "交易日期",
	"transaction_time":      "交易时间",
	"transaction_type":      "交易类型",
	"balance":               "交易后余额",
	"counterparty_name":     "银行流水对手方名称",
	"counterparty_account":  "银行流水对手方账号",
	"contract_id":           "合同编号",
	"customer_name":         "客户名称",
	"contract_content":      "合同/项目内容",
	"contract_start_date":   "合同开始日期",
	"contract_end_date":     "合同终止日期",
	"settlement_cycle":      "结算周期",
	"settlement_unit_price": "结算单价",
	"created_at":            "创建时间",
	"year_month":            "结算月份（YYYY-MM）",
	"quantity":              "结算工作量/数量",
	"settlement_amount":     "结算金额",
	"is_invoiced":           "是否已开票",
	"invoice_amount":        "开票金额",
	"paid_amount":           "实际付款金额",
	"received_amount":       "实际到账金额",
	"source_report_type":    "导入来源的逻辑报表类型",
	"source_sheet_name":     "导入来源的工作表名称",
	"table_name":            "表名",
	"update_mode":           "幂等更新模式",
	"dedupe_key_columns":    "幂等去重键字段列表",
	"enabled":               "是否启用",
	"updated_at":            "最后更新时间",
	"column_name":           "字段名",
	"comment":               "注释内容",
	"code":                  "编码",
	"name":                  "名称",
	"type":                  "类型",
	"description":           "说明",
	"is_hierarchical":       "是否支持层级",
	"is_active":             "是否启用",
	"metadata":              "扩展元数据（JSON）",
	"dimension_id":          "所属维度ID",
	"parent_id":             "父级成员ID",
	"level":                 "层级深度",
	"path":                  "层级路径",
	"sort_order":            "排序号",
	"rule_name":             "规则名称",
	"priority":              "规则优先级，数值越小越优先",
	"account_code_pattern":  "科目编码匹配模式",
	"account_name_pattern":  "科目名称匹配模式",
	"summary_pattern":       "摘要匹配模式",
	"counterparty_pattern":  "对手方匹配模式",
	"dimension_code":        "目标维度编码",
	"member_code":           "目标维度成员编码",
	"allocation_ratio":      "分摊比例",
	"valid_from":            "规则生效起始期间/日期",
	"valid_to":              "规则生效截止期间/日期",
	"item_name":             "利润表项目名称",
}

var schemaAnnotations = map[string]SchemaTableAnnotation{
	"balance_sheet": buildSchemaTableAnnotation(
		"资产负债表导入结果，按公司、会计期间和科目存储期初/期末余额。",
		map[string]string{
			"period": "会计期间期末月份（YYYY-MM）",
		},
	),
	"income_statement": buildSchemaTableAnnotation(
		"利润表导入结果，按公司、会计期间和项目存储本期发生额与累计发生额。",
		map[string]string{
			"period":            "会计期间月份（YYYY-MM）",
			"current_amount":    "本期发生额",
			"cumulative_amount": "本年累计发生额",
		},
	),
	"balance_detail": buildSchemaTableAnnotation(
		"科目余额表导入结果，按公司、会计期间和科目存储期初余额、本期借贷发生额及期末余额。",
		map[string]string{
			"period": "会计期间期末月份（YYYY-MM）",
		},
	),
	"journal": buildSchemaTableAnnotation(
		"序时账/凭证明细，按借贷分录粒度存储每条会计凭证记录。",
		map[string]string{
			"period": "所属会计期间（YYYY-MM）",
			"amount": "分录金额（通常与借方或贷方金额一致）",
		},
	),
	"bank_statement": buildSchemaTableAnnotation(
		"银行流水明细，按银行账户与交易时间存储每笔收付款记录。",
		map[string]string{
			"summary": "银行流水摘要/附言",
		},
	),
	"fin_contracts": buildSchemaTableAnnotation(
		"合同维度主数据，维护老板口径下的合同/项目基础信息。",
		nil,
	),
	"fin_revenue_settlements": buildSchemaTableAnnotation(
		"已废弃的合同收入结算表，仅保留历史兼容数据，代码默认不再读取。",
		map[string]string{
			"settlement_amount": "收入结算金额（已废弃口径）",
		},
	),
	"fin_cost_settlements": buildSchemaTableAnnotation(
		"合同成本结算明细，记录合同维度的成本结算、开票情况与科目映射。",
		map[string]string{
			"quantity":          "结算工作量/数量（保留原始文本）",
			"settlement_amount": "成本结算金额",
		},
	),
	"fin_fund_income": buildSchemaTableAnnotation(
		"合同资金收入与回款明细，记录合同维度的账面结算、实际到账与开票情况。",
		map[string]string{
			"settlement_amount": "账面结算收入金额",
			"received_amount":   "实际回款金额",
		},
	),
	"table_idempotency_policies": buildSchemaTableAnnotation(
		"导入幂等策略配置，定义各表的数据覆盖方式与唯一去重键。",
		nil,
	),
	"meta_table_comments": buildSchemaTableAnnotation(
		"SQLite 表注释元数据，保存各表的功能说明或来源注释。",
		nil,
	),
	"meta_column_comments": buildSchemaTableAnnotation(
		"SQLite 字段注释元数据，保存各表字段的说明。",
		nil,
	),
	"dimensions": buildSchemaTableAnnotation(
		"财务维度主数据，定义维度编码、名称、类型与层级能力。",
		nil,
	),
	"dimension_members": buildSchemaTableAnnotation(
		"维度成员主数据，维护维度下成员的层级、路径与排序信息。",
		nil,
	),
	"mapping_rules": buildSchemaTableAnnotation(
		"凭证维度映射规则，定义会计分录如何自动映射到维度成员。",
		nil,
	),
}

func buildSchemaTableAnnotation(description string, overrides map[string]string) SchemaTableAnnotation {
	columns := make(map[string]string, len(commonColumnComments)+len(overrides))
	for key, value := range commonColumnComments {
		columns[key] = value
	}
	for key, value := range overrides {
		columns[key] = value
	}
	return SchemaTableAnnotation{
		Description: strings.TrimSpace(description),
		Columns:     columns,
	}
}

func reverseTableAliasMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for logical, physical := range input {
		out[physical] = logical
	}
	return out
}

func schemaAnnotationKey(tableName string) string {
	base := baseTableName(tableName)
	if logical, ok := physicalToLogicalTableNames[base]; ok {
		return logical
	}
	return base
}

func schemaAnnotationForTable(tableName string) (SchemaTableAnnotation, bool) {
	annotation, ok := schemaAnnotations[schemaAnnotationKey(tableName)]
	return annotation, ok
}

func defaultTableDescription(tableName string) string {
	annotation, ok := schemaAnnotationForTable(tableName)
	if !ok {
		return ""
	}
	return strings.TrimSpace(annotation.Description)
}

func defaultColumnComments(tableName string) map[string]string {
	annotation, ok := schemaAnnotationForTable(tableName)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(annotation.Columns))
	for key, value := range annotation.Columns {
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func sqliteBootstrapTableNames() []string {
	out := append([]string(nil), sqliteSchemaTables...)
	sort.Strings(out)
	return out
}

func sqliteSourceMetadataTableNames() []string {
	out := append([]string(nil), sqliteSourceMetadataTables...)
	sort.Strings(out)
	return out
}

func postgresBootstrapTableNames(schema string) []string {
	qualified := make([]string, 0, len(postgresSchemaTables))
	for _, tableName := range postgresSchemaTables {
		qualified = append(qualified, qualifyTableName(schema, tableName))
	}
	sort.Strings(qualified)
	return qualified
}

func postgresSourceMetadataTableNames(schema string) []string {
	qualified := make([]string, 0, len(postgresSourceMetadataTables))
	for _, tableName := range postgresSourceMetadataTables {
		qualified = append(qualified, qualifyTableName(schema, tableName))
	}
	sort.Strings(qualified)
	return qualified
}

func qualifyTableName(schema, tableName string) string {
	schema = strings.TrimSpace(schema)
	tableName = strings.TrimSpace(tableName)
	if schema == "" {
		return tableName
	}
	return fmt.Sprintf("%s.%s", schema, tableName)
}
