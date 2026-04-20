package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/query"
	"financeqa/internal/support"
)

type realdataQuestion struct {
	ID       int    `json:"id"`
	Question string `json:"question"`
}

func TestRealdataQuestionSuites(t *testing.T) {
	if os.Getenv("FINANCEQA_RUN_LIVE_DB_TESTS") != "1" {
		t.Skip("set FINANCEQA_RUN_LIVE_DB_TESTS=1 to run live database suites")
	}

	cwd, _ := os.Getwd()
	root := filepath.Join(cwd, "..", "..")
	_ = support.LoadDotEnv(filepath.Join(root, ".env"))
	_ = support.LoadDotEnv("/root/finance_qa/.env")
	dbPath := support.DefaultDBPath(root)
	if dbPath == "" {
		t.Skip("database is not configured; skipping real-data suite")
	}

	suites := []string{
		filepath.Join(root, "tests/testdata/top20_questions_2026-04-14.json"),
		filepath.Join(root, "tests/testdata/user19_questions_2026-04-20.json"),
		filepath.Join(root, "tests/testdata/user15_questions_2026-04-20.json"),
	}

	engine, err := query.NewEngine(dbPath, "南京优集数据科技有限公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	for _, suitePath := range suites {
		raw, err := os.ReadFile(suitePath)
		if err != nil {
			t.Fatalf("read suite %s: %v", suitePath, err)
		}
		var questions []realdataQuestion
		if err := json.Unmarshal(raw, &questions); err != nil {
			t.Fatalf("parse suite %s: %v", suitePath, err)
		}
		if len(questions) == 0 {
			t.Fatalf("suite %s is empty", suitePath)
		}

		for _, item := range questions {
			res := engine.Query(item.Question)
			if !res.Success {
				t.Fatalf("suite=%s qid=%d question=%q failed: %s", filepath.Base(suitePath), item.ID, item.Question, res.Message)
			}
			if len(res.ExecutedSQL) == 0 || len(res.CalculationLogs) == 0 {
				t.Fatalf("suite=%s qid=%d missing trace: sql=%d logs=%d", filepath.Base(suitePath), item.ID, len(res.ExecutedSQL), len(res.CalculationLogs))
			}
		}
	}
}
