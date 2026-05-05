package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
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
	contractWorkbookMixed       contractWorkbookKind = "contract_mixed_finance"
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
	SourceCellNotes     string
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
	SourceCellNotes     string
}

type contractCostSettlementCleanupRow struct {
	contractKey
	SourceSheetName string
	YearMonth       string
}

type contractCostSettlementGroupRow struct {
	CustomerName        string
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
	SourceStartRow      int
	SourceEndRow        int
	MergeRange          string
	SourceCellNotes     string
	Members             []contractKey
	MemberSourceRows    map[contractKey]int
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
	SourceCellNotes     string
}

type contractFundIncomeCleanupRow struct {
	contractKey
	SourceSheetName string
	YearMonth       string
}

type contractFundIncomeGroupRow struct {
	CustomerName        string
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
	SourceStartRow      int
	SourceEndRow        int
	MergeRange          string
	SourceCellNotes     string
	Members             []contractKey
	MemberSourceRows    map[contractKey]int
}

type contractSourceCellNote struct {
	Author string `json:"author,omitempty"`
	Text   string `json:"text"`
}

type contractSourceCellNotes map[string]contractSourceCellNote

type contractMergedCellRange struct {
	StartRow int
	EndRow   int
	StartCol int
	EndCol   int
}

type contractMetricMergeGroup struct {
	Range   contractMergedCellRange
	Metrics map[string]bool
}

type contractImportBundle struct {
	Kind                 contractWorkbookKind
	RevenueRows          []contractRevenueSettlementRow
	CostRows             []contractCostSettlementRow
	CostGroupRows        []contractCostSettlementGroupRow
	CostCleanupRows      []contractCostSettlementCleanupRow
	FundRows             []contractFundIncomeRow
	FundGroupRows        []contractFundIncomeGroupRow
	FundCleanupRows      []contractFundIncomeCleanupRow
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
	case (hasRevenue || hasCost) && hasFundQuarter:
		return contractWorkbookMixed, true, nil
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
	case contractWorkbookRevenueCost, contractWorkbookMixed:
		if err := replaceRevenueCostSettlements(ctx, tx, bundle, contractIDs, opts.Incremental); err != nil {
			return ImportSummary{}, err
		}
		if kind == contractWorkbookMixed {
			if err := replaceFundIncomeRows(ctx, tx, bundle, contractIDs, opts.Incremental); err != nil {
				return ImportSummary{}, err
			}
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
	case contractWorkbookRevenueCost, contractWorkbookMixed:
		if containsString(f.GetSheetList(), "收入-月度结算") {
			rows, err := readContractSheetRows(f, "收入-月度结算")
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read 收入-月度结算: %w", err)
			}
			mergedRanges, err := readContractMergedCellRanges(f, "收入-月度结算")
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read merged ranges 收入-月度结算: %w", err)
			}
			cellNotes, err := readContractSourceCellNotes(f, "收入-月度结算")
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read cell notes 收入-月度结算: %w", err)
			}
			addContractDimensionRows(&bundle, rows, mergedRanges, 0, 1, 2, 3, 4, 5)
			bundle.RevenueRows = parseRevenueSettlementRows("收入-月度结算", rows, mergedRanges, cellNotes)
			bundle.TableSourceSheets["fin_fund_income"] = append(bundle.TableSourceSheets["fin_fund_income"], "收入-月度结算")
			bundle.ContractSourceSheets = append(bundle.ContractSourceSheets, "收入-月度结算")
		}
		if containsString(f.GetSheetList(), "成本-月度结算") {
			rows, err := readContractSheetRows(f, "成本-月度结算")
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read 成本-月度结算: %w", err)
			}
			mergedRanges, err := readContractMergedCellRanges(f, "成本-月度结算")
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read merged ranges 成本-月度结算: %w", err)
			}
			cellNotes, err := readContractSourceCellNotes(f, "成本-月度结算")
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read cell notes 成本-月度结算: %w", err)
			}
			addContractDimensionRows(&bundle, rows, mergedRanges, 0, 1, 2, 3, 4, 5, 6)
			costRows, groupRows, cleanupRows := parseCostSettlementRows("成本-月度结算", rows, mergedRanges, cellNotes)
			bundle.CostRows = append(bundle.CostRows, costRows...)
			bundle.CostGroupRows = append(bundle.CostGroupRows, groupRows...)
			bundle.CostCleanupRows = append(bundle.CostCleanupRows, cleanupRows...)
			bundle.TableSourceSheets["fin_cost_settlements"] = append(bundle.TableSourceSheets["fin_cost_settlements"], "成本-月度结算")
			bundle.ContractSourceSheets = append(bundle.ContractSourceSheets, "成本-月度结算")
		}
		if kind != contractWorkbookMixed {
			break
		}
		fallthrough
	case contractWorkbookFund:
		for _, sheetName := range fundQuarterSheetNames(f.GetSheetList()) {
			rows, err := readContractSheetRows(f, sheetName)
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read %s: %w", sheetName, err)
			}
			mergedRanges, err := readContractMergedCellRanges(f, sheetName)
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read merged ranges %s: %w", sheetName, err)
			}
			cellNotes, err := readContractSourceCellNotes(f, sheetName)
			if err != nil {
				return contractImportBundle{}, fmt.Errorf("read cell notes %s: %w", sheetName, err)
			}
			addContractDimensionRows(&bundle, rows, mergedRanges, 0, 1, 2, 3, 4, 5)
			fundRows, groupRows, cleanupRows := parseFundIncomeQuarterRows(sheetName, rows, mergedRanges, cellNotes)
			bundle.FundRows = append(bundle.FundRows, fundRows...)
			bundle.FundGroupRows = append(bundle.FundGroupRows, groupRows...)
			bundle.FundCleanupRows = append(bundle.FundCleanupRows, cleanupRows...)
			bundle.TableSourceSheets["fin_fund_income"] = append(bundle.TableSourceSheets["fin_fund_income"], sheetName)
			bundle.ContractSourceSheets = append(bundle.ContractSourceSheets, sheetName)
		}
	default:
		return contractImportBundle{}, fmt.Errorf("unsupported contract workbook kind: %s", kind)
	}

	periods := make([]string, 0, len(bundle.RevenueRows)+len(bundle.CostRows)+len(bundle.CostGroupRows)+len(bundle.FundRows)+len(bundle.FundGroupRows))
	for _, row := range bundle.RevenueRows {
		addContractDimension(&bundle, row.contractKey, contractDimensionMeta{
			ContractStartDate: row.ContractStartDate,
			ContractEndDate:   row.ContractEndDate,
			SettlementCycle:   row.SettlementCycle,
		})
		periods = append(periods, row.YearMonth)
	}
	for _, row := range bundle.CostRows {
		addContractDimension(&bundle, row.contractKey, contractDimensionMeta{
			ContractStartDate: row.ContractStartDate,
			ContractEndDate:   row.ContractEndDate,
			SettlementCycle:   row.SettlementCycle,
		})
		periods = append(periods, row.YearMonth)
	}
	for _, row := range bundle.CostGroupRows {
		periods = append(periods, row.YearMonth)
		for _, member := range row.Members {
			addContractDimension(&bundle, member, contractDimensionMeta{
				ContractStartDate: row.ContractStartDate,
				ContractEndDate:   row.ContractEndDate,
				SettlementCycle:   row.SettlementCycle,
			})
		}
	}
	for _, row := range bundle.FundRows {
		addContractDimension(&bundle, row.contractKey, contractDimensionMeta{
			ContractStartDate: row.ContractStartDate,
			ContractEndDate:   row.ContractEndDate,
			SettlementCycle:   row.SettlementCycle,
		})
		periods = append(periods, row.YearMonth)
	}
	for _, row := range bundle.FundGroupRows {
		periods = append(periods, row.YearMonth)
		for _, member := range row.Members {
			addContractDimension(&bundle, member, contractDimensionMeta{
				ContractStartDate: row.ContractStartDate,
				ContractEndDate:   row.ContractEndDate,
				SettlementCycle:   row.SettlementCycle,
			})
		}
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
	bundle.TotalRecordCount = len(bundle.RevenueRows) + len(bundle.CostRows) + len(bundle.CostGroupRows) + len(bundle.FundRows) + len(bundle.FundGroupRows)
	return bundle, nil
}

func addContractDimensionRows(bundle *contractImportBundle, rows [][]string, mergedRanges []contractMergedCellRange, carryColumns ...int) {
	if len(rows) < 3 {
		return
	}
	rows = normalizeContractRows(rows, mergedRanges, carryColumns...)
	for idx := 2; idx < len(rows); idx++ {
		row := rows[idx]
		name := strings.TrimSpace(cellValue(row, 0))
		content := strings.TrimSpace(cellValue(row, 1))
		if name == "" || content == "" || content == "分配" {
			continue
		}
		addContractDimension(bundle, contractKey{Name: name, Content: content}, contractDimensionMeta{
			ContractStartDate: normalizeContractDateString(cellValue(row, 2)),
			ContractEndDate:   normalizeContractDateString(cellValue(row, 3)),
			SettlementCycle:   strings.TrimSpace(cellValue(row, 4)),
		})
	}
}

func addContractDimension(bundle *contractImportBundle, key contractKey, meta contractDimensionMeta) {
	key.Name = strings.TrimSpace(key.Name)
	key.Content = strings.TrimSpace(key.Content)
	if key.Name == "" || key.Content == "" {
		return
	}
	if _, ok := bundle.ContractDetails[key]; !ok {
		bundle.ContractKeys = append(bundle.ContractKeys, key)
	}
	bundle.ContractDetails[key] = mergeContractDimensionMeta(bundle.ContractDetails[key], meta)
}

func readContractSheetRows(f *excelize.File, sheetName string) ([][]string, error) {
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}
	// Keep merged facts anchored to their top-left source cell. Dimension carry-forward
	// is handled explicitly by normalizeContractRows so shared amount cells are not duplicated.
	return rows, nil
}

func readContractMergedCellRanges(f *excelize.File, sheetName string) ([]contractMergedCellRange, error) {
	mergedCells, err := f.GetMergeCells(sheetName)
	if err != nil {
		return nil, err
	}
	ranges := make([]contractMergedCellRange, 0, len(mergedCells))
	for _, mergedCell := range mergedCells {
		startCol, startRow, err := excelize.CellNameToCoordinates(mergedCell.GetStartAxis())
		if err != nil {
			return nil, err
		}
		endCol, endRow, err := excelize.CellNameToCoordinates(mergedCell.GetEndAxis())
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, contractMergedCellRange{
			StartRow: startRow - 1,
			EndRow:   endRow - 1,
			StartCol: startCol - 1,
			EndCol:   endCol - 1,
		})
	}
	return ranges, nil
}

func readContractSourceCellNotes(f *excelize.File, sheetName string) (contractSourceCellNotes, error) {
	comments, err := f.GetComments(sheetName)
	if err != nil {
		return nil, err
	}
	out := contractSourceCellNotes{}
	for _, comment := range comments {
		cell := strings.ToUpper(strings.TrimSpace(comment.Cell))
		text := contractSourceCellNoteText(comment)
		if cell == "" || text == "" {
			continue
		}
		out[cell] = contractSourceCellNote{
			Author: strings.TrimSpace(comment.Author),
			Text:   text,
		}
	}
	return out, nil
}

func contractSourceCellNoteText(comment excelize.Comment) string {
	text := strings.TrimSpace(comment.Text)
	if text != "" {
		return text
	}
	var parts []string
	for _, run := range comment.Paragraph {
		if part := strings.TrimSpace(run.Text); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func cellNotesForRowColumns(notes contractSourceCellNotes, mergedRanges []contractMergedCellRange, rowIdx int, cols ...int) contractSourceCellNotes {
	return cellNotesForRowRangeColumns(notes, mergedRanges, rowIdx, rowIdx, cols...)
}

func cellNotesForRowRangeColumns(notes contractSourceCellNotes, mergedRanges []contractMergedCellRange, startRow, endRow int, cols ...int) contractSourceCellNotes {
	if len(notes) == 0 || len(cols) == 0 {
		return nil
	}
	if startRow > endRow {
		startRow, endRow = endRow, startRow
	}
	selectedCols := map[int]struct{}{}
	for _, col := range cols {
		if col < 0 {
			continue
		}
		selectedCols[col] = struct{}{}
	}
	if len(selectedCols) == 0 {
		return nil
	}

	out := contractSourceCellNotes{}
	for cell, note := range notes {
		col, row, err := excelize.CellNameToCoordinates(cell)
		if err != nil {
			continue
		}
		row--
		col--
		if sourceCellInSelection(row, col, startRow, endRow, selectedCols) || sourceCellMergeIntersectsSelection(row, col, startRow, endRow, selectedCols, mergedRanges) {
			out[cell] = note
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sourceCellInSelection(row, col, startRow, endRow int, selectedCols map[int]struct{}) bool {
	if row < startRow || row > endRow {
		return false
	}
	_, ok := selectedCols[col]
	return ok
}

func sourceCellMergeIntersectsSelection(row, col, startRow, endRow int, selectedCols map[int]struct{}, mergedRanges []contractMergedCellRange) bool {
	for _, mergeRange := range mergedRanges {
		if row < mergeRange.StartRow || row > mergeRange.EndRow || col < mergeRange.StartCol || col > mergeRange.EndCol {
			continue
		}
		if mergeRange.EndRow < startRow || mergeRange.StartRow > endRow {
			return false
		}
		for selectedCol := range selectedCols {
			if selectedCol >= mergeRange.StartCol && selectedCol <= mergeRange.EndCol {
				return true
			}
		}
		return false
	}
	return false
}

func sourceCellNotesJSON(notes contractSourceCellNotes) string {
	if len(notes) == 0 {
		return ""
	}
	data, err := json.Marshal(notes)
	if err != nil {
		return ""
	}
	return string(data)
}

func intRange(start, end int) []int {
	if end < start {
		return nil
	}
	out := make([]int, 0, end-start+1)
	for idx := start; idx <= end; idx++ {
		out = append(out, idx)
	}
	return out
}

func parseRevenueSettlementRows(sheetName string, rows [][]string, mergedRanges []contractMergedCellRange, cellNotes contractSourceCellNotes) []contractRevenueSettlementRow {
	if len(rows) < 3 {
		return nil
	}
	rows = normalizeContractRows(rows, mergedRanges, 0, 1, 2, 3, 4, 5)
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
			yearMonth := resolveContractYearMonth(monthCol.Label, cellValue(row, 2), cellValue(row, 3), monthColumnDefaultYear(monthCol, defaultYears), content)
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
				SourceCellNotes:     sourceCellNotesJSON(cellNotesForRowColumns(cellNotes, mergedRanges, idx, append(intRange(0, 5), intRange(monthCol.Index, monthCol.Index+3)...)...)),
			})
		}
	}
	return out
}

func parseCostSettlementRows(sheetName string, rows [][]string, mergedRanges []contractMergedCellRange, cellNotes contractSourceCellNotes) ([]contractCostSettlementRow, []contractCostSettlementGroupRow, []contractCostSettlementCleanupRow) {
	if len(rows) < 3 {
		return nil, nil, nil
	}
	rows = normalizeContractRows(rows, mergedRanges, 0, 1, 2, 3, 4, 5, 6)
	header := rows[0]
	defaultYears := inferSheetDefaultYears(rows, 1)
	out := make([]contractCostSettlementRow, 0)
	groups := make([]contractCostSettlementGroupRow, 0)
	cleanup := make([]contractCostSettlementCleanupRow, 0)
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
			yearMonth := resolveContractYearMonth(monthCol.Label, cellValue(row, 2), cellValue(row, 3), monthColumnDefaultYear(monthCol, defaultYears), content)
			if yearMonth == "" {
				continue
			}
			metricRanges := costMergedAmountRanges(mergedRanges, idx, monthCol)
			if len(metricRanges) > 0 {
				metricGroups := contractMetricMergeGroups(metricRanges)
				if len(metricGroups) == 1 {
					metricGroups[0].Metrics = costAmountMetricSet()
				}
				for _, metricGroup := range metricGroups {
					cleanup = append(cleanup, mergedCostSettlementCleanupRows(sheetName, rows, metricGroup.Range, yearMonth)...)
					mergedRow, ok := buildMergedCostSettlementGroupRow(sheetName, rows, idx, metricGroup.Range, monthCol, yearMonth, metricGroup.Metrics, mergedRanges, cellNotes)
					if ok {
						groups = append(groups, mergedRow)
					}
				}
				continue
			}
			directMetricRanges := costMergedAmountContainingRanges(mergedRanges, idx, monthCol)
			sourceNotes := sourceCellNotesJSON(cellNotesForRowColumns(cellNotes, mergedRanges, idx, append(intRange(0, 6), intRange(monthCol.Index, monthCol.Index+4)...)...))
			if directRow, ok := buildDirectCostSettlementRow(sheetName, row, monthCol, yearMonth, contractKey{Name: name, Content: content}, accountCode, contractStartDate, contractEndDate, settlementCycle, settlementUnitPrice, directMetricRanges, sourceNotes); ok {
				out = append(out, directRow)
			}
		}
	}
	return out, groups, cleanup
}

func buildDirectCostSettlementRow(sheetName string, row []string, monthCol monthColumn, yearMonth string, key contractKey, accountCode, contractStartDate, contractEndDate, settlementCycle, settlementUnitPrice string, metricRanges map[string]contractMergedCellRange, sourceCellNotes string) (contractCostSettlementRow, bool) {
	settlement := 0.0
	if _, merged := metricRanges["settlement"]; !merged {
		settlement = parseContractFloat(cellValue(row, monthCol.Index+1))
	}
	invoice := 0.0
	if _, merged := metricRanges["invoice"]; !merged {
		invoice = parseContractFloat(cellValue(row, monthCol.Index+3))
	}
	paid := 0.0
	if _, merged := metricRanges["paid"]; !merged {
		paid = parseContractFloat(cellValue(row, monthCol.Index+4))
	}
	if settlement <= 0 && invoice <= 0 && paid <= 0 {
		return contractCostSettlementRow{}, false
	}
	return contractCostSettlementRow{
		contractKey:         key,
		SourceSheetName:     strings.TrimSpace(sheetName),
		YearMonth:           yearMonth,
		Quantity:            defaultContractText(cellValue(row, monthCol.Index), "/"),
		SettlementAmount:    settlement,
		IsInvoiced:          defaultContractText(cellValue(row, monthCol.Index+2), "否"),
		InvoiceAmount:       invoice,
		PaidAmount:          paid,
		AccountCode:         accountCode,
		ContractStartDate:   contractStartDate,
		ContractEndDate:     contractEndDate,
		SettlementCycle:     settlementCycle,
		SettlementUnitPrice: settlementUnitPrice,
		SourceCellNotes:     sourceCellNotes,
	}, true
}

func costMergedAmountRanges(mergedRanges []contractMergedCellRange, rowIdx int, monthCol monthColumn) map[string]contractMergedCellRange {
	return contractMergedAmountRanges(mergedRanges, rowIdx, map[string]int{
		"settlement": monthCol.Index + 1,
		"invoice":    monthCol.Index + 3,
		"paid":       monthCol.Index + 4,
	})
}

func costMergedAmountContainingRanges(mergedRanges []contractMergedCellRange, rowIdx int, monthCol monthColumn) map[string]contractMergedCellRange {
	return contractMergedAmountContainingRanges(mergedRanges, rowIdx, map[string]int{
		"settlement": monthCol.Index + 1,
		"invoice":    monthCol.Index + 3,
		"paid":       monthCol.Index + 4,
	})
}

func costAmountMetricSet() map[string]bool {
	return map[string]bool{
		"settlement": true,
		"invoice":    true,
		"paid":       true,
	}
}

func contractMergedAmountRanges(mergedRanges []contractMergedCellRange, rowIdx int, metricCols map[string]int) map[string]contractMergedCellRange {
	out := map[string]contractMergedCellRange{}
	for metric, colIdx := range metricCols {
		for _, mergeRange := range mergedRanges {
			if mergeRange.EndRow <= mergeRange.StartRow {
				continue
			}
			if mergeRange.StartRow != rowIdx {
				continue
			}
			if colIdx < mergeRange.StartCol || colIdx > mergeRange.EndCol {
				continue
			}
			out[metric] = mergeRange
			break
		}
	}
	return out
}

func contractMergedAmountContainingRanges(mergedRanges []contractMergedCellRange, rowIdx int, metricCols map[string]int) map[string]contractMergedCellRange {
	out := map[string]contractMergedCellRange{}
	for metric, colIdx := range metricCols {
		for _, mergeRange := range mergedRanges {
			if mergeRange.EndRow <= mergeRange.StartRow {
				continue
			}
			if rowIdx < mergeRange.StartRow || rowIdx > mergeRange.EndRow {
				continue
			}
			if colIdx < mergeRange.StartCol || colIdx > mergeRange.EndCol {
				continue
			}
			out[metric] = mergeRange
			break
		}
	}
	return out
}

func contractMetricMergeGroups(metricRanges map[string]contractMergedCellRange) []contractMetricMergeGroup {
	type rowSpanKey struct {
		StartRow int
		EndRow   int
	}
	grouped := map[rowSpanKey]*contractMetricMergeGroup{}
	for metric, mergeRange := range metricRanges {
		key := rowSpanKey{StartRow: mergeRange.StartRow, EndRow: mergeRange.EndRow}
		group := grouped[key]
		if group == nil {
			group = &contractMetricMergeGroup{
				Range:   mergeRange,
				Metrics: map[string]bool{},
			}
			grouped[key] = group
		}
		if mergeRange.StartCol < group.Range.StartCol {
			group.Range.StartCol = mergeRange.StartCol
		}
		if mergeRange.EndCol > group.Range.EndCol {
			group.Range.EndCol = mergeRange.EndCol
		}
		group.Metrics[metric] = true
	}
	groups := make([]contractMetricMergeGroup, 0, len(grouped))
	for _, group := range grouped {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return contractMergedRangeLess(groups[i].Range, groups[j].Range)
	})
	return groups
}

func contractMergedRangeLess(left, right contractMergedCellRange) bool {
	if left.StartRow != right.StartRow {
		return left.StartRow < right.StartRow
	}
	if left.EndRow != right.EndRow {
		return left.EndRow < right.EndRow
	}
	if left.StartCol != right.StartCol {
		return left.StartCol < right.StartCol
	}
	return left.EndCol < right.EndCol
}

func mergedCostSettlementCleanupRows(sheetName string, rows [][]string, mergeRange contractMergedCellRange, yearMonth string) []contractCostSettlementCleanupRow {
	out := make([]contractCostSettlementCleanupRow, 0)
	members, _ := mergedContractGroupMembers(rows, mergeRange)
	for _, member := range members {
		out = append(out, contractCostSettlementCleanupRow{
			contractKey:     member,
			SourceSheetName: strings.TrimSpace(sheetName),
			YearMonth:       yearMonth,
		})
	}
	return out
}

func buildMergedCostSettlementGroupRow(sheetName string, rows [][]string, rowIdx int, mergeRange contractMergedCellRange, monthCol monthColumn, yearMonth string, metrics map[string]bool, mergedRanges []contractMergedCellRange, cellNotes contractSourceCellNotes) (contractCostSettlementGroupRow, bool) {
	if rowIdx < 0 || rowIdx >= len(rows) {
		return contractCostSettlementGroupRow{}, false
	}
	row := rows[rowIdx]
	name := strings.TrimSpace(cellValue(row, 0))
	if name == "" {
		return contractCostSettlementGroupRow{}, false
	}
	settlement := 0.0
	if metrics["settlement"] {
		settlement = parseContractFloat(cellValue(row, monthCol.Index+1))
	}
	invoice := 0.0
	if metrics["invoice"] {
		invoice = parseContractFloat(cellValue(row, monthCol.Index+3))
	}
	paid := 0.0
	if metrics["paid"] {
		paid = parseContractFloat(cellValue(row, monthCol.Index+4))
	}
	if settlement <= 0 && invoice <= 0 && paid <= 0 {
		return contractCostSettlementGroupRow{}, false
	}
	members, memberSourceRows := mergedContractGroupMembers(rows, mergeRange)
	return contractCostSettlementGroupRow{
		CustomerName:        name,
		SourceSheetName:     strings.TrimSpace(sheetName),
		YearMonth:           yearMonth,
		Quantity:            defaultContractText(cellValue(row, monthCol.Index), "/"),
		SettlementAmount:    settlement,
		IsInvoiced:          defaultContractText(cellValue(row, monthCol.Index+2), "否"),
		InvoiceAmount:       invoice,
		PaidAmount:          paid,
		AccountCode:         strings.TrimSpace(cellValue(row, 6)),
		ContractStartDate:   normalizeContractDateString(cellValue(row, 2)),
		ContractEndDate:     normalizeContractDateString(cellValue(row, 3)),
		SettlementCycle:     strings.TrimSpace(cellValue(row, 4)),
		SettlementUnitPrice: strings.TrimSpace(cellValue(row, 5)),
		SourceStartRow:      mergeRange.StartRow + 1,
		SourceEndRow:        mergeRange.EndRow + 1,
		MergeRange:          mergeRangeLabel(mergeRange),
		SourceCellNotes:     sourceCellNotesJSON(cellNotesForRowRangeColumns(cellNotes, mergedRanges, mergeRange.StartRow, mergeRange.EndRow, append(intRange(0, 6), intRange(monthCol.Index, monthCol.Index+4)...)...)),
		Members:             members,
		MemberSourceRows:    memberSourceRows,
	}, true
}

func parseFundIncomeQuarterRows(sheetName string, rows [][]string, mergedRanges []contractMergedCellRange, cellNotes contractSourceCellNotes) ([]contractFundIncomeRow, []contractFundIncomeGroupRow, []contractFundIncomeCleanupRow) {
	if len(rows) < 3 {
		return nil, nil, nil
	}
	year := extractSheetYear(sheetName)
	if year == "" {
		return nil, nil, nil
	}
	rows = normalizeContractRows(rows, mergedRanges, 0, 1, 2, 3, 4, 5)
	header := rows[0]
	out := make([]contractFundIncomeRow, 0)
	groups := make([]contractFundIncomeGroupRow, 0)
	cleanup := make([]contractFundIncomeCleanupRow, 0)
	monthCols := monthColumns(header)
	if len(monthCols) == 0 {
		return nil, nil, nil
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
			metricRanges := fundMergedAmountRanges(mergedRanges, idx, monthCol, simpleAmountLayout)
			if len(metricRanges) > 0 {
				metricGroups := contractMetricMergeGroups(metricRanges)
				if len(metricGroups) == 1 {
					metricGroups[0].Metrics = fundAmountMetricSet(simpleAmountLayout)
				}
				for _, metricGroup := range metricGroups {
					cleanup = append(cleanup, mergedFundIncomeCleanupRows(sheetName, rows, metricGroup.Range, yearMonth)...)
					mergedRow, ok := buildMergedFundIncomeGroupRow(sheetName, rows, idx, metricGroup.Range, monthCol, yearMonth, simpleAmountLayout, metricGroup.Metrics, mergedRanges, cellNotes)
					if ok {
						groups = append(groups, mergedRow)
					}
				}
				continue
			}
			directMetricRanges := fundMergedAmountContainingRanges(mergedRanges, idx, monthCol, simpleAmountLayout)
			metricEnd := monthCol.Index
			if !simpleAmountLayout {
				metricEnd = monthCol.Index + 4
			}
			sourceNotes := sourceCellNotesJSON(cellNotesForRowColumns(cellNotes, mergedRanges, idx, append(intRange(0, 5), intRange(monthCol.Index, metricEnd)...)...))
			if directRow, ok := buildDirectFundIncomeRow(sheetName, row, monthCol, yearMonth, contractKey{Name: name, Content: content}, contractStartDate, contractEndDate, settlementCycle, settlementUnitPrice, simpleAmountLayout, directMetricRanges, sourceNotes); ok {
				out = append(out, directRow)
			}
		}
	}
	return out, groups, cleanup
}

func buildDirectFundIncomeRow(sheetName string, row []string, monthCol monthColumn, yearMonth string, key contractKey, contractStartDate, contractEndDate, settlementCycle, settlementUnitPrice string, simpleAmountLayout bool, metricRanges map[string]contractMergedCellRange, sourceCellNotes string) (contractFundIncomeRow, bool) {
	if simpleAmountLayout {
		if _, merged := metricRanges["settlement"]; merged {
			return contractFundIncomeRow{}, false
		}
		if _, merged := metricRanges["received"]; merged {
			return contractFundIncomeRow{}, false
		}
		amount := parseContractFloat(cellValue(row, monthCol.Index))
		if amount <= 0 {
			return contractFundIncomeRow{}, false
		}
		return contractFundIncomeRow{
			contractKey:         key,
			SourceSheetName:     strings.TrimSpace(sheetName),
			YearMonth:           yearMonth,
			SettlementAmount:    amount,
			ReceivedAmount:      amount,
			IsInvoiced:          "否",
			ContractStartDate:   contractStartDate,
			ContractEndDate:     contractEndDate,
			SettlementCycle:     settlementCycle,
			SettlementUnitPrice: settlementUnitPrice,
			SourceCellNotes:     sourceCellNotes,
		}, true
	}

	settlement := 0.0
	if _, merged := metricRanges["settlement"]; !merged {
		settlement = parseContractFloat(cellValue(row, monthCol.Index+1))
	}
	invoice := 0.0
	if _, merged := metricRanges["invoice"]; !merged {
		invoice = parseContractFloat(cellValue(row, monthCol.Index+3))
	}
	received := 0.0
	if _, merged := metricRanges["received"]; !merged {
		received = parseContractFloat(cellValue(row, monthCol.Index+4))
	}
	if settlement <= 0 && received <= 0 && invoice <= 0 {
		return contractFundIncomeRow{}, false
	}
	return contractFundIncomeRow{
		contractKey:         key,
		SourceSheetName:     strings.TrimSpace(sheetName),
		YearMonth:           yearMonth,
		Quantity:            strings.TrimSpace(cellValue(row, monthCol.Index)),
		SettlementAmount:    settlement,
		ReceivedAmount:      received,
		IsInvoiced:          defaultContractText(cellValue(row, monthCol.Index+2), "否"),
		InvoiceAmount:       invoice,
		ContractStartDate:   contractStartDate,
		ContractEndDate:     contractEndDate,
		SettlementCycle:     settlementCycle,
		SettlementUnitPrice: settlementUnitPrice,
		SourceCellNotes:     sourceCellNotes,
	}, true
}

func fundMergedAmountRanges(mergedRanges []contractMergedCellRange, rowIdx int, monthCol monthColumn, simpleAmountLayout bool) map[string]contractMergedCellRange {
	if simpleAmountLayout {
		return contractMergedAmountRanges(mergedRanges, rowIdx, map[string]int{
			"settlement": monthCol.Index,
			"received":   monthCol.Index,
		})
	}
	return contractMergedAmountRanges(mergedRanges, rowIdx, map[string]int{
		"settlement": monthCol.Index + 1,
		"invoice":    monthCol.Index + 3,
		"received":   monthCol.Index + 4,
	})
}

func fundMergedAmountContainingRanges(mergedRanges []contractMergedCellRange, rowIdx int, monthCol monthColumn, simpleAmountLayout bool) map[string]contractMergedCellRange {
	if simpleAmountLayout {
		return contractMergedAmountContainingRanges(mergedRanges, rowIdx, map[string]int{
			"settlement": monthCol.Index,
			"received":   monthCol.Index,
		})
	}
	return contractMergedAmountContainingRanges(mergedRanges, rowIdx, map[string]int{
		"settlement": monthCol.Index + 1,
		"invoice":    monthCol.Index + 3,
		"received":   monthCol.Index + 4,
	})
}

func fundAmountMetricSet(simpleAmountLayout bool) map[string]bool {
	if simpleAmountLayout {
		return map[string]bool{
			"settlement": true,
			"received":   true,
		}
	}
	return map[string]bool{
		"settlement": true,
		"invoice":    true,
		"received":   true,
	}
}

func mergedFundIncomeCleanupRows(sheetName string, rows [][]string, mergeRange contractMergedCellRange, yearMonth string) []contractFundIncomeCleanupRow {
	out := make([]contractFundIncomeCleanupRow, 0)
	startRow := mergeRange.StartRow
	if startRow < 2 {
		startRow = 2
	}
	endRow := mergeRange.EndRow
	if endRow >= len(rows) {
		endRow = len(rows) - 1
	}
	members, _ := mergedContractGroupMembers(rows, contractMergedCellRange{
		StartRow: startRow,
		EndRow:   endRow,
		StartCol: mergeRange.StartCol,
		EndCol:   mergeRange.EndCol,
	})
	for _, member := range members {
		out = append(out, contractFundIncomeCleanupRow{
			contractKey:     member,
			SourceSheetName: strings.TrimSpace(sheetName),
			YearMonth:       yearMonth,
		})
	}
	return out
}

func buildMergedFundIncomeGroupRow(sheetName string, rows [][]string, rowIdx int, mergeRange contractMergedCellRange, monthCol monthColumn, yearMonth string, simpleAmountLayout bool, metrics map[string]bool, mergedRanges []contractMergedCellRange, cellNotes contractSourceCellNotes) (contractFundIncomeGroupRow, bool) {
	if rowIdx < 0 || rowIdx >= len(rows) {
		return contractFundIncomeGroupRow{}, false
	}
	row := rows[rowIdx]
	name := strings.TrimSpace(cellValue(row, 0))
	if name == "" {
		return contractFundIncomeGroupRow{}, false
	}
	contractStartDate := normalizeContractDateString(cellValue(row, 2))
	contractEndDate := normalizeContractDateString(cellValue(row, 3))
	settlementCycle := strings.TrimSpace(cellValue(row, 4))
	settlementUnitPrice := strings.TrimSpace(cellValue(row, 5))
	members, memberSourceRows := mergedFundIncomeGroupMembers(rows, mergeRange)

	if simpleAmountLayout {
		amount := parseContractFloat(cellValue(row, monthCol.Index))
		settlement := 0.0
		if metrics["settlement"] {
			settlement = amount
		}
		received := 0.0
		if metrics["received"] {
			received = amount
		}
		if settlement <= 0 && received <= 0 {
			return contractFundIncomeGroupRow{}, false
		}
		return contractFundIncomeGroupRow{
			CustomerName:        name,
			SourceSheetName:     strings.TrimSpace(sheetName),
			YearMonth:           yearMonth,
			SettlementAmount:    settlement,
			ReceivedAmount:      received,
			IsInvoiced:          "否",
			ContractStartDate:   contractStartDate,
			ContractEndDate:     contractEndDate,
			SettlementCycle:     settlementCycle,
			SettlementUnitPrice: settlementUnitPrice,
			SourceStartRow:      mergeRange.StartRow + 1,
			SourceEndRow:        mergeRange.EndRow + 1,
			MergeRange:          mergeRangeLabel(mergeRange),
			SourceCellNotes:     sourceCellNotesJSON(cellNotesForRowRangeColumns(cellNotes, mergedRanges, mergeRange.StartRow, mergeRange.EndRow, append(intRange(0, 5), monthCol.Index)...)),
			Members:             members,
			MemberSourceRows:    memberSourceRows,
		}, true
	}

	settlement := 0.0
	if metrics["settlement"] {
		settlement = parseContractFloat(cellValue(row, monthCol.Index+1))
	}
	received := 0.0
	if metrics["received"] {
		received = parseContractFloat(cellValue(row, monthCol.Index+4))
	}
	invoice := 0.0
	if metrics["invoice"] {
		invoice = parseContractFloat(cellValue(row, monthCol.Index+3))
	}
	if settlement <= 0 && received <= 0 && invoice <= 0 {
		return contractFundIncomeGroupRow{}, false
	}
	return contractFundIncomeGroupRow{
		CustomerName:        name,
		SourceSheetName:     strings.TrimSpace(sheetName),
		YearMonth:           yearMonth,
		Quantity:            strings.TrimSpace(cellValue(row, monthCol.Index)),
		SettlementAmount:    settlement,
		ReceivedAmount:      received,
		IsInvoiced:          defaultContractText(cellValue(row, monthCol.Index+2), "否"),
		InvoiceAmount:       invoice,
		ContractStartDate:   contractStartDate,
		ContractEndDate:     contractEndDate,
		SettlementCycle:     settlementCycle,
		SettlementUnitPrice: settlementUnitPrice,
		SourceStartRow:      mergeRange.StartRow + 1,
		SourceEndRow:        mergeRange.EndRow + 1,
		MergeRange:          mergeRangeLabel(mergeRange),
		SourceCellNotes:     sourceCellNotesJSON(cellNotesForRowRangeColumns(cellNotes, mergedRanges, mergeRange.StartRow, mergeRange.EndRow, append(intRange(0, 5), intRange(monthCol.Index, monthCol.Index+4)...)...)),
		Members:             members,
		MemberSourceRows:    memberSourceRows,
	}, true
}

func mergedFundIncomeGroupMembers(rows [][]string, mergeRange contractMergedCellRange) ([]contractKey, map[contractKey]int) {
	return mergedContractGroupMembers(rows, mergeRange)
}

func mergedContractGroupMembers(rows [][]string, mergeRange contractMergedCellRange) ([]contractKey, map[contractKey]int) {
	members := make([]contractKey, 0)
	sourceRows := map[contractKey]int{}
	seen := map[contractKey]struct{}{}
	startRow := mergeRange.StartRow
	if startRow < 2 {
		startRow = 2
	}
	endRow := mergeRange.EndRow
	if endRow >= len(rows) {
		endRow = len(rows) - 1
	}
	for rowIdx := startRow; rowIdx <= endRow; rowIdx++ {
		row := rows[rowIdx]
		name := strings.TrimSpace(cellValue(row, 0))
		content := strings.TrimSpace(cellValue(row, 1))
		if name == "" || content == "" || content == "分配" {
			continue
		}
		member := contractKey{Name: name, Content: content}
		if _, ok := seen[member]; ok {
			continue
		}
		seen[member] = struct{}{}
		members = append(members, member)
		sourceRows[member] = rowIdx + 1
	}
	return members, sourceRows
}

func mergeRangeLabel(mergeRange contractMergedCellRange) string {
	start, err := excelize.CoordinatesToCellName(mergeRange.StartCol+1, mergeRange.StartRow+1)
	if err != nil {
		return ""
	}
	end, err := excelize.CoordinatesToCellName(mergeRange.EndCol+1, mergeRange.EndRow+1)
	if err != nil {
		return start
	}
	return start + ":" + end
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
SET contract_start_date = ?, contract_end_date = ?, settlement_cycle = ?, updated_at = CURRENT_TIMESTAMP
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
INSERT INTO fin_contracts(contract_id, customer_name, contract_content, contract_start_date, contract_end_date, settlement_cycle, updated_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
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
			if err := deleteContractRowsBySourceScope(ctx, tx, "fin_cost_settlements", compatibleContractReportTypes(bundle.Kind, contractWorkbookRevenueCost), bundle.TableSourceSheets["fin_cost_settlements"]); err != nil {
				return err
			}
			if err := deleteCostSettlementGroupsBySourceScope(ctx, tx, compatibleContractReportTypes(bundle.Kind, contractWorkbookRevenueCost), bundle.TableSourceSheets["fin_cost_settlements"]); err != nil {
				return err
			}
		}
		if contractBundleTouchesTable(bundle, "fin_fund_income") {
			if err := deleteContractRowsBySourceScope(ctx, tx, "fin_fund_income", compatibleContractReportTypes(bundle.Kind, contractWorkbookRevenueCost, contractWorkbookFund), bundle.TableSourceSheets["fin_fund_income"]); err != nil {
				return err
			}
			if err := deleteFundIncomeGroupsBySourceScope(ctx, tx, compatibleContractReportTypes(bundle.Kind, contractWorkbookFund), bundle.TableSourceSheets["fin_fund_income"]); err != nil {
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
		for _, row := range bundle.CostCleanupRows {
			contractID := strings.TrimSpace(contractIDs[row.contractKey])
			if contractID == "" {
				continue
			}
			if err := deleteContractRowByIdentity(ctx, tx, "fin_cost_settlements", bundle.Kind, row.SourceSheetName, contractID, row.YearMonth); err != nil {
				return fmt.Errorf("delete merged child fin_cost_settlements: %w", err)
			}
			if err := deleteLegacyContractRow(ctx, tx, "fin_cost_settlements", contractID, row.YearMonth); err != nil {
				return fmt.Errorf("delete legacy merged child fin_cost_settlements row: %w", err)
			}
		}
		for _, row := range bundle.CostGroupRows {
			if err := deleteCostSettlementGroupByIdentity(ctx, tx, bundle.Kind, row.SourceSheetName, row.CustomerName, row.YearMonth, row.SourceStartRow, row.SourceEndRow); err != nil {
				return fmt.Errorf("delete prior fin_cost_settlement group: %w", err)
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
	contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price, source_cell_notes, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`, contractIDs[row.contractKey], row.YearMonth, string(bundle.Kind), nullableContractValue(row.SourceSheetName), nullableContractValue(row.Quantity), row.SettlementAmount, 0, row.IsInvoiced, row.InvoiceAmount, nullableContractValue(row.ContractStartDate), nullableContractValue(row.ContractEndDate), nullableContractValue(row.SettlementCycle), nullableContractValue(row.SettlementUnitPrice), nullableContractValue(row.SourceCellNotes)); err != nil {
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
	contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price, source_cell_notes, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`, contractIDs[row.contractKey], row.YearMonth, string(bundle.Kind), nullableContractValue(row.SourceSheetName), nullableContractValue(row.Quantity), row.SettlementAmount, row.IsInvoiced, row.InvoiceAmount, row.PaidAmount, nullableContractValue(row.AccountCode), nullableContractValue(row.ContractStartDate), nullableContractValue(row.ContractEndDate), nullableContractValue(row.SettlementCycle), nullableContractValue(row.SettlementUnitPrice), nullableContractValue(row.SourceCellNotes)); err != nil {
			return fmt.Errorf("insert fin_cost_settlements: %w", err)
		}
	}
	for _, row := range bundle.CostGroupRows {
		groupID, err := insertCostSettlementGroup(ctx, tx, bundle.Kind, row)
		if err != nil {
			return err
		}
		for _, member := range row.Members {
			contractID := strings.TrimSpace(contractIDs[member])
			if contractID == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_cost_settlement_group_members(group_id, contract_id, source_row_number, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
`, groupID, contractID, sourceRowNumberForCostGroupMember(row, member)); err != nil {
				return fmt.Errorf("insert fin_cost_settlement_group_members: %w", err)
			}
		}
	}
	return nil
}

func contractBundleTouchesTable(bundle contractImportBundle, tableName string) bool {
	return len(bundle.TableSourceSheets[tableName]) > 0
}

func deleteContractRowsBySourceScope(ctx context.Context, tx *sql.Tx, tableName string, reportTypes []contractWorkbookKind, sheetNames []string) error {
	sheetNames = dedupeContractStrings(sheetNames)
	reportTypes = dedupeContractWorkbookKinds(reportTypes)
	if len(sheetNames) == 0 || len(reportTypes) == 0 {
		return nil
	}

	args := make([]any, 0, len(reportTypes)+len(sheetNames))
	reportPlaceholders := make([]string, 0, len(reportTypes))
	for _, reportType := range reportTypes {
		reportPlaceholders = append(reportPlaceholders, "?")
		args = append(args, string(reportType))
	}
	sheetPlaceholders := make([]string, 0, len(sheetNames))
	for _, sheetName := range sheetNames {
		sheetPlaceholders = append(sheetPlaceholders, "?")
		args = append(args, sheetName)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %s
WHERE source_report_type IN (%s)
  AND source_sheet_name IN (%s)
`, tableName, strings.Join(reportPlaceholders, ", "), strings.Join(sheetPlaceholders, ", ")), args...); err != nil {
		return fmt.Errorf("clear %s by source scope: %w", tableName, err)
	}
	return nil
}

func compatibleContractReportTypes(current contractWorkbookKind, legacyKinds ...contractWorkbookKind) []contractWorkbookKind {
	out := []contractWorkbookKind{current}
	if current == contractWorkbookMixed {
		out = append(out, legacyKinds...)
	}
	return out
}

func dedupeContractWorkbookKinds(values []contractWorkbookKind) []contractWorkbookKind {
	out := make([]contractWorkbookKind, 0, len(values))
	seen := map[contractWorkbookKind]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(string(value)) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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

func deleteLegacyMergedFundIncomeContracts(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
DELETE FROM fin_fund_income
WHERE contract_id IN (
	SELECT contract_id
	FROM fin_contracts
	WHERE contract_content LIKE '合并金额组%'
)
`); err != nil {
		return fmt.Errorf("delete legacy merged-group fin_fund_income rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM fin_contracts
WHERE contract_content LIKE '合并金额组%'
`); err != nil {
		return fmt.Errorf("delete legacy merged-group fin_contracts rows: %w", err)
	}
	return nil
}

func deleteCostSettlementGroupsBySourceScope(ctx context.Context, tx *sql.Tx, reportTypes []contractWorkbookKind, sheetNames []string) error {
	sheetNames = dedupeContractStrings(sheetNames)
	reportTypes = dedupeContractWorkbookKinds(reportTypes)
	if len(sheetNames) == 0 || len(reportTypes) == 0 {
		return nil
	}

	args := make([]any, 0, len(reportTypes)+len(sheetNames))
	reportPlaceholders := make([]string, 0, len(reportTypes))
	for _, reportType := range reportTypes {
		reportPlaceholders = append(reportPlaceholders, "?")
		args = append(args, string(reportType))
	}
	sheetPlaceholders := make([]string, 0, len(sheetNames))
	for _, sheetName := range sheetNames {
		sheetPlaceholders = append(sheetPlaceholders, "?")
		args = append(args, sheetName)
	}
	filter := fmt.Sprintf(`source_report_type IN (%s) AND source_sheet_name IN (%s)`, strings.Join(reportPlaceholders, ", "), strings.Join(sheetPlaceholders, ", "))
	if err := deleteCostSettlementGroupsByFilter(ctx, tx, filter, args...); err != nil {
		return fmt.Errorf("clear fin_cost_settlement_groups by source scope: %w", err)
	}
	return nil
}

func deleteCostSettlementGroupByIdentity(ctx context.Context, tx *sql.Tx, reportType contractWorkbookKind, sheetName, customerName, yearMonth string, sourceStartRow, sourceEndRow int) error {
	return deleteCostSettlementGroupsByFilter(ctx, tx, `
source_report_type = ?
  AND source_sheet_name = ?
  AND customer_name = ?
  AND year_month = ?
  AND source_start_row = ?
  AND source_end_row = ?
`, string(reportType), strings.TrimSpace(sheetName), strings.TrimSpace(customerName), strings.TrimSpace(yearMonth), sourceStartRow, sourceEndRow)
}

func deleteCostSettlementGroupsByFilter(ctx context.Context, tx *sql.Tx, filter string, args ...any) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM fin_cost_settlement_groups WHERE `+filter, args...)
	if err != nil {
		return err
	}
	groupIDs := []int64{}
	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			_ = rows.Close()
			return err
		}
		groupIDs = append(groupIDs, groupID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, groupID := range groupIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fin_cost_settlement_group_members WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM fin_cost_settlement_groups WHERE id = ?`, groupID); err != nil {
			return err
		}
	}
	return nil
}

func deleteFundIncomeGroupsBySourceScope(ctx context.Context, tx *sql.Tx, reportTypes []contractWorkbookKind, sheetNames []string) error {
	sheetNames = dedupeContractStrings(sheetNames)
	reportTypes = dedupeContractWorkbookKinds(reportTypes)
	if len(sheetNames) == 0 || len(reportTypes) == 0 {
		return nil
	}

	args := make([]any, 0, len(reportTypes)+len(sheetNames))
	reportPlaceholders := make([]string, 0, len(reportTypes))
	for _, reportType := range reportTypes {
		reportPlaceholders = append(reportPlaceholders, "?")
		args = append(args, string(reportType))
	}
	sheetPlaceholders := make([]string, 0, len(sheetNames))
	for _, sheetName := range sheetNames {
		sheetPlaceholders = append(sheetPlaceholders, "?")
		args = append(args, sheetName)
	}
	filter := fmt.Sprintf(`source_report_type IN (%s) AND source_sheet_name IN (%s)`, strings.Join(reportPlaceholders, ", "), strings.Join(sheetPlaceholders, ", "))
	if err := deleteFundIncomeGroupsByFilter(ctx, tx, filter, args...); err != nil {
		return fmt.Errorf("clear fin_fund_income_groups by source scope: %w", err)
	}
	return nil
}

func deleteFundIncomeGroupByIdentity(ctx context.Context, tx *sql.Tx, reportType contractWorkbookKind, sheetName, customerName, yearMonth string, sourceStartRow, sourceEndRow int) error {
	return deleteFundIncomeGroupsByFilter(ctx, tx, `
source_report_type = ?
  AND source_sheet_name = ?
  AND customer_name = ?
  AND year_month = ?
  AND source_start_row = ?
  AND source_end_row = ?
`, string(reportType), strings.TrimSpace(sheetName), strings.TrimSpace(customerName), strings.TrimSpace(yearMonth), sourceStartRow, sourceEndRow)
}

func deleteFundIncomeGroupsByFilter(ctx context.Context, tx *sql.Tx, filter string, args ...any) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM fin_fund_income_groups WHERE `+filter, args...)
	if err != nil {
		return err
	}
	groupIDs := []int64{}
	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			_ = rows.Close()
			return err
		}
		groupIDs = append(groupIDs, groupID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, groupID := range groupIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fin_fund_income_group_members WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM fin_fund_income_groups WHERE id = ?`, groupID); err != nil {
			return err
		}
	}
	return nil
}

func replaceFundIncomeRows(ctx context.Context, tx *sql.Tx, bundle contractImportBundle, contractIDs map[contractKey]string, incremental bool) error {
	if err := deleteLegacyMergedFundIncomeContracts(ctx, tx); err != nil {
		return err
	}
	if !incremental {
		if err := deleteContractRowsBySourceScope(ctx, tx, "fin_fund_income", compatibleContractReportTypes(bundle.Kind, contractWorkbookFund), bundle.TableSourceSheets["fin_fund_income"]); err != nil {
			return err
		}
		if err := deleteFundIncomeGroupsBySourceScope(ctx, tx, compatibleContractReportTypes(bundle.Kind, contractWorkbookFund), bundle.TableSourceSheets["fin_fund_income"]); err != nil {
			return err
		}
	} else {
		for _, row := range bundle.FundCleanupRows {
			contractID := strings.TrimSpace(contractIDs[row.contractKey])
			if contractID == "" {
				continue
			}
			if err := deleteContractRowByIdentity(ctx, tx, "fin_fund_income", bundle.Kind, row.SourceSheetName, contractID, row.YearMonth); err != nil {
				return fmt.Errorf("delete merged child fin_fund_income: %w", err)
			}
			if err := deleteLegacyContractRow(ctx, tx, "fin_fund_income", contractID, row.YearMonth); err != nil {
				return fmt.Errorf("delete legacy merged child fin_fund_income row: %w", err)
			}
		}
		for _, row := range bundle.FundRows {
			if err := deleteContractRowByIdentity(ctx, tx, "fin_fund_income", bundle.Kind, row.SourceSheetName, contractIDs[row.contractKey], row.YearMonth); err != nil {
				return fmt.Errorf("delete prior fin_fund_income: %w", err)
			}
		}
		for _, row := range bundle.FundGroupRows {
			if err := deleteFundIncomeGroupByIdentity(ctx, tx, bundle.Kind, row.SourceSheetName, row.CustomerName, row.YearMonth, row.SourceStartRow, row.SourceEndRow); err != nil {
				return fmt.Errorf("delete prior fin_fund_income group: %w", err)
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
	contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price, source_cell_notes, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`, contractIDs[row.contractKey], row.YearMonth, string(bundle.Kind), nullableContractValue(row.SourceSheetName), nullableContractValue(row.Quantity), row.SettlementAmount, row.ReceivedAmount, row.IsInvoiced, row.InvoiceAmount, nullableContractValue(row.ContractStartDate), nullableContractValue(row.ContractEndDate), nullableContractValue(row.SettlementCycle), nullableContractValue(row.SettlementUnitPrice), nullableContractValue(row.SourceCellNotes)); err != nil {
			return fmt.Errorf("insert fin_fund_income: %w", err)
		}
	}
	for _, row := range bundle.FundGroupRows {
		groupID, err := insertFundIncomeGroup(ctx, tx, bundle.Kind, row)
		if err != nil {
			return err
		}
		for _, member := range row.Members {
			contractID := strings.TrimSpace(contractIDs[member])
			if contractID == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_fund_income_group_members(group_id, contract_id, source_row_number, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
`, groupID, contractID, sourceRowNumberForGroupMember(row, member)); err != nil {
				return fmt.Errorf("insert fin_fund_income_group_members: %w", err)
			}
		}
	}
	return nil
}

func insertFundIncomeGroup(ctx context.Context, tx *sql.Tx, reportType contractWorkbookKind, row contractFundIncomeGroupRow) (int64, error) {
	var groupID int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO fin_fund_income_groups(
	customer_name, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, merge_range,
	quantity, settlement_amount, received_amount, is_invoiced, invoice_amount,
	contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price, source_cell_notes, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
RETURNING id
`, row.CustomerName, row.YearMonth, string(reportType), nullableContractValue(row.SourceSheetName), row.SourceStartRow, row.SourceEndRow, nullableContractValue(row.MergeRange), nullableContractValue(row.Quantity), row.SettlementAmount, row.ReceivedAmount, row.IsInvoiced, row.InvoiceAmount, nullableContractValue(row.ContractStartDate), nullableContractValue(row.ContractEndDate), nullableContractValue(row.SettlementCycle), nullableContractValue(row.SettlementUnitPrice), nullableContractValue(row.SourceCellNotes)).Scan(&groupID); err != nil {
		return 0, fmt.Errorf("insert fin_fund_income_groups: %w", err)
	}
	return groupID, nil
}

func insertCostSettlementGroup(ctx context.Context, tx *sql.Tx, reportType contractWorkbookKind, row contractCostSettlementGroupRow) (int64, error) {
	var groupID int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO fin_cost_settlement_groups(
	customer_name, year_month, source_report_type, source_sheet_name, source_start_row, source_end_row, merge_range,
	quantity, settlement_amount, is_invoiced, invoice_amount, paid_amount, account_code,
	contract_start_date, contract_end_date, settlement_cycle, settlement_unit_price, source_cell_notes, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
RETURNING id
`, row.CustomerName, row.YearMonth, string(reportType), nullableContractValue(row.SourceSheetName), row.SourceStartRow, row.SourceEndRow, nullableContractValue(row.MergeRange), nullableContractValue(row.Quantity), row.SettlementAmount, row.IsInvoiced, row.InvoiceAmount, row.PaidAmount, nullableContractValue(row.AccountCode), nullableContractValue(row.ContractStartDate), nullableContractValue(row.ContractEndDate), nullableContractValue(row.SettlementCycle), nullableContractValue(row.SettlementUnitPrice), nullableContractValue(row.SourceCellNotes)).Scan(&groupID); err != nil {
		return 0, fmt.Errorf("insert fin_cost_settlement_groups: %w", err)
	}
	return groupID, nil
}

func sourceRowNumberForCostGroupMember(row contractCostSettlementGroupRow, member contractKey) int {
	member.Name = strings.TrimSpace(member.Name)
	member.Content = strings.TrimSpace(member.Content)
	if row.MemberSourceRows != nil {
		if sourceRow, ok := row.MemberSourceRows[member]; ok {
			return sourceRow
		}
	}
	for idx, candidate := range row.Members {
		if strings.TrimSpace(candidate.Name) == member.Name && strings.TrimSpace(candidate.Content) == member.Content {
			return row.SourceStartRow + idx
		}
	}
	return 0
}

func sourceRowNumberForGroupMember(row contractFundIncomeGroupRow, member contractKey) int {
	member.Name = strings.TrimSpace(member.Name)
	member.Content = strings.TrimSpace(member.Content)
	if row.MemberSourceRows != nil {
		if sourceRow, ok := row.MemberSourceRows[member]; ok {
			return sourceRow
		}
	}
	for idx, candidate := range row.Members {
		if strings.TrimSpace(candidate.Name) == member.Name && strings.TrimSpace(candidate.Content) == member.Content {
			return row.SourceStartRow + idx
		}
	}
	return 0
}

type monthColumn struct {
	Label string
	Index int
	Year  string
}

func monthColumns(header []string) []monthColumn {
	out := make([]monthColumn, 0, 12)
	for idx, raw := range header {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}
		if parsed, ok := parseYearMonthHeader(label); ok {
			parsed.Index = idx
			out = append(out, parsed)
			continue
		}
		if parsed, ok := parseMonthHeader(label); ok {
			parsed.Index = idx
			out = append(out, parsed)
		}
	}
	return out
}

func parseYearMonthHeader(label string) (monthColumn, bool) {
	match := regexp.MustCompile(`(\d{2,4})\s*年\s*(\d{1,2})\s*月`).FindStringSubmatch(strings.TrimSpace(label))
	if len(match) != 3 {
		return monthColumn{}, false
	}
	year := normalizeHeaderYear(match[1])
	monthValue, err := strconv.Atoi(match[2])
	if year == "" || err != nil || monthValue < 1 || monthValue > 12 {
		return monthColumn{}, false
	}
	return monthColumn{
		Label: fmt.Sprintf("%d月", monthValue),
		Year:  year,
	}, true
}

func parseMonthHeader(label string) (monthColumn, bool) {
	match := regexp.MustCompile(`^(\d{1,2})\s*月$`).FindStringSubmatch(strings.TrimSpace(label))
	if len(match) != 2 {
		return monthColumn{}, false
	}
	monthValue, err := strconv.Atoi(match[1])
	if err != nil || monthValue < 1 || monthValue > 12 {
		return monthColumn{}, false
	}
	return monthColumn{Label: fmt.Sprintf("%d月", monthValue)}, true
}

func normalizeHeaderYear(raw string) string {
	year := strings.TrimSpace(raw)
	if len(year) == 2 {
		return "20" + year
	}
	if len(year) == 4 && strings.HasPrefix(year, "20") {
		return year
	}
	return ""
}

func monthColumnDefaultYear(monthCol monthColumn, defaults map[string]string) string {
	if strings.TrimSpace(monthCol.Year) != "" {
		return strings.TrimSpace(monthCol.Year)
	}
	return strings.TrimSpace(defaults[monthCol.Label])
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
	raw = strings.TrimSpace(raw)
	if isContractBlankDate(raw) {
		return ""
	}
	if parsed, ok := parseContractDate(raw); ok {
		return parsed.Format("2006-01-02")
	}
	return raw
}

func isContractBlankDate(raw string) bool {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, ",", ""))
	if raw == "" {
		return true
	}
	numeric, err := strconv.ParseFloat(raw, 64)
	return err == nil && numeric == 0
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
	if strings.TrimSpace(defaultYear) != "" {
		return normalizeContractYearMonth(monthLabel, strings.TrimSpace(defaultYear))
	}
	if inferredYear := inferYearFromText(textHint, ""); strings.TrimSpace(inferredYear) != "" {
		return normalizeContractYearMonth(monthLabel, inferredYear)
	}
	if yearMonth := inferYearMonthFromContractRange(monthLabel, startText, endText); yearMonth != "" {
		return yearMonth
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
	if isContractBlankDate(raw) {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/01/02",
		"2006/1/2",
		"01-02-2006",
		"1-2-2006",
		"01-02-06",
		"1-2-06",
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

func normalizeContractRows(rows [][]string, mergedRanges []contractMergedCellRange, carryColumns ...int) [][]string {
	out := make([][]string, len(rows))
	for rowIdx, row := range rows {
		next := append([]string(nil), row...)
		for _, col := range carryColumns {
			next = ensureRowLength(next, col+1)
			value := strings.TrimSpace(next[col])
			if value != "" {
				continue
			}
			if carried, ok := mergedContractCellValue(rows, mergedRanges, rowIdx, col); ok {
				next[col] = carried
			}
		}
		out[rowIdx] = next
	}
	return out
}

func mergedContractCellValue(rows [][]string, mergedRanges []contractMergedCellRange, rowIdx, colIdx int) (string, bool) {
	for _, mergeRange := range mergedRanges {
		if mergeRange.EndRow <= mergeRange.StartRow {
			continue
		}
		if rowIdx <= mergeRange.StartRow || rowIdx > mergeRange.EndRow {
			continue
		}
		if colIdx < mergeRange.StartCol || colIdx > mergeRange.EndCol {
			continue
		}
		value := strings.TrimSpace(cellValue(contractCellRow(rows, mergeRange.StartRow), mergeRange.StartCol))
		if value == "" {
			return "", false
		}
		return value, true
	}
	return "", false
}

func contractCellRow(rows [][]string, idx int) []string {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	return rows[idx]
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
