package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

func buildContractWorkbookAudit(path string) ([]auditSection, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var sections []auditSection
	if hasSheet(f, "26年Q1收入明细") {
		rows, err := f.GetRows("26年Q1收入明细")
		if err != nil {
			return nil, err
		}
		mergedRanges, err := readWorkbookAuditMergedRanges(f, "26年Q1收入明细")
		if err != nil {
			return nil, err
		}
		rows = normalizeWorkbookAuditRows(rows, mergedRanges, 0, 1, 2, 3, 4, 5)
		sections = append(sections, auditWorkbookTotalRow(f, "excel_contract_2026_03_total_row", "26年Q1收入明细", "R", "U", "T"))
		sections = append(sections, auditWorkbookTotalRowSum(f, "excel_contract_2026_q1_total_row", "26年Q1收入明细",
			[]string{"H", "M", "R"},
			[]string{"K", "P", "U"},
			[]string{"J", "O", "T"},
		))
		sections = append(sections, auditFundIncomeRows("excel_contract_2026_q1", rows, "")...)
		sections = append(sections, auditFundIncomeRows("excel_contract_2026_03", rows, "2026-03")...)
		sections = append(sections, auditFundIncomeEntity("excel_feiweiyunke_2026_q1", rows, "飞未云科", "")...)
		sections = append(sections, auditFundIncomeEntity("excel_feiweiyunke_2026_03", rows, "飞未云科", "2026-03")...)
		sections = append(sections, auditFundIncomeEntity("excel_zhongxin_revenue_2026_03", rows, "众信数通", "2026-03")...)
	}
	if hasSheet(f, "成本-月度结算") {
		rows, err := f.GetRows("成本-月度结算")
		if err != nil {
			return nil, err
		}
		mergedRanges, err := readWorkbookAuditMergedRanges(f, "成本-月度结算")
		if err != nil {
			return nil, err
		}
		rows = normalizeWorkbookAuditRows(rows, mergedRanges, 0, 1, 2, 3, 4, 5, 6)
		sections = append(sections, auditWorkbookTotalRow(f, "excel_cost_2026_03_total_row", "成本-月度结算", "S", "V", "U"))
		sections = append(sections, auditWorkbookTotalRowSum(f, "excel_cost_2026_q1_total_row", "成本-月度结算",
			[]string{"I", "N", "S"},
			[]string{"L", "Q", "V"},
			[]string{"K", "P", "U"},
		))
		sections = append(sections, auditCostRows("excel_cost_2026_q1", rows, "")...)
		sections = append(sections, auditCostRows("excel_cost_2026_03", rows, "2026-03")...)
		sections = append(sections, auditCostEntity("excel_linyue_cost_2026_03", rows, "林悦", "2026-03")...)
		sections = append(sections, auditCostEntity("excel_zhongxin_cost_2026_03", rows, "众信数通", "2026-03")...)
	}
	return sections, nil
}

func auditWorkbookTotalRow(f *excelize.File, name, sheet, settlementCol, cashCol, invoiceCol string) auditSection {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return auditSection{Name: name}
	}
	totalRow := findWorkbookTotalRow(rows, settlementCol, cashCol, invoiceCol)
	if totalRow <= 0 {
		return auditSection{Name: name}
	}
	section := auditSection{Name: name, Rows: 1}
	section.Amounts = appendCellAmount(section.Amounts, f, sheet, totalRow, "settlement_amount", settlementCol)
	section.Amounts = appendCellAmount(section.Amounts, f, sheet, totalRow, "cash_amount", cashCol)
	section.Amounts = appendCellAmount(section.Amounts, f, sheet, totalRow, "invoice_amount", invoiceCol)
	return section
}

func auditWorkbookTotalRowSum(f *excelize.File, name, sheet string, settlementCols, cashCols, invoiceCols []string) auditSection {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return auditSection{Name: name}
	}
	totalRow := findWorkbookTotalRow(rows, firstNonEmptyColumn(settlementCols), firstNonEmptyColumn(cashCols), firstNonEmptyColumn(invoiceCols))
	if totalRow <= 0 {
		return auditSection{Name: name}
	}
	return auditSection{
		Name:    name,
		Rows:    1,
		Amounts: buildSumCellAmounts(f, sheet, totalRow, settlementCols, cashCols, invoiceCols),
	}
}

func buildSumCellAmounts(f *excelize.File, sheet string, row int, settlementCols, cashCols, invoiceCols []string) []auditAmount {
	var amounts []auditAmount
	if hasAnyCellValue(f, sheet, row, settlementCols) {
		amounts = append(amounts, auditAmount{Name: "settlement_amount", Value: roundAuditWorkbookAmount(sumCellFloats(f, sheet, row, settlementCols))})
	}
	if hasAnyCellValue(f, sheet, row, cashCols) {
		amounts = append(amounts, auditAmount{Name: "cash_amount", Value: roundAuditWorkbookAmount(sumCellFloats(f, sheet, row, cashCols))})
	}
	if hasAnyCellValue(f, sheet, row, invoiceCols) {
		amounts = append(amounts, auditAmount{Name: "invoice_amount", Value: roundAuditWorkbookAmount(sumCellFloats(f, sheet, row, invoiceCols))})
	}
	return amounts
}

func appendCellAmount(amounts []auditAmount, f *excelize.File, sheet string, row int, name, col string) []auditAmount {
	value, ok := cellFloatOK(f, sheet, col, row)
	if !ok {
		return amounts
	}
	return append(amounts, auditAmount{Name: name, Value: roundAuditWorkbookAmount(value)})
}

func firstNonEmptyColumn(cols []string) string {
	for _, col := range cols {
		if strings.TrimSpace(col) != "" {
			return strings.TrimSpace(col)
		}
	}
	return ""
}

func sumCellFloats(f *excelize.File, sheet string, row int, cols []string) float64 {
	total := 0.0
	for _, col := range cols {
		total += cellFloat(f, sheet, col, row)
	}
	return total
}

func hasAnyCellValue(f *excelize.File, sheet string, row int, cols []string) bool {
	for _, col := range cols {
		_, ok := cellFloatOK(f, sheet, col, row)
		if ok {
			return true
		}
	}
	return false
}

func findWorkbookTotalRow(rows [][]string, settlementCol, cashCol, invoiceCol string) int {
	bestRow := 0
	bestScore := 0.0
	for idx, row := range rows {
		if idx < 2 {
			continue
		}
		score := 0.0
		for _, col := range []string{settlementCol, cashCol, invoiceCol} {
			colIdx := columnNameToIndex(col)
			if colIdx < 0 {
				continue
			}
			score += parseAuditFloat(cellAt(row, colIdx))
		}
		if score > bestScore {
			bestScore = score
			bestRow = idx + 1
		}
	}
	return bestRow
}

func cellFloat(f *excelize.File, sheet, col string, row int) float64 {
	value, _ := cellFloatOK(f, sheet, col, row)
	return value
}

func cellFloatOK(f *excelize.File, sheet, col string, row int) (float64, bool) {
	if strings.TrimSpace(col) == "" || row <= 0 {
		return 0, false
	}
	value, err := f.GetCellValue(sheet, fmt.Sprintf("%s%d", strings.ToUpper(strings.TrimSpace(col)), row))
	if err != nil {
		return 0, false
	}
	if strings.TrimSpace(value) == "" {
		return 0, false
	}
	return parseAuditFloat(value), true
}

type workbookAuditMergedRange struct {
	StartRow int
	EndRow   int
	StartCol int
	EndCol   int
}

func readWorkbookAuditMergedRanges(f *excelize.File, sheetName string) ([]workbookAuditMergedRange, error) {
	mergedCells, err := f.GetMergeCells(sheetName)
	if err != nil {
		return nil, err
	}
	ranges := make([]workbookAuditMergedRange, 0, len(mergedCells))
	for _, mergedCell := range mergedCells {
		startCol, startRow, err := excelize.CellNameToCoordinates(mergedCell.GetStartAxis())
		if err != nil {
			return nil, err
		}
		endCol, endRow, err := excelize.CellNameToCoordinates(mergedCell.GetEndAxis())
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, workbookAuditMergedRange{
			StartRow: startRow - 1,
			EndRow:   endRow - 1,
			StartCol: startCol - 1,
			EndCol:   endCol - 1,
		})
	}
	return ranges, nil
}

func normalizeWorkbookAuditRows(rows [][]string, mergedRanges []workbookAuditMergedRange, carryColumns ...int) [][]string {
	out := make([][]string, len(rows))
	for rowIdx, row := range rows {
		next := append([]string(nil), row...)
		for _, col := range carryColumns {
			next = ensureWorkbookAuditRowLength(next, col+1)
			if strings.TrimSpace(next[col]) != "" {
				continue
			}
			if carried, ok := mergedWorkbookAuditCellValue(rows, mergedRanges, rowIdx, col); ok {
				next[col] = carried
			}
		}
		out[rowIdx] = next
	}
	return out
}

func mergedWorkbookAuditCellValue(rows [][]string, mergedRanges []workbookAuditMergedRange, rowIdx, colIdx int) (string, bool) {
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
		value := strings.TrimSpace(cellAt(workbookAuditRowAt(rows, mergeRange.StartRow), mergeRange.StartCol))
		if value == "" {
			return "", false
		}
		return value, true
	}
	return "", false
}

func workbookAuditRowAt(rows [][]string, idx int) []string {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	return rows[idx]
}

func ensureWorkbookAuditRowLength(row []string, want int) []string {
	if len(row) >= want {
		return row
	}
	grown := make([]string, want)
	copy(grown, row)
	return grown
}

func columnNameToIndex(col string) int {
	col = strings.ToUpper(strings.TrimSpace(col))
	if col == "" {
		return -1
	}
	idx := 0
	for _, ch := range col {
		if ch < 'A' || ch > 'Z' {
			return -1
		}
		idx = idx*26 + int(ch-'A'+1)
	}
	return idx - 1
}

func hasSheet(f *excelize.File, sheet string) bool {
	for _, name := range f.GetSheetList() {
		if strings.TrimSpace(name) == sheet {
			return true
		}
	}
	return false
}

func auditFundIncomeRows(name string, rows [][]string, targetMonth string) []auditSection {
	var count int
	var settlement, received, invoice float64
	for _, item := range collectFundIncomeAuditRows(rows) {
		if targetMonth != "" && item.Month != targetMonth {
			continue
		}
		count++
		settlement += item.Settlement
		received += item.Cash
		invoice += item.Invoice
	}
	return []auditSection{{Name: name, Rows: count, Amounts: []auditAmount{{Name: "settlement_amount", Value: settlement}, {Name: "cash_amount", Value: received}, {Name: "invoice_amount", Value: invoice}}}}
}

func auditFundIncomeEntity(name string, rows [][]string, entity, targetMonth string) []auditSection {
	var count int
	var settlement, received, invoice float64
	for _, item := range collectFundIncomeAuditRows(rows) {
		if targetMonth != "" && item.Month != targetMonth {
			continue
		}
		if !strings.Contains(item.Customer, entity) {
			continue
		}
		count++
		settlement += item.Settlement
		received += item.Cash
		invoice += item.Invoice
	}
	return []auditSection{{Name: name, Rows: count, Amounts: []auditAmount{{Name: "settlement_amount", Value: settlement}, {Name: "cash_amount", Value: received}, {Name: "invoice_amount", Value: invoice}}}}
}

func auditCostRows(name string, rows [][]string, targetMonth string) []auditSection {
	var count int
	var settlement, paid, invoice float64
	for _, item := range collectCostAuditRows(rows) {
		if targetMonth != "" && item.Month != targetMonth {
			continue
		}
		count++
		settlement += item.Settlement
		paid += item.Cash
		invoice += item.Invoice
	}
	return []auditSection{{Name: name, Rows: count, Amounts: []auditAmount{{Name: "settlement_amount", Value: settlement}, {Name: "cash_amount", Value: paid}, {Name: "invoice_amount", Value: invoice}}}}
}

func auditCostEntity(name string, rows [][]string, entity, targetMonth string) []auditSection {
	var count int
	var settlement, paid, invoice float64
	for _, item := range collectCostAuditRows(rows) {
		if targetMonth != "" && item.Month != targetMonth {
			continue
		}
		if !strings.Contains(item.Customer, entity) {
			continue
		}
		count++
		settlement += item.Settlement
		paid += item.Cash
		invoice += item.Invoice
	}
	return []auditSection{{Name: name, Rows: count, Amounts: []auditAmount{{Name: "settlement_amount", Value: settlement}, {Name: "cash_amount", Value: paid}, {Name: "invoice_amount", Value: invoice}}}}
}

type workbookAuditRow struct {
	Customer   string
	Month      string
	Settlement float64
	Cash       float64
	Invoice    float64
}

func collectFundIncomeAuditRows(rows [][]string) []workbookAuditRow {
	monthCols := detectMonthColumns(rows, 6)
	var out []workbookAuditRow
	for rowIdx, row := range rows {
		if rowIdx < 2 {
			continue
		}
		customer := strings.TrimSpace(cellAt(row, 0))
		if customer == "" || strings.Contains(customer, "客户") {
			continue
		}
		for _, mc := range monthCols {
			settlement := parseAuditFloat(cellAt(row, mc.Index+1))
			invoice := parseAuditFloat(cellAt(row, mc.Index+3))
			received := parseAuditFloat(cellAt(row, mc.Index+4))
			if settlement == 0 && received == 0 && invoice == 0 {
				continue
			}
			out = append(out, workbookAuditRow{
				Customer:   customer,
				Month:      mc.Month,
				Settlement: settlement,
				Cash:       received,
				Invoice:    invoice,
			})
		}
	}
	return out
}

func collectCostAuditRows(rows [][]string) []workbookAuditRow {
	monthCols := detectMonthColumns(rows, 7)
	var out []workbookAuditRow
	for rowIdx, row := range rows {
		if rowIdx < 2 {
			continue
		}
		customer := strings.TrimSpace(cellAt(row, 0))
		if customer == "" || strings.Contains(customer, "供应商") || strings.Contains(customer, "客户") {
			continue
		}
		for _, mc := range monthCols {
			settlement := parseAuditFloat(cellAt(row, mc.Index+1))
			invoice := parseAuditFloat(cellAt(row, mc.Index+3))
			paid := parseAuditFloat(cellAt(row, mc.Index+4))
			if settlement == 0 && paid == 0 && invoice == 0 {
				continue
			}
			out = append(out, workbookAuditRow{
				Customer:   customer,
				Month:      mc.Month,
				Settlement: settlement,
				Cash:       paid,
				Invoice:    invoice,
			})
		}
	}
	return out
}

type auditMonthColumn struct {
	Month string
	Index int
}

func detectMonthColumns(rows [][]string, start int) []auditMonthColumn {
	var year string
	headerLimit := len(rows)
	if headerLimit > 5 {
		headerLimit = 5
	}
	for i := 0; i < headerLimit; i++ {
		for _, value := range rows[i] {
			text := strings.TrimSpace(value)
			if strings.Contains(text, "2026") {
				year = "2026"
			}
		}
	}
	if year == "" {
		year = "2026"
	}
	seen := map[string]bool{}
	var out []auditMonthColumn
	for i := 0; i < headerLimit; i++ {
		for colIdx, value := range rows[i] {
			if colIdx < start {
				continue
			}
			month, ok := parseAuditMonthHeader(value, year)
			if !ok || seen[month] {
				continue
			}
			seen[month] = true
			out = append(out, auditMonthColumn{Month: month, Index: colIdx})
		}
	}
	return out
}

func parseAuditMonthHeader(value, year string) (string, bool) {
	text := strings.TrimSpace(value)
	text = strings.ReplaceAll(text, "月", "")
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	if strings.Contains(text, "2026") {
		year = "2026"
	}
	for _, token := range []string{"1", "2", "3"} {
		if text == token || strings.Contains(text, token+"月") {
			return fmt.Sprintf("%s-%02s", year, token), true
		}
	}
	if strings.Contains(text, "01") {
		return year + "-01", true
	}
	if strings.Contains(text, "02") {
		return year + "-02", true
	}
	if strings.Contains(text, "03") {
		return year + "-03", true
	}
	return "", false
}

func cellAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func parseAuditFloat(raw string) float64 {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, ",", "")
	raw = strings.ReplaceAll(raw, "，", "")
	raw = strings.ReplaceAll(raw, "￥", "")
	raw = strings.ReplaceAll(raw, "元", "")
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

func roundAuditWorkbookAmount(v float64) float64 {
	return math.Round(v*100) / 100
}
