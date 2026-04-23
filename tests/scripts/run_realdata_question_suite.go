//go:build scriptmain
// +build scriptmain

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"financeqa/internal/query"
	"financeqa/internal/support"
)

type suiteQuestion struct {
	ID       int    `json:"id"`
	Question string `json:"question"`
}

type suiteRow struct {
	ID        int
	Question  string
	ElapsedMS int64
	Success   bool
	Method    string
	HasSQL    bool
	HasLogs   bool
	Message   string
}

func main() {
	var (
		dbPath  = flag.String("db", "", "database dsn/path (defaults to configured PostgreSQL target)")
		company = flag.String("company", "南京优集数据科技有限公司", "company name")
		suite   = flag.String("suite", "", "path to question suite json")
		report  = flag.String("report", "", "path to markdown report")
		title   = flag.String("title", "真实数据问题回归报告", "report title")
	)
	flag.Parse()

	if strings.TrimSpace(*suite) == "" {
		fmt.Fprintln(os.Stderr, "missing -suite")
		os.Exit(2)
	}
	if strings.TrimSpace(*report) == "" {
		fmt.Fprintln(os.Stderr, "missing -report")
		os.Exit(2)
	}

	suitePath, err := filepath.Abs(*suite)
	if err != nil {
		panic(err)
	}
	reportPath, err := filepath.Abs(*report)
	if err != nil {
		panic(err)
	}
	dbTarget := strings.TrimSpace(*dbPath)
	if dbTarget == "" {
		dbTarget, err = resolveConfiguredDBTarget()
		if err != nil {
			panic(err)
		}
	}

	dbLabel := dbTarget
	if !looksLikeDSN(dbTarget) {
		dbLabel, err = filepath.Abs(dbTarget)
		if err != nil {
			panic(err)
		}
	}

	engine, err := query.NewEngine(dbTarget, *company)
	if err != nil {
		panic(err)
	}
	defer engine.Close()

	raw, err := os.ReadFile(suitePath)
	if err != nil {
		panic(err)
	}
	var questions []suiteQuestion
	if err := json.Unmarshal(raw, &questions); err != nil {
		panic(err)
	}
	if len(questions) == 0 {
		panic("question suite is empty")
	}

	rows := make([]suiteRow, 0, len(questions))
	passCount := 0
	for _, item := range questions {
		t0 := time.Now()
		res := engine.Query(item.Question)
		elapsedMS := time.Since(t0).Milliseconds()
		row := suiteRow{
			ID:        item.ID,
			Question:  item.Question,
			ElapsedMS: elapsedMS,
			Success:   res.Success,
			Method:    res.AnswerMethod,
			HasSQL:    len(res.ExecutedSQL) > 0,
			HasLogs:   len(res.CalculationLogs) > 0,
			Message:   strings.ReplaceAll(res.Message, "\n", " "),
		}
		if row.Success {
			passCount++
		}
		rows = append(rows, row)
	}

	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		panic(err)
	}
	f, err := os.Create(reportPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "# %s\n\n", *title)
	fmt.Fprintf(f, "- 生成时间: %s\n", now)
	fmt.Fprintf(f, "- 数据库: `%s`\n", redactDBTarget(dbLabel))
	fmt.Fprintf(f, "- 题库: `%s`\n", suitePath)
	fmt.Fprintf(f, "- 公司: `%s`\n", *company)
	fmt.Fprintf(f, "- 结果概览: %d/%d 成功\n\n", passCount, len(rows))
	fmt.Fprintln(f, "## 汇总表\n")
	fmt.Fprintln(f, "| ID | 问题 | success | 方法 | SQL过程 | 计算过程 | 耗时(ms) |")
	fmt.Fprintln(f, "|---:|---|:---:|:---:|:---:|:---:|---:|")
	for _, row := range rows {
		fmt.Fprintf(f, "| %d | %s | %s | %s | %s | %s | %d |\n",
			row.ID,
			strings.ReplaceAll(row.Question, "|", "\\|"),
			boolMark(row.Success),
			emptyDash(row.Method),
			boolMark(row.HasSQL),
			boolMark(row.HasLogs),
			row.ElapsedMS,
		)
	}

	fmt.Fprintln(f, "\n## 逐题结果\n")
	for _, row := range rows {
		fmt.Fprintf(f, "### %d. %s\n\n", row.ID, row.Question)
		fmt.Fprintf(f, "- success: `%t`\n", row.Success)
		fmt.Fprintf(f, "- answer_method: `%s`\n", emptyDash(row.Method))
		fmt.Fprintf(f, "- elapsed_ms: `%d`\n", row.ElapsedMS)
		fmt.Fprintf(f, "- executed_sql_present: `%t`\n", row.HasSQL)
		fmt.Fprintf(f, "- calculation_logs_present: `%t`\n", row.HasLogs)
		fmt.Fprintf(f, "- 回答: %s\n\n", row.Message)
	}

	fmt.Printf("suite_pass=%d/%d\n", passCount, len(rows))
	fmt.Println(reportPath)
	if passCount != len(rows) {
		os.Exit(1)
	}
}

func resolveConfiguredDBTarget() (string, error) {
	root := support.FindProjectRoot()
	_ = support.LoadDotEnv(filepath.Join(root, ".env"))
	_ = support.LoadDotEnv("/root/finance_qa/.env")
	dbTarget := strings.TrimSpace(support.DefaultDBPath(root))
	if dbTarget == "" {
		return "", errors.New("database is not configured; pass -db or set FINANCEQA_DB / PostgreSQL env vars")
	}
	return dbTarget, nil
}

func looksLikeDSN(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return strings.Contains(s, "host=") && strings.Contains(s, "dbname=")
}

func redactDBTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return target
	}
	if !looksLikeDSN(target) {
		return target
	}
	parts := strings.Fields(target)
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		lower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lower, "host="):
			kept = append(kept, part)
		case strings.HasPrefix(lower, "port="):
			kept = append(kept, part)
		case strings.HasPrefix(lower, "dbname="):
			kept = append(kept, part)
		case strings.HasPrefix(lower, "search_path="):
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return "<redacted dsn>"
	}
	return strings.Join(kept, " ")
}

func boolMark(v bool) string {
	if v {
		return "✅"
	}
	return "❌"
}

func emptyDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}
