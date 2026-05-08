#!/usr/bin/env python3
"""Capture full FinanceQA payloads for accuracy audits."""

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path

SUITES = [
    "tests/testdata/top20_questions_2026-04-14.json",
    "tests/testdata/user19_questions_2026-04-20.json",
    "tests/testdata/user15_questions_2026-04-20.json",
    "tests/testdata/q1_finance_feedback_cases.json",
]


def load_questions(root):
    questions = []
    seen = set()
    for rel in SUITES:
        data = json.loads((root / rel).read_text(encoding="utf-8"))
        for item in data:
            question = str(item.get("question") or item.get("query") or "").strip()
            if not question or question in seen:
                continue
            seen.add(question)
            questions.append({"id": len(questions) + 1, "source": rel, "question": question})
    return questions


def mcp_payload(stdout):
    outer = None
    for line in stdout.splitlines():
        if not line.strip():
            continue
        message = json.loads(line)
        if message.get("id") == 2:
            outer = message.get("result")
            break
    if not isinstance(outer, dict):
        raise RuntimeError("MCP tools/call response not found")
    content = outer.get("content")
    if isinstance(content, list):
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text":
                return json.loads(item.get("text") or "{}")
    return outer


def run_local(root, question, timeout):
    financeqa_bin = root / "bin" / "financeqa"
    proc = subprocess.run(
        [str(financeqa_bin), "query", question],
        cwd=str(root),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        universal_newlines=True,
        timeout=timeout,
    )
    return proc.returncode, json.loads(proc.stdout), proc.stderr


def run_mcp(root, question, timeout):
    env = os.environ.copy()
    env.setdefault("FINANCEQA_BIN", str(root / "bin" / "financeqa"))
    env.setdefault("FINANCEQA_SKILL_PATH", str(root / "SKILL.md"))
    env.setdefault("FINANCEQA_APPENDIX_PATH", str(root / "docs/SKILL_APPENDIX_FULL.md"))
    requests = [
        {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}},
        {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/call",
            "params": {"name": "finance-query", "arguments": {"query": question}},
        },
    ]
    proc = subprocess.run(
        [
            env["FINANCEQA_BIN"],
            "serve",
            "--skill",
            env["FINANCEQA_SKILL_PATH"],
            "--appendix",
            env["FINANCEQA_APPENDIX_PATH"],
        ],
        cwd=str(root),
        input="\n".join(json.dumps(request, ensure_ascii=False) for request in requests) + "\n",
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        universal_newlines=True,
        timeout=timeout,
        env=env,
    )
    return proc.returncode, mcp_payload(proc.stdout), proc.stderr


def compact(text):
    return " ".join(str(text or "").split())[:500]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default=".")
    parser.add_argument("--mode", choices=["local-cli", "mcp"], required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--timeout", type=int, default=150)
    args = parser.parse_args()

    root = Path(args.root).resolve()
    rows = []
    started = time.time()
    questions = load_questions(root)
    for item in questions:
        t0 = time.time()
        row = dict(item)
        row["mode"] = args.mode
        try:
            if args.mode == "local-cli":
                code, payload, stderr = run_local(root, item["question"], args.timeout)
            else:
                code, payload, stderr = run_mcp(root, item["question"], args.timeout)
            row.update(
                {
                    "exit_code": code,
                    "elapsed_ms": int((time.time() - t0) * 1000),
                    "success": payload.get("success") if isinstance(payload, dict) else None,
                    "payload": payload,
                    "stderr": compact(stderr),
                }
            )
        except Exception as exc:
            row.update(
                {
                    "exit_code": 1,
                    "elapsed_ms": int((time.time() - t0) * 1000),
                    "success": False,
                    "error": compact(f"{type(exc).__name__}: {exc}"),
                }
            )
        rows.append(row)
        status = "PASS" if row.get("exit_code") == 0 and row.get("success") is True else "FAIL"
        print(f"[{status}] {row['id']:02d}/{len(questions)} {row['question']}", flush=True)
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(
        json.dumps(
            {
                "mode": args.mode,
                "total": len(rows),
                "elapsed_ms": int((time.time() - started) * 1000),
                "rows": rows,
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )
    return 0 if all(row.get("exit_code") == 0 and row.get("success") is True for row in rows) else 1


if __name__ == "__main__":
    sys.exit(main())
