package integration_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlineAgentFinalAnswerCheckScriptValidatesBridgeFinalAnswer(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	expectedPath := filepath.Join(tmp, "bridge_expected.jsonl")
	passAnswersPath := filepath.Join(tmp, "openclaw_pass.jsonl")
	failAnswersPath := filepath.Join(tmp, "openclaw_fail.jsonl")
	summaryPath := filepath.Join(tmp, "summary.json")

	question := "2026年3月收入、成本、利润分别是多少？"
	expectedLine := `{"question":"` + question + `","expected":{"success":true,"final_answer":"2026-03先看合同经营口径：营收 5612513.29 元，合同成本 2024957.12 元，利润 3587556.17 元。\n\n来源：《优集资金收入计算表-副本.xlsx》；《优集成本计算表-4.23-池.xlsx》；补充参考：《合同信息表》","data":{"period":"2026-03","source_priority":"contract_first","requested_metrics":["收入","成本","利润"],"source_note":"来源：《优集资金收入计算表-副本.xlsx》；《优集成本计算表-4.23-池.xlsx》；补充参考：《合同信息表》","contract_summary":{"scope":"company","revenue_settlement":5612513.29,"cost_settlement":2024957.12,"profit":3587556.17}}}}` + "\n"
	passLine := `{"question":"` + question + `","answer":"2026-03先看合同经营口径：营收 5612513.29 元，合同成本 2024957.12 元，利润 3587556.17 元。\n\n来源：《优集资金收入计算表-副本.xlsx》；《优集成本计算表-4.23-池.xlsx》；补充参考：《合同信息表》"}` + "\n"
	failLine := `{"question":"` + question + `","answer":"2026年3月：收入 310.63 万，成本及费用 281.50 万，利润 29.13 万。来源：《利润表》"}` + "\n"

	if err := os.WriteFile(expectedPath, []byte(expectedLine), 0o644); err != nil {
		t.Fatalf("write expected: %v", err)
	}
	if err := os.WriteFile(passAnswersPath, []byte(passLine), 0o644); err != nil {
		t.Fatalf("write pass answers: %v", err)
	}
	if err := os.WriteFile(failAnswersPath, []byte(failLine), 0o644); err != nil {
		t.Fatalf("write fail answers: %v", err)
	}

	scriptPath := filepath.Join("..", "..", "tests", "scripts", "run_online_agent_final_answer_check.py")
	passCmd := exec.Command("python3", scriptPath,
		"--expected-jsonl", expectedPath,
		"--answers-jsonl", passAnswersPath,
		"--host", "openclaw",
		"--summary-json", summaryPath,
	)
	var passOut bytes.Buffer
	passCmd.Stdout = &passOut
	passCmd.Stderr = &passOut
	if err := passCmd.Run(); err != nil {
		t.Fatalf("pass fixture should validate: %v\n%s", err, passOut.String())
	}
	if !strings.Contains(passOut.String(), "pass=1/1") {
		t.Fatalf("pass output should report 1/1, got %s", passOut.String())
	}

	failCmd := exec.Command("python3", scriptPath,
		"--expected-jsonl", expectedPath,
		"--answers-jsonl", failAnswersPath,
		"--host", "openclaw",
		"--summary-json", summaryPath,
	)
	var failOut bytes.Buffer
	failCmd.Stdout = &failOut
	failCmd.Stderr = &failOut
	if err := failCmd.Run(); err == nil {
		t.Fatalf("bad fixture should fail validation\n%s", failOut.String())
	}
	outText := failOut.String()
	if !strings.Contains(outText, "missing_contract_expected_amounts") {
		t.Fatalf("failure should explain missing contract amounts, got %s", outText)
	}
	if !strings.Contains(outText, "used_profit_statement_or_book_instead_of_contract") {
		t.Fatalf("failure should flag profit statement fallback, got %s", outText)
	}
}

func TestOnlineAgentFinalAnswerCheckScriptExtractsOpenClawPayloadText(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	expectedPath := filepath.Join(tmp, "bridge_expected.jsonl")
	answersPath := filepath.Join(tmp, "openclaw_payload.jsonl")
	summaryPath := filepath.Join(tmp, "summary.json")

	question := "2026年3月收入、成本、利润分别是多少？"
	finalAnswer := "合同经营口径：营收 5612513.29 元，合同成本 2024957.12 元，利润 3587556.17 元。来源：《优集资金收入计算表-副本.xlsx》"
	expectedLine := `{"question":"` + question + `","expected":{"success":true,"final_answer":"` + finalAnswer + `","data":{"source_priority":"contract_first","requested_metrics":["收入","成本","利润"],"source_note":"来源：《优集资金收入计算表-副本.xlsx》","contract_summary":{"revenue_settlement":5612513.29,"cost_settlement":2024957.12,"profit":3587556.17}}}}` + "\n"
	stdoutJSON := `{"status":"ok","result":{"payloads":[{"text":"` + finalAnswer + `","mediaUrl":null}]}}`
	answerLine := `{"question":"` + question + `","stdout":` + strconvQuote(stdoutJSON) + `}` + "\n"

	if err := os.WriteFile(expectedPath, []byte(expectedLine), 0o644); err != nil {
		t.Fatalf("write expected: %v", err)
	}
	if err := os.WriteFile(answersPath, []byte(answerLine), 0o644); err != nil {
		t.Fatalf("write answers: %v", err)
	}

	scriptPath := filepath.Join("..", "..", "tests", "scripts", "run_online_agent_final_answer_check.py")
	cmd := exec.Command("python3", scriptPath,
		"--expected-jsonl", expectedPath,
		"--answers-jsonl", answersPath,
		"--host", "openclaw",
		"--summary-json", summaryPath,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("OpenClaw payload stdout should validate: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "pass=1/1") {
		t.Fatalf("output should report 1/1, got %s", out.String())
	}
}

func TestOnlineAgentFinalAnswerCheckScriptUsesOpenClawAgentMessageCommand(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join("..", "..", "tests", "scripts", "run_online_agent_final_answer_check.py")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if !strings.Contains(string(script), `openclaw agent --agent main --json --message {question}`) {
		t.Fatalf("default OpenClaw command should use --message so live tests hit the same online agent path")
	}
}

func TestOnlineAgentFinalAnswerCheckScriptUsesClaudeFinanceWrapper(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join("..", "..", "tests", "scripts", "run_online_agent_final_answer_check.py")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	text := string(script)
	if !strings.Contains(text, `claude_finance_final_answer.sh`) {
		t.Fatalf("default Claude command should use the finance final_answer wrapper")
	}
	if strings.Contains(text, `"claude -p {question}"`) {
		t.Fatalf("default Claude command should not rely on the model-only claude -p path")
	}
}

func TestClaudeFinanceFinalAnswerWrapperPrintsBridgeFinalAnswer(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	bridgePath := filepath.Join(tmp, "finance_bridge_stub.py")
	if err := os.WriteFile(bridgePath, []byte(`#!/usr/bin/env python3
import json
import sys

request = json.load(sys.stdin)
query = request["arguments"]["query"]
payload = {
    "success": True,
    "final_answer": "FINAL:" + query + "\n来源：《测试来源表》",
    "boss_reply_text": "SHOULD_NOT_USE",
}
print(json.dumps({"content": [{"type": "text", "text": json.dumps(payload, ensure_ascii=False)}]}, ensure_ascii=False))
`), 0o755); err != nil {
		t.Fatalf("write bridge stub: %v", err)
	}

	wrapperPath := filepath.Join("..", "..", "tests", "scripts", "claude_finance_final_answer.sh")
	cmd := exec.Command(wrapperPath, "2026年3月收入多少？")
	cmd.Env = append(os.Environ(), "FINANCE_BRIDGE_PATH="+bridgePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("wrapper should print bridge final_answer: %v\n%s", err, out.String())
	}
	got := strings.TrimSpace(out.String())
	want := "FINAL:2026年3月收入多少？\n来源：《测试来源表》"
	if got != want {
		t.Fatalf("wrapper output = %q, want exact final_answer %q", got, want)
	}
}

func strconvQuote(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replacer.Replace(value) + `"`
}
