package query_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestProfitQueryExposesProfitCashBridge(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "profit-bridge-query.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (company TEXT, period TEXT, item_name TEXT, current_amount REAL, cumulative_amount REAL)`,
		`CREATE TABLE balance_detail (company TEXT, year INTEGER, period TEXT, account_code TEXT, account_name TEXT, opening_debit REAL, opening_credit REAL, current_debit REAL, current_credit REAL, closing_debit REAL, closing_credit REAL)`,
		`CREATE TABLE bank_statement (company TEXT, transaction_date TEXT, debit_amount REAL, credit_amount REAL, counterparty_name TEXT, summary TEXT)`,
		`CREATE TABLE journal (company TEXT, period TEXT, voucher_date TEXT, voucher_no TEXT, account_code TEXT, account_name TEXT, summary TEXT, direction TEXT, amount REAL, debit_amount REAL, credit_amount REAL, counterparty TEXT)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','一、营业收入',1000,1000)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','减：营业成本',700,700)`,
		`INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','四、净利润（净亏损以"－"号填列）',100,100)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','1602','累计折旧',0,0,0,10,0,10)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','1602','累计折旧',0,0,0,20,0,20)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','1122','应收账款',80,0,130,30,80,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','1122','应收账款',0,50,220,120,50,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','1123','预付账款',70,0,70,0,70,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','1123','预付账款',30,0,100,70,30,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','1221','其他应收款',10,0,10,0,10,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','1221','其他应收款',50,0,50,0,50,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2241','其他应付款',0,5,0,5,0,5)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2241','其他应付款',0,10,0,10,0,10)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2202','应付账款',0,120,0,120,0,120)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2202','应付账款',0,200,80,280,0,200)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2203','预收账款',0,0,0,0,0,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2203','预收账款',0,20,0,20,0,20)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2211','应付职工薪酬',0,15,0,15,0,15)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2211','应付职工薪酬',0,40,10,50,0,40)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2221','应交税费',0,3,0,3,0,3)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2221','应交税费',0,8,0,8,0,8)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','22210101','进项税额',20,0,20,0,20,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','22210101','进项税额',35,0,35,0,35,0)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','22210106','销项税额',0,10,0,10,0,10)`,
		`INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','22210106','销项税额',0,20,0,20,0,20)`,
		`INSERT INTO bank_statement VALUES ('模拟财务科技有限公司','2026-03-10',800,500,'往来单位','经营收支混合')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-01','V-AR-1','100201','招商银行','客户回款','借',100,100,0,'客户A')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-01','V-AR-1','112201','应收账款','客户回款','贷',100,0,100,'客户A')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-02','V-AP-1','220201','应付账款','供应商付款','借',60,60,0,'供应商A')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-02','V-AP-1','100201','招商银行','供应商付款','贷',60,0,60,'供应商A')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-03','V-INTERNAL-1','122101','其他应收款','内部调拨','借',40,40,0,'深圳分公司')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-03','V-INTERNAL-1','100201','招商银行','内部调拨','贷',40,0,40,'深圳分公司')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-04','V-FA-1','160101','电脑','购置设备','借',30,30,0,'设备商')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-04','V-FA-1','100201','招商银行','购置设备','贷',30,0,30,'设备商')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-05','V-ADV-1','100201','招商银行','预收合同款','借',20,20,0,'客户B')`,
		`INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-05','V-ADV-1','220301','预收账款','预收合同款','贷',20,0,20,'客户B')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	res := engine.Query("2026年3月利润是多少")
	if !res.Success {
		t.Fatalf("query failed: %+v", res)
	}

	bridge, ok := res.Data["profit_cash_bridge"].(map[string]any)
	if !ok {
		t.Fatalf("profit_cash_bridge missing: %+v", res.Data)
	}
	if got := bridge["estimated_operating_cash"]; got != float64(300) {
		t.Fatalf("estimated_operating_cash = %v, want 300", got)
	}
	if got := bridge["bank_net_cash"]; got != float64(-300) {
		t.Fatalf("bank_net_cash = %v, want -300", got)
	}
	if got := bridge["advance_receipt_increase"]; got != float64(20) {
		t.Fatalf("advance_receipt_increase = %v, want 20", got)
	}
	if got := bridge["other_receivable_increase"]; got != float64(40) {
		t.Fatalf("other_receivable_increase = %v, want 40", got)
	}
	if got := bridge["tax_balance_increase"]; got != float64(5) {
		t.Fatalf("tax_balance_increase = %v, want 5", got)
	}
	if got := bridge["tax_timing_adjustment"]; got != float64(5) {
		t.Fatalf("tax_timing_adjustment = %v, want 5", got)
	}
	if got := bridge["adjusted_operating_cash_estimate"]; got != float64(305) {
		t.Fatalf("adjusted_operating_cash_estimate = %v, want 305", got)
	}
	if got := bridge["non_operating_cash_delta"]; got != float64(-240) {
		t.Fatalf("non_operating_cash_delta = %v, want -240", got)
	}
	if got := bridge["adjusted_operating_cash_gap"]; got != float64(-245) {
		t.Fatalf("adjusted_operating_cash_gap = %v, want -245", got)
	}
	if got := bridge["operating_cash_net"]; got != float64(60) {
		t.Fatalf("operating_cash_net = %v, want 60", got)
	}
	if got := bridge["non_operating_cash_out"]; got != float64(70) {
		t.Fatalf("non_operating_cash_out = %v, want 70", got)
	}
}
