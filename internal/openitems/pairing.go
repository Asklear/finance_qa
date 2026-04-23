package openitems

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

type AccountKind string

const (
	Receivable AccountKind = "receivable"
	Payable    AccountKind = "payable"
)

type Options struct {
	Company           string
	Period            string
	AccountCodePrefix string
	Kind              AccountKind
	Counterparty      string
}

type OpenItem struct {
	Counterparty string  `json:"counterparty"`
	SourceDate   string  `json:"source_date"`
	VoucherNo    string  `json:"voucher_no"`
	Amount       float64 `json:"amount"`
	AgeDays      int     `json:"age_days"`
}

type CounterpartySummary struct {
	Counterparty            string               `json:"counterparty"`
	OpeningBalance          float64              `json:"opening_balance"`
	CurrentIncrease         float64              `json:"current_increase"`
	CurrentDecrease         float64              `json:"current_decrease"`
	OfficialOpeningBalance  float64              `json:"official_opening_balance"`
	OfficialCurrentIncrease float64              `json:"official_current_increase"`
	OfficialCurrentDecrease float64              `json:"official_current_decrease"`
	OfficialClosingBalance  float64              `json:"official_closing_balance"`
	OpenItemOpeningBalance  float64              `json:"open_item_opening_balance"`
	OpenItemClosingBalance  float64              `json:"open_item_closing_balance"`
	HistoricalSettlement    float64              `json:"historical_settlement"`
	CurrentSettlement       float64              `json:"current_period_settlement"`
	SettlementConfidence    SettlementConfidence `json:"settlement_confidence"`
	ClosingBalance          float64              `json:"closing_balance"`
	OpenItems               []OpenItem           `json:"open_items"`
}

type Summary struct {
	Company                 string                `json:"company"`
	Period                  string                `json:"period"`
	AccountCodePrefix       string                `json:"account_code_prefix"`
	Kind                    AccountKind           `json:"kind"`
	OpeningBalance          float64               `json:"opening_balance"`
	CurrentIncrease         float64               `json:"current_increase"`
	CurrentDecrease         float64               `json:"current_decrease"`
	OfficialOpeningBalance  float64               `json:"official_opening_balance"`
	OfficialCurrentIncrease float64               `json:"official_current_increase"`
	OfficialCurrentDecrease float64               `json:"official_current_decrease"`
	OfficialClosingBalance  float64               `json:"official_closing_balance"`
	OpenItemOpeningBalance  float64               `json:"open_item_opening_balance"`
	OpenItemClosingBalance  float64               `json:"open_item_closing_balance"`
	HistoricalSettlement    float64               `json:"historical_settlement"`
	CurrentSettlement       float64               `json:"current_period_settlement"`
	SettlementConfidence    SettlementConfidence  `json:"settlement_confidence"`
	ClosingBalance          float64               `json:"closing_balance"`
	CounterpartySummaries   []CounterpartySummary `json:"counterparties"`
	OpenItems               []OpenItem            `json:"open_items"`
	HasData                 bool                  `json:"has_data"`
}

type journalRow struct {
	Period             string
	VoucherDate        string
	AccountCode        string
	VoucherNo          string
	AccountName        string
	Summary            string
	Counterparty       string
	Debit              float64
	Credit             float64
	Date               time.Time
	CounterpartySource CounterpartyEvidenceSource
	IsExplicitOffset   bool
}

type openLayer struct {
	date      time.Time
	dateText  string
	voucherNo string
	amount    float64
	source    CounterpartyEvidenceSource
}

type counterpartyState struct {
	name               string
	queue              []openLayer
	summary            CounterpartySummary
	officialOpeningNet float64
	periodDebitTotal   float64
	periodCreditTotal  float64
}

func BuildSummary(ctx context.Context, db *sql.DB, opts Options) (Summary, error) {
	summary := Summary{
		Company:           opts.Company,
		Period:            opts.Period,
		AccountCodePrefix: opts.AccountCodePrefix,
		Kind:              opts.Kind,
	}
	startDate, endDate, err := monthBounds(opts.Period)
	if err != nil {
		return summary, err
	}

	rows, err := loadJournalRows(ctx, db, opts.Company, opts.AccountCodePrefix, opts.Counterparty, endDate)
	if err != nil {
		return summary, err
	}
	if len(rows) == 0 {
		return summary, nil
	}
	summary.HasData = true

	states := make(map[string]*counterpartyState)
	for _, row := range rows {
		state := ensureState(states, row.Counterparty)
		if row.Date.Before(startDate) {
			state.officialOpeningNet = round2(state.officialOpeningNet + officialSignedDelta(opts.Kind, row.Debit, row.Credit))
			applyHistory(state, row, opts.Kind)
		}
	}
	for _, state := range states {
		state.summary.OfficialOpeningBalance = round2(state.officialOpeningNet)
		state.summary.OpenItemOpeningBalance = round2(queueBalance(state.queue))
		state.summary.OpeningBalance = round2(queueBalance(state.queue))
	}

	for _, row := range rows {
		if row.Date.Before(startDate) || row.Date.After(endDate) {
			continue
		}
		state := ensureState(states, row.Counterparty)
		state.periodDebitTotal = round2(state.periodDebitTotal + row.Debit)
		state.periodCreditTotal = round2(state.periodCreditTotal + row.Credit)
		applyCurrentPeriod(state, row, opts.Kind, startDate)
	}

	counterparties := make([]CounterpartySummary, 0, len(states))
	openItems := make([]OpenItem, 0)
	for _, state := range states {
		officialIncrease, officialDecrease := officialRollforwardSides(opts.Kind, state.periodDebitTotal, state.periodCreditTotal)
		state.summary.OfficialCurrentIncrease = officialIncrease
		state.summary.OfficialCurrentDecrease = officialDecrease
		state.summary.OfficialClosingBalance = round2(state.summary.OfficialOpeningBalance + officialIncrease - officialDecrease)
		state.summary.OpenItemClosingBalance = round2(queueBalance(state.queue))
		state.summary.ClosingBalance = round2(queueBalance(state.queue))
		state.summary.HistoricalSettlement = round2(state.summary.HistoricalSettlement)
		state.summary.CurrentSettlement = round2(state.summary.CurrentSettlement)
		state.summary.SettlementConfidence = roundSettlementConfidence(state.summary.SettlementConfidence)
		state.summary.OpenItems = buildOpenItems(state.name, state.queue, endDate)
		if state.summary.OpeningBalance == 0 &&
			state.summary.CurrentIncrease == 0 &&
			state.summary.CurrentDecrease == 0 &&
			state.summary.ClosingBalance == 0 {
			continue
		}
		counterparties = append(counterparties, state.summary)
		openItems = append(openItems, state.summary.OpenItems...)
		summary.OfficialOpeningBalance += state.summary.OfficialOpeningBalance
		summary.OfficialCurrentIncrease += state.summary.OfficialCurrentIncrease
		summary.OfficialCurrentDecrease += state.summary.OfficialCurrentDecrease
		summary.OfficialClosingBalance += state.summary.OfficialClosingBalance
		summary.OpenItemOpeningBalance += state.summary.OpenItemOpeningBalance
		summary.OpenItemClosingBalance += state.summary.OpenItemClosingBalance
		summary.OpeningBalance += state.summary.OpeningBalance
		summary.CurrentIncrease += state.summary.CurrentIncrease
		summary.CurrentDecrease += state.summary.CurrentDecrease
		summary.HistoricalSettlement += state.summary.HistoricalSettlement
		summary.CurrentSettlement += state.summary.CurrentSettlement
		summary.SettlementConfidence = summary.SettlementConfidence.merge(state.summary.SettlementConfidence)
		summary.ClosingBalance += state.summary.ClosingBalance
	}

	sort.Slice(counterparties, func(i, j int) bool {
		if counterparties[i].ClosingBalance == counterparties[j].ClosingBalance {
			return counterparties[i].Counterparty < counterparties[j].Counterparty
		}
		return counterparties[i].ClosingBalance > counterparties[j].ClosingBalance
	})
	sort.Slice(openItems, func(i, j int) bool {
		if openItems[i].AgeDays == openItems[j].AgeDays {
			if openItems[i].Counterparty == openItems[j].Counterparty {
				return openItems[i].SourceDate < openItems[j].SourceDate
			}
			return openItems[i].Counterparty < openItems[j].Counterparty
		}
		return openItems[i].AgeDays > openItems[j].AgeDays
	})

	summary.CounterpartySummaries = counterparties
	summary.OpenItems = openItems
	summary.OfficialOpeningBalance = round2(summary.OfficialOpeningBalance)
	summary.OfficialCurrentIncrease = round2(summary.OfficialCurrentIncrease)
	summary.OfficialCurrentDecrease = round2(summary.OfficialCurrentDecrease)
	summary.OfficialClosingBalance = round2(summary.OfficialClosingBalance)
	summary.OpenItemOpeningBalance = round2(summary.OpenItemOpeningBalance)
	summary.OpenItemClosingBalance = round2(summary.OpenItemClosingBalance)
	summary.OpeningBalance = round2(summary.OpeningBalance)
	summary.CurrentIncrease = round2(summary.CurrentIncrease)
	summary.CurrentDecrease = round2(summary.CurrentDecrease)
	summary.HistoricalSettlement = round2(summary.HistoricalSettlement)
	summary.CurrentSettlement = round2(summary.CurrentSettlement)
	summary.SettlementConfidence = roundSettlementConfidence(summary.SettlementConfidence)
	summary.ClosingBalance = round2(summary.ClosingBalance)
	return summary, nil
}

func ensureState(states map[string]*counterpartyState, counterparty string) *counterpartyState {
	key := normalizedKey(counterparty)
	if key == "" {
		key = "未识别往来单位"
		counterparty = key
	}
	if state, ok := states[key]; ok {
		return state
	}
	state := &counterpartyState{name: counterparty, summary: CounterpartySummary{Counterparty: counterparty}}
	states[key] = state
	return state
}

func applyHistory(state *counterpartyState, row journalRow, kind AccountKind) {
	increase, decrease := classifyMovement(kind, row.Debit, row.Credit)
	if increase > 0 {
		state.queue = append(state.queue, openLayer{
			date:      row.Date,
			dateText:  row.VoucherDate,
			voucherNo: row.VoucherNo,
			amount:    increase,
			source:    row.CounterpartySource,
		})
	}
	consumeQueue(state, decrease, time.Time{}, row)
}

func applyCurrentPeriod(state *counterpartyState, row journalRow, kind AccountKind, monthStart time.Time) {
	increase, decrease := classifyMovement(kind, row.Debit, row.Credit)
	if increase > 0 {
		state.summary.CurrentIncrease = round2(state.summary.CurrentIncrease + increase)
		state.queue = append(state.queue, openLayer{
			date:      row.Date,
			dateText:  row.VoucherDate,
			voucherNo: row.VoucherNo,
			amount:    increase,
			source:    row.CounterpartySource,
		})
	}
	if decrease > 0 {
		state.summary.CurrentDecrease = round2(state.summary.CurrentDecrease + decrease)
		applied := consumeQueue(state, decrease, monthStart, row)
		applySettlementBucket(&state.summary, applied)
	}
}

func consumeQueue(state *counterpartyState, decrease float64, monthStart time.Time, row journalRow) SettlementConfidence {
	if decrease <= 0 {
		return SettlementConfidence{}
	}
	remaining := round2(decrease)
	applied := SettlementConfidence{}
	for remaining > 0 && len(state.queue) > 0 {
		layer := &state.queue[0]
		consume := round2(min(layer.amount, remaining))
		if consume <= 0 {
			break
		}
		applied = applied.merge(settlementConfidence(layer, row, monthStart, consume))
		layer.amount = round2(layer.amount - consume)
		remaining = round2(remaining - consume)
		if layer.amount <= 0 {
			state.queue = state.queue[1:]
		}
	}
	if !monthStart.IsZero() && remaining > 0 {
		applied.UnmatchedDecrease = round2(applied.UnmatchedDecrease + remaining)
	}
	return roundSettlementConfidence(applied)
}

func settlementConfidence(layer *openLayer, row journalRow, monthStart time.Time, amount float64) SettlementConfidence {
	if monthStart.IsZero() || amount <= 0 {
		return SettlementConfidence{}
	}
	confidence := classifySettlementConfidence(layer.source, row.CounterpartySource, row.IsExplicitOffset)
	historical := layer.date.Before(monthStart)
	applied := SettlementConfidence{}
	switch confidence {
	case MatchConfidenceConfirmed:
		if historical {
			applied.ConfirmedHistoricalSettlement = amount
		} else {
			applied.ConfirmedCurrentSettlement = amount
		}
	case MatchConfidenceProbable:
		if historical {
			applied.ProbableHistoricalSettlement = amount
		} else {
			applied.ProbableCurrentSettlement = amount
		}
	default:
		applied.UnmatchedDecrease = amount
	}
	return roundSettlementConfidence(applied)
}

func classifySettlementConfidence(layerSource, rowSource CounterpartyEvidenceSource, rowIsExplicitOffset bool) MatchConfidence {
	if rowIsExplicitOffset && layerSource != CounterpartyEvidenceUnknown && rowSource != CounterpartyEvidenceUnknown {
		return MatchConfidenceConfirmed
	}
	if layerSource == CounterpartyEvidenceDirect && rowSource == CounterpartyEvidenceDirect {
		return MatchConfidenceConfirmed
	}
	if layerSource == CounterpartyEvidenceUnknown || rowSource == CounterpartyEvidenceUnknown {
		return MatchConfidenceUnmatched
	}
	return MatchConfidenceProbable
}

func applySettlementBucket(summary *CounterpartySummary, applied SettlementConfidence) {
	summary.HistoricalSettlement = round2(summary.HistoricalSettlement + applied.ConfirmedHistoricalSettlement)
	summary.CurrentSettlement = round2(summary.CurrentSettlement + applied.ConfirmedCurrentSettlement)
	summary.SettlementConfidence = summary.SettlementConfidence.merge(applied)
}

func roundSettlementConfidence(v SettlementConfidence) SettlementConfidence {
	return SettlementConfidence{
		ConfirmedHistoricalSettlement: round2(v.ConfirmedHistoricalSettlement),
		ProbableHistoricalSettlement:  round2(v.ProbableHistoricalSettlement),
		ConfirmedCurrentSettlement:    round2(v.ConfirmedCurrentSettlement),
		ProbableCurrentSettlement:     round2(v.ProbableCurrentSettlement),
		UnmatchedDecrease:             round2(v.UnmatchedDecrease),
	}
}

func classifyMovement(kind AccountKind, debit, credit float64) (float64, float64) {
	signed := debit - credit
	switch kind {
	case Payable:
		signed = credit - debit
		if signed >= 0 {
			return round2(signed), 0
		}
		return 0, round2(-signed)
	default:
		if signed >= 0 {
			return round2(signed), 0
		}
		return 0, round2(-signed)
	}
}

func buildOpenItems(counterparty string, queue []openLayer, endDate time.Time) []OpenItem {
	out := make([]OpenItem, 0, len(queue))
	for _, layer := range queue {
		if layer.amount <= 0 {
			continue
		}
		out = append(out, OpenItem{
			Counterparty: counterparty,
			SourceDate:   layer.dateText,
			VoucherNo:    layer.voucherNo,
			Amount:       round2(layer.amount),
			AgeDays:      int(endDate.Sub(layer.date).Hours() / 24),
		})
	}
	return out
}

func officialSignedDelta(kind AccountKind, debit, credit float64) float64 {
	if kind == Payable {
		return round2(credit - debit)
	}
	return round2(debit - credit)
}

func officialRollforwardSides(kind AccountKind, debitTotal, creditTotal float64) (float64, float64) {
	debitNet := round2(debitTotal)
	creditNet := round2(creditTotal)
	if debitNet < 0 {
		creditNet = round2(creditNet + math.Abs(debitNet))
		debitNet = 0
	}
	if creditNet < 0 {
		debitNet = round2(debitNet + math.Abs(creditNet))
		creditNet = 0
	}
	if kind == Payable {
		return creditNet, debitNet
	}
	return debitNet, creditNet
}

func queueBalance(queue []openLayer) float64 {
	total := 0.0
	for _, layer := range queue {
		total += layer.amount
	}
	return total
}

func loadJournalRows(ctx context.Context, db *sql.DB, company, prefix, counterparty string, endDate time.Time) ([]journalRow, error) {
	cols, err := tableColumns(ctx, db, "journal")
	if err != nil {
		return nil, fmt.Errorf("load journal columns: %w", err)
	}
	query := fmt.Sprintf(`
SELECT
  %s AS period,
  voucher_date,
  account_code,
  %s AS voucher_no,
  %s AS account_name,
  %s AS summary,
  %s AS counterparty,
  %s AS debit_amount,
  %s AS credit_amount
FROM journal
WHERE (? LIKE '%%' || company || '%%' OR company LIKE '%%' || ? || '%%')
  AND account_code LIKE ?
  AND DATE(voucher_date) <= DATE(?)
ORDER BY %s
`, textExpr(cols, "period"), textExpr(cols, "voucher_no"), textExpr(cols, "account_name"), textExpr(cols, "summary"), textExpr(cols, "counterparty"), numberExpr(cols, "debit_amount"), numberExpr(cols, "credit_amount"), openItemsJournalOrderByClause())

	rows, err := db.QueryContext(ctx, query, company, company, prefix+"%", endDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("query journal open items: %w", err)
	}
	defer rows.Close()

	out := make([]journalRow, 0)
	contexts := make(map[string]journalRow)
	for rows.Next() {
		var row journalRow
		if err := rows.Scan(&row.Period, &row.VoucherDate, &row.AccountCode, &row.VoucherNo, &row.AccountName, &row.Summary, &row.Counterparty, &row.Debit, &row.Credit); err != nil {
			return nil, fmt.Errorf("scan journal open item row: %w", err)
		}
		row.Counterparty, row.CounterpartySource = normalizeCounterparty(row.Counterparty, row.AccountName, row.Summary)
		row.IsExplicitOffset = isExplicitOffsetRow(row.Summary, row.Debit, row.Credit)
		row.Date = mustParseDate(row.VoucherDate)
		out = append(out, row)
		if row.CounterpartySource == CounterpartyEvidenceUnknown && row.Period != "" && row.VoucherDate != "" && row.VoucherNo != "" {
			contexts[voucherContextKey(row.Period, row.VoucherDate, row.VoucherNo)] = row
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal open item rows: %w", err)
	}
	if len(contexts) > 0 {
		batch := make([]journalRow, 0, len(contexts))
		for _, row := range contexts {
			batch = append(batch, row)
		}
		hints, hintErr := loadVoucherCounterpartyHints(ctx, db, company, batch)
		if hintErr == nil {
			for idx := range out {
				if out[idx].CounterpartySource != CounterpartyEvidenceUnknown {
					continue
				}
				hint := hints[voucherContextKey(out[idx].Period, out[idx].VoucherDate, out[idx].VoucherNo)]
				if hint == "" {
					continue
				}
				out[idx].Counterparty = hint
				out[idx].CounterpartySource = CounterpartyEvidenceSummary
			}
		}
	}
	if counterparty == "" {
		return out, nil
	}
	filtered := make([]journalRow, 0, len(out))
	for _, row := range out {
		if sameCounterparty(row.Counterparty, counterparty) {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func voucherContextKey(period, voucherDate, voucherNo string) string {
	return strings.Join([]string{
		strings.TrimSpace(period),
		strings.TrimSpace(voucherDate),
		strings.TrimSpace(voucherNo),
	}, "\x1f")
}

func loadVoucherCounterpartyHints(ctx context.Context, db *sql.DB, company string, contexts []journalRow) (map[string]string, error) {
	if len(contexts) == 0 {
		return map[string]string{}, nil
	}

	const chunkSize = 80
	hints := map[string]string{}
	for start := 0; start < len(contexts); start += chunkSize {
		end := start + chunkSize
		if end > len(contexts) {
			end = len(contexts)
		}
		chunk := contexts[start:end]

		var clause strings.Builder
		args := make([]any, 0, 2+len(chunk)*3)
		args = append(args, company, company)
		clause.WriteString(`
SELECT IFNULL(period, ''), voucher_date, IFNULL(voucher_no, ''), IFNULL(TRIM(counterparty), ''), IFNULL(TRIM(account_name), ''), IFNULL(TRIM(summary), '')
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND (
`)
		for i, row := range chunk {
			if i > 0 {
				clause.WriteString(" OR ")
			}
			clause.WriteString("(IFNULL(period, '') = ? AND voucher_date = ? AND IFNULL(voucher_no, '') = ?)")
			args = append(args, row.Period, row.VoucherDate, row.VoucherNo)
		}
		clause.WriteString(")")

		rows, err := db.QueryContext(ctx, clause.String(), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var period, voucherDate, voucherNo, rawCounterparty, accountName, summary string
			if scanErr := rows.Scan(&period, &voucherDate, &voucherNo, &rawCounterparty, &accountName, &summary); scanErr != nil {
				continue
			}
			hint, _ := normalizeCounterparty(rawCounterparty, accountName, summary)
			if hint == "" || hint == "未识别往来单位" {
				continue
			}
			key := voucherContextKey(period, voucherDate, voucherNo)
			if _, exists := hints[key]; !exists {
				hints[key] = hint
			}
		}
		_ = rows.Close()
	}
	return hints, nil
}

func tableColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	query, err := journalColumnProbeQuery(table)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	cols := make(map[string]bool, len(names))
	for _, name := range names {
		cols[strings.ToLower(name)] = true
	}
	return cols, rows.Err()
}

var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func journalColumnProbeQuery(table string) (string, error) {
	if !sqlIdentifierPattern.MatchString(table) {
		return "", fmt.Errorf("invalid table name: %s", table)
	}
	return fmt.Sprintf("SELECT * FROM %s LIMIT 0", table), nil
}

func openItemsJournalOrderByClause() string {
	return strings.Join([]string{
		"DATE(voucher_date)",
		"voucher_no",
		"CASE WHEN COALESCE(debit_amount, 0) > 0 AND COALESCE(credit_amount, 0) = 0 THEN 0 WHEN COALESCE(credit_amount, 0) > 0 AND COALESCE(debit_amount, 0) = 0 THEN 1 ELSE 2 END",
		"account_code",
		"account_name",
		"summary",
		"counterparty",
		"COALESCE(debit_amount, 0)",
		"COALESCE(credit_amount, 0)",
	}, ", ")
}

func textExpr(cols map[string]bool, column string) string {
	if cols[strings.ToLower(column)] {
		return fmt.Sprintf("IFNULL(TRIM(%s), '')", column)
	}
	return "''"
}

func numberExpr(cols map[string]bool, column string) string {
	if cols[strings.ToLower(column)] {
		return fmt.Sprintf("COALESCE(%s, 0)", column)
	}
	return "0"
}

func normalizeCounterparty(raw, accountName, summary string) (string, CounterpartyEvidenceSource) {
	candidates := []struct {
		value  string
		source CounterpartyEvidenceSource
	}{
		{value: raw, source: CounterpartyEvidenceDirect},
		{value: extractCompanyName(summary), source: CounterpartyEvidenceSummary},
		{value: extractFromAccountName(accountName), source: CounterpartyEvidenceAccount},
	}
	for _, candidate := range candidates {
		name := cleanEntity(candidate.value)
		if name != "" {
			return name, candidate.source
		}
	}
	return "未识别往来单位", CounterpartyEvidenceUnknown
}

func extractFromAccountName(accountName string) string {
	name := strings.TrimSpace(accountName)
	if name == "" {
		return ""
	}
	separators := []string{"-", "_", "－", "—", ":", "：", "/", "／"}
	for _, sep := range separators {
		if strings.Contains(name, sep) {
			parts := strings.Split(name, sep)
			return parts[len(parts)-1]
		}
	}
	return ""
}

func extractCompanyName(summary string) string {
	text := strings.TrimSpace(summary)
	if text == "" {
		return ""
	}
	candidates := make([]string, 0, 8)
	trimmed := trimEntityNoise(text)
	for _, matched := range companyNamePattern.FindAllString(trimmed, -1) {
		candidates = append(candidates, matched)
	}
	separators := []string{"_", "-", "－", "—", ":", "：", "/", "／", "(", ")", "（", "）"}
	for _, sep := range separators {
		text = strings.ReplaceAll(text, sep, " ")
	}
	for _, part := range strings.Fields(text) {
		part = trimEntityNoise(part)
		part = cleanEntity(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "公司") || strings.Contains(part, "中心") || strings.Contains(part, "事务所") {
			candidates = append(candidates, part)
		}
	}
	best := ""
	bestLen := 0
	for _, candidate := range candidates {
		candidate = cleanEntity(candidate)
		if candidate == "" {
			continue
		}
		candidateLen := len([]rune(normalizedKey(candidate)))
		if candidateLen < 6 {
			continue
		}
		if best == "" || candidateLen < bestLen {
			best = candidate
			bestLen = candidateLen
		}
	}
	return best
}

func isExplicitOffsetRow(summary string, debit, credit float64) bool {
	if debit < 0 || credit < 0 {
		return true
	}
	text := strings.TrimSpace(summary)
	return strings.Contains(text, "冲回") ||
		strings.Contains(text, "冲销") ||
		strings.Contains(text, "红字") ||
		strings.Contains(text, "冲减")
}

func cleanEntity(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	replacements := []string{
		"应收账款", "", "应付账款", "", "其他应收款", "", "其他应付款", "",
		"预付账款", "", "预收账款", "", "单位往来", "", "客户往来", "", "供应商往来", "",
	}
	r := strings.NewReplacer(replacements...)
	name = strings.TrimSpace(r.Replace(name))
	name = strings.Trim(name, "-_：:()（）/ ")
	name = trimEntityNoise(name)
	if name == "" {
		return ""
	}
	if meaninglessEntityPattern.MatchString(name) {
		return ""
	}
	if strings.Contains(name, "回款冲应收") || strings.Contains(name, "付款冲应付") || strings.Contains(name, "历史应收") || strings.Contains(name, "历史应付") {
		return ""
	}
	if matched := companyNamePattern.FindString(name); matched != "" {
		name = trimEntityNoise(strings.TrimSpace(matched))
	}
	return name
}

func normalizedKey(name string) string {
	name = cleanEntity(name)
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "（", "", "）", "", "(", "", ")", "", "-", "", "_", "", ",", "", "，", "", ".", "", "。", "")
	return strings.ToLower(replacer.Replace(strings.TrimSpace(name)))
}

func mustParseDate(v string) time.Time {
	v = strings.TrimSpace(v)
	if len(v) >= 10 {
		v = v[:10]
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}
	}
	return t
}

func monthBounds(period string) (time.Time, time.Time, error) {
	start, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period %q: %w", period, err)
	}
	end := start.AddDate(0, 1, -1)
	return start, end, nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func sameCounterparty(left, right string) bool {
	return normalizedKey(left) != "" && normalizedKey(left) == normalizedKey(right)
}

var companyNamePattern = regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]+?(?:有限责任公司|股份有限公司|有限公司|事务所|中心|分公司|公司)`)
var meaninglessEntityPattern = regexp.MustCompile(`^(?:单位|客户|供应商|个人|员工|银行|公司|摘要|对方|往来单位|未识别往来单位)$`)

func trimEntityNoise(name string) string {
	name = strings.TrimSpace(name)
	prefixes := []string{"收到", "转账", "支付", "付款", "付", "为", "向", "给", "预提成本", "冲销", "冲回", "结转", "购买", "采购", "购入", "购买了", "采购了", "代付", "代收"}
	suffixes := []string{"发票", "服务", "服务费", "转账", "结算款", "款"}
	changed := true
	for changed {
		changed = false
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				name = strings.TrimSpace(strings.TrimPrefix(name, prefix))
				changed = true
			}
		}
		for _, suffix := range suffixes {
			if strings.HasSuffix(name, suffix) {
				name = strings.TrimSpace(strings.TrimSuffix(name, suffix))
				changed = true
			}
		}
	}
	return strings.TrimSpace(name)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
