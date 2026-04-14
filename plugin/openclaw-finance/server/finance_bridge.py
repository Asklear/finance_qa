#!/usr/bin/env python3
"""OpenClaw Finance bridge server.
Bridge OpenClaw finance tools to finance_qa Go CLI.
"""
import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path

FINANCEQA_BIN = Path(os.environ.get("FINANCEQA_BIN", "/root/finance_qa/financeqa"))
FINANCEQA_DB = Path(os.environ.get("FINANCEQA_DB", "/root/finance_qa/finance.db"))
DEFAULT_COMPANY = os.environ.get("FINANCEQA_DEFAULT_COMPANY", "南京优集数据科技有限公司")
DEFAULT_SKILL_CANDIDATES = [
    Path("/root/.openclaw/skills/finance/SKILL.md"),
    Path("/root/.openclaw/skills/finance/skill.md"),
    Path("/root/finance_qa/SKILL.md"),
    Path("/root/finance_qa/skill.md"),
]


def resolve_skill_path():
    env_path = os.environ.get("FINANCEQA_SKILL_PATH")
    if env_path:
        return Path(env_path)
    for cand in DEFAULT_SKILL_CANDIDATES:
        if cand.exists():
            return cand
    return DEFAULT_SKILL_CANDIDATES[0]


SKILL_PATH = resolve_skill_path()

TOOLS = [
    {
        "name": "finance-query",
        "description": "老板财务问答（结构化结果 + 过程追踪 + 兜底数据包）",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "老板自然语言问题"}
            },
            "required": ["query"]
        }
    },
    {
        "name": "finance-host-data",
        "description": "输出宿主LLM兜底所需的全量财报原始数据包",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "原始问题（可选）"},
                "from": {"type": "string", "description": "开始期间 YYYY-MM（可选）"},
                "to": {"type": "string", "description": "结束期间 YYYY-MM（可选）"}
            }
        }
    },
    {
        "name": "finance-upload",
        "description": "导入单个财务文件",
        "inputSchema": {
            "type": "object",
            "properties": {
                "filePath": {"type": "string", "description": "Excel/报表文件路径"}
            },
            "required": ["filePath"]
        }
    }
]


def text_result(text):
    return {"content": [{"type": "text", "text": text}]}


def run_cmd(args):
    return subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)


def parse_json_or_none(text):
    txt = (text or "").strip()
    if not txt:
        return None
    try:
        return json.loads(txt)
    except Exception:
        return None


def ensure_runtime_ready():
    if not FINANCEQA_BIN.exists():
        raise RuntimeError(f"financeqa binary not found: {FINANCEQA_BIN}")
    if not FINANCEQA_DB.exists():
        raise RuntimeError(f"finance database not found: {FINANCEQA_DB}")


def load_skill_meta():
    if not SKILL_PATH.exists():
        return {"path": str(SKILL_PATH), "exists": False}
    content = SKILL_PATH.read_text(encoding="utf-8", errors="ignore")
    digest = hashlib.sha256(content.encode("utf-8")).hexdigest()
    version_hint = "unknown"
    m = re.search(r'description:\s*"([^"]+)"', content)
    if m:
        version_hint = m.group(1)
    return {
        "path": str(SKILL_PATH),
        "exists": True,
        "sha256": digest,
        "version_hint": version_hint,
    }


def ensure_trace_fields(payload):
    payload = payload or {}
    payload.setdefault("success", False)
    payload.setdefault("message", "")
    payload.setdefault("answer_method", "sql")
    payload.setdefault("data", {})
    payload.setdefault("executed_sql", [])
    payload.setdefault("calculation_logs", [])

    data = payload.get("data") or {}
    trace = data.get("trace") or {}
    trace["executed_sql"] = payload.get("executed_sql") or []
    trace["calculation_logs"] = payload.get("calculation_logs") or []
    data["trace"] = trace
    data["answer_method"] = payload.get("answer_method")
    payload["data"] = data
    return payload


def build_boss_reply(payload, query):
    data = payload.get("data") or {}
    msg = payload.get("message") or ""

    if payload.get("answer_method") == "llm_payload" or not payload.get("success"):
        return {
            "结论": "这个问题当前不能直接精算，已切到财报原始数据模式继续判断。",
            "原因": msg or "原问题语义不够具体，系统已准备全量财报数据供二次推理。",
            "建议": "请补充时间和对象（如客户/项目/月份）；或让宿主LLM基于 llm_payload 给最终口径。",
        }

    metric = data.get("metric")
    period = data.get("period") or "当前期间"
    if metric and ("money_value" in data or "account_value" in data):
        return {
            "结论": f"{period}{metric}：卡上实际进出账约 {float(data.get('money_value', 0)):.2f} 元，报表确认约 {float(data.get('account_value', 0)):.2f} 元。",
            "原因": "两边差异通常来自确认时点、预提和冲回。",
            "建议": "优先盯回款与大额支出节奏，避免下月利润波动。",
        }

    if "suppliers" in data:
        suppliers = data.get("suppliers") or []
        top = suppliers[0]["name"] if suppliers else "暂无"
        return {
            "结论": f"当前识别供应商约 {data.get('count', 0)} 个，净流出最大的对手方是 {top}。",
            "原因": "按银行流水净流出大于净流入识别供应商。",
            "建议": "优先核对前五大供应商付款计划与发票匹配。",
        }

    if "total" in data and "period" in data:
        return {
            "结论": f"{data.get('period')} 核心金额约 {float(data.get('total', 0)):.2f} 元。",
            "原因": msg,
            "建议": "建议结合回款、应收应付和税额一起看，不单看一个数字。",
        }

    return {
        "结论": msg,
        "原因": "本次结果来自财务引擎规则计算。",
        "建议": "如需更细，请指定对象 + 月份 + 指标。",
    }


def build_structured_response(payload, query):
    payload = ensure_trace_fields(payload)
    payload["boss_reply"] = build_boss_reply(payload, query)
    payload["bridge_meta"] = {
        "company": DEFAULT_COMPANY,
        "db": str(FINANCEQA_DB),
        "skill": load_skill_meta(),
    }
    return payload


def run_finance_query(question):
    proc = run_cmd([
        str(FINANCEQA_BIN),
        "query",
        "--db", str(FINANCEQA_DB),
        "--company", DEFAULT_COMPANY,
        question,
    ])

    if proc.returncode == 0:
        payload = parse_json_or_none(proc.stdout) or {"raw": (proc.stdout or "").strip()}
        return build_structured_response(payload, question)

    err_msg = (proc.stderr or proc.stdout or "").strip() or "query failed"

    host_proc = run_cmd([
        str(FINANCEQA_BIN),
        "host-data",
        "--db", str(FINANCEQA_DB),
        "--company", DEFAULT_COMPANY,
        question,
    ])
    host_payload = parse_json_or_none(host_proc.stdout) or {}
    host_data = host_payload.get("data") if isinstance(host_payload, dict) else {}

    fallback = {
        "success": False,
        "message": err_msg,
        "answer_method": "llm_payload",
        "data": {
            "fallback_attempted": True,
            "hint": "请补充时间+对象+指标，例如：2026年2月飞未云科回款多少",
            "llm_payload": (host_data or {}).get("llm_payload", host_payload),
        },
        "executed_sql": (host_payload.get("executed_sql") if isinstance(host_payload, dict) else []) or [],
        "calculation_logs": [
            "[bridge] finance-query failed, switched to host-data fallback",
            f"[bridge] original_error={err_msg}",
        ],
    }
    return build_structured_response(fallback, question)


def run_host_data(arguments):
    question = ((arguments or {}).get("query") or "输出全量财报原始数据给宿主LLM").strip()
    from_period = ((arguments or {}).get("from") or "").strip()
    to_period = ((arguments or {}).get("to") or "").strip()

    cmd = [
        str(FINANCEQA_BIN),
        "host-data",
        "--db", str(FINANCEQA_DB),
        "--company", DEFAULT_COMPANY,
    ]
    if from_period:
        cmd.extend(["--from", from_period])
    if to_period:
        cmd.extend(["--to", to_period])
    cmd.append(question)

    proc = run_cmd(cmd)
    if proc.returncode != 0:
        raise RuntimeError((proc.stderr or proc.stdout or "").strip())

    payload = parse_json_or_none(proc.stdout) or {"raw": (proc.stdout or "").strip()}
    return build_structured_response(payload, question)


def run_upload(file_path):
    proc = run_cmd([
        str(FINANCEQA_BIN),
        "import",
        "--db", str(FINANCEQA_DB),
        file_path,
    ])
    if proc.returncode != 0:
        raise RuntimeError((proc.stderr or proc.stdout or "").strip())
    payload = parse_json_or_none(proc.stdout) or {"raw": (proc.stdout or "").strip()}
    payload["bridge_meta"] = {
        "company": DEFAULT_COMPANY,
        "db": str(FINANCEQA_DB),
        "skill": load_skill_meta(),
    }
    return payload


def handle_list_tools():
    return {"tools": TOOLS}


def handle_call_tool(name, arguments):
    ensure_runtime_ready()

    if name == "finance-query":
        query = (arguments or {}).get("query", "").strip()
        if not query:
            raise ValueError("query is required")
        payload = run_finance_query(query)
        return text_result(json.dumps(payload, ensure_ascii=False, indent=2))

    if name == "finance-host-data":
        payload = run_host_data(arguments or {})
        return text_result(json.dumps(payload, ensure_ascii=False, indent=2))

    if name == "finance-upload":
        file_path = (arguments or {}).get("filePath", "").strip()
        if not file_path:
            raise ValueError("filePath is required")
        payload = run_upload(file_path)
        return text_result(json.dumps(payload, ensure_ascii=False, indent=2))

    raise ValueError(f"Unknown tool: {name}")


def main():
    raw = sys.stdin.read().strip()
    if not raw:
        print(json.dumps(text_result(json.dumps({"error": "empty request"}, ensure_ascii=False)), ensure_ascii=False))
        return

    req = json.loads(raw)
    action = req.get("action")

    if action == "list":
        print(json.dumps(handle_list_tools(), ensure_ascii=False))
        return

    if action == "call":
        name = req.get("name")
        arguments = req.get("arguments") or {}
        try:
            print(json.dumps(handle_call_tool(name, arguments), ensure_ascii=False))
        except Exception as e:
            print(json.dumps(text_result(json.dumps({"error": str(e)}, ensure_ascii=False)), ensure_ascii=False))
        return

    print(json.dumps(text_result(json.dumps({"error": f"unsupported action: {action}"}, ensure_ascii=False)), ensure_ascii=False))


if __name__ == "__main__":
    main()
