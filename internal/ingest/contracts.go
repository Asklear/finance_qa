package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	dbschema "financeqa/internal/db"

	"github.com/xuri/excelize/v2"
)

var fundQuarterSheetPattern = regexp.MustCompile(`^(\d{2,4})年Q([1-4])收入明细$`)

type contractWorkbookKind string

const (
	contractWorkbookUnknown     contractWorkbookKind = ""
	contractWorkbookRevenueCost contractWorkbookKind = "contract_revenue_cost"
	contractWorkbookFund        contractWorkbookKind = "contract_fund_income"
)

type contractKey struct {
	Name    string
	Content string
}

type contractDimensionMeta struct {
	ContractStartDate string
	ContractEndDate   string
	SettlementCycle   string
}

type contractRevenueSettlementRow struct {
	contractKey
	SourceSheetName     string
	YearMonth           string
	Quantity            string
	SettlementAmount    float64
	IsInvoiced          string
	InvoiceAmount       float64
	ContractStartDate   string
	ContractEndDate     string
	SettlementCycle     string
	SettlementUnitPrice string
}

type contractCostSettlementRow struct {
	contractKey
	SourceSheetName     string
	YearMonth           string
	Quantity            string
	SettlementAmount    float64
	IsInvoiced          string
	InvoiceAmount       float64
	PaidAmount          float64
	AccountCode         string
	ContractStartDate   string
	ContractEndDate     string
	SettlementCycle     string
	SettlementUnitPrice string
}

type contractFundIncomeRow struct {
	contractKey
	SourceSheetName     string
	YearMonth           string
	Quantity            string
	SettlementAmount    float64
	ReceivedAmount      float64
	IsInvoiced          string
	InvoiceAmount       float64
	ContractStartDate   string
	ContractEndDate     string
	SettlementCycle     string
	SettlementUnitPrice string
}

type contractImportBundle struct {
	Kind                 contractWorkbookKind
	RevenueRows          []contractRevenueSettlementRow
	CostRows             []contractCostSettlementRow
	FundRows             []contractFundIncomeRow
	ContractKeys         []contractKey
	ContractDetails      map[contractKey]contractDimensionMeta
	PeriodStart          string
	PeriodEnd            string
	TotalRecordCount     int
	TableSourceSheets    map[string][]string
	ContractSourceSheets []string
}

func detectContractWorkbookKind(path string) (contractWorkbookKind, bool, error) {
	if strings.ToLower(filepath.Ext(path)) != ".xlsx" {
		return contractWorkbookUnknown, false, nil
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		return contractWorkbookUnknown, false, nil
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	hasRevenue := containsString(sheets, "收入-月度结算")
	hasCost := containsString(sheets, "成本-月度结算")
	hasFundQuarter := len(fundQuarterSheetNames(sheets)) > 0

	switch {
	case hasRevenue || hasCost:
		return contractWorkbookRevenueCost, true, nil
	case hasFundQuarter:
		return contractWorkbookFund, true, nil
	default:
		return contractWorkbookUnknown, false, nil
	}
}

func (i *Importer) importContractWorkbook(ctx context.Context, dbPath, filePath string, kind contractWorkbookKind, opts ImportOptions) (ImportSummary, error) {
	if err := dbschema.Bootstrap(ctx, dbPath); err != nil {
		return ImportSummary{}, fmt.Errorf("bootstrap db: %w", err)
	}

	db, err := dbschema.Open(ctx, dbPath)
	if err != nil {
		return ImportSummary{}, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	bundle, err := parseContractWorkbook(filePath, kind)
	if err != nil {
		return ImportSummary{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ImportSummary{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	contractIDs, err := ensureContractDimensionRows(ctx, tx, bundle.ContractKeys, bundle.ContractDetails)
	if err != nil {
		return ImportSummary{}, err
	}

	switch kind {
	case contractWorkbookRevenueCost:
		if err := replaceRevenueCostSettlements(ctx, tx, bundle, contractIDs, opts.Incremental); err != nil {
			return ImportSummary{}, err
		}
	case contractWorkbookFund:
		if err := replaceFundIncomeRows(ctx, tx, bundle, contractIDs, opts.Incremental); err != nil {
			return ImportSummary{}, err
		}
	default:
		return ImportSummary{}, fmt.Errorf("unsupported contract workbook kind: %s", kind)
	}
	if err := annotateContractWorkbookSource(ctx, tx, dbPath, filePath, bundle); err != nil {
		return ImportSummary{}, err
	}

	if err := tx.Commit(); err != nil {
		return ImportSummary{}, fmt.Errorf("commit contract import: %w", err)
	}

	company := strings.TrimSpace(opts.CompanyOverride)
	if company == "" {
		company = "DefaultCompany"
	}
	return ImportSummary{
		FilePath:    filePath,
		ReportType:  string(kind),
		Company:     company,
		PeriodStart: bundle.PeriodStart,
		PeriodEnd:   bundle.PeriodEnd,
		RecordCount: bundle.TotalRecordCount,
	}, nil
}

func parseContractWorkbook(path string, kind contractWorkbookKind) (contractImportBundle, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return contractImportBundle{}, fmt.Errorf("open contract workbook: %w", err)
	}
	defer func() { _ = f.Close() }()

	bundle := contractImportBundle{
		Kind:              kind,
		ContractDetails:   map[contractKey]contractDimensionMeta{},
		TableSourceSheets: map[string][]string{},
	}
	switch kind {
	case contractWorkbookRevenueCost:
		if containsString(f.GetSheetList(), "收入-月度结算") {
			rows, err := readContractSheetRows(f, "收入-月度结算")
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read 收入-月度结算: %w", err)
			}
			bundle.RevenueRows = parseRevenueSettlementRows("收入-月度结算", rows)
			bundle.TableSourceSheets["fin_fund_income"] = append(bundle.TableSourceSheets["fin_fund_income"], "收入-月度结算")
			bundle.ContractSourceSheets = append(bundle.ContractSourceSheets, "收入-月度结算")
		}
		if containsString(f.GetSheetList(), "成本-月度结算") {
			rows, err := readContractSheetRows(f, "成本-月度结算")
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read 成本-月度结算: %w", err)
			}
			bundle.CostRows = parseCostSettlementRows("成本-月度结算", rows)
			bundle.TableSourceSheets["fin_cost_settlements"] = append(bundle.TableSourceSheets["fin_cost_settlements"], "成本-月度结算")
			bundle.ContractSourceSheets = append(bundle.ContractSourceSheets, "成本-月度结算")
		}
	case contractWorkbookFund:
		for _, sheetName := range fundQuarterSheetNames(f.GetSheetList()) {
			rows, err := readContractSheetRows(f, sheetName)
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read %s: %w", sheetName, err)
			}
			bundle.FundRows = append(bundle.FundRows, parseFundIncomeQuarterRows(sheetName, rows)...)
			bundle.TableSourceSheets["fin_fund_income"] = append(bundle.TableSourceSheets["fin_fund_income"], sheetName)
			bundle.ContractSourceSheets = append(bundle.ContractSourceSheets, sheetName)
		}
	default:
		return contractImportBundle{}, fmt.Errorf("unsupported contract workbook kind: %s", kind)
	}

	keySet := map[contractKey]struct{}{}
	periods := make([]string, 0, len(bundle.RevenueRows)+len(bundle.CostRows)+len(bundle.FundRows))
	for _, row := range bundle.RevenueRows {
		keySet[row.contractKey] = struct{}{}
		bundle.ContractDetails[row.contractKey] = mergeContractDimensionMeta(bundle.ContractDetails[row.contractKey], contractDimensionMeta{
			ContractStartDate: row.ContractStartDate,
			ContractEndDate:   row.ContractEndDate,
			SettlementCycle:   row.SettlementCycle,
		})
		periods = append(periods, row.YearMonth)
	}
	for _, row := range bundle.CostRows {
		keySet[row.contractKey] = struct{}{}
		bundle.ContractDetails[row.contractKey] = mergeContractDimensionMeta(bundle.ContractDetails[row.contractKey], contractDimensionMeta{
			ContractStartDate: row.ContractStartDate,
			ContractEndDate:   row.ContractEndDate,
			SettlementCycle:   row.SettlementCycle,
		})
		periods = append(periods, row.YearMonth)
	}
	for _, row := range bundle.FundRows {
		keySet[row.contractKey] = struct{}{}
		bundle.ContractDetails[row.contractKey] = mergeContractDimensionMeta(bundle.ContractDetails[row.contractKey], contractDimensionMeta{
			ContractStartDate: row.ContractStartDate,
			ContractEndDate:   row.ContractEndDate,
			SettlementCycle:   row.SettlementCycle,
		})
		periods = append(periods, row.YearMonth)
	}
	for key := range keySet {
		bundle.ContractKeys = append(bundle.ContractKeys, key)
	}
	sort.Slice(bundle.ContractKeys, func(i, j int) bool {
		if bundle.ContractKeys[i].Name == bundle.ContractKeys[j].Name {
			return bundle.ContractKeys[i].Content < bundle.ContractKeys[j].Content
		}
		return bundle.ContractKeys[i].Name < bundle.ContractKeys[j].Name
	})
	sort.Strings(periods)
	if len(periods) > 0 {
		bundle.PeriodStart = periods[0]
		bundle.PeriodEnd = periods[len(periods)-1]
	}
	for tableName, sheets := range bundle.TableSourceSheets {
		sort.Strings(sheets)
		bundle.TableSourceSheets[tableName] = dedupeContractStrings(sheets)
	}
	sort.Strings(bundle.ContractSourceSheets)
	bundle.ContractSourceSheets = dedupeContractStrings(bundle.ContractSourceSheets)
	bundle.TotalRecordCount = len(bundle.RevenueRows) + len(bundle.CostRows) + len(bundle.FundRows)
	return bundle, nil
}

func readContractSheetRows(f *excelize.File, sheetName string) ([][]string, error) {
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}
	mergedCells, err := f.GetMergeCells(sheetName)
	if err != nil {
		return nil, err
	}
	for _, mergedCell := range mergedCells {
		startCol, startRow, err := excelize.CellNameToCoordinates(mergedCell.GetStartAxis())
		if err != nil {
			return nil, err
		}
		endCol, endRow, err := excelize.CellNameToCoordinates(mergedCell.GetEndAxis())
		if err != nil {
			return nil, err
		}
		// Horizontal header merges (for example "1月" spanning quantity/amount columns)
		// should stay anchored at the first column; duplicating them creates fake months.
		if endRow <= startRow {
			continue
		}
		value := strings.TrimSpace(mergedCell.GetCellValue())
		if value == "" {
			value = cellValue(cellRow(rows, startRow-1), startCol-1)
		}
		rows = fillMergedCellRange(rows, startRow, endRow, startCol, endCol, value)
	}
	return rows, nil
}

func fillMergedCellRange(rows [][]string, startRow, endRow, startCol, endCol int, value string) [][]string {
	if startRow <= 0 || endRow < startRow || startCol <= 0 || endCol < startCol {
		return rows
	}
	if len(rows) < endRow {
		grown := make([][]string, endRow)
		copy(grown, rows)
		rows = grown
	}
	for rowIdx := startRow - 1; rowIdx <= endRow-1; rowIdx++ {
		row := ensureRowLength(rows[rowIdx], endCol)
		for colIdx := startCol - 1; colIdx <= endCol-1; colIdx++ {
			row[colIdx] = value
		}
		rows[rowIdx] = row
	}
	return rows
}

func cellRow(rows [][]string, idx int) []string {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	return rows[idx]
}

func parseRevenueSettlementRows(sheetName string, rows [][]string) []contractRevenueSettlementRow {
	if len(rows) < 3 {
		return nil
	}
	rows = normalizeContractRows(rows, 0, 1, 2, 3, 4, 5)
	header := rows[0]
	defaultYears := inferSheetDefaultYears(rows, 1)
	out := make([]contractRevenueSettlementRow, 0)
	for idx := 2; idx < len(rows); idx++ {
		row := rows[idx]
		name := strings.TrimSpace(cellValue(row, 0))
		content := strings.TrimSpace(cellValue(row, 1))
		if name == "" || content == "" {
			continue
		}
		contractStartDate := normalizeContractDateString(cellValue(row, 2))
		contractEndDate := normalizeContractDateString(cellValue(row, 3))
		settlementCycle := strings.TrimSpace(cellValue(row, 4))
		settlementUnitPrice := strings.TrimSpace(cellValue(row, 5))
		for _, monthCol := range monthColumns(header) {
			yearMonth := resolveContractYearMonth(monthCol.Label, cellValue(row, 2), cellValue(row, 3), defaultYears[monthCol.Label], content)
			if yearMonth == "" {
				continue
			}
			amount := parseContractFloat(cellValue(row, monthCol.Index+1))
			if amount <= 0 {
				continue
			}
			out = append(out, contractRevenueSettlementRow{
				contractKey:         contractKey{Name: name, Content: content},
				SourceSheetName:     strings.TrimSpace(sheetName),
				YearMonth:           yearMonth,
				Quantity:            strings.TrimSpace(cellValue(row, monthCol.Index)),
				SettlementAmount:    amount,
				IsInvoiced:          defaultContractText(cellValue(row, monthCol.Index+2), "否"),
				InvoiceAmount:       parseContractFloat(cellValue(row, monthCol.Index+3)),
				ContractStartDate:   contractStartDate,
				ContractEndDate:     contractEndDate,
				SettlementCycle:     settlementCycle,
				SettlementUnitPrice: settlementUnitPrice,
			})
		}
	}
	return out
}

func parseCostSettlementRows(sheetName string, rows [][]string) []contractCostSettlementRow {
	if len(rows) < 3 {
		return nil
	}
	rows = normalizeContractRows(rows, 0, 1, 2, 3, 4, 5, 6)
	header := rows[0]
	defaultYears := inferSheetDefaultYears(rows, 1)
	out := make([]contractCostSettlementRow, 0)
	for idx := 2; idx < len(rows); idx++ {
		row := rows[idx]
		name := strings.TrimSpace(cellValue(row, 0))
		content := strings.TrimSpace(cellValue(row, 1))
		if name == "" || content == "" || content == "分配" {
			continue
		}
		accountCode := strings.TrimSpace(cellValue(row, 6))
		contractStartDate := normalizeContractDateString(cellValue(row, 2))
		contractEndDate := normalizeContractDateString(cellValue(row, 3))
		settlementCycle := strings.TrimSpace(cellValue(row, 4))
		settlementUnitPrice := strings.TrimSpace(cellValue(row, 5))
		for _, monthCol := range monthColumns(header) {
			yearMonth := resolveContractYearMonth(monthCol.Label, cellValue(row, 2), cellValue(row, 3), defaultYears[monthCol.Label], content)
			if yearMonth == "" {
				continue
			}
			amount := parseContractFloat(cellValue(row, monthCol.Index+1))
			if amount <= 0 {
				continue
			}
			out = append(out, contractCostSettlementRow{
				contractKey:         contractKey{Name: name, Content: content},
				SourceSheetName:     strings.TrimSpace(sheetName),
				YearMonth:           yearMonth,
				Quantity:            defaultContractText(cellValue(row, monthCol.Index), "/"),
				SettlementAmount:    amount,
				IsInvoiced:          defaultContractText(cellValue(row, monthCol.Index+2), "否"),
				InvoiceAmount:       parseContractFloat(cellValue(row, monthCol.Index+3)),
				PaidAmount:          parseContractFloat(cellValue(row, monthCol.Index+4)),
				AccountCode:         accountCode,
				ContractStartDate:   contractStartDate,
				ContractEndDate:     contractEndDate,
				SettlementCycle:     settlementCycle,
				SettlementUnitPrice: settlementUnitPrice,
			})
		}
	}
	return out
}

func parseFundIncomeQuarterRows(sheetName string, rows [][]string) []contractFundIncomeRow {
	if len(rows) < 3 {
		return nil
	}
	year := extractSheetYear(sheetName)
	if year == "" {
		return nil
	}
	rows = normalizeContractRows(rows, 0, 1, 2, 3, 4, 5)
	header := rows[0]
	out := make([]contractFundIncomeRow, 0)
	monthCols := monthColumns(header)
	if len(monthCols) == 0 {
		return nil
	}
	simpleAmountLayout := strings.Contains(cellValue(rows[1], monthCols[0].Index), "收入金额")
	for idx := 2; idx < len(rows); idx++ {
		row := rows[idx]
		name := strings.TrimSpace(cellValue(row, 0))
		content := strings.TrimSpace(cellValue(row, 1))
		if name == "" || content == "" {
			continue
		}
		contractStartDate := normalizeContractDateString(cellValue(row, 2))
		contractEndDate := normalizeContractDateString(cellValue(row, 3))
		settlementCycle := strings.TrimSpace(cellValue(row, 4))
		settlementUnitPrice := strings.TrimSpace(cellValue(row, 5))
		for _, monthCol := range monthCols {
			yearMonth := normalizeContractYearMonth(monthCol.Label, year)
			if yearMonth == "" {
				continue
			}
			if simpleAmountLayout {
				amount := parseContractFloat(cellValue(row, monthCol.Index))
				if amount <= 0 {
					continue
				}
				out = append(out, contractFundIncomeRow{
					contractKey:         contractKey{Name: name, Content: content},
					SourceSheetName:     strings.TrimSpace(sheetName),
					YearMonth:           yearMonth,
					SettlementAmount:    amount,
					ReceivedAmount:      amount,
					IsInvoiced:          "否",
					ContractStartDate:   contractStartDate,
					ContractEndDate:     contractEndDate,
					SettlementCycle:     settlementCycle,
					SettlementUnitPrice: settlementUnitPrice,
				})
				continue
			}

			settlement := parseContractFloat(cellValue(row, monthCol.Index+1))
			received := parseContractFloat(cellValue(row, monthCol.Index+4))
			if settlement <= 0 && received <= 0 {
				continue
			}
			out = append(out, contractFundIncomeRow{
				contractKey:         contractKey{Name: name, Content: content},
				SourceSheetName:     strings.TrimSpace(sheetName),
				YearMonth:           yearMonth,
				Quantity:            strings.TrimSpace(cellValue(row, monthCol.Index)),
				SettlementAmount:    settlement,
				ReceivedAmount:      received,
				IsInvoiced:          defaultContractText(cellValue(row, monthCol.Index+2), "否"),
				InvoiceAmount:       parseContractFloat(cellValue(row, monthCol.Index+3)),
				ContractStartDate:   contractStartDate,
				ContractEndDate:     contractEndDate,
				SettlementCycle:     settlementCycle,
				SettlementUnitPrice: settlementUnitPrice,
			})
		}
	}
	return out
}

func ensureContractDimensionRows(ctx context.Context, tx *sql.Tx, keys []contractKey, details map[contractKey]contractDimensionMeta) (map[contractKey]string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT contract_id, customer_name, contract_content, contract_start_date, contract_end_date, settlement_cycle
FROM fin_contracts
`)
	if err != nil {
		return nil, fmt.Errorf("query fin_contracts: %w", err)
	}
	defer rows.Close()

	existing := map[contractKey]string{}
	existingDetails := map[contractKey]contractDimensionMeta{}
	maxSeq := 0
	for rows.Next() {
		var contractID, name, content string
		var contractStartDate, contractEndDate, settlementCycle sql.NullString
		if err := rows.Scan(&contractID, &name, &content, &contractStartDate, &contractEndDate, &settlementCycle); err != nil {
			return nil, fmt.Errorf("scan fin_contracts: %w", err)
		}
		key := contractKey{Name: name, Content: content}
		existing[key] = contractID
		existingDetails[key] = contractDimensionMeta{
			ContractStartDate: nullContractString(contractStartDate),
			ContractEndDate:   nullContractString(contractEndDate),
			SettlementCycle:   nullContractString(settlementCycle),
		}
		if seq := parseContractSequence(contractID); seq > maxSeq {
			maxSeq = seq
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fin_contracts: %w", err)
	}

	for _, key := range keys {
		incoming := details[key]
		if contractID, ok := existing[key]; ok {
			merged := mergeContractDimensionMeta(existingDetails[key], incoming)
			if _, err := tx.ExecContext(ctx, `
UPDATE fin_contracts
SET contract_start_date = ?, contract_end_date = ?, settlement_cycle = ?
WHERE contract_id = ?
`, nullableContractValue(merged.ContractStartDate), nullableContractValue(merged.ContractEndDate), nullableContractValue(merged.SettlementCycle), contractID); err != nil {
				return nil, fmt.Errorf("update fin_contracts: %w", err)
			}
			existingDetails[key] = merged
			continue
		}
		maxSeq++
		contractID := fmt.Sprintf("C%03d", maxSeq)
		merged := mergeContractDimensionMeta(contractDimensionMeta{}, incoming)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_contracts(contract_id, customer_name, contract_content, contract_start_date, contract_end_date, settlement_cycle)
VALUES (?, ?, ?, ?, ?, ?)
`, contractID, key.Name, key.Content, nullableContractValue(merged.ContractStartDate), nullableContractValue(merged.ContractEndDate), nullableContractValue(merged.SettlementCycle)); err != nil {
			return nil, fmt.Errorf("insert fin_contracts: %w", err)
		}
		existing[key] = contractID
		existingDetails[key] = merged
	}
	return existing, nil
}

func replaceRevenueCostSettlements(ctx context.Context, tx *sql.Tx, bundle contractImportBundle, contractIDs map[contractKey]string, incremental bool) error {
	if !incremental {
		if contractBundleTouchesTable(bundle, "fin_cost_settlements") {
			if err := deleteContractRowsBySourceScope(ctx, tx, "fin_cost_settlements", bundle.Kind, bundle.TableSourceSheets["fin_cost_settlements"]); err != nil {
				return err
			}
		}
		if contractBundleTouchesTable(bundle, "fin_fund_income") {
			if err := deleteContractRowsBySourceScope(ctx, tx, "fin_fund_income", bundle.Kind, bundle.TableSourceSheets["fin_fund_income"]); err != nil {
				return err
			}
		}
	} else {
		for _, row := range bundle.RevenueRows {
			if err := deleteContractRowByIdentity(ctx, tx, "fin_fund_income", bundle.Kind, row.SourceSheetName, contractIDs[row.contractKey], row.YearMonth); err != nil {
				return fmt.Errorf("delete prior fin_fund_income from revenue workbook: %w", err)
			}
		}
		for _, row := range bundle.CostRows {
			if err := deleteContractRowByIdentity(ctx, tx, "fin_cost_settlements", bundle.Kind, row.SourceSheetName, contractIDs[row.contractKey], row.YearMonth); err != nil {
				return fmt.Errorf("delete prior fin_cost_settlements: %w", err)
			}
		}
	}

	for _, row := range bundle.RevenueRows {
		if err := deleteLegacyContractRow(ctx, tx, "fin_fund_income", contractIDs[row.contractKey], row.YearMonth); err != nil {
			return fmt.Errorf("delete legacy fin_fund_income row: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_fund_income(
	contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, received_amount, is_invoiced, invoice_amount,
	contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, contractIDs[row.contractKey], row.YearMonth, string(bundle.Kind), nullableContractValue(row.SourceSheetName), nullableContractValue(row.Quantity), row.SettlementAmount, 0, row.IsInvoiced, row.InvoiceAmount, nullableContractValue(row.ContractStartDate), nullableContractValue(row.ContractEndDate), nullableContractValue(row.SettlementCycle), nullableContractValue(row.SettlementUnitPrice)); err != nil {
			return fmt.Errorf("insert fin_fund_income from revenue workbook: %w", err)
		}
	}
	for _, row := range bundle.CostRows {
		if err := deleteLegacyContractRow(ctx, tx, "fin_cost_settlements", contractIDs[row.contractKey], row.YearMonth); err != nil {
			return fmt.Errorf("delete legacy fin_cost_settlements row: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_cost_settlements(
	contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code,
	contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, contractIDs[row.contractKey], row.YearMonth, string(bundle.Kind), nullableContractValue(row.SourceSheetName), nullableContractValue(row.Quantity), row.SettlementAmount, row.IsInvoiced, row.InvoiceAmount, row.PaidAmount, nullableContractValue(row.AccountCode), nullableContractValue(row.ContractStartDate), nullableContractValue(row.ContractEndDate), nullableContractValue(row.SettlementCycle), nullableContractValue(row.SettlementUnitPrice)); err != nil {
			return fmt.Errorf("insert fin_cost_settlements: %w", err)
		}
	}
	return nil
}

func contractBundleTouchesTable(bundle contractImportBundle, tableName string) bool {
	return len(bundle.TableSourceSheets[tableName]) > 0
}

func deleteContractRowsBySourceScope(ctx context.Context, tx *sql.Tx, tableName string, reportType contractWorkbookKind, sheetNames []string) error {
	sheetNames = dedupeContractStrings(sheetNames)
	if len(sheetNames) == 0 {
		return nil
	}

	args := make([]any, 0, len(sheetNames)+1)
	args = append(args, string(reportType))
	placeholders := make([]string, 0, len(sheetNames))
	for _, sheetName := range sheetNames {
		placeholders = append(placeholders, "?")
		args = append(args, sheetName)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %s
WHERE source_report_type = ?
  AND source_sheet_name IN (%s)
`, tableName, strings.Join(placeholders, ", ")), args...); err != nil {
		return fmt.Errorf("clear %s by source scope: %w", tableName, err)
	}
	return nil
}

func deleteContractRowByIdentity(ctx context.Context, tx *sql.Tx, tableName string, reportType contractWorkbookKind, sheetName, contractID, yearMonth string) error {
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %s
WHERE contract_id = ?
  AND year_month = ?
  AND source_report_type = ?
  AND source_sheet_name = ?
`, tableName), contractID, yearMonth, string(reportType), strings.TrimSpace(sheetName)); err != nil {
		return fmt.Errorf("delete %s by row identity: %w", tableName, err)
	}
	return nil
}

func deleteLegacyContractRow(ctx context.Context, tx *sql.Tx, tableName, contractID, yearMonth string) error {
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %s
WHERE contract_id = ?
  AND year_month = ?
  AND source_report_type IS NULL
  AND source_sheet_name IS NULL
`, tableName), contractID, yearMonth); err != nil {
		return fmt.Errorf("delete legacy %s row: %w", tableName, err)
	}
	return nil
}

func replaceFundIncomeRows(ctx context.Context, tx *sql.Tx, bundle contractImportBundle, contractIDs map[contractKey]string, incremental bool) error {
	if !incremental {
		if err := deleteContractRowsBySourceScope(ctx, tx, "fin_fund_income", bundle.Kind, bundle.TableSourceSheets["fin_fund_income"]); err != nil {
			return err
		}
	} else {
		for _, row := range bundle.FundRows {
			if err := deleteContractRowByIdentity(ctx, tx, "fin_fund_income", bundle.Kind, row.SourceSheetName, contractIDs[row.contractKey], row.YearMonth); err != nil {
				return fmt.Errorf("delete prior fin_fund_income: %w", err)
			}
		}
	}

	for _, row := range bundle.FundRows {
		if err := deleteLegacyContractRow(ctx, tx, "fin_fund_income", contractIDs[row.contractKey], row.YearMonth); err != nil {
			return fmt.Errorf("delete legacy fin_fund_income row: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_fund_income(
	contract_id, year_month, source_report_type, source_sheet_name, quantity, settlement_amount, received_amount, is_invoiced, invoice_amount,
	contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, contractIDs[row.contractKey], row.YearMonth, string(bundle.Kind), nullableContractValue(row.SourceSheetName), nullableContractValue(row.Quantity), row.SettlementAmount, row.ReceivedAmount, row.IsInvoiced, row.InvoiceAmount, nullableContractValue(row.ContractStartDate), nullableContractValue(row.ContractEndDate), nullableContractValue(row.SettlementCycle), nullableContractValue(row.SettlementUnitPrice)); err != nil {
			return fmt.Errorf("insert fin_fund_income: %w", err)
		}
	}
	return nil
}

type monthColumn struct {
	Label string
	Index int
}

func monthColumns(header []string) []monthColumn {
	out := make([]monthColumn, 0, 12)
	for idx, raw := range header {
		label := strings.TrimSpace(raw)
		if !strings.HasSuffix(label, "月") {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimSuffix(label, "月")); err != nil {
			continue
		}
		out = append(out, monthColumn{Label: label, Index: idx})
	}
	return out
}

func cellValue(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func defaultContractText(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func parseContractFloat(v string) float64 {
	v = strings.TrimSpace(strings.ReplaceAll(v, ",", ""))
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

func nullableContractValue(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}

func nullContractString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return strings.TrimSpace(v.String)
}

func normalizeContractDateString(raw string) string {
	if parsed, ok := parseContractDate(raw); ok {
		return parsed.Format("2006-01-02")
	}
	return strings.TrimSpace(raw)
}

func mergeContractDimensionMeta(existing, incoming contractDimensionMeta) contractDimensionMeta {
	merged := contractDimensionMeta{
		ContractStartDate: earlierContractDate(existing.ContractStartDate, incoming.ContractStartDate),
		ContractEndDate:   laterContractDate(existing.ContractEndDate, incoming.ContractEndDate),
		SettlementCycle:   mergeContractTextValue(existing.SettlementCycle, incoming.SettlementCycle),
	}
	return merged
}

func mergeContractTextValue(existing, incoming string) string {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	switch {
	case existing == "":
		return incoming
	case incoming == "":
		return existing
	case existing == incoming:
		return existing
	default:
		return ""
	}
}

func earlierContractDate(left, right string) string {
	return mergeContractDateByOrder(left, right, func(leftTime, rightTime time.Time) bool {
		return leftTime.Before(rightTime)
	}, true)
}

func laterContractDate(left, right string) string {
	return mergeContractDateByOrder(left, right, func(leftTime, rightTime time.Time) bool {
		return leftTime.After(rightTime)
	}, false)
}

func mergeContractDateByOrder(left, right string, prefer func(time.Time, time.Time) bool, preferSmallerText bool) string {
	left = normalizeContractDateString(left)
	right = normalizeContractDateString(right)
	switch {
	case left == "":
		return right
	case right == "":
		return left
	}
	leftTime, leftOK := parseContractDate(left)
	rightTime, rightOK := parseContractDate(right)
	switch {
	case leftOK && rightOK:
		if prefer(leftTime, rightTime) {
			return leftTime.Format("2006-01-02")
		}
		return rightTime.Format("2006-01-02")
	case leftOK:
		return leftTime.Format("2006-01-02")
	case rightOK:
		return rightTime.Format("2006-01-02")
	default:
		if compareContractText(left, right, preferSmallerText) {
			return left
		}
		return right
	}
}

func compareContractText(left, right string, preferSmaller bool) bool {
	cmp := strings.Compare(strings.TrimSpace(left), strings.TrimSpace(right))
	if preferSmaller {
		return cmp <= 0
	}
	return cmp >= 0
}

func normalizeContractYearMonth(monthLabel, year string) string {
	monthValue, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(monthLabel), "月"))
	if err != nil || monthValue < 1 || monthValue > 12 {
		return ""
	}
	return fmt.Sprintf("%s-%02d", strings.TrimSpace(year), monthValue)
}

func inferYearFromText(text, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return strings.TrimSpace(fallback)
	}
	if match := regexp.MustCompile(`20\d{2}`).FindString(text); match != "" {
		return match
	}
	if match := regexp.MustCompile(`(?:FY|fy|FY-|fy-|)(\d{2})(?:年|季度|Q|$)`).FindStringSubmatch(text); len(match) == 2 {
		if year, err := strconv.Atoi(match[1]); err == nil {
			return fmt.Sprintf("20%02d", year)
		}
	}
	if match := regexp.MustCompile(`(\d{2})年`).FindStringSubmatch(text); len(match) == 2 {
		if year, err := strconv.Atoi(match[1]); err == nil {
			return fmt.Sprintf("20%02d", year)
		}
	}
	return fallback
}

func resolveContractYearMonth(monthLabel, startText, endText, defaultYear, textHint string) string {
	if yearMonth := inferYearMonthFromContractRange(monthLabel, startText, endText); yearMonth != "" {
		return yearMonth
	}
	if strings.TrimSpace(defaultYear) != "" {
		return normalizeContractYearMonth(monthLabel, strings.TrimSpace(defaultYear))
	}
	if inferredYear := inferYearFromText(textHint+" "+startText+" "+endText, ""); strings.TrimSpace(inferredYear) != "" {
		return normalizeContractYearMonth(monthLabel, inferredYear)
	}
	return ""
}

func inferSheetDefaultYears(rows [][]string, amountOffset int) map[string]string {
	defaults := make(map[string]string)
	if len(rows) < 3 {
		return defaults
	}
	counts := map[string]map[string]int{}
	for _, monthCol := range monthColumns(rows[0]) {
		for idx := 2; idx < len(rows); idx++ {
			row := rows[idx]
			if parseContractFloat(cellValue(row, monthCol.Index+amountOffset)) <= 0 {
				continue
			}
			yearMonth := inferYearMonthFromContractRange(monthCol.Label, cellValue(row, 2), cellValue(row, 3))
			if yearMonth == "" {
				continue
			}
			year := strings.Split(yearMonth, "-")[0]
			if counts[monthCol.Label] == nil {
				counts[monthCol.Label] = map[string]int{}
			}
			counts[monthCol.Label][year]++
		}
	}
	for month, yearCounts := range counts {
		bestYear := ""
		bestCount := -1
		for year, count := range yearCounts {
			if count > bestCount || (count == bestCount && year > bestYear) {
				bestYear = year
				bestCount = count
			}
		}
		if bestYear != "" {
			defaults[month] = bestYear
		}
	}
	return defaults
}

func inferYearMonthFromContractRange(monthLabel, startText, endText string) string {
	monthValue, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(monthLabel), "月"))
	if err != nil || monthValue < 1 || monthValue > 12 {
		return ""
	}
	startDate, okStart := parseContractDate(startText)
	endDate, okEnd := parseContractDate(endText)
	if !okStart || !okEnd {
		return ""
	}
	if endDate.Before(startDate) {
		startDate, endDate = endDate, startDate
	}

	candidates := make([]int, 0, endDate.Year()-startDate.Year()+1)
	for year := startDate.Year(); year <= endDate.Year(); year++ {
		monthStart := time.Date(year, time.Month(monthValue), 1, 0, 0, 0, 0, time.UTC)
		monthEnd := monthStart.AddDate(0, 1, -1)
		if !monthEnd.Before(startDate) && !monthStart.After(endDate) {
			candidates = append(candidates, year)
		}
	}
	if len(candidates) != 1 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d", candidates[0], monthValue)
}

func parseContractDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/01/02",
		"2006/1/2",
		"1/2/2006",
		"1/2/06",
		"2006.01.02",
		"2006.1.2",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, time.UTC); err == nil {
			return parsed, true
		}
	}
	if numeric, err := strconv.ParseFloat(raw, 64); err == nil && numeric > 20000 {
		if parsed, err := excelize.ExcelDateToTime(numeric, false); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func normalizeContractRows(rows [][]string, carryColumns ...int) [][]string {
	out := make([][]string, len(rows))
	last := make(map[int]string, len(carryColumns))
	for rowIdx, row := range rows {
		next := append([]string(nil), row...)
		for _, col := range carryColumns {
			next = ensureRowLength(next, col+1)
			value := strings.TrimSpace(next[col])
			if value == "" {
				if carried, ok := last[col]; ok {
					next[col] = carried
				}
				continue
			}
			last[col] = value
		}
		out[rowIdx] = next
	}
	return out
}

func ensureRowLength(row []string, want int) []string {
	if len(row) >= want {
		return row
	}
	grown := make([]string, want)
	copy(grown, row)
	return grown
}

func fundQuarterSheetNames(sheets []string) []string {
	matched := make([]string, 0, len(sheets))
	for _, sheet := range sheets {
		if fundQuarterSheetPattern.MatchString(strings.TrimSpace(sheet)) {
			matched = append(matched, strings.TrimSpace(sheet))
		}
	}
	sort.Strings(matched)
	return matched
}

func extractSheetYear(sheetName string) string {
	match := fundQuarterSheetPattern.FindStringSubmatch(strings.TrimSpace(sheetName))
	if len(match) != 3 {
		return ""
	}
	year := strings.TrimSpace(match[1])
	if len(year) == 2 {
		return "20" + year
	}
	return year
}

func parseContractSequence(contractID string) int {
	if len(contractID) < 2 {
		return 0
	}
	raw := strings.TrimLeft(contractID[1:], "0")
	if raw == "" {
		return 0
	}
	seq, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return seq
}

func containsString(items []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, item := range items {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}

func dedupeContractStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
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
