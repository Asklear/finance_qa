package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"financeqa/internal/accounting"
	dbpkg "financeqa/internal/db"
)

type ProfitCashBridge struct {
	Company                       string            `json:"company"`
	Period                        string            `json:"period"`
	NetProfit                     float64           `json:"net_profit"`
	Depreciation                  float64           `json:"depreciation"`
	FixedAssetPurchasePrincipal   float64           `json:"fixed_asset_purchase_principal"`
	ARIncrease                    float64           `json:"ar_increase"`
	PrepaymentIncrease            float64           `json:"prepayment_increase"`
	OtherReceivableIncrease       float64           `json:"other_receivable_increase"`
	OtherPayableIncrease          float64           `json:"other_payable_increase"`
	APIncrease                    float64           `json:"ap_increase"`
	AdvanceReceiptIncrease        float64           `json:"advance_receipt_increase"`
	PayrollIncrease               float64           `json:"payroll_increase"`
	TaxBalanceIncrease            float64           `json:"tax_balance_increase"`
	TaxTimingAdjustment           float64           `json:"tax_timing_adjustment"`
	EstimatedOperatingCash        float64           `json:"estimated_operating_cash"`
	AdjustedOperatingCashEstimate float64           `json:"adjusted_operating_cash_estimate"`
	OperatingCashIn               float64           `json:"operating_cash_in"`
	OperatingCashOut              float64           `json:"operating_cash_out"`
	OperatingCashNet              float64           `json:"operating_cash_net"`
	NonOperatingCashIn            float64           `json:"non_operating_cash_in"`
	NonOperatingCashOut           float64           `json:"non_operating_cash_out"`
	NonOperatingCashNet           float64           `json:"non_operating_cash_net"`
	MixedCashIn                   float64           `json:"mixed_cash_in"`
	MixedCashOut                  float64           `json:"mixed_cash_out"`
	MixedCashNet                  float64           `json:"mixed_cash_net"`
	BankNetCash                   float64           `json:"bank_net_cash"`
	ExcludedCashNet               float64           `json:"excluded_cash_net"`
	OperatingCashGap              float64           `json:"operating_cash_gap"`
	AdjustedOperatingCashGap      float64           `json:"adjusted_operating_cash_gap"`
	NonOperatingCashDelta         float64           `json:"non_operating_cash_delta"`
	BankCashGap                   float64           `json:"bank_cash_gap"`
	AdjustedBankCashGap           float64           `json:"adjusted_bank_cash_gap"`
	DeltaSources                  map[string]string `json:"delta_sources,omitempty"`
}

func AnalyzeProfitCashBridge(dbPath, company, period string) (ProfitCashBridge, error) {
	db, err := dbpkg.Open(context.Background(), dbPath)
	if err != nil {
		return ProfitCashBridge{}, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()
	return AnalyzeProfitCashBridgeWithDB(context.Background(), db, company, period)
}

func AnalyzeProfitCashBridgeWithDB(ctx context.Context, db *sql.DB, company, period string) (ProfitCashBridge, error) {
	bridge := ProfitCashBridge{
		Company:      company,
		Period:       period,
		DeltaSources: make(map[string]string),
	}
	if db == nil {
		return bridge, fmt.Errorf("db not available")
	}

	netProfit, err := loadBridgeNetProfit(ctx, db, company, period)
	if err != nil {
		return bridge, err
	}
	depreciation, depreciationSource, err := loadPeriodDelta(ctx, db, company, period, "1602", "累计折旧", creditNormal)
	if err != nil {
		return bridge, err
	}
	fixedAssetPurchasePrincipal, fixedAssetSource, err := loadPeriodDelta(ctx, db, company, period, "1601", "固定资产", debitNormal)
	if err != nil {
		return bridge, err
	}
	arIncrease, arSource, err := loadPeriodDelta(ctx, db, company, period, "1122", "应收账款", debitNormal)
	if err != nil {
		return bridge, err
	}
	prepaymentIncrease, prepaymentSource, err := loadPeriodDelta(ctx, db, company, period, "1123", "预付账款", debitNormal)
	if err != nil {
		return bridge, err
	}
	otherReceivableIncrease, otherReceivableSource, err := loadPeriodDelta(ctx, db, company, period, "1221", "其他应收款", debitNormal)
	if err != nil {
		return bridge, err
	}
	otherPayableIncrease, otherPayableSource, err := loadPeriodDelta(ctx, db, company, period, "2241", "其他应付款", creditNormal)
	if err != nil {
		return bridge, err
	}
	apIncrease, apSource, err := loadPeriodDelta(ctx, db, company, period, "2202", "应付账款", creditNormal)
	if err != nil {
		return bridge, err
	}
	advanceReceiptIncrease, advanceReceiptSource, err := loadPeriodDelta(ctx, db, company, period, "2203", "预收账款", creditNormal)
	if err != nil {
		return bridge, err
	}
	payrollIncrease, payrollSource, err := loadPeriodDelta(ctx, db, company, period, "2211", "应付职工薪酬", creditNormal)
	if err != nil {
		return bridge, err
	}
	taxBalanceIncrease, taxBalanceSource, err := loadPeriodDelta(ctx, db, company, period, "2221", "应交税费", creditNormal)
	if err != nil {
		return bridge, err
	}
	taxTimingAdjustment, err := loadVATTimingAdjustment(ctx, db, company, period)
	if err != nil {
		return bridge, err
	}
	operatingCash, err := loadOperatingCashFilter(ctx, db, company, period)
	if err != nil {
		return bridge, err
	}
	bankNetCash, err := loadBankNetCash(ctx, db, company, period)
	if err != nil {
		return bridge, err
	}

	// 老板对账口径优先还原“净现金流”，要把经营性往来、税费留抵和固定资产购置都纳入桥接，
	// 避免把利润桥只算成“过滤后的经营现金”。
	estimated := netProfit + depreciation - arIncrease - prepaymentIncrease - otherReceivableIncrease + apIncrease + advanceReceiptIncrease + payrollIncrease + taxBalanceIncrease - fixedAssetPurchasePrincipal
	bridge.NetProfit = round2(netProfit)
	bridge.Depreciation = round2(depreciation)
	bridge.FixedAssetPurchasePrincipal = round2(fixedAssetPurchasePrincipal)
	bridge.ARIncrease = round2(arIncrease)
	bridge.PrepaymentIncrease = round2(prepaymentIncrease)
	bridge.OtherReceivableIncrease = round2(otherReceivableIncrease)
	bridge.OtherPayableIncrease = round2(otherPayableIncrease)
	bridge.APIncrease = round2(apIncrease)
	bridge.AdvanceReceiptIncrease = round2(advanceReceiptIncrease)
	bridge.PayrollIncrease = round2(payrollIncrease)
	bridge.TaxBalanceIncrease = round2(taxBalanceIncrease)
	bridge.TaxTimingAdjustment = round2(taxTimingAdjustment)
	bridge.EstimatedOperatingCash = round2(estimated)
	bridge.AdjustedOperatingCashEstimate = round2(estimated + taxTimingAdjustment)
	bridge.OperatingCashIn = round2(operatingCash.OperatingIn)
	bridge.OperatingCashOut = round2(operatingCash.OperatingOut)
	bridge.OperatingCashNet = round2(operatingCash.OperatingIn - operatingCash.OperatingOut)
	bridge.NonOperatingCashIn = round2(operatingCash.NonOperatingIn)
	bridge.NonOperatingCashOut = round2(operatingCash.NonOperatingOut)
	bridge.NonOperatingCashNet = round2(operatingCash.NonOperatingIn - operatingCash.NonOperatingOut)
	bridge.MixedCashIn = round2(operatingCash.MixedIn)
	bridge.MixedCashOut = round2(operatingCash.MixedOut)
	bridge.MixedCashNet = round2(operatingCash.MixedIn - operatingCash.MixedOut)
	bridge.BankNetCash = round2(bankNetCash)
	bridge.ExcludedCashNet = round2(bridge.NonOperatingCashNet + bridge.MixedCashNet)
	bridge.OperatingCashGap = round2(bridge.OperatingCashNet - estimated)
	bridge.AdjustedOperatingCashGap = round2(bridge.OperatingCashNet - bridge.AdjustedOperatingCashEstimate)
	bridge.BankCashGap = round2(bridge.BankNetCash - bridge.EstimatedOperatingCash)
	bridge.AdjustedBankCashGap = round2(bridge.BankNetCash - bridge.AdjustedOperatingCashEstimate)
	bridge.NonOperatingCashDelta = bridge.OperatingCashGap
	bridge.DeltaSources["depreciation"] = depreciationSource
	bridge.DeltaSources["fixed_asset_purchase_principal"] = fixedAssetSource
	bridge.DeltaSources["ar_increase"] = arSource
	bridge.DeltaSources["prepayment_increase"] = prepaymentSource
	bridge.DeltaSources["other_receivable_increase"] = otherReceivableSource
	bridge.DeltaSources["other_payable_increase"] = otherPayableSource
	bridge.DeltaSources["ap_increase"] = apSource
	bridge.DeltaSources["advance_receipt_increase"] = advanceReceiptSource
	bridge.DeltaSources["payroll_increase"] = payrollSource
	bridge.DeltaSources["tax_balance_increase"] = taxBalanceSource
	bridge.DeltaSources["tax_timing_adjustment"] = "vat_balance_delta"
	return bridge, nil
}

type accountNormal int

const (
	debitNormal accountNormal = iota + 1
	creditNormal
)

func loadBridgeNetProfit(ctx context.Context, db *sql.DB, company, period string) (float64, error) {
	var profit sql.NullFloat64
	err := db.QueryRowContext(ctx, `
SELECT current_amount
FROM income_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND item_name LIKE '%净利润%'
LIMIT 1
`, company, company, period).Scan(&profit)
	if err == nil && profit.Valid {
		return round2(profit.Float64), nil
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("query net profit: %w", err)
	}

	year, month := 0, 0
	if _, scanErr := fmt.Sscanf(period, "%d-%d", &year, &month); scanErr != nil {
		return 0, fmt.Errorf("parse period %q: %w", period, scanErr)
	}
	metrics, calcErr := accounting.NewCalculator(db).ComputeMonthlyFromJournal(company, year, month)
	if calcErr != nil {
		return 0, fmt.Errorf("fallback net profit from journal: %w", calcErr)
	}
	return round2(metrics.Profit), nil
}

func loadPeriodDelta(ctx context.Context, db *sql.DB, company, period, rootCode, accountName string, normal accountNormal) (float64, string, error) {
	current, currentFound, err := loadBalanceNet(ctx, db, company, period, rootCode, accountName, normal, balanceNetClosing)
	if err != nil {
		return 0, "", err
	}
	opening, openingFound, err := loadBalanceNet(ctx, db, company, period, rootCode, accountName, normal, balanceNetOpening)
	if err != nil {
		return 0, "", err
	}
	openingPeriodReady, err := balanceDetailOpeningPeriodAvailable(ctx, db, company, period, rootCode, accountName)
	if err != nil {
		return 0, "", err
	}
	if currentFound && openingFound && openingPeriodReady {
		return round2(current - opening), "opening_balance", nil
	}
	prevPeriod, err := previousPeriod(period)
	if err == nil {
		previous, previousFound, loadErr := loadBalanceNet(ctx, db, company, prevPeriod, rootCode, accountName, normal, balanceNetClosing)
		if loadErr != nil {
			return 0, "", loadErr
		}
		if currentFound && previousFound {
			return round2(current - previous), "previous_period", nil
		}
	}
	if currentFound {
		return round2(current), "closing_balance_only", nil
	}
	return 0, "no_data", nil
}

type balanceNetKind int

const (
	balanceNetOpening balanceNetKind = iota + 1
	balanceNetClosing
)

func loadBalanceNet(ctx context.Context, db *sql.DB, company, period, rootCode, accountName string, normal accountNormal, kind balanceNetKind) (float64, bool, error) {
	debitColumn := "closing_debit"
	creditColumn := "closing_credit"
	columnLabel := "closing"
	if kind == balanceNetOpening {
		debitColumn = "opening_debit"
		creditColumn = "opening_credit"
		columnLabel = "opening"
	}
	var closingDebit, closingCredit sql.NullFloat64
	var found int
	err := db.QueryRowContext(ctx, balanceNetQuery(debitColumn, creditColumn, "1", "AND account_code = ?\nLIMIT 1"), company, company, period, rootCode).Scan(&closingDebit, &closingCredit, &found)
	if err != nil && err != sql.ErrNoRows {
		return 0, false, fmt.Errorf("query balance_detail %s %s: %w", rootCode, columnLabel, err)
	}
	if found == 0 {
		err = db.QueryRowContext(ctx, balanceNetQuery("SUM("+debitColumn+")", "SUM("+creditColumn+")", "COUNT(1)", "AND account_code LIKE ?"), company, company, period, rootCode+"%").Scan(&closingDebit, &closingCredit, &found)
		if err != nil {
			return 0, false, fmt.Errorf("query balance_detail by prefix %s %s: %w", rootCode, columnLabel, err)
		}
	}
	if found == 0 {
		err = db.QueryRowContext(ctx, balanceNetQuery("SUM("+debitColumn+")", "SUM("+creditColumn+")", "COUNT(1)", "AND account_name = ?"), company, company, period, accountName).Scan(&closingDebit, &closingCredit, &found)
		if err != nil {
			return 0, false, fmt.Errorf("query balance_detail by name %s %s: %w", accountName, columnLabel, err)
		}
	}
	if found == 0 {
		return 0, false, nil
	}
	switch normal {
	case creditNormal:
		return round2(closingCredit.Float64 - closingDebit.Float64), true, nil
	default:
		return round2(closingDebit.Float64 - closingCredit.Float64), true, nil
	}
}

func balanceNetQuery(debitExpr, creditExpr, foundExpr, selector string) string {
	return strings.Join([]string{
		"SELECT COALESCE(" + debitExpr + ", 0), COALESCE(" + creditExpr + ", 0), " + foundExpr,
		"FROM balance_detail",
		"WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')",
		"  AND period = ?",
		"  " + selector,
	}, "\n")
}

func balanceDetailOpeningPeriodAvailable(ctx context.Context, db *sql.DB, company, period, rootCode, accountName string) (bool, error) {
	hasColumn, err := balanceDetailHasOpeningPeriod(ctx, db)
	if err != nil || !hasColumn {
		return false, err
	}
	selectors := []struct {
		clause string
		arg    string
	}{
		{clause: "AND account_code = ?", arg: rootCode},
		{clause: "AND account_code LIKE ?", arg: rootCode + "%"},
		{clause: "AND account_name = ?", arg: accountName},
	}
	for _, selector := range selectors {
		available, found, err := queryOpeningPeriodAvailability(ctx, db, company, period, selector.clause, selector.arg)
		if err != nil {
			return false, err
		}
		if found {
			return available, nil
		}
	}
	return false, nil
}

func balanceDetailHasOpeningPeriod(ctx context.Context, db *sql.DB) (bool, error) {
	queries := []string{
		`SELECT COUNT(1) FROM information_schema.columns WHERE table_name IN ('balance_detail', 'fin_balance_detail') AND column_name = 'opening_period'`,
		`SELECT COUNT(1) FROM pragma_table_info('balance_detail') WHERE name = 'opening_period'`,
	}
	var lastErr error
	for _, query := range queries {
		var count int
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			lastErr = err
			continue
		}
		return count > 0, nil
	}
	if lastErr != nil {
		return false, fmt.Errorf("detect balance_detail opening_period column: %w", lastErr)
	}
	return false, nil
}

func queryOpeningPeriodAvailability(ctx context.Context, db *sql.DB, company, period, selector, arg string) (bool, bool, error) {
	var totalCount, usableCount int
	err := db.QueryRowContext(ctx, openingPeriodAvailabilityQuery(selector), company, company, period, arg).Scan(&totalCount, &usableCount)
	if err != nil {
		return false, false, fmt.Errorf("query opening_period availability: %w", err)
	}
	if totalCount == 0 {
		return false, false, nil
	}
	return usableCount > 0, true, nil
}

func openingPeriodAvailabilityQuery(selector string) string {
	return strings.Join([]string{
		"SELECT COUNT(1),",
		"       COALESCE(SUM(CASE WHEN opening_period IS NOT NULL AND TRIM(opening_period) <> '' THEN 1 ELSE 0 END), 0)",
		"FROM balance_detail",
		"WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')",
		"  AND period = ?",
		"  " + selector,
	}, "\n")
}

func loadVATTimingAdjustment(ctx context.Context, db *sql.DB, company, period string) (float64, error) {
	inputVATIncrease, _, err := loadPeriodDelta(ctx, db, company, period, "22210101", "进项税额", debitNormal)
	if err != nil {
		return 0, err
	}
	outputVATIncrease, _, err := loadPeriodDelta(ctx, db, company, period, "22210106", "销项税额", creditNormal)
	if err != nil {
		return 0, err
	}
	return round2(inputVATIncrease - outputVATIncrease), nil
}

func loadBankNetCash(ctx context.Context, db *sql.DB, company, period string) (float64, error) {
	var income, expense float64
	err := db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(COALESCE(credit_amount, 0)), 0), COALESCE(SUM(COALESCE(debit_amount, 0)), 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
`, company, company, period+"-01", lastDayOfMonth(period)).Scan(&income, &expense)
	if err != nil {
		return 0, fmt.Errorf("query bank net cash: %w", err)
	}
	return round2(income - expense), nil
}

func lastDayOfMonth(period string) string {
	switch period {
	case "":
		return ""
	}
	var year, month int
	_, _ = fmt.Sscanf(period, "%d-%d", &year, &month)
	if month == 12 {
		return fmt.Sprintf("%04d-12-31", year)
	}
	nextMonth := month + 1
	nextYear := year
	if nextMonth == 13 {
		nextMonth = 1
		nextYear++
	}
	t := fmt.Sprintf("%04d-%02d-01", nextYear, nextMonth)
	return periodEndDateString(t)
}

func periodEndDateString(nextMonthFirstDay string) string {
	var year, month, day int
	_, _ = fmt.Sscanf(nextMonthFirstDay, "%d-%d-%d", &year, &month, &day)
	month--
	if month == 0 {
		month = 12
		year--
	}
	days := 31
	switch month {
	case 4, 6, 9, 11:
		days = 30
	case 2:
		days = 28
		if year%400 == 0 || (year%4 == 0 && year%100 != 0) {
			days = 29
		}
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, days)
}

type operatingCashSummary struct {
	OperatingIn     float64
	OperatingOut    float64
	NonOperatingIn  float64
	NonOperatingOut float64
	MixedIn         float64
	MixedOut        float64
}

type voucherCashRow struct {
	VoucherDate string
	VoucherNo   string
	AccountCode string
	Debit       float64
	Credit      float64
}

type voucherCashState struct {
	operating    bool
	nonOperating bool
	bankIn       float64
	bankOut      float64
	roots        map[string]float64
}

func loadOperatingCashFilter(ctx context.Context, db *sql.DB, company, period string) (operatingCashSummary, error) {
	rows, err := db.QueryContext(ctx, `
SELECT voucher_date, IFNULL(voucher_no, ''), account_code, COALESCE(debit_amount, 0), COALESCE(credit_amount, 0)
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
ORDER BY `+voucherCashOrderByClause()+`
`, company, company, period)
	if err != nil {
		return operatingCashSummary{}, fmt.Errorf("query journal cash vouchers: %w", err)
	}
	defer rows.Close()

	vouchers := make(map[string]*voucherCashState)
	for rows.Next() {
		var row voucherCashRow
		if err := rows.Scan(&row.VoucherDate, &row.VoucherNo, &row.AccountCode, &row.Debit, &row.Credit); err != nil {
			return operatingCashSummary{}, fmt.Errorf("scan journal cash voucher: %w", err)
		}
		key := strings.Join([]string{row.VoucherDate, row.VoucherNo}, "\x1f")
		state := vouchers[key]
		if state == nil {
			state = &voucherCashState{roots: make(map[string]float64)}
			vouchers[key] = state
		}
		if strings.HasPrefix(row.AccountCode, "1001") || strings.HasPrefix(row.AccountCode, "1002") {
			state.bankIn += row.Debit
			state.bankOut += row.Credit
			continue
		}
		root := row.AccountCode
		if len(root) > 4 {
			root = root[:4]
		}
		state.roots[root] += row.Debit + row.Credit
		if isOperatingCashRoot(root) {
			state.operating = true
		}
		if isNonOperatingCashRoot(root) {
			state.nonOperating = true
		}
	}
	if err := rows.Err(); err != nil {
		return operatingCashSummary{}, fmt.Errorf("iterate journal cash vouchers: %w", err)
	}

	var summary operatingCashSummary
	for _, state := range vouchers {
		if state.bankIn == 0 && state.bankOut == 0 {
			continue
		}
		if shouldTreatMixedPayrollCashAsOperating(state) {
			summary.OperatingIn += state.bankIn
			summary.OperatingOut += state.bankOut
			continue
		}
		switch {
		case state.operating && !state.nonOperating:
			summary.OperatingIn += state.bankIn
			summary.OperatingOut += state.bankOut
		case state.nonOperating && !state.operating:
			summary.NonOperatingIn += state.bankIn
			summary.NonOperatingOut += state.bankOut
		default:
			summary.MixedIn += state.bankIn
			summary.MixedOut += state.bankOut
		}
	}
	return summary, nil
}

func voucherCashOrderByClause() string {
	return strings.Join([]string{
		"voucher_date",
		"COALESCE(NULLIF(TRIM(voucher_no), ''), '')",
		"account_code",
		"COALESCE(debit_amount, 0)",
		"COALESCE(credit_amount, 0)",
	}, ", ")
}

func shouldTreatMixedPayrollCashAsOperating(state *voucherCashState) bool {
	if state == nil || state.bankOut == 0 || !state.operating || !state.nonOperating {
		return false
	}
	hasPayrollRoot := false
	for root := range state.roots {
		switch root {
		case "2211", "6602", "6603":
			hasPayrollRoot = true
		case "1221", "2221":
			// 员工代扣代缴/个税通常与工资、社保、公积金同单据结算，现金仍属于经营性人力支出。
		default:
			return false
		}
	}
	return hasPayrollRoot
}

func isOperatingCashRoot(root string) bool {
	switch root {
	case "1122", "1123", "2202", "2203", "2211", "2221", "6401", "6601", "6602", "6603", "6001", "6051":
		return true
	default:
		return false
	}
}

func isNonOperatingCashRoot(root string) bool {
	switch root {
	case "1221", "1601", "1606":
		return true
	default:
		return false
	}
}
