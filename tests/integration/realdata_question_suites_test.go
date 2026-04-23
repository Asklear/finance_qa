package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type realdataQuestion struct {
	ID       int    `json:"id"`
	Question string `json:"question"`
}

func TestRealdataQuestionSuites(t *testing.T) {
	root, _ := requireLiveDBConfig(t)

	suites := []string{
		filepath.Join(root, "tests/testdata/top20_questions_2026-04-14.json"),
		filepath.Join(root, "tests/testdata/user19_questions_2026-04-20.json"),
		filepath.Join(root, "tests/testdata/user15_questions_2026-04-20.json"),
	}

	engine := requireLiveDBEngine(t, "南京优集数据科技有限公司")

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

		suiteName := filepath.Base(suitePath)
		t.Run(suiteName, func(t *testing.T) {
			for _, item := range questions {
				item := item
				t.Run(fmt.Sprintf("qid_%02d", item.ID), func(t *testing.T) {
					start := time.Now()
					res := engine.Query(item.Question)
					elapsed := time.Since(start)
					t.Logf("question=%q elapsed=%s success=%v sql=%d logs=%d", item.Question, elapsed.Round(time.Millisecond), res.Success, len(res.ExecutedSQL), len(res.CalculationLogs))
					if !res.Success {
						t.Fatalf("suite=%s qid=%d question=%q failed: %s", suiteName, item.ID, item.Question, res.Message)
					}
					if len(res.ExecutedSQL) == 0 || len(res.CalculationLogs) == 0 {
						t.Fatalf("suite=%s qid=%d missing trace: sql=%d logs=%d", suiteName, item.ID, len(res.ExecutedSQL), len(res.CalculationLogs))
					}
				})
			}
		})
	}
}
