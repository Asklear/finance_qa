package ingest

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	dbschema "financeqa/internal/db"
	"financeqa/internal/parser"
)

type ImportSummary struct {
	FilePath    string `json:"filePath"`
	ReportType  string `json:"reportType"`
	Company     string `json:"company"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
	RecordCount int    `json:"recordCount"`
}

type Importer struct{}

func NewImporter() *Importer {
	return &Importer{}
}

func (i *Importer) ParseFile(path string) (parser.ParseResult, error) {
	return parser.ParseFile(path)
}

func (i *Importer) ImportFile(ctx context.Context, dbPath, filePath string) (ImportSummary, error) {
	result, err := parser.ParseFile(filePath)
	if err != nil {
		return ImportSummary{}, err
	}
	if err := i.ImportParsed(ctx, dbPath, result); err != nil {
		return ImportSummary{}, err
	}
	return ImportSummary{
		FilePath:    filePath,
		ReportType:  result.Metadata.ReportType,
		Company:     result.Metadata.Company,
		PeriodStart: result.Metadata.PeriodStart,
		PeriodEnd:   result.Metadata.PeriodEnd,
		RecordCount: len(result.Data),
	}, nil
}

func (i *Importer) ImportParsed(ctx context.Context, dbPath string, result parser.ParseResult) error {
	if err := dbschema.Bootstrap(ctx, dbPath); err != nil {
		return fmt.Errorf("bootstrap sqlite db: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite db: %w", err)
	}
	defer func() { _ = db.Close() }()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := clearExisting(ctx, tx, result); err != nil {
		return err
	}
	for _, row := range result.Data {
		if err := insertRecord(ctx, tx, result.Metadata.ReportType, row); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}
	return nil
}

func clearExisting(ctx context.Context, tx *sql.Tx, result parser.ParseResult) error {
	company := result.Metadata.Company
	period := result.Metadata.PeriodEnd

	switch result.Metadata.ReportType {
	case "bank_statement":
		_, err := tx.ExecContext(ctx, `DELETE FROM bank_statement WHERE company = ?`, company)
		return err
	case "income_statement":
		// Clear all data for this company + period to avoid duplicates
		_, err := tx.ExecContext(ctx, `DELETE FROM income_statement WHERE company = ? AND period = ?`, company, period)
		return err
	case "balance_sheet":
		_, err := tx.ExecContext(ctx, `DELETE FROM balance_sheet WHERE company = ? AND period = ?`, company, period)
		return err
	case "balance_detail":
		// Clear ALL balance_detail for this company (余额表是全量替换)
		_, err := tx.ExecContext(ctx, `DELETE FROM balance_detail WHERE company = ?`, company)
		return err
	case "journal":
		_, err := tx.ExecContext(ctx, `DELETE FROM journal WHERE company = ?`, company)
		return err
	default:
		return nil
	}
}

func insertRecord(ctx context.Context, tx *sql.Tx, reportType string, row parser.Record) error {
	switch reportType {
	case "bank_statement":
		_, err := tx.ExecContext(ctx, `
INSERT INTO bank_statement
  (company, account_no, account_name, currency, transaction_date, transaction_time, transaction_type, debit_amount, credit_amount, balance, summary, counterparty_name, counterparty_account)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["account_no"], row["account_name"], row["currency"], row["transaction_date"], row["transaction_time"], row["transaction_type"], row["debit_amount"], row["credit_amount"], row["balance"], row["summary"], row["counterparty_name"], row["counterparty_account"])
		return wrapInsertErr(reportType, err)
	case "income_statement":
		_, err := tx.ExecContext(ctx, `
INSERT INTO income_statement
  (company, period, item_name, current_amount, cumulative_amount)
VALUES (?, ?, ?, ?, ?)
`, row["company"], row["period"], row["item_name"], row["current_amount"], row["cumulative_amount"])
		return wrapInsertErr(reportType, err)
	case "balance_sheet":
		_, err := tx.ExecContext(ctx, `
INSERT INTO balance_sheet
  (company, period, account_name, opening_balance, closing_balance)
VALUES (?, ?, ?, ?, ?)
`, row["company"], row["period"], row["account_name"], row["opening_balance"], row["closing_balance"])
		return wrapInsertErr(reportType, err)
	case "balance_detail":
		_, err := tx.ExecContext(ctx, `
INSERT INTO balance_detail
  (company, year, period, account_code, account_name, account_level, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["year"], row["period"], row["account_code"], row["account_name"], row["account_level"], row["opening_debit"], row["opening_credit"], row["current_debit"], row["current_credit"], row["closing_debit"], row["closing_credit"])
		return wrapInsertErr(reportType, err)
	case "journal":
		voucherDate := row["voucher_date"]
		if voucherDate == nil {
			voucherDate = row["date"]
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO journal
  (company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["period"], voucherDate, row["voucher_no"], row["account_code"], row["account_name"], row["summary"], row["direction"], row["amount"], row["debit_amount"], row["credit_amount"], row["counterparty"])
		return wrapInsertErr(reportType, err)
	default:
		return fmt.Errorf("unsupported report type %q", reportType)
	}
}

func wrapInsertErr(reportType string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("insert %s row: %w", reportType, err)
}
