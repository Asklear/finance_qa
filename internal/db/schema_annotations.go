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
	"fin_cost_settlement_groups",
	"fin_cost_settlement_group_members",
	"fin_fund_income",
	"fin_fund_income_groups",
	"fin_fund_income_group_members",
	"contract_main",
	"contract_pages",
	"contract_invoices",
	"feishu_sync_sources",
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
	"fin_cost_settlement_groups",
	"fin_cost_settlement_group_members",
	"fin_fund_income",
	"fin_fund_income_groups",
	"fin_fund_income_group_members",
	"contract_main",
	"contract_pages",
	"contract_invoices",
	"feishu_sync_sources",
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
	"price_unit":            "价格单位",
	"created_at":            "创建时间",
	"year_month":            "结算月份（YYYY-MM）",
	"quantity":              "结算工作量/数量",
	"settlement_amount":     "结算金额",
	"is_invoiced":           "是否已开票",
	"invoice_amount":        "开票金额",
	"remarks":               "来源表可见备注列内容",
	"paid_amount":           "实际付款金额",
	"received_amount":       "实际到账金额",
	"source_report_type":    "导入来源的逻辑报表类型",
	"source_sheet_name":     "导入来源的工作表名称",
	"source_cell_notes":     "导入来源单元格备注 JSON，按 Excel 单元格坐标保存作者和备注文本",
	"source_start_row":      "导入来源合并区域起始行号",
	"source_end_row":        "导入来源合并区域结束行号",
	"merge_range":           "导入来源合并单元格范围",
	"group_id":              "合并金额组ID",
	"source_row_number":     "导入来源成员合同行号",
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
	"file_name":             "文件名",
	"relative_path":         "相对路径",
	"storage_key":           "对象存储相对路径，正式环境为 OSS object key，例如 tenant/uhub/contract/xxx.pdf",
	"file_hash":             "文件内容 SHA256",
	"file_size":             "文件大小（字节）",
	"sync_status":           "同步状态：active、deleted、error 等",
	"ocr_status":            "OCR 处理状态：pending、running、done、failed 等",
	"ocr_engine":            "OCR 引擎或模型名称",
	"processed_at":          "OCR 处理完成时间",
	"total_pages":           "PDF 总页数",
	"extension_data":        "扩展数据 JSON，保存 OCR 原始结构化结果及错误信息",
	"custom_metrics":        "自定义指标 JSON，保存 OCR 成本、耗时、质量等指标",
	"feishu_file_token":     "飞书文件 token",
	"feishu_root_token":     "飞书扫描来源根文件夹 token",
	"feishu_parent_token":   "飞书父文件夹 token",
	"feishu_relative_path":  "相对飞书来源根目录的文件路径",
	"feishu_folder_path":    "相对飞书来源根目录的父文件夹路径",
	"feishu_slot_key":       "飞书业务位置唯一键，通常由来源根目录和相对路径组成",
	"feishu_file_name":      "飞书侧文件名",
	"feishu_deleted_at":     "飞书侧文件删除或失联标记时间",
	"feishu_relation_key":   "合同/发票目录关联 key",
	"last_seen_at":          "最近一次扫描看到该文件的时间",
	"page_num":              "页码索引，从 0 开始",
	"page_number":           "PDF 原始页码，从 1 开始",
	"markdown_text":         "该页 OCR Markdown 文本",
	"plain_text":            "该页 OCR 纯文本",
	"raw_ocr_json":          "OCR 原始 JSON 结果",
	"has_images":            "该页是否包含图片或视觉内容",
	"has_table":             "该页是否包含表格",
	"has_signature":         "该页是否包含签章或签名",
	"word_count":            "该页文本词数",
	"char_count":            "该页文本字符数",
	"ocr_confidence":        "该页 OCR 置信度",
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
	"fin_cost_settlement_groups": buildSchemaTableAnnotation(
		"合同成本结算合并金额组，记录 Excel 合并单元格代表的供应商级成本事实，不拆分到单个合同。",
		map[string]string{
			"customer_name":     "合并金额组所属供应商/客户名称",
			"settlement_amount": "合并金额组成本结算金额",
			"paid_amount":       "合并金额组实际付款金额",
		},
	),
	"fin_cost_settlement_group_members": buildSchemaTableAnnotation(
		"合同成本结算合并金额组成员表，关联合并金额组与其覆盖的真实合同。",
		nil,
	),
	"fin_fund_income": buildSchemaTableAnnotation(
		"合同资金收入与回款明细，记录合同维度的账面结算、实际到账与开票情况。",
		map[string]string{
			"settlement_amount": "账面结算收入金额",
			"received_amount":   "实际回款金额",
		},
	),
	"fin_fund_income_groups": buildSchemaTableAnnotation(
		"合同资金收入合并金额组，记录 Excel 合并单元格代表的客户级金额事实，不拆分到单个合同。",
		map[string]string{
			"customer_name":     "合并金额组所属客户名称",
			"settlement_amount": "合并金额组账面结算收入金额",
			"received_amount":   "合并金额组实际回款金额",
		},
	),
	"fin_fund_income_group_members": buildSchemaTableAnnotation(
		"合同资金收入合并金额组成员表，关联合并金额组与其覆盖的真实合同。",
		nil,
	),
	"feishu_sync_sources": buildSchemaTableAnnotation(
		"飞书主动扫描来源状态表，记录云盘文件夹和财务表格的扫描游标、同步状态与快照元数据。",
		map[string]string{
			"source_type":       "飞书来源类型，例如 pdf_folder、finance_workbook 或 finance_workbook_folder",
			"source_token":      "飞书文件夹或文件 token",
			"source_url":        "飞书来源访问 URL",
			"display_name":      "来源展示名称",
			"parent_token":      "父级文件夹 token",
			"sync_mode":         "同步模式，V1 固定为 active_scan",
			"sync_status":       "同步状态：active、pending、error 等",
			"last_revision":     "最近成功扫描的飞书 revision",
			"last_content_hash": "最近成功处理的内容 SHA256",
			"last_event_at":     "最近接收飞书事件时间",
			"next_scan_at":      "下一次计划扫描时间",
			"last_sync_at":      "最近一次扫描完成时间",
			"last_success_at":   "最近一次成功扫描时间",
			"error_message":     "最近一次同步失败错误信息",
			"metadata_json":     "同步扩展元数据 JSON，例如 OSS storage_key 与导入统计",
		},
	),
	"contract_main": buildSchemaTableAnnotation(
		"合同 OCR 主表，保存合同 PDF 的飞书同步状态、对象存储位置、OCR 结构化字段和扩展结果。",
		map[string]string{
			"job_id":              "合同导入/OCR 任务 ID",
			"category_id":         "合同分类 ID",
			"sub_category":        "合同子分类；由合同内容识别得出，用于区分不同合同类型",
			"contract_number":     "合同编号",
			"contract_title":      "合同名称或标题",
			"party_a":             "合同甲方名称",
			"party_a_credit_code": "合同甲方统一社会信用代码",
			"party_b":             "合同乙方名称",
			"party_b_credit_code": "合同乙方统一社会信用代码",
			"sign_date":           "合同签署日期",
			"start_date":          "合同开始日期",
			"end_date":            "合同终止日期",
			"contract_amount":     "合同总金额",
			"amount_currency":     "合同金额币种",
			"payment_terms":       "合同付款条款",
			"payment_method":      "合同付款方式",
			"tax_rate":            "合同税率",
			"service_scope":       "合同服务范围",
		},
	),
	"contract_pages": buildSchemaTableAnnotation(
		"合同 OCR 分页正文表，按合同和页码保存全文 OCR 文本、页面结构标记和置信度。",
		nil,
	),
	"contract_invoices": buildSchemaTableAnnotation(
		"合同发票 OCR 表，保存发票 PDF 的合同关联、飞书同步状态、对象存储位置和 OCR 结构化字段。",
		map[string]string{
			"contract_id":              "关联的 contract_main.id",
			"invoice_code":             "发票代码",
			"invoice_number":           "发票号码",
			"invoice_type":             "发票类型",
			"issue_date":               "发票开具日期",
			"check_code":               "发票校验码",
			"machine_number":           "税控机器编号",
			"tax_bureau_code":          "税务机关代码",
			"tax_bureau_name":          "税务机关名称",
			"buyer_name":               "购买方名称",
			"buyer_tax_id":             "购买方纳税人识别号",
			"buyer_address_phone":      "购买方地址电话",
			"buyer_bank_account":       "购买方开户行及账号",
			"seller_name":              "销售方名称",
			"seller_tax_id":            "销售方纳税人识别号",
			"seller_address_phone":     "销售方地址电话",
			"seller_bank_account":      "销售方开户行及账号",
			"total_amount_without_tax": "不含税金额合计",
			"total_tax_amount":         "税额合计",
			"total_amount":             "价税合计金额",
			"total_amount_cn":          "价税合计大写金额",
			"items_json":               "发票明细项目 JSON",
			"remarks":                  "备注",
			"payee":                    "收款人",
			"reviewer":                 "复核人",
			"drawer":                   "开票人",
			"status":                   "发票业务状态",
			"verification_result":      "发票验真或校验结果",
			"verified_at":              "发票验真或校验时间",
			"match_method":             "发票与合同的匹配方式",
			"match_confidence":         "发票与合同的匹配置信度",
			"feishu_relation_key":      "合同/发票目录关联 key，去掉发票目录后的业务目录",
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
