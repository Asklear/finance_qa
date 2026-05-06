//go:build accuracy

package business

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"financeqa/tests/testutil"
)

type realdataQuestion struct {
	ID       int    `json:"id"`
	Question string `json:"question"`
}

func TestRealdataQuestionSuitesHaveAnswersAndTrace(t *testing.T) {
	t.Parallel()

	root, _ := testutil.RequireLiveDBConfig(t)
	suites := []string{
		filepath.Join(root, "tests/testdata/top20_questions_2026-04-14.json"),
		filepath.Join(root, "tests/testdata/user19_questions_2026-04-20.json"),
		filepath.Join(root, "tests/testdata/user15_questions_2026-04-20.json"),
	}

	for _, suitePath := range suites {
		suitePath := suitePath
		t.Run(filepath.Base(suitePath), func(t *testing.T) {
			t.Parallel()

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
				item := item
				t.Run(fmt.Sprintf("qid_%02d", item.ID), func(t *testing.T) {
					t.Parallel()

					testutil.RunLiveDBCase(t, func() {
						engine := testutil.RequireLiveDBEngine(t, testutil.DefaultBusinessCompany)
						start := time.Now()
						res := engine.Query(item.Question)
						elapsed := time.Since(start)
						t.Logf("question=%q elapsed=%s success=%v sql=%d logs=%d", item.Question, elapsed.Round(time.Millisecond), res.Success, len(res.ExecutedSQL), len(res.CalculationLogs))
						if !res.Success {
							t.Fatalf("suite=%s qid=%d question=%q failed: %s", filepath.Base(suitePath), item.ID, item.Question, res.Message)
						}
						if len(res.ExecutedSQL) == 0 || len(res.CalculationLogs) == 0 {
							t.Fatalf("suite=%s qid=%d missing trace: sql=%d logs=%d", filepath.Base(suitePath), item.ID, len(res.ExecutedSQL), len(res.CalculationLogs))
						}
					})
				})
			}
		})
	}
}
