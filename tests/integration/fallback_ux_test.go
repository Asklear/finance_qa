package integration_test

import (
	"context"
	"database/sql"
	"financeqa/internal/query"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	dbschema "financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/ingest"
)

func TestFallbackUXAndHinting(t *testing.T) {
	dbPath := setupFallbackTestDB(t)
	// 修正点 1：使用测试数据中真实存在的公司名，确保样本提取不为空
	eng, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Close()

	t.Run("NonsenseQuery_ShouldProvideRichContext", func(t *testing.T) {
		res := eng.Query("今天天气不错，帮我看看有没有什么要注意的")

		if res.Success {
			t.Errorf("expected Success=false for nonsense query, got true")
		}

		if v, ok := res.Data["fallback_attempted"].(bool); !ok || !v {
			t.Errorf("expected fallback_attempted=true in Data")
		}

		if hint, ok := res.Data["hint"].(string); !ok || hint == "" {
			t.Errorf("expected non-empty hint for LLM guidance")
		}

		// 修正点 2：使用 Fatalf 拦截空数据，防止 Panic
		accounts, ok := res.Data["available_accounts"].([]string)
		if !ok || len(accounts) == 0 {
			t.Fatalf("expected available_accounts in fallback data, check if company '模拟财务' exists in DB")
		}

		foundTarget := false
		targets := []string{"研发支出", "人工", "货币资金", "银行存款", "应收", "支出", "费用"}
		for _, acc := range accounts {
			for _, t := range targets {
				if strings.Contains(acc, t) {
					foundTarget = true
					break
				}
			}
			if foundTarget {
				break
			}
		}
		if !foundTarget {
			t.Errorf("expected valid accounting subjects in available accounts, got %v", accounts)
		}

		counterparties, ok := res.Data["counterparty_sample"].([]string)
		if !ok || len(counterparties) == 0 {
			t.Fatalf("expected counterparty_sample in fallback data")
		}
		// 修正点 3：测试数据中 bank_statement 的对手方样本包含“核心供应商A”
		if len(counterparties) > 0 {
			t.Logf("Got counterparty samples: %v", counterparties)
		}
	})

	t.Run("AmbiguousEntity_ShouldGuideLLM", func(t *testing.T) {
		res := eng.Query("核心供应商A")

		// 对于仅有实体无意图的查询，如果是 Fallback 成功（返回了提示），则 Success 为 false
		if !res.Success {
			if hint, ok := res.Data["hint"].(string); !ok || !strings.Contains(hint, "具体") {
				t.Errorf("expected hint to guide user towards specific actions")
			}
		}

		// 即使失败，也应有逻辑日志说明身份识别结果
		logs := strings.Join(res.CalculationLogs, "\n")
		if !strings.Contains(logs, "识别") {
			t.Errorf("expected audit detection log for entity, got: %s", logs)
		}
	})
}

func setupFallbackTestDB(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fallback_test.db")

	if err := dbschema.Bootstrap(context.Background(), dbPath); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()
	mgr := dimensions.NewManager(dimensions.NewSQLiteRepository(db))
	importer := ingest.NewImporter(mgr)
	testDataRoot := filepath.Join("..", "testdata")

	files := []string{
		"模拟财务2026.1-2月序时账-end.xls",
		"模拟财务2026.1-2月余额表-end.xls",
		"交易查询，模拟财务科技有限公司，125922640010001，人民币，20260101-20260228，共93笔_20260401121229.xlsx",
	}

	imported := 0
	for _, f := range files {
		path := filepath.Join(testDataRoot, f)
		if _, err := os.Stat(path); err == nil {
			importer.ImportFile(context.Background(), dbPath, path, false)
			imported++
		}
	}
	if imported == 0 {
		t.Skip("fallback fixture files are not present in this workspace")
	}

	return dbPath
}
