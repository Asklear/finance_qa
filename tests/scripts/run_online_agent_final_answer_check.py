#!/usr/bin/env python3
"""Validate OpenClaw/Claude answers against Go MCP-produced finance facts.

Default mode checks amounts, source notes, and business口径. Strict matching
to the exact Go MCP `final_answer` text is opt-in for regression debugging.
Live agent execution is opt-in via FINANCEQA_RUN_ONLINE_AGENT_TESTS=1.
"""

import argparse
import json
import math
import os
import re
import shlex
import subprocess
import sys
import time
from pathlib import Path


DEFAULT_SUITES = [
    "tests/testdata/top20_questions_2026-04-14.json",
    "tests/testdata/user19_questions_2026-04-20.json",
    "tests/testdata/user15_questions_2026-04-20.json",
]

PROFIT_STATEMENT_MARCH_VALUES = [
    3106310.34,
    2815018.91,
    291291.55,
]

INTERNAL_PATTERNS = [
    re.compile(r"\bcontract_id\b", re.IGNORECASE),
    re.compile(r"\bexecuted_sql\b", re.IGNORECASE),
    re.compile(r"\bsource_report_type\b", re.IGNORECASE),
    re.compile(r"\bsource_sheet_name\b", re.IGNORECASE),
    re.compile(r"\baccount_code\b", re.IGNORECASE),
    re.compile(r"\bC\d{3,}\b"),
]

VISIBLE_PROMPT_LEAK_PATTERNS = [
    re.compile(r"Current authoritative finance-query result", re.IGNORECASE),
    re.compile(r"Use these current facts", re.IGNORECASE),
    re.compile(r"You may rephrase the final wording", re.IGNORECASE),
    re.compile(r"\bThe user is asking\b", re.IGNORECASE),
    re.compile(r"\bWe have the authoritative\b", re.IGNORECASE),
    re.compile(r"\bprior user message context\b", re.IGNORECASE),
    re.compile(r"\bMust not use\b", re.IGNORECASE),
    re.compile(r"\bNeed preserve\b", re.IGNORECASE),
    re.compile(r"```json", re.IGNORECASE),
    re.compile(r"内部财务事实"),
]


def load_jsonl(path):
    rows = []
    with open(path, "r", encoding="utf-8") as f:
        for line_no, line in enumerate(f, 1):
            text = line.strip()
            if not text:
                continue
            try:
                rows.append(json.loads(text))
            except Exception as exc:
                raise SystemExit("%s:%d invalid JSON: %s" % (path, line_no, exc))
    return rows


def write_jsonl(path, rows):
    with open(path, "w", encoding="utf-8") as f:
        for row in rows:
            f.write(json.dumps(row, ensure_ascii=False, sort_keys=True) + "\n")


def load_questions(paths):
    questions = []
    seen = set()
    for raw_path in paths:
        path = Path(raw_path)
        if not path.exists():
            continue
        with open(str(path), "r", encoding="utf-8") as f:
            data = json.load(f)
        for item in data:
            question = str(item.get("question") or item.get("query") or "").strip()
            if question and question not in seen:
                seen.add(question)
                questions.append({"source": str(path), "id": item.get("id"), "question": question})
    return questions


def normalize_text(value):
    text = str(value or "")
    text = re.sub(r"[\s*_`|,，]+", "", text)
    text = text.replace("：", ":")
    return text


def contains_text(answer, expected):
    expected = str(expected or "").strip()
    if not expected:
        return True
    return normalize_text(expected) in normalize_text(answer)


def row_question(row):
    return str(row.get("question") or row.get("query") or "").strip()


def extract_answer(row):
    for key in ("answer", "final_answer", "result", "output"):
        value = row.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
        if isinstance(value, dict):
            answer = extract_answer_from_parsed_json(value)
            if answer:
                return answer
    stdout = row.get("stdout")
    if isinstance(stdout, str) and stdout.strip():
        try:
            parsed = json.loads(stdout)
            answer = extract_answer_from_parsed_json(parsed)
            if answer:
                return answer
        except Exception:
            return stdout.strip()
    return ""


def extract_answer_from_parsed_json(parsed):
    if not isinstance(parsed, dict):
        return ""
    for key in ("answer", "final_answer", "output"):
        value = parsed.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    result = parsed.get("result")
    if isinstance(result, str) and result.strip():
        return result.strip()
    if isinstance(result, dict):
        value = extract_openclaw_payload_text(result)
        if value:
            return value
        return extract_answer_from_parsed_json(result)
    value = extract_openclaw_payload_text(parsed)
    if value:
        return value
    return ""


def extract_openclaw_payload_text(value):
    if not isinstance(value, dict):
        return ""
    payloads = value.get("payloads")
    if not isinstance(payloads, list):
        return ""
    texts = []
    for payload in payloads:
        if isinstance(payload, dict) and isinstance(payload.get("text"), str) and payload.get("text").strip():
            texts.append(payload.get("text").strip())
    return "\n\n".join(texts)


def as_dict(value):
    return value if isinstance(value, dict) else {}


def as_list(value):
    return value if isinstance(value, list) else []


def first_number(*values):
    for value in values:
        if value in (None, ""):
            continue
        try:
            number = float(value)
        except Exception:
            continue
        if math.isfinite(number):
            return number
    return None


def amount_variants(value):
    try:
        amount = float(value)
    except Exception:
        return []
    variants = {
        "%.2f" % amount,
        format(amount, ",.2f"),
    }
    if abs(amount - round(amount)) < 0.005:
        variants.add(str(int(round(amount))))
        variants.add(format(int(round(amount)), ",d"))
    wan = amount / 10000.0
    variants.add("%.2f万" % wan)
    variants.add("%.1f万" % wan)
    variants.add("%.0f万" % wan)
    return sorted(variants, key=len, reverse=True)


def amount_present(answer, value):
    normalized_answer = normalize_text(answer)
    for variant in amount_variants(value):
        if normalize_text(variant) in normalized_answer:
            return True
    return False


def collect_contract_amounts(payload):
    data = as_dict(payload.get("data"))
    contract_summary = as_dict(data.get("contract_summary"))
    account_view = as_dict(data.get("account_view"))
    book_view = as_dict(data.get("book_view"))
    requested = [str(v).strip() for v in as_list(data.get("requested_metrics")) if str(v).strip()]
    metric = str(data.get("metric") or "").strip()
    if not requested and metric:
        requested = [metric]

    is_contract = (
        str(data.get("source_priority") or "") == "contract_first"
        or bool(contract_summary)
        or as_dict(payload.get("host_summary_contract")).get("kind") == "contract_aggregate"
    )
    if not is_contract:
        return []

    pairs = []

    def add(name, *values):
        number = first_number(*values)
        if number is not None:
            pairs.append((name, number))

    requested_set = set(requested)
    if not requested_set or requested_set.intersection({"收入", "营收", "销售额", "GMV", "gmv"}):
        add("营收", account_view.get("营收"), book_view.get("营收"), contract_summary.get("revenue_settlement"))
    if not requested_set or requested_set.intersection({"成本", "合同成本", "总成本"}):
        add("合同成本", account_view.get("合同成本"), book_view.get("合同成本"), contract_summary.get("cost_settlement"))
    if not requested_set or requested_set.intersection({"利润", "净利润"}):
        add("利润", account_view.get("利润"), book_view.get("利润"), contract_summary.get("profit"))
    if requested_set.intersection({"已开票未回款", "已开票未付款", "已开票未收款", "应收", "应收账款"}):
        add(
            "已开票未回款",
            account_view.get("已开票未回款"),
            book_view.get("已开票未回款"),
            contract_summary.get("invoiced_unreceived_amount"),
        )
    if requested_set.intersection({"已收票未付款", "收到发票未付款", "应付", "应付账款"}):
        add(
            "已收票未付款",
            account_view.get("已收票未付款"),
            book_view.get("已收票未付款"),
            contract_summary.get("invoiced_unpaid_amount"),
        )
    return dedupe_amount_pairs(pairs)


def dedupe_amount_pairs(pairs):
    seen = set()
    out = []
    for name, value in pairs:
        key = (name, round(float(value), 2))
        if key in seen:
            continue
        seen.add(key)
        out.append((name, value))
    return out


def source_labels(payload):
    data = as_dict(payload.get("data"))
    texts = []
    for key in ("source_note", "source_summary"):
        value = data.get(key) or payload.get(key)
        if isinstance(value, str) and value.strip():
            texts.append(value)
    for key in ("source_documents", "source_tables"):
        for item in as_list(data.get(key)):
            if isinstance(item, str) and item.strip():
                texts.append(item)
    labels = []
    for text in texts:
        for match in re.findall(r"《[^》]+》", text):
            if match not in labels:
                labels.append(match)
    return labels


def invoice_open_items(payload):
    data = as_dict(payload.get("data"))
    summary = as_dict(data.get("contract_summary"))
    return [item for item in as_list(summary.get("invoice_open_items")) if isinstance(item, dict)]


def expected_payload(row):
    payload = row.get("expected", row)
    if isinstance(payload, str):
        try:
            payload = json.loads(payload)
        except Exception:
            payload = {}
    return as_dict(payload)


def validate_answer(question, payload, answer, strict_final_answer=False):
    fails = []
    answer = str(answer or "").strip()
    if not answer:
        return ["empty_answer"]

    for pattern in VISIBLE_PROMPT_LEAK_PATTERNS:
        if pattern.search(answer):
            fails.append("leaked_prompt_or_reasoning_context:" + pattern.pattern)
            break

    final_answer = str(payload.get("final_answer") or payload.get("boss_reply_text") or "").strip()
    if strict_final_answer and final_answer and not contains_text(answer, final_answer):
        fails.append("final_answer_not_used")

    missing_amounts = []
    for name, value in collect_contract_amounts(payload):
        if not amount_present(answer, value):
            missing_amounts.append("%s=%.2f" % (name, float(value)))
    if missing_amounts:
        fails.append("missing_contract_expected_amounts:" + ",".join(missing_amounts))

    data = as_dict(payload.get("data"))
    if str(data.get("source_priority") or "") == "contract_first" and missing_amounts:
        if "利润表" in answer or any(amount_present(answer, value) for value in PROFIT_STATEMENT_MARCH_VALUES):
            fails.append("used_profit_statement_or_book_instead_of_contract")

    labels = source_labels(payload)
    if labels and "来源" not in answer:
        fails.append("missing_source_note")
    elif labels and not any(label in answer for label in labels):
        fails.append("missing_source_document:" + ",".join(labels[:3]))

    item_fails = []
    for item in invoice_open_items(payload)[:3]:
        customer = str(item.get("customer_name") or "").strip()
        content = str(item.get("contract_content") or "").strip()
        open_amount = first_number(item.get("open_amount"), item.get("invoiced_unreceived_amount"))
        if customer and customer not in answer:
            item_fails.append("customer=" + customer)
        if content and content not in answer:
            item_fails.append("project=" + content)
        if open_amount is not None and not amount_present(answer, open_amount):
            item_fails.append("open_amount=%.2f" % open_amount)
    if item_fails:
        fails.append("missing_invoice_open_item:" + ",".join(item_fails))

    for pattern in INTERNAL_PATTERNS:
        if pattern.search(answer):
            fails.append("leaked_internal_identifier:" + pattern.pattern)
            break

    return fails


def run_agent(host, question, timeout_sec):
    if host == "openclaw":
        template = os.environ.get("OPENCLAW_AGENT_CMD", "openclaw agent --agent main --json --message {question}")
    elif host == "claude":
        template = os.environ.get("CLAUDE_AGENT_CMD", "tests/scripts/claude_finance_final_answer.sh {question}")
    else:
        raise ValueError("unsupported host: " + host)
    command = template.format(question=shlex.quote(question))
    start = time.time()
    proc = subprocess.run(command, shell=True, universal_newlines=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout_sec)
    elapsed = round(time.time() - start, 2)
    answer = ""
    raw_stdout = proc.stdout or ""
    if raw_stdout.strip():
        try:
            parsed = json.loads(raw_stdout)
            answer = extract_answer_from_parsed_json(parsed)
        except Exception:
            answer = raw_stdout.strip()
    return {
        "host": host,
        "question": question,
        "returncode": proc.returncode,
        "elapsed_sec": elapsed,
        "answer": answer,
        "stdout": raw_stdout,
        "stderr": proc.stderr or "",
    }


def maybe_run_agents(args, questions):
    if not args.run_agents and os.environ.get("FINANCEQA_RUN_ONLINE_AGENT_TESTS") != "1":
        return []
    hosts = args.hosts
    rows = []
    for host in hosts:
        host_rows = []
        for item in questions:
            row = run_agent(host, item["question"], args.timeout_sec)
            row["source"] = item["source"]
            row["id"] = item["id"]
            host_rows.append(row)
            rows.append(row)
        out_path = Path(args.out_dir) / ("%s_final_answers.jsonl" % host)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        write_jsonl(str(out_path), host_rows)
    return rows


def validate(expected_rows, answer_rows, host, strict_final_answer):
    expected_by_question = {row_question(row): expected_payload(row) for row in expected_rows if row_question(row)}
    answers_by_question = {row_question(row): extract_answer(row) for row in answer_rows if row_question(row)}
    results = []
    pass_count = 0
    for question, payload in expected_by_question.items():
        answer = answers_by_question.get(question, "")
        fails = validate_answer(question, payload, answer, strict_final_answer=strict_final_answer)
        ok = not fails
        if ok:
            pass_count += 1
        results.append({
            "host": host,
            "question": question,
            "pass": ok,
            "fails": fails,
            "answer_preview": re.sub(r"\s+", " ", answer)[:260],
        })
    return pass_count, results


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--expected-jsonl", required=True, help="Go MCP expected JSONL; each row has question and expected payload")
    parser.add_argument("--answers-jsonl", help="agent final answer JSONL to validate")
    parser.add_argument("--host", default="agent", help="host label for validation output")
    parser.add_argument("--summary-json", required=True, help="write validation summary JSON")
    parser.add_argument("--question-suite", action="append", default=[], help="question suite JSON; repeatable")
    parser.add_argument("--run-agents", action="store_true", help="run live online agents; also enabled by FINANCEQA_RUN_ONLINE_AGENT_TESTS=1")
    parser.add_argument("--hosts", nargs="+", default=["openclaw", "claude"], choices=["openclaw", "claude"])
    parser.add_argument("--out-dir", default="tmp/online_eval", help="output dir for live agent answer JSONL")
    parser.add_argument("--timeout-sec", type=int, default=180)
    parser.add_argument("--strict-final-answer", action="store_true", help="require the host answer to include the exact Go MCP final_answer text")
    parser.add_argument("--no-strict-final-answer", action="store_true", help="compatibility flag; the default mode is already non-strict")
    args = parser.parse_args()

    suites = args.question_suite or DEFAULT_SUITES
    questions = load_questions(suites)
    if args.run_agents or os.environ.get("FINANCEQA_RUN_ONLINE_AGENT_TESTS") == "1":
        maybe_run_agents(args, questions)

    if not args.answers_jsonl:
        raise SystemExit("--answers-jsonl is required unless you only want to capture live answers separately")

    expected_rows = load_jsonl(args.expected_jsonl)
    answer_rows = load_jsonl(args.answers_jsonl)
    pass_count, results = validate(
        expected_rows,
        answer_rows,
        args.host,
        strict_final_answer=args.strict_final_answer and not args.no_strict_final_answer,
    )
    summary = {
        "host": args.host,
        "pass_count": pass_count,
        "total": len(results),
        "results": results,
    }
    Path(args.summary_json).parent.mkdir(parents=True, exist_ok=True)
    with open(args.summary_json, "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)
    print("%s pass=%d/%d" % (args.host, pass_count, len(results)))
    for row in results:
        if not row["pass"]:
            print("FAIL %s: %s" % (row["question"], ";".join(row["fails"])))
    if pass_count != len(results):
        sys.exit(1)


if __name__ == "__main__":
    main()
