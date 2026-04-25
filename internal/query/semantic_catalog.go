package query

import (
	"context"
	"sort"
	"strings"

	dbpkg "financeqa/internal/db"
)

const (
	SourceCapabilityContractDimension = "contract_dimension"
	SourceCapabilityCustomerDimension = "customer_dimension"
	SourceCapabilitySupplierDimension = "supplier_dimension"
	SourceCapabilityProjectDimension  = "project_dimension"
	SourceCapabilityOpenItems         = "open_items"
	SourceCapabilityOfficialBalance   = "official_balance"
	SourceCapabilityCashEvidence      = "cash_evidence"
	SourceCapabilityAccrualEvidence   = "accrual_evidence"
)

type SourceCapabilityProfile struct {
	Table           string
	Display         string
	LogicalLabel    string
	Metrics         []BossMetric
	Capabilities    []string
	Fields          map[string]string
	Deprecated      bool
	Eligible        bool
	SourceDocuments []string
	ReportTypes     []string
	Notes           []string
	Description     string
}

type SemanticCatalog struct {
	Profiles map[string]SourceCapabilityProfile
}

func (e *Engine) BuildSemanticCatalog(ctx context.Context, tables []string) SemanticCatalog {
	catalog := SemanticCatalog{Profiles: map[string]SourceCapabilityProfile{}}
	if len(tables) == 0 || e == nil || e.db == nil {
		return catalog
	}

	metadata, err := dbpkg.LoadTableSourceMetadata(ctx, e.db, e.dbPath, tables)
	if err != nil {
		metadata = make(map[string]dbpkg.TableSourceMetadata, len(tables))
		for _, tableName := range tables {
			metadata[tableName] = dbpkg.DefaultTableSourceMetadata(tableName)
		}
	}
	columnComments, err := dbpkg.LoadTableColumnComments(ctx, e.db, e.dbPath, tables)
	if err != nil {
		columnComments = dbpkg.TableColumnComments{}
	}

	for _, tableName := range tables {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		meta, ok := metadata[tableName]
		if !ok {
			meta = dbpkg.DefaultTableSourceMetadata(tableName)
		}
		fields := columnComments[tableName]
		if fields == nil {
			fields = map[string]string{}
		}
		profile := buildSourceCapabilityProfile(tableName, meta, fields)
		catalog.Profiles[tableName] = profile
	}

	return catalog
}

func buildSourceCapabilityProfile(tableName string, meta dbpkg.TableSourceMetadata, fields map[string]string) SourceCapabilityProfile {
	normalized := semanticBaseTableName(tableName)
	metrics, capabilities := inferSourceCapabilities(normalized, meta, fields)
	deprecated := isDeprecatedSemanticSource(normalized, meta)

	profile := SourceCapabilityProfile{
		Table:           tableName,
		Display:         strings.TrimSpace(meta.Display),
		LogicalLabel:    strings.TrimSpace(meta.LogicalLabel),
		Metrics:         dedupeBossMetrics(metrics),
		Capabilities:    dedupeStrings(capabilities),
		Fields:          cloneSemanticStringMap(fields),
		Deprecated:      deprecated,
		Eligible:        !deprecated,
		SourceDocuments: sourceDocumentsFromMetadata(meta),
		ReportTypes:     dedupeStrings(meta.ReportTypes),
		Notes:           trimStringSlice(meta.Notes),
		Description:     strings.TrimSpace(meta.Description),
	}
	if profile.LogicalLabel == "" {
		profile.LogicalLabel = normalized
	}
	sort.Strings(profile.SourceDocuments)
	sort.Strings(profile.ReportTypes)
	sort.Strings(profile.Notes)
	return profile
}

func inferSourceCapabilities(tableName string, meta dbpkg.TableSourceMetadata, fields map[string]string) ([]BossMetric, []string) {
	switch tableName {
	case "fin_fund_income":
		return []BossMetric{BossMetricRevenue, BossMetricReceipts, BossMetricInvoice}, []string{
			SourceCapabilityContractDimension,
			SourceCapabilityCustomerDimension,
			SourceCapabilityProjectDimension,
		}
	case "fin_cost_settlements":
		return []BossMetric{BossMetricCost, BossMetricPayments, BossMetricInvoice}, []string{
			SourceCapabilityContractDimension,
			SourceCapabilitySupplierDimension,
			SourceCapabilityProjectDimension,
		}
	case "fin_contracts":
		return []BossMetric{}, []string{
			SourceCapabilityContractDimension,
			SourceCapabilityCustomerDimension,
			SourceCapabilitySupplierDimension,
			SourceCapabilityProjectDimension,
		}
	case "fin_bank_statement", "bank_statement":
		return []BossMetric{BossMetricReceipts, BossMetricPayments, BossMetricCashFlow}, []string{
			SourceCapabilityCashEvidence,
			SourceCapabilityCustomerDimension,
			SourceCapabilitySupplierDimension,
		}
	case "fin_journal", "journal":
		return []BossMetric{
				BossMetricRevenue,
				BossMetricCost,
				BossMetricProfit,
				BossMetricARAP,
				BossMetricTax,
				BossMetricHRCost,
			}, []string{
				SourceCapabilityAccrualEvidence,
				SourceCapabilityOpenItems,
				SourceCapabilityCustomerDimension,
				SourceCapabilitySupplierDimension,
			}
	case "fin_income_statement", "income_statement":
		return []BossMetric{BossMetricRevenue, BossMetricCost, BossMetricProfit}, []string{
			SourceCapabilityAccrualEvidence,
		}
	case "fin_balance_detail", "balance_detail", "fin_balance_sheet", "balance_sheet":
		return []BossMetric{BossMetricARAP}, []string{
			SourceCapabilityOfficialBalance,
			SourceCapabilityOpenItems,
		}
	case "fin_revenue_settlements":
		return []BossMetric{BossMetricRevenue, BossMetricInvoice}, []string{
			SourceCapabilityContractDimension,
			SourceCapabilityCustomerDimension,
		}
	default:
		return inferCapabilitiesFromComments(meta, fields)
	}
}

func inferCapabilitiesFromComments(meta dbpkg.TableSourceMetadata, fields map[string]string) ([]BossMetric, []string) {
	textParts := []string{meta.Display, meta.LogicalLabel, meta.Description}
	textParts = append(textParts, meta.ReportTypes...)
	for name, comment := range fields {
		textParts = append(textParts, name, comment)
	}
	text := strings.ToLower(strings.Join(textParts, " "))

	var metrics []BossMetric
	var capabilities []string
	if strings.Contains(text, "收入") || strings.Contains(text, "营收") || strings.Contains(text, "sales") || strings.Contains(text, "revenue") {
		metrics = append(metrics, BossMetricRevenue)
	}
	if strings.Contains(text, "成本") || strings.Contains(text, "cost") {
		metrics = append(metrics, BossMetricCost)
	}
	if strings.Contains(text, "利润") || strings.Contains(text, "profit") {
		metrics = append(metrics, BossMetricProfit)
	}
	if strings.Contains(text, "回款") || strings.Contains(text, "到账") || strings.Contains(text, "received") {
		metrics = append(metrics, BossMetricReceipts)
	}
	if strings.Contains(text, "付款") || strings.Contains(text, "支付") || strings.Contains(text, "paid") {
		metrics = append(metrics, BossMetricPayments)
	}
	if strings.Contains(text, "发票") || strings.Contains(text, "开票") || strings.Contains(text, "invoice") {
		metrics = append(metrics, BossMetricInvoice)
	}
	if strings.Contains(text, "合同") || strings.Contains(text, "项目") || strings.Contains(text, "contract") {
		capabilities = append(capabilities, SourceCapabilityContractDimension, SourceCapabilityProjectDimension)
	}
	if strings.Contains(text, "客户") || strings.Contains(text, "customer") {
		capabilities = append(capabilities, SourceCapabilityCustomerDimension)
	}
	if strings.Contains(text, "供应商") || strings.Contains(text, "supplier") {
		capabilities = append(capabilities, SourceCapabilitySupplierDimension)
	}
	return metrics, capabilities
}

func isDeprecatedSemanticSource(tableName string, meta dbpkg.TableSourceMetadata) bool {
	if tableName == "fin_revenue_settlements" {
		return true
	}
	text := strings.ToLower(strings.Join(append(append([]string{}, meta.ReportTypes...), meta.Notes...), " "))
	return strings.Contains(text, "deprecated") || strings.Contains(text, "废弃") || strings.Contains(text, "暂停使用")
}

func sourceDocumentsFromMetadata(meta dbpkg.TableSourceMetadata) []string {
	if display := strings.TrimSpace(meta.Display); display != "" {
		return []string{display}
	}
	docs := make([]string, 0, len(meta.FileNames))
	for _, fileName := range meta.FileNames {
		fileName = strings.TrimSpace(fileName)
		if fileName == "" {
			continue
		}
		if len(meta.SheetNames) == 0 {
			docs = append(docs, "《"+fileName+"》")
			continue
		}
		for _, sheet := range meta.SheetNames {
			sheet = strings.TrimSpace(sheet)
			if sheet == "" {
				continue
			}
			docs = append(docs, "《"+fileName+"》的【"+sheet+"】")
		}
	}
	return dedupeStrings(docs)
}

func semanticBaseTableName(tableName string) string {
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return ""
	}
	if idx := strings.LastIndex(tableName, "."); idx >= 0 {
		tableName = tableName[idx+1:]
	}
	return strings.Trim(tableName, `"`)
}

func dedupeBossMetrics(items []BossMetric) []BossMetric {
	seen := map[BossMetric]struct{}{}
	out := make([]BossMetric, 0, len(items))
	for _, item := range items {
		if item == "" || item == BossMetricUnknown {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func cloneSemanticStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func trimStringSlice(input []string) []string {
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return dedupeStrings(out)
}
