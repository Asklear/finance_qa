package parser

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

func ParseFile(path string) (ParseResult, error) {
	meta, err := ExtractMetadata(path)
	if err != nil {
		return ParseResult{}, err
	}
	return parseFileWithMeta(path, meta)
}

// ParseFileAsType forces parser to read the same file as a specific report type.
// Useful for combined "财报" files where sheet1 is balance_sheet and sheet2 is income_statement.
func ParseFileAsType(path, reportType string) (ParseResult, error) {
	meta, err := ExtractMetadata(path)
	if err != nil {
		return ParseResult{}, err
	}
	meta.ReportType = reportType
	return parseFileWithMeta(path, meta)
}

func parseFileWithMeta(path string, meta FileMetadata) (ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch meta.ReportType {
	case "bank_statement":
		if ext == ".xlsx" {
			rows, err := parseBankStatementXLSX(path, meta)
			if err != nil {
				return ParseResult{}, err
			}
			return ParseResult{Metadata: meta, Data: rows}, nil
		}
	case "income_statement", "balance_sheet", "balance_detail", "journal":
		if ext == ".xls" {
			rows, err := parseLegacyXLSReport(path, meta)
			if err != nil {
				return ParseResult{}, err
			}
			return ParseResult{Metadata: meta, Data: rows}, nil
		}
	}
	return ParseResult{}, fmt.Errorf("unsupported file type %q for %s", meta.ReportType, path)
}

func parseBankStatementXLSX(path string, meta FileMetadata) ([]Record, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read xlsx rows: %w", err)
	}

	records := make([]Record, 0, len(rows))
	for idx, row := range rows {
		if idx < 13 || len(row) < 10 {
			continue
		}
		date := cell(row, 3)
		if date == "" || date == "交易日" {
			continue
		}
		records = append(records, Record{
			"company":              meta.Company,
			"account_no":           cell(row, 0),
			"account_name":         cell(row, 1),
			"currency":             defaultString(cell(row, 2), "人民币"),
			"transaction_date":     date,
			"transaction_time":     cell(row, 4),
			"transaction_type":     cell(row, 6),
			"debit_amount":         parseFloat(cell(row, 7)),
			"credit_amount":        parseFloat(cell(row, 8)),
			"balance":              parseFloat(cell(row, 9)),
			"summary":              cell(row, 10),
			"counterparty_name":    cell(row, 19),
			"counterparty_account": cell(row, 20),
		})
	}
	return records, nil
}

func parseLegacyXLSReport(path string, meta FileMetadata) ([]Record, error) {
	// Kingdee/UFIDA exported .xls files are notoriously malformed (e.g. OLE2 inconsistency).
	// Go's extrame/xls often fails silently by returning empty strings.
	// As requested ("use the most stable way"), we fallback to a small Python script
	// using xlrd which natively handles these quirks on macOS/Linux.

	pythonScript := `
import xlrd, json, sys
try:
    wb = xlrd.open_workbook(sys.argv[1])
    all_sheets = []
    for sheet in wb.sheets():
        sheet_data = []
        for r in range(sheet.nrows):
            row_data = []
            for c in range(sheet.ncols):
                val = sheet.cell_value(r, c)
                if sheet.cell_type(r, c) == xlrd.XL_CELL_DATE:
                    try:
                        dt = xlrd.xldate_as_tuple(val, wb.datemode)
                        val = f'{dt[0]}-{dt[1]:02d}-{dt[2]:02d}'
                    except:
                        pass
                row_data.append(str(val).strip())
            sheet_data.append(row_data)
        all_sheets.append(sheet_data)
    print(json.dumps(all_sheets))
except Exception as e:
    sys.stderr.write(str(e))
    sys.exit(1)
`
	pythonExecs := []string{"/usr/bin/python3", "python3", "python"}
	var cmd *exec.Cmd
	for _, p := range pythonExecs {
		// Verify xlrd is available
		if err := exec.Command(p, "-c", "import xlrd").Run(); err == nil {
			cmd = exec.Command(p, "-c", pythonScript, path)
			break
		}
	}

	if cmd != nil {
		output, err := cmd.Output()
		if err == nil && len(output) > 0 {
			var sheets [][][]string
			outStr := string(output)
			idx := strings.IndexByte(outStr, '[')
			if idx != -1 {
				if jsonErr := json.Unmarshal([]byte(outStr[idx:]), &sheets); jsonErr == nil {
					rows := pickPreferredSheetRows(sheets, meta)
					return parseLegacyRowsByType(rows, meta)
				}
			}
		}
	}

	// Double fallback to Go's methods just in case
	wb, err := xls.Open(path, "utf-8")
	if err != nil {
		rows, xlsxErr := readOOXMLRows(path, preferredSheetIndex(meta))
		if xlsxErr != nil {
			return nil, fmt.Errorf("open xls: %w", err)
		}
		return parseLegacyRowsByType(rows, meta)
	}
	sheet := wb.GetSheet(preferredSheetIndex(meta))
	if sheet == nil {
		sheet = wb.GetSheet(0)
	}
	if sheet == nil {
		return nil, fmt.Errorf("missing first sheet in %s", path)
	}

	rows := make([][]string, 0, sheet.MaxRow+1)
	for i := 0; i <= int(sheet.MaxRow); i++ {
		row := sheet.Row(i)
		if row == nil {
			rows = append(rows, nil)
			continue
		}
		cols := make([]string, row.LastCol())
		for j := 0; j < row.LastCol(); j++ {
			cols[j] = strings.TrimSpace(row.Col(j))
		}
		rows = append(rows, cols)
	}

	return parseLegacyRowsByType(rows, meta)
}

func preferredSheetIndex(meta FileMetadata) int {
	// 财报常见导出中，第2个sheet通常是利润表。
	if meta.ReportType == "income_statement" {
		return 1
	}
	return 0
}

func pickPreferredSheetRows(sheets [][][]string, meta FileMetadata) [][]string {
	if len(sheets) == 0 {
		return nil
	}
	idx := preferredSheetIndex(meta)
	if idx >= 0 && idx < len(sheets) {
		return sheets[idx]
	}
	return sheets[0]
}

func parseLegacyRowsByType(rows [][]string, meta FileMetadata) ([]Record, error) {
	switch meta.ReportType {
	case "income_statement":
		return parseIncomeStatementRows(rows, meta), nil
	case "balance_sheet":
		return parseBalanceSheetRows(rows, meta), nil
	case "balance_detail":
		return parseBalanceDetailRows(rows, meta), nil
	case "journal":
		return parseJournalRows(rows, meta), nil
	default:
		return nil, fmt.Errorf("unsupported legacy report type %q", meta.ReportType)
	}
}

func readOOXMLRows(path string, sheetIndex int) ([][]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	sheet := f.GetSheetName(sheetIndex)
	if sheet == "" {
		sheet = f.GetSheetName(0)
	}
	return f.GetRows(sheet)
}

func parseIncomeStatementRows(rows [][]string, meta FileMetadata) []Record {
	company := extractCompanyFromRows(rows, meta.Company)
	out := []Record{}
	for i := 3; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 3 {
			continue
		}
		name := strings.TrimSpace(cell(row, 0))
		if name == "" || (strings.Contains(name, "项") && strings.Contains(name, "目")) {
			continue
		}
		out = append(out, Record{
			"company":           company,
			"period":            meta.PeriodEnd,
			"item_name":         name,
			"current_amount":    parseFloat(cell(row, 1)),
			"cumulative_amount": parseFloat(cell(row, 2)),
		})
	}
	return out
}

func parseBalanceSheetRows(rows [][]string, meta FileMetadata) []Record {
	company := extractCompanyFromRows(rows, meta.Company)
	out := []Record{}
	for i := 4; i < len(rows); i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}
		leftName := strings.TrimSpace(cell(row, 0))
		if leftName != "" && !strings.HasSuffix(leftName, "：") && !strings.HasSuffix(leftName, ":") {
			out = append(out, Record{
				"company":         company,
				"period":          meta.PeriodEnd,
				"account_name":    leftName,
				"opening_balance": parseFloat(cell(row, 2)),
				"closing_balance": parseFloat(cell(row, 1)),
				"category":        "asset",
			})
		}
		rightName := strings.TrimSpace(cell(row, 3))
		if rightName != "" && !strings.HasSuffix(rightName, "：") && !strings.HasSuffix(rightName, ":") {
			out = append(out, Record{
				"company":         company,
				"period":          meta.PeriodEnd,
				"account_name":    rightName,
				"opening_balance": parseFloat(cell(row, 5)),
				"closing_balance": parseFloat(cell(row, 4)),
				"category":        "liability",
			})
		}
	}
	return out
}

func parseBalanceDetailRows(rows [][]string, meta FileMetadata) []Record {
	company := extractCompanyFromRows(rows, meta.Company)
	out := []Record{}
	openingPeriod := detectBalanceOpeningPeriod(rows, meta.PeriodStart)
	defaultYear, defaultMonth := parsePeriodYYYYMM(meta.PeriodEnd)
	// Row 0 is header; data starts from row 1.
	// Excel columns: 会计年度(0), 会计期间(1), 科目编码(2), 科目名称(3), 外币名称(4),
	//                期初借方(5), 期初贷方(6), 本期借方(7), 本期贷方(8), 期末借方(9), 期末贷方(10)
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 11 {
			continue
		}
		yearStr := strings.TrimSpace(cell(row, 0))
		year, _ := strconv.Atoi(yearStr)
		monthStr := strings.TrimSpace(cell(row, 1))
		month, err := strconv.Atoi(monthStr)
		if err != nil {
			_, end := parsePeriodRangeText(monthStr)
			_, month = parsePeriodYYYYMM(end)
		}
		if year == 0 {
			year = defaultYear
		}
		if month == 0 {
			month = defaultMonth
		}
		if year == 0 || month == 0 {
			continue
		}
		code := strings.TrimSpace(cell(row, 2))
		name := strings.TrimSpace(cell(row, 3))
		if code == "" || name == "" {
			continue
		}
		// Skip summary rows (合计, 小计)
		if code == "合计" || strings.HasSuffix(code, "小计") {
			continue
		}
		level := accountLevel(code)
		out = append(out, Record{
			"company":        company,
			"year":           year,
			"period":         fmt.Sprintf("%d-%02d", year, month),
			"opening_period": openingPeriod,
			"account_code":   code,
			"account_name":   name,
			"account_level":  level,
			"opening_debit":  parseFloat(cell(row, 5)),
			"opening_credit": parseFloat(cell(row, 6)),
			"current_debit":  parseFloat(cell(row, 7)),
			"current_credit": parseFloat(cell(row, 8)),
			"closing_debit":  parseFloat(cell(row, 9)),
			"closing_credit": parseFloat(cell(row, 10)),
		})
	}
	if openingPeriod == "" {
		openingPeriod = minPeriodFromBalanceRows(out)
		for idx := range out {
			out[idx]["opening_period"] = openingPeriod
		}
	}
	return out
}

var balancePeriodRangePattern = regexp.MustCompile(`(?i)(\d{4})[.\-/年](\d{1,2})\s*[-~至]\s*(\d{4})?[.\-/年]?(\d{1,2})`)

func detectBalanceOpeningPeriod(rows [][]string, periodStart string) string {
	if p := normalizePeriod(periodStart); p != "" {
		return p
	}
	for _, row := range rows {
		candidate := strings.TrimSpace(cell(row, 1))
		if candidate == "" {
			continue
		}
		start, _ := parsePeriodRangeText(candidate)
		if start != "" {
			return start
		}
	}
	return ""
}

func parsePeriodRangeText(v string) (string, string) {
	m := balancePeriodRangePattern.FindStringSubmatch(strings.TrimSpace(v))
	if len(m) != 5 {
		return "", ""
	}
	startYear, _ := strconv.Atoi(m[1])
	startMonth, _ := strconv.Atoi(m[2])
	endYear := startYear
	if strings.TrimSpace(m[3]) != "" {
		endYear, _ = strconv.Atoi(m[3])
	}
	endMonth, _ := strconv.Atoi(m[4])
	if startYear == 0 || startMonth == 0 || endYear == 0 || endMonth == 0 {
		return "", ""
	}
	return fmt.Sprintf("%04d-%02d", startYear, startMonth), fmt.Sprintf("%04d-%02d", endYear, endMonth)
}

func parsePeriodYYYYMM(v string) (int, int) {
	n := normalizePeriod(v)
	if n == "" {
		return 0, 0
	}
	parts := strings.Split(n, "-")
	if len(parts) != 2 {
		return 0, 0
	}
	year, _ := strconv.Atoi(parts[0])
	month, _ := strconv.Atoi(parts[1])
	return year, month
}

func normalizePeriod(v string) string {
	s := strings.TrimSpace(v)
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "/", "-")
	if len(s) == 6 && strings.IndexByte(s, '-') == -1 {
		return s[:4] + "-" + s[4:]
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return ""
	}
	year, err1 := strconv.Atoi(parts[0])
	month, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || year <= 0 || month < 1 || month > 12 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d", year, month)
}

func minPeriodFromBalanceRows(rows []Record) string {
	minPeriod := ""
	for _, row := range rows {
		p := normalizePeriod(fmt.Sprintf("%v", row["period"]))
		if p == "" {
			continue
		}
		if minPeriod == "" || p < minPeriod {
			minPeriod = p
		}
	}
	return minPeriod
}

func parseJournalRows(rows [][]string, meta FileMetadata) []Record {
	company := extractCompanyFromRows(rows, meta.Company)
	out := []Record{}
	// Excel columns: 日期(0), 凭证号数(1), 科目编码(2), 科目名称(3), 摘要(4),
	//                方向(5), 数量(6), 外币(7), 金额(8)
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 9 {
			continue
		}
		date := strings.ReplaceAll(strings.TrimSpace(cell(row, 0)), ".", "-")
		code := strings.TrimSpace(cell(row, 2))
		name := strings.TrimSpace(cell(row, 3))
		if date == "" || code == "" || name == "" {
			continue
		}
		period := periodFromDate(date)
		amount := parseFloat(cell(row, 8))
		direction := strings.TrimSpace(cell(row, 5))
		var debitAmt, creditAmt float64
		if direction == "借" {
			debitAmt = amount
		} else {
			creditAmt = amount
		}
		// Clean up \r\n in summary field
		summary := strings.TrimSpace(cell(row, 4))
		summary = strings.ReplaceAll(summary, "\r\n", "")
		summary = strings.ReplaceAll(summary, "\r", "")
		summary = strings.ReplaceAll(summary, "\n", "")
		out = append(out, Record{
			"company":       company,
			"date":          date,
			"period":        period,
			"voucher_no":    cell(row, 1),
			"account_code":  code,
			"account_name":  name,
			"summary":       summary,
			"direction":     direction,
			"amount":        amount,
			"debit_amount":  debitAmt,
			"credit_amount": creditAmt,
		})
	}
	return out
}

func periodFromDate(v string) string {
	s := strings.TrimSpace(v)
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "/", "-")
	if len(s) >= 7 {
		candidate := normalizePeriod(s[:7])
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func extractCompanyFromRows(rows [][]string, fallback string) string {
	raw := fallback
	for _, idx := range []int{1, 2} {
		if idx >= len(rows) {
			continue
		}
		first := cell(rows[idx], 0)
		for _, prefix := range []string{"单位名称:", "单位名称："} {
			if strings.Contains(first, prefix) {
				raw = strings.TrimSpace(strings.TrimPrefix(first, prefix))
				break
			}
		}
	}
	return sanitizeCompanyName(raw)
}

func cell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func parseFloat(v string) float64 {
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
	if err != nil {
		return 0
	}
	return f
}

// accountLevel determines the account hierarchy level based on code length.
// Chinese accounting standard: 4-digit = level 1, 6-digit = level 2, 8-digit = level 3.
func accountLevel(code string) int {
	switch {
	case len(code) <= 4:
		return 1
	case len(code) <= 6:
		return 2
	default:
		return 3
	}
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
