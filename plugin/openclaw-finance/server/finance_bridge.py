#!/usr/bin/env python3
"""OpenClaw Finance bridge server.
Bridge OpenClaw finance tools to finance_qa Go CLI.
"""
import json
import os
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

FINANCEQA_BIN = Path(os.environ.get("FINANCEQA_BIN", "/root/finance_qa/financeqa"))
REPO_ROOT = Path(__file__).resolve().parents[3]
FINANCEQA_SKILL_PATH = Path(os.environ.get("FINANCEQA_SKILL_PATH", str(REPO_ROOT / "SKILL.md")))


def load_dotenv_if_exists(path):
    p = Path(path)
    if not p.exists():
        return
    for raw in p.read_text(encoding="utf-8", errors="ignore").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[len("export "):].strip()
        if "=" not in line:
            continue
        k, v = line.split("=", 1)
        key = k.strip()
        if not key or key in os.environ:
            continue
        os.environ[key] = v.strip().strip("'").strip('"')


load_dotenv_if_exists(".env")
load_dotenv_if_exists("/root/finance_qa/.env")


def load_skill_contract(skill_path):
    raw = skill_path.read_text(encoding="utf-8")

    def capture(name):
        pattern = re.compile(r"`" + re.escape(name) + r"`:\s*`([^`]+)`")
        match = pattern.search(raw)
        if not match:
            raise RuntimeError(f"missing {name} in skill contract file: {skill_path}")
        return match.group(1).strip()

    appendix_pattern = re.compile(r"`(docs/[^`\n]*SKILL_APPENDIX[^`\n]*\.md)`")
    appendix_match = appendix_pattern.search(raw)
    if not appendix_match:
        raise RuntimeError(f"missing appendix relative path in skill contract file: {skill_path}")

    appendix_relative_path = appendix_match.group(1).strip()
    appendix_path = (skill_path.parent / appendix_relative_path).resolve()

    return {
        "skill_contract_version": capture("skill_contract_version"),
        "bridge_protocol_version": capture("bridge_protocol_version"),
        "skill_appendix_relative_path": appendix_relative_path,
        "skill_appendix_path": appendix_path,
    }


def default_db_target():
    explicit = os.environ.get("FINANCEQA_DB")
    if explicit:
        return explicit
    pg_host = os.environ.get("PGHOST", "")
    pg_port = os.environ.get("PGPORT", "5432")
    pg_user = os.environ.get("PGUSER", "")
    pg_pass = os.environ.get("PGPASSWORD", "")
    pg_db = os.environ.get("PGDATABASE", "")
    pg_schema = (os.environ.get("FINANCEQA_PG_SCHEMA") or "").strip()
    if pg_host and pg_user and pg_db:
        dsn = (
            f"host={pg_host} port={pg_port} user={pg_user} password={pg_pass} "
            f"dbname={pg_db}"
        )
        if pg_schema:
            dsn += f" search_path={pg_schema},public"
        return dsn
    return ""


FINANCEQA_DB = default_db_target()
DEFAULT_COMPANY = (os.environ.get("FINANCEQA_DEFAULT_COMPANY") or "").strip()
SKILL_CONTRACT = load_skill_contract(FINANCEQA_SKILL_PATH)
SKILL_CONTRACT_VERSION = SKILL_CONTRACT["skill_contract_version"]
BRIDGE_PROTOCOL_VERSION = SKILL_CONTRACT["bridge_protocol_version"]
SKILL_APPENDIX_RELATIVE_PATH = SKILL_CONTRACT["skill_appendix_relative_path"]
SKILL_APPENDIX_PATH = SKILL_CONTRACT["skill_appendix_path"]

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
    if not FINANCEQA_SKILL_PATH.exists():
        raise RuntimeError(f"skill contract file not found: {FINANCEQA_SKILL_PATH}")
    if not SKILL_APPENDIX_PATH.exists():
        raise RuntimeError(f"skill appendix not found: {SKILL_APPENDIX_PATH}")
    db_text = str(FINANCEQA_DB)
    if not db_text.strip():
        raise RuntimeError("finance database is not configured; set FINANCEQA_DB or PostgreSQL env vars")
    # FINANCEQA_DB can be either an explicit local sqlite path or a PostgreSQL DSN.
    if "host=" in db_text and "dbname=" in db_text:
        return
    if not Path(FINANCEQA_DB).exists():
        raise RuntimeError(f"finance database/dsn not found: {FINANCEQA_DB}")


def now_utc_iso():
    return datetime.now(timezone.utc).isoformat()


def normalize_exposed_fields(data):
    data = data or {}
    exposed = {
        "dual_perspective": data.get("dual_perspective"),
        "hr_breakdown": data.get("hr_breakdown"),
        "arithmetic_checks": data.get("arithmetic_checks"),
        "intent_trace": data.get("intent_trace"),
    }
    data["exposed_fields"] = exposed
    return data


def ensure_trace_fields(payload):
    payload = payload or {}
    payload.setdefault("success", False)
    payload.setdefault("message", "")
    payload.setdefault("answer_method", "sql")
    payload.setdefault("data", {})
    payload.setdefault("executed_sql", [])
    payload.setdefault("calculation_logs", [])

    data = normalize_exposed_fields(payload.get("data") or {})
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

    requested_metrics = data.get("requested_metrics") or []
    if isinstance(requested_metrics, tuple):
        requested_metrics = list(requested_metrics)
    if not isinstance(requested_metrics, list):
        requested_metrics = []

    if payload.get("answer_method") == "llm_payload" or not payload.get("success"):
        return {
            "结论": "这个问题当前不能直接精算，已切到财报原始数据模式继续判断。",
            "原因": msg or "原问题语义不够具体，系统已准备全量财报数据供二次推理。",
            "建议": "请补充时间和对象（如客户/项目/月份）；或让宿主LLM基于 llm_payload 给最终口径。",
        }

    metric = data.get("metric")
    period = data.get("period") or "当前期间"
    if len(requested_metrics) > 1 and "money_view" in data and "account_view" in data:
        cash_view = data.get("money_view") or {}
        account_view = data.get("account_view") or {}
        cash_in = float(cash_view.get("现金流入", data.get("现金流入", 0)))
        cash_out = float(cash_view.get("现金流出", data.get("现金流出", 0)))
        cash_net = float(cash_view.get("净现金流", data.get("净现金流", 0)))
        revenue = float(account_view.get("营业收入", 0))
        cost = float(account_view.get("营业成本及费用", 0))
        profit = float(account_view.get("账面利润", 0))
        return {
            "结论": f"{period}先看现金口径：实际到账 {cash_in:.2f} 元、实际支出 {cash_out:.2f} 元、净增加 {cash_net:.2f} 元；再看经营口径：收入 {revenue:.2f} 元、成本及费用 {cost:.2f} 元、利润 {profit:.2f} 元。",
            "原因": "默认先展示现金收付，再补经营确认结果，避免把到账/付款和经营利润混成一个口径。",
            "建议": "如果要继续追差异，请直接问利润和现金流为什么不一致。",
        }
    if metric and "money_value" in data and "account_value" in data:
        return {
            "结论": f"{period}{metric}：现金口径 {float(data.get('money_value', 0)):.2f} 元，经营口径 {float(data.get('account_value', 0)):.2f} 元。",
            "原因": "两边差异通常来自确认时点、预提和冲回。",
            "建议": "优先盯回款与大额支出节奏，避免下月利润波动。",
        }
    if metric and "account_value" in data:
        return {
            "结论": f"{period}{metric}约 {float(data.get('account_value', 0)):.2f} 元。",
            "原因": "当前结果只落在经营口径；若该问题走双视角，会先展示现金口径，再补经营口径。",
            "建议": "如果要继续核对现金收付，请直接追问到账、付款或现金流。",
        }

    if "suppliers" in data:
        suppliers = data.get("suppliers") or []
        top = suppliers[0]["name"] if suppliers else "暂无"
        top_amount = float(suppliers[0].get("out_amount", 0)) if suppliers else 0
        return {
            "结论": f"{period}外部供应商付款共 {data.get('count', 0)} 家，合计 {float(data.get('total', 0)):.2f} 元。",
            "原因": f"已按期间内银行实际付款统计，并剔除员工、内部往来、税费和手续费等非供应商项；付款额最高的是 {top} {top_amount:.2f} 元。",
            "建议": "优先核对前五大供应商付款、发票和应付冲销是否一致。",
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
        "skill_contract_version": SKILL_CONTRACT_VERSION,
        "protocol_version": BRIDGE_PROTOCOL_VERSION,
        "generated_at": now_utc_iso(),
        "query": query,
        "company": DEFAULT_COMPANY,
        "db": str(FINANCEQA_DB),
        "skill_path": str(FINANCEQA_SKILL_PATH),
        "skill_appendix_relative_path": SKILL_APPENDIX_RELATIVE_PATH,
        "skill_appendix_path": str(SKILL_APPENDIX_PATH),
        "skill_appendix_exists": SKILL_APPENDIX_PATH.exists(),
        "capabilities": {
            "trace": True,
            "answer_method": True,
            "llm_fallback": True,
            "exposed_fields": [
                "dual_perspective",
                "hr_breakdown",
                "arithmetic_checks",
                "intent_trace",
            ],
        },
    }
    return payload


def run_finance_query(question):
    cmd = [
        str(FINANCEQA_BIN),
        "query",
        "--db", str(FINANCEQA_DB),
    ]
    if DEFAULT_COMPANY:
        cmd.extend(["--company", DEFAULT_COMPANY])
    cmd.append(question)
    proc = run_cmd(cmd)

    if proc.returncode == 0:
        payload = parse_json_or_none(proc.stdout) or {"raw": (proc.stdout or "").strip()}
        return build_structured_response(payload, question)

    err_msg = (proc.stderr or proc.stdout or "").strip() or "query failed"

    host_cmd = [
        str(FINANCEQA_BIN),
        "host-data",
        "--db", str(FINANCEQA_DB),
    ]
    if DEFAULT_COMPANY:
        host_cmd.extend(["--company", DEFAULT_COMPANY])
    host_cmd.append(question)
    host_proc = run_cmd(host_cmd)
    host_payload = parse_json_or_none(host_proc.stdout) or {}
    host_data = host_payload.get("data") if isinstance(host_payload, dict) else {}

    fallback = {
        "success": False,
        "message": err_msg,
        "answer_method": "llm_payload",
        "data": {
            "fallback_attempted": True,
            "hint": "请补充时间+对象+指标，例如：某年某月某客户回款多少",
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
    ]
    if DEFAULT_COMPANY:
        cmd.extend(["--company", DEFAULT_COMPANY])
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
        "skill_contract_version": SKILL_CONTRACT_VERSION,
        "protocol_version": BRIDGE_PROTOCOL_VERSION,
        "generated_at": now_utc_iso(),
        "company": DEFAULT_COMPANY,
        "db": str(FINANCEQA_DB),
        "skill_path": str(FINANCEQA_SKILL_PATH),
        "skill_appendix_relative_path": SKILL_APPENDIX_RELATIVE_PATH,
        "skill_appendix_path": str(SKILL_APPENDIX_PATH),
        "skill_appendix_exists": SKILL_APPENDIX_PATH.exists(),
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
