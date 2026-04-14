#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

/opt/homebrew/bin/go build -o financeqa ./cmd/financeqa

python3 - <<'PY'
import json
import subprocess
import time
import datetime
from pathlib import Path

root = Path("/Users/gaorongvc/work/other/finance_qa")
question_file = root / "tests/testdata/top20_questions_2026-04-14.json"
report_file = root / "docs/2026-04-14-20问真实数据测试报告.md"

questions = json.loads(question_file.read_text(encoding="utf-8"))
cmd_base = ["./financeqa", "query", "--db", "finance.db", "--company", "南京优集数据科技有限公司"]

rows = []
for item in questions:
    qid = int(item["id"])
    q = str(item["question"])
    t0 = time.perf_counter()
    proc = subprocess.run(cmd_base + [q], cwd=str(root), capture_output=True, text=True)
    elapsed_ms = int((time.perf_counter() - t0) * 1000)

    stdout = proc.stdout.strip()
    stderr = proc.stderr.strip()
    success = False
    method = ""
    message = ""
    has_sql = False
    has_logs = False
    parse_error = ""
    try:
        payload = json.loads(stdout) if stdout else {}
        success = bool(payload.get("success", False))
        method = str(payload.get("answer_method", ""))
        message = str(payload.get("message", "")).replace("\n", " ")
        has_sql = len(payload.get("executed_sql") or []) > 0
        has_logs = len(payload.get("calculation_logs") or []) > 0
    except Exception as e:
        parse_error = str(e)
        message = (stdout or stderr)[:500]

    rows.append({
        "id": qid,
        "question": q,
        "elapsed_ms": elapsed_ms,
        "success": success,
        "method": method,
        "has_sql": has_sql,
        "has_logs": has_logs,
        "message": message,
        "stderr": stderr,
        "parse_error": parse_error,
    })

pass_count = sum(1 for r in rows if r["success"])
q10 = next((r for r in rows if r["id"] == 10), None)
q10_ok = bool(q10 and q10["success"] and q10["method"] == "sql")

now = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
with report_file.open("w", encoding="utf-8") as f:
    f.write("# 20道老板高频财务问题真实数据测试报告\n\n")
    f.write(f"- 生成时间: {now}\n")
    f.write("- 数据库: `/Users/gaorongvc/work/other/finance_qa/finance.db`\n")
    f.write("- 执行命令: `./financeqa query --db finance.db --company \"南京优集数据科技有限公司\" \"<问题>\"`\n")
    f.write(f"- 结果概览: {pass_count}/{len(rows)} 成功\n")
    f.write(f"- Q10专项断言（不得fallback）: {'通过' if q10_ok else '失败'}\n\n")

    f.write("## 汇总表\n\n")
    f.write("| ID | 问题 | success | 方法 | SQL过程 | 计算过程 | 耗时(ms) |\n")
    f.write("|---:|---|:---:|:---:|:---:|:---:|---:|\n")
    for r in rows:
        q = r["question"].replace("|", "\\|")
        f.write(f"| {r['id']} | {q} | {'✅' if r['success'] else '❌'} | {r['method'] or '-'} | {'✅' if r['has_sql'] else '❌'} | {'✅' if r['has_logs'] else '❌'} | {r['elapsed_ms']} |\n")

    f.write("\n## 逐题结果\n\n")
    for r in rows:
        f.write(f"### {r['id']}. {r['question']}\n\n")
        f.write(f"- success: `{str(r['success']).lower()}`\n")
        f.write(f"- answer_method: `{r['method'] or '-'}`\n")
        f.write(f"- elapsed_ms: `{r['elapsed_ms']}`\n")
        f.write(f"- executed_sql_present: `{str(r['has_sql']).lower()}`\n")
        f.write(f"- calculation_logs_present: `{str(r['has_logs']).lower()}`\n")
        if r["parse_error"]:
            f.write(f"- parse_error: `{r['parse_error']}`\n")
        if r["stderr"]:
            f.write(f"- stderr: `{r['stderr'][:300]}`\n")
        f.write(f"- 回答摘要: {r['message'][:900]}\n\n")

print(f"top20_pass={pass_count}/{len(rows)}")
print(f"q10_ok={q10_ok}")
print(str(report_file))

if pass_count != len(rows):
    raise SystemExit(1)
if not q10_ok:
    raise SystemExit(2)
PY
