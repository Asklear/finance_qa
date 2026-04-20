package analysis_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/analysis"
)

func TestAnalyzeProfitCashBridge(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cash-bridge.db")
	schema := `
CREATE TABLE balance_detail (
  company TEXT,
  year INTEGER,
  period TEXT,
  account_code TEXT,
  account_name TEXT,
  opening_debit REAL,
  opening_credit REAL,
  current_debit REAL,
  current_credit REAL,
  closing_debit REAL,
  closing_credit REAL
);
CREATE TABLE income_statement (
  company TEXT,
  period TEXT,
  item_name TEXT,
  current_amount REAL,
  cumulative_amount REAL
);
CREATE TABLE bank_statement (
  company TEXT,
  transaction_date TEXT,
  debit_amount REAL,
  credit_amount REAL,
  counterparty_name TEXT,
  summary TEXT
);
CREATE TABLE journal (
  company TEXT,
  period TEXT,
  voucher_date TEXT,
  voucher_no TEXT,
  account_code TEXT,
  account_name TEXT,
  summary TEXT,
  direction TEXT,
  amount REAL,
  debit_amount REAL,
  credit_amount REAL,
  counterparty TEXT
);

INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','四、净利润（净亏损以"－"号填列）',100,300);

INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','1602','累计折旧',0,0,0,10,0,10);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','1602','累计折旧',0,0,0,20,0,20);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','1122','应收账款',80,0,130,30,80,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','1122','应收账款',0,50,220,120,50,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','1123','预付账款',70,0,70,0,70,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','1123','预付账款',30,0,100,70,30,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','1221','其他应收款',10,0,10,0,10,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','1221','其他应收款',50,0,50,0,50,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2241','其他应付款',0,5,0,5,0,5);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2241','其他应付款',0,10,0,10,0,10);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2202','应付账款',0,120,0,120,0,120);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2202','应付账款',0,200,80,280,0,200);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2203','预收账款',0,0,0,0,0,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2203','预收账款',0,20,0,20,0,20);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2211','应付职工薪酬',0,15,0,15,0,15);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2211','应付职工薪酬',0,40,10,50,0,40);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','2221','应交税费',0,3,0,3,0,3);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','2221','应交税费',0,8,0,8,0,8);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','22210101','进项税额',20,0,20,0,20,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','22210101','进项税额',35,0,35,0,35,0);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-02','22210106','销项税额',0,10,0,10,0,10);
INSERT INTO balance_detail VALUES ('模拟财务科技有限公司',2026,'2026-03','22210106','销项税额',0,20,0,20,0,20);

INSERT INTO bank_statement VALUES ('模拟财务科技有限公司','2026-03-10',800,500,'往来单位','经营收支混合');

INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-01','V-AR-1','100201','招商银行','客户回款','借',100,100,0,'客户A');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-01','V-AR-1','112201','应收账款','客户回款','贷',100,0,100,'客户A');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-02','V-AP-1','220201','应付账款','供应商付款','借',60,60,0,'供应商A');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-02','V-AP-1','100201','招商银行','供应商付款','贷',60,0,60,'供应商A');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-03','V-INTERNAL-1','122101','其他应收款','内部调拨','借',40,40,0,'深圳分公司');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-03','V-INTERNAL-1','100201','招商银行','内部调拨','贷',40,0,40,'深圳分公司');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-04','V-FA-1','160101','电脑','购置设备','借',30,30,0,'设备商');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-04','V-FA-1','100201','招商银行','购置设备','贷',30,0,30,'设备商');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-05','V-ADV-1','100201','招商银行','预收合同款','借',20,20,0,'客户B');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-05','V-ADV-1','220301','预收账款','预收合同款','贷',20,0,20,'客户B');
`
	cmd := exec.Command("sqlite3", dbPath)
	cmd.Stdin = strings.NewReader(schema)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 failed: %v\n%s", err, string(out))
	}

	bridge, err := analysis.AnalyzeProfitCashBridge(dbPath, "模拟财务科技有限公司", "2026-03")
	if err != nil {
		t.Fatalf("AnalyzeProfitCashBridge failed: %v", err)
	}

	if bridge.NetProfit != 100 {
		t.Fatalf("net profit = %.2f, want 100", bridge.NetProfit)
	}
	if bridge.Depreciation != 10 {
		t.Fatalf("depreciation = %.2f, want 10", bridge.Depreciation)
	}
	if bridge.ARIncrease != -30 {
		t.Fatalf("AR increase = %.2f, want -30", bridge.ARIncrease)
	}
	if bridge.PrepaymentIncrease != -40 {
		t.Fatalf("prepayment increase = %.2f, want -40", bridge.PrepaymentIncrease)
	}
	if bridge.OtherPayableIncrease != 5 {
		t.Fatalf("other payable increase = %.2f, want 5", bridge.OtherPayableIncrease)
	}
	if bridge.OtherReceivableIncrease != 40 {
		t.Fatalf("other receivable increase = %.2f, want 40", bridge.OtherReceivableIncrease)
	}
	if bridge.APIncrease != 80 {
		t.Fatalf("AP increase = %.2f, want 80", bridge.APIncrease)
	}
	if bridge.AdvanceReceiptIncrease != 20 {
		t.Fatalf("advance receipt increase = %.2f, want 20", bridge.AdvanceReceiptIncrease)
	}
	if bridge.PayrollIncrease != 25 {
		t.Fatalf("payroll increase = %.2f, want 25", bridge.PayrollIncrease)
	}
	if bridge.TaxBalanceIncrease != 5 {
		t.Fatalf("tax balance increase = %.2f, want 5", bridge.TaxBalanceIncrease)
	}
	if bridge.TaxTimingAdjustment != 5 {
		t.Fatalf("tax timing adjustment = %.2f, want 5", bridge.TaxTimingAdjustment)
	}
	if bridge.EstimatedOperatingCash != 300 {
		t.Fatalf("estimated operating cash = %.2f, want 300", bridge.EstimatedOperatingCash)
	}
	if bridge.AdjustedOperatingCashEstimate != 305 {
		t.Fatalf("adjusted operating cash estimate = %.2f, want 305", bridge.AdjustedOperatingCashEstimate)
	}
	if bridge.BankNetCash != -300 {
		t.Fatalf("bank net cash = %.2f, want -300", bridge.BankNetCash)
	}
	if bridge.NonOperatingCashDelta != -240 {
		t.Fatalf("non-operating delta = %.2f, want -240", bridge.NonOperatingCashDelta)
	}
	if bridge.AdjustedOperatingCashGap != -245 {
		t.Fatalf("adjusted operating cash gap = %.2f, want -245", bridge.AdjustedOperatingCashGap)
	}
	if bridge.OperatingCashIn != 120 {
		t.Fatalf("operating cash in = %.2f, want 120", bridge.OperatingCashIn)
	}
	if bridge.OperatingCashOut != 60 {
		t.Fatalf("operating cash out = %.2f, want 60", bridge.OperatingCashOut)
	}
	if bridge.OperatingCashNet != 60 {
		t.Fatalf("operating cash net = %.2f, want 60", bridge.OperatingCashNet)
	}
	if bridge.NonOperatingCashOut != 70 {
		t.Fatalf("non-operating cash out = %.2f, want 70", bridge.NonOperatingCashOut)
	}
}

func TestAnalyzeProfitCashBridgeTreatsPayrollWithholdingsAsOperatingCash(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cash-bridge-payroll.db")
	schema := `
CREATE TABLE balance_detail (
  company TEXT,
  year INTEGER,
  period TEXT,
  account_code TEXT,
  account_name TEXT,
  opening_debit REAL,
  opening_credit REAL,
  current_debit REAL,
  current_credit REAL,
  closing_debit REAL,
  closing_credit REAL
);
CREATE TABLE income_statement (
  company TEXT,
  period TEXT,
  item_name TEXT,
  current_amount REAL,
  cumulative_amount REAL
);
CREATE TABLE bank_statement (
  company TEXT,
  transaction_date TEXT,
  debit_amount REAL,
  credit_amount REAL,
  counterparty_name TEXT,
  summary TEXT
);
CREATE TABLE journal (
  company TEXT,
  period TEXT,
  voucher_date TEXT,
  voucher_no TEXT,
  account_code TEXT,
  account_name TEXT,
  summary TEXT,
  direction TEXT,
  amount REAL,
  debit_amount REAL,
  credit_amount REAL,
  counterparty TEXT
);

INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','四、净利润（净亏损以"－"号填列）',0,0);
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-10','V-WAGE-1','100201','招商银行','发放2月工资','贷',100,0,100,'员工');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-10','V-WAGE-1','221101','应付职工薪酬-工资','发放2月工资','借',120,120,0,'员工');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-10','V-WAGE-1','122103','其他应收款-社保','发放2月工资','贷',10,0,10,'员工');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-03','2026-03-10','V-WAGE-1','222119','应交个人所得税','发放2月工资','贷',10,0,10,'税局');
`
	cmd := exec.Command("sqlite3", dbPath)
	cmd.Stdin = strings.NewReader(schema)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 failed: %v\n%s", err, string(out))
	}

	bridge, err := analysis.AnalyzeProfitCashBridge(dbPath, "模拟财务科技有限公司", "2026-03")
	if err != nil {
		t.Fatalf("AnalyzeProfitCashBridge failed: %v", err)
	}
	if bridge.OperatingCashOut != 100 {
		t.Fatalf("operating cash out = %.2f, want 100", bridge.OperatingCashOut)
	}
	if bridge.MixedCashOut != 0 {
		t.Fatalf("mixed cash out = %.2f, want 0", bridge.MixedCashOut)
	}
}
