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
from urllib.parse import parse_qs, urlparse

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
        "description": "老板财务问答（优先原样使用 final_answer；附结构化结果、过程追踪和兜底数据包）",
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
    },
    {
        "name": "finance-sync",
        "description": "批量同步目录下的财务文件到数据库",
        "inputSchema": {
            "type": "object",
            "properties": {
                "directoryPath": {"type": "string", "description": "待同步目录路径"},
                "incremental": {"type": "boolean", "description": "是否增量同步"},
                "company": {"type": "string", "description": "覆盖导入公司名（可选）"},
            },
            "required": ["directoryPath"]
        }
    },
    {
        "name": "finance-dimensions",
        "description": "维度管理工具，承载 dimensions 子命令",
        "inputSchema": {
            "type": "object",
            "properties": {
                "subcommand": {"type": "string", "description": "dimensions 子命令"},
                "code": {"type": "string", "description": "dimension/member code"},
                "name": {"type": "string", "description": "dimension/member name"},
                "type": {"type": "string", "description": "dimension type 或 preview type"},
                "hierarchical": {"type": "boolean", "description": "是否层级维度"},
                "dimension": {"type": "string", "description": "dimension code"},
                "company": {"type": "string", "description": "company（可选）"},
                "outputPath": {"type": "string", "description": "导出文件路径"},
                "filePath": {"type": "string", "description": "导入/预览文件路径"},
                "format": {"type": "string", "description": "格式，默认 json"},
                "validateOnly": {"type": "boolean", "description": "仅校验不写入"},
                "skipExisting": {"type": "boolean", "description": "跳过已存在记录"},
                "updateExisting": {"type": "boolean", "description": "更新已存在记录"}
            },
            "required": ["subcommand"]
        }
    }
]

DIMENSIONS_SUBCOMMAND_SPECS = {
    "list": {"args": []},
    "add-dimension": {
        "args": [
            {"name": "code", "flag": "--code", "required": True},
            {"name": "name", "flag": "--name", "required": True},
            {"name": "type", "flag": "--type"},
            {"name": "hierarchical", "flag": "--hierarchical", "type": "bool"},
        ],
    },
    "add-member": {
        "args": [
            {"name": "dimension", "flag": "--dimension", "required": True},
            {"name": "code", "flag": "--code", "required": True},
            {"name": "name", "flag": "--name", "required": True},
        ],
    },
    "mapping-stats": {
        "args": [
            {"name": "company", "flag": "--company"},
        ],
    },
    "seed-standard": {
        "args": [
            {"name": "company", "flag": "--company", "required": True},
        ],
    },
    "export-package": {
        "args": [
            {"name": "outputPath", "flag": "--output", "required": True},
            {"name": "format", "flag": "--format"},
        ],
    },
    "import-dimensions": {
        "args": [
            {"name": "filePath", "flag": "--file", "required": True},
            {"name": "format", "flag": "--format"},
            {"name": "validateOnly", "flag": "--validate-only", "type": "bool"},
            {"name": "skipExisting", "flag": "--skip-existing", "type": "bool"},
            {"name": "updateExisting", "flag": "--update-existing", "type": "bool"},
        ],
    },
    "import-members": {
        "args": [
            {"name": "dimension", "flag": "--dimension", "required": True},
            {"name": "filePath", "flag": "--file", "required": True},
            {"name": "format", "flag": "--format"},
            {"name": "validateOnly", "flag": "--validate-only", "type": "bool"},
            {"name": "skipExisting", "flag": "--skip-existing", "type": "bool"},
            {"name": "updateExisting", "flag": "--update-existing", "type": "bool"},
        ],
    },
    "import-rules": {
        "args": [
            {"name": "filePath", "flag": "--file", "required": True},
            {"name": "company", "flag": "--company"},
            {"name": "format", "flag": "--format"},
            {"name": "validateOnly", "flag": "--validate-only", "type": "bool"},
            {"name": "skipExisting", "flag": "--skip-existing", "type": "bool"},
            {"name": "updateExisting", "flag": "--update-existing", "type": "bool"},
        ],
    },
    "preview-import": {
        "args": [
            {"name": "type", "flag": "--type", "required": True},
            {"name": "dimension", "flag": "--dimension"},
            {"name": "filePath", "flag": "--file", "required": True},
            {"name": "format", "flag": "--format"},
        ],
    },
}


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


def bridge_capabilities():
    return {
        "boss_reply": True,
        "final_answer": True,
        "contract_summary": True,
        "supplier_payment_summary": True,
        "trace": True,
        "answer_method": True,
        "llm_fallback": True,
        "tax_disclosure": True,
        "route_decision": True,
        "probe_results": True,
        "source_attribution": True,
        "contract_strict_missing": True,
        "exposed_tools": [
            "finance-query",
            "finance-host-data",
            "finance-upload",
            "finance-sync",
            "finance-dimensions",
        ],
        "result_structures": [
            "boss_reply",
            "host_summary_contract",
            "host_summary_supplier_payments",
            "route_decision",
        ],
        "exposed_fields": [
            "dual_perspective",
            "hr_breakdown",
            "arithmetic_checks",
            "intent_trace",
            "tax_inclusion",
            "tax_inclusion_note",
            "route_decision",
            "source_documents",
            "source_note",
            "source_summary",
            "contract_fallback_reason",
            "contract_answer_status",
        ],
    }


def build_bridge_meta(query=None, tool_name=None, tool_operation=None):
    meta = {
        "skill_contract_version": SKILL_CONTRACT_VERSION,
        "protocol_version": BRIDGE_PROTOCOL_VERSION,
        "generated_at": now_utc_iso(),
        "company": DEFAULT_COMPANY,
        "db": summarize_db_target(FINANCEQA_DB),
        "skill_path": str(FINANCEQA_SKILL_PATH),
        "skill_appendix_relative_path": SKILL_APPENDIX_RELATIVE_PATH,
        "skill_appendix_path": str(SKILL_APPENDIX_PATH),
        "skill_appendix_exists": SKILL_APPENDIX_PATH.exists(),
        "capabilities": bridge_capabilities(),
    }
    if query is not None:
        meta["query"] = query
    if tool_name:
        meta["tool_name"] = tool_name
    if tool_operation:
        meta["tool_operation"] = tool_operation
    return meta


def normalize_cli_command_response(stdout_text, tool_name, tool_operation):
    parsed = parse_json_or_none(stdout_text)
    if isinstance(parsed, dict):
        payload = parsed
        payload.setdefault("success", True)
        payload.setdefault("message", "")
        payload.setdefault("answer_method", "cli_json")
    elif isinstance(parsed, list):
        payload = {
            "success": True,
            "message": "",
            "answer_method": "cli_json",
            "data": parsed,
        }
    else:
        payload = {
            "success": True,
            "message": (stdout_text or "").strip(),
            "answer_method": "cli_text",
            "data": {"stdout": (stdout_text or "").strip()},
        }
    payload["tool_name"] = tool_name
    payload["bridge_meta"] = build_bridge_meta(tool_name=tool_name, tool_operation=tool_operation)
    return payload


def append_optional_flag(cmd, flag, value):
    text = str(value or "").strip()
    if text:
        cmd.extend([flag, text])


def append_optional_bool_flag(cmd, flag, value):
    if bool(value):
        cmd.append(flag)


def build_dimensions_command(arguments):
    subcommand = str((arguments or {}).get("subcommand") or "").strip()
    if not subcommand:
        raise ValueError("subcommand is required")
    spec = DIMENSIONS_SUBCOMMAND_SPECS.get(subcommand)
    if spec is None:
        raise ValueError(f"unsupported dimensions subcommand: {subcommand}")

    cmd = [
        str(FINANCEQA_BIN),
        "dimensions",
        subcommand,
        "--db", str(FINANCEQA_DB),
    ]
    for arg in spec.get("args") or []:
        name = arg["name"]
        flag = arg["flag"]
        required = bool(arg.get("required"))
        arg_type = arg.get("type") or "string"
        value = (arguments or {}).get(name)
        if arg_type == "bool":
            append_optional_bool_flag(cmd, flag, value)
            continue
        text = str(value or "").strip()
        if required and not text:
            raise ValueError(f"{name} is required for dimensions:{subcommand}")
        append_optional_flag(cmd, flag, text)

    preview_type = str((arguments or {}).get("type") or "").strip()
    if subcommand == "preview-import" and preview_type == "members":
        dimension = str((arguments or {}).get("dimension") or "").strip()
        if not dimension:
            raise ValueError("dimension is required for dimensions:preview-import when type=members")
    return cmd, f"dimensions:{subcommand}"


def summarize_db_target(db_target):
    text = str(db_target or "").strip()
    if not text:
        return ""

    lowered = text.lower()
    if text in (":memory:", "file::memory:?cache=shared"):
        return "sqlite(memory)"

    is_pg_kv = "host=" in lowered and "dbname=" in lowered
    is_pg_url = lowered.startswith("postgres://") or lowered.startswith("postgresql://")
    if is_pg_kv or is_pg_url:
        schema = ""
        if is_pg_kv:
            match = re.search(r"(?:^|\s)search_path=([^\s]+)", text)
            if match:
                schema = match.group(1).split(",", 1)[0].strip()
        else:
            parsed = urlparse(text)
            query = parse_qs(parsed.query)
            search_path = (query.get("search_path") or [""])[0]
            if search_path:
                schema = search_path.split(",", 1)[0].strip()
        if schema:
            return f"postgresql(schema={schema})"
        return "postgresql"

    if lowered.endswith((".db", ".sqlite", ".sqlite3")) or "/" in text or "\\" in text:
        return "sqlite(local)"

    return "configured"


SOURCE_TABLE_DOCUMENT_LABELS = [
    ("tenant_uhub.fin_fund_income", "《优集资金收入计算表-副本.xlsx》"),
    ("tenant_uhub.fin_cost_settlements", "《优集成本计算表-4.23-池.xlsx》"),
    ("tenant_uhub.fin_contracts", "《合同信息表》"),
    ("tenant_uhub.fin_revenue_settlements", "《收入结算表（已废弃）》"),
    ("tenant_uhub.bank_statement", "《银行流水》"),
    ("tenant_uhub.journal", "《序时账》"),
    ("tenant_uhub.balance_detail", "《科目余额表》"),
    ("tenant_uhub.balance_sheet", "《资产负债表》"),
    ("tenant_uhub.income_statement", "《利润表》"),
    ("fin_fund_income", "《优集资金收入计算表-副本.xlsx》"),
    ("fin_cost_settlements", "《优集成本计算表-4.23-池.xlsx》"),
    ("fin_contracts", "《合同信息表》"),
    ("fin_revenue_settlements", "《收入结算表（已废弃）》"),
    ("bank_statement", "《银行流水》"),
    ("journal", "《序时账》"),
    ("balance_detail", "《科目余额表》"),
    ("balance_sheet", "《资产负债表》"),
    ("income_statement", "《利润表》"),
]

HIDDEN_RESPONSE_KEYS = {
    "id",
    "contract_id",
    "contract_ids",
    "account_code",
    "account_codes",
    "subject_code",
    "subject_codes",
    "source_report_type",
    "source_sheet_name",
    "counterparty_account",
    "counterparty_account_no",
    "counterparty_bank_account",
    "bank_account",
    "bank_account_no",
    "account_no",
    "rowid",
    "row_id",
    "executed_sql",
    "sql",
    "raw_sql",
    "query_sql",
}

TECHNICAL_TRACE_MARKERS = (
    "select ",
    " from ",
    " join ",
    " where ",
    "account_code",
    "contract_id",
    "source_report_type",
    "source_sheet_name",
)


def source_label_for_table(value):
    text = str(value or "").strip()
    if not text:
        return ""
    for raw, label in SOURCE_TABLE_DOCUMENT_LABELS:
        if raw == text or text.endswith("." + raw) or raw in text:
            return label
    return humanize_source_text(text)


def humanize_source_tables(values):
    labels = []
    for item in values or []:
        label = source_label_for_table(item)
        if label and label not in labels:
            labels.append(label)
    return labels


def humanize_source_text(value):
    text = str(value or "")
    for raw, label in SOURCE_TABLE_DOCUMENT_LABELS:
        if "." in raw:
            text = text.replace(raw, label)
            continue
        pattern = re.compile(r"(?<![A-Za-z0-9_.])" + re.escape(raw) + r"(?![A-Za-z0-9_])")
        text = pattern.sub(label, text)
    return text


def sanitize_bridge_string(value):
    text = humanize_source_text(value)
    lowered = text.lower()
    if any(marker in lowered for marker in TECHNICAL_TRACE_MARKERS):
        return "[内部计算过程已隐藏]"
    text = re.sub(r"\bC\d{3,}\b", "[合同辅助编号已隐藏]", text)
    return text


def sanitize_bridge_payload(value, key_hint=""):
    if isinstance(value, dict):
        cleaned = {}
        for key, item in value.items():
            key_text = str(key or "")
            key_lower = key_text.lower()
            if key_lower in HIDDEN_RESPONSE_KEYS or key_lower.endswith("_id"):
                continue
            if key_lower in ("source_tables", "primary_tables"):
                labels = humanize_source_tables(item if isinstance(item, list) else [item])
                if labels:
                    cleaned[key_text] = labels
                    cleaned.setdefault("source_documents", labels)
                continue
            cleaned_item = sanitize_bridge_payload(item, key_lower)
            if key_lower == "source_documents" and not cleaned_item and cleaned.get("source_documents"):
                continue
            cleaned[key_text] = cleaned_item
        return cleaned
    if isinstance(value, list):
        return [sanitize_bridge_payload(item, key_hint) for item in value]
    if isinstance(value, str):
        return sanitize_bridge_string(value)
    return value


def normalize_exposed_fields(data):
    data = data or {}
    exposed = {
        "dual_perspective": data.get("dual_perspective"),
        "hr_breakdown": data.get("hr_breakdown"),
        "arithmetic_checks": data.get("arithmetic_checks"),
        "intent_trace": data.get("intent_trace"),
        "tax_inclusion": data.get("tax_inclusion"),
        "tax_inclusion_note": data.get("tax_inclusion_note"),
        "route_decision": data.get("route_decision"),
        "source_documents": data.get("source_documents") or [],
        "source_note": data.get("source_note") or "",
        "source_summary": data.get("source_summary") or "",
        "contract_fallback_reason": data.get("contract_fallback_reason") or "",
        "contract_answer_status": data.get("contract_answer_status") or "",
        "contract_continuity_candidates": data.get("contract_continuity_candidates") or [],
        "contract_continuity_note": data.get("contract_continuity_note") or "",
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


def first_float(*values):
    for value in values:
        if value is None or value == "":
            continue
        try:
            return float(value)
        except (TypeError, ValueError):
            continue
    return None


def is_receipt_question(query):
    text = (query or "").strip()
    if not text:
        return False
    keywords = ("回款", "到账", "收款")
    return any(keyword in text for keyword in keywords)


def is_contract_strict_missing(data):
    status = str((data or {}).get("contract_answer_status") or "").strip()
    source_priority = str((data or {}).get("source_priority") or "").strip()
    return status == "missing" or (
        bool((data or {}).get("contract_source_required"))
        and source_priority == "contract_strict"
    )


def build_contract_strict_missing_summary(data):
    data = data or {}
    source_tables = data.get("source_tables") or data.get("source_primary_tables") or []
    if not isinstance(source_tables, list):
        source_tables = [source_tables]
    source_documents = data.get("source_documents") or humanize_source_tables(source_tables)
    reason = str(data.get("contract_fallback_reason") or data.get("message") or "").strip()
    if not reason:
        reason = "合同/项目台账在请求期间没有足够记录"
    source_note = str(data.get("source_note") or data.get("source_summary") or "").strip()
    if not source_note and source_documents:
        source_note = "本次尝试的合同口径来源：" + "、".join(str(item) for item in source_documents if item)
    return {
        "kind": "contract_strict_missing",
        "period": data.get("period"),
        "entity": data.get("entity") or "",
        "reason": reason,
        "source_note": source_note,
        "source_documents": source_documents,
        "continuity_candidates": data.get("contract_continuity_candidates") or [],
        "continuity_note": data.get("contract_continuity_note") or "",
        "safe_to_quote_message": True,
    }


def build_host_summary_contract(payload, query):
    data = payload.get("data") or {}
    if payload.get("answer_method") == "llm_payload" or not payload.get("success"):
        return None

    if is_contract_strict_missing(data):
        return build_contract_strict_missing_summary(data)

    query_spec = data.get("query_spec") or {}
    query_family = str(query_spec.get("query_family") or "").strip()
    asked_topic = str(data.get("asked_topic") or "").strip()
    if query_family == "contract_dimension":
        entity = str(
            data.get("entity")
            or data.get("counterparty")
            or data.get("name")
            or ""
        ).strip()
        contract = {
            "kind": "contract_dimension",
            "entity": entity,
            "role": data.get("role"),
            "period": data.get("period"),
            "asked_topic": asked_topic or "generic",
            "contracts": data.get("contracts") or [],
            "cash_view": data.get("cash_view") or data.get("money_view") or {},
            "book_view": data.get("book_view") or data.get("account_view") or {},
            "source_tables": data.get("source_tables") or [],
            "source_summary": data.get("source_summary") or "",
            "source_note": data.get("source_note") or "",
            "source_documents": data.get("source_documents") or [],
            "route_decision": data.get("route_decision") or (query_spec.get("route_decision") if isinstance(query_spec, dict) else {}),
            "contract_fallback_reason": data.get("contract_fallback_reason") or "",
            "safe_to_quote_message": True,
        }
        if "sub_period" in data:
            contract["sub_period"] = data.get("sub_period")
        if "sub_period_receipts" in data:
            contract["sub_period_receipts"] = data.get("sub_period_receipts")
        return contract

    if query_family == "core_metric" and str(data.get("source_priority") or "").strip() == "contract_first":
        requested_metrics = data.get("requested_metrics") or []
        if isinstance(requested_metrics, tuple):
            requested_metrics = list(requested_metrics)
        if not isinstance(requested_metrics, list):
            requested_metrics = []
        contract_summary = data.get("contract_summary") or {}
        contract = {
            "kind": "contract_aggregate",
            "entity": str(contract_summary.get("entity") or data.get("entity") or "").strip(),
            "period": data.get("period"),
            "metric": data.get("metric"),
            "requested_metrics": requested_metrics,
            "cash_view": data.get("money_view") or {},
            "book_view": data.get("account_view") or {},
            "contract_summary": contract_summary,
            "source_tables": data.get("source_tables") or [],
            "source_summary": data.get("source_summary") or "",
            "source_note": data.get("source_note") or "",
            "source_documents": data.get("source_documents") or [],
            "route_decision": data.get("route_decision") or (query_spec.get("route_decision") if isinstance(query_spec, dict) else {}),
            "contract_fallback_reason": data.get("contract_fallback_reason") or "",
            "safe_to_quote_message": True,
        }
        return contract

    total_amount = first_float(
        data.get("amount"),
        data.get("total"),
        data.get("bank_in"),
    )
    sub_period = str(data.get("sub_period") or "").strip()
    sub_period_amount = first_float(data.get("sub_period_receipts"))

    if not is_receipt_question(query):
        return None
    if total_amount is None or not sub_period or sub_period_amount is None:
        return None

    entity = str(
        data.get("entity")
        or data.get("counterparty")
        or data.get("name")
        or ""
    ).strip()
    contract = {
        "kind": "counterparty_receipts_with_subperiod",
        "total_amount": total_amount,
        "sub_period": sub_period,
        "sub_period_amount": sub_period_amount,
        "safe_to_quote_message": True,
    }
    if entity:
        contract["entity"] = entity
    if data.get("role"):
        contract["role"] = data.get("role")
    if data.get("comparison_basis"):
        contract["comparison_basis"] = data.get("comparison_basis")
    return contract


def build_host_summary_supplier_payments(payload, query):
    data = payload.get("data") or {}
    if payload.get("answer_method") == "llm_payload" or not payload.get("success"):
        return None

    suppliers = data.get("suppliers") or []
    if not isinstance(suppliers, list) or not suppliers:
        return None

    excluded = data.get("excluded_counterparties") or []
    if not isinstance(excluded, list):
        excluded = []

    executed_sql = payload.get("executed_sql") or []
    calculation_logs = payload.get("calculation_logs") or []
    source_tables = []
    for text in list(executed_sql) + list(calculation_logs):
        line = str(text or "")
        if "bank_statement" in line and "bank_statement" not in source_tables:
            source_tables.append("bank_statement")
        if (
            "collectCounterpartyEvidence" in line or "journal" in line
        ) and "journal" not in source_tables:
            source_tables.append("journal")

    evidence_used = any((item or {}).get("signals") for item in suppliers + excluded)
    if not evidence_used:
        evidence_used = any(
            "collectCounterpartyEvidence" in str(text or "") or "journal" in str(text or "")
            for text in list(executed_sql) + list(calculation_logs)
        )

    exclusion_reasons = []
    for item in excluded:
        reason = str((item or {}).get("exclude_reason") or "").strip()
        if reason and reason not in exclusion_reasons:
            exclusion_reasons.append(reason)

    top_supplier = suppliers[0] if suppliers else {}
    return {
        "kind": "supplier_payments_period_summary",
        "period": data.get("period"),
        "count": data.get("count"),
        "total": first_float(data.get("total"), data.get("amount")),
        "suppliers": suppliers,
        "top_supplier": top_supplier,
        "excluded_counterparties": excluded,
        "exclusion_reasons": exclusion_reasons,
        "supporting_evidence_used": evidence_used,
        "source_tables": source_tables,
        "source_summary": data.get("source_summary") or "",
        "source_note": data.get("source_note") or "",
        "source_documents": data.get("source_documents") or [],
        "safe_to_quote_message": True,
    }


def build_boss_reply(payload, query):
    data = payload.get("data") or {}
    msg = payload.get("message") or ""
    summary_contract = build_host_summary_contract(payload, query)
    summary_supplier = build_host_summary_supplier_payments(payload, query)
    tax_inclusion = str(data.get("tax_inclusion") or "").strip()
    tax_inclusion_note = str(data.get("tax_inclusion_note") or "").strip()
    source_note = str(data.get("source_summary") or data.get("source_note") or "").strip()

    def append_tax_note(reason):
        reason = str(reason or "").strip()
        if not tax_inclusion_note:
            return reason
        if tax_inclusion_note in reason:
            return reason
        if not reason:
            return tax_inclusion_note
        return f"{reason} {tax_inclusion_note}"

    def append_source_note(reason, source_hint=None):
        reason = str(reason or "").strip()
        note = str(source_hint or source_note or "").strip()
        if not note:
            return reason
        if note in reason:
            return reason
        if not reason:
            return note
        return f"{reason} {note}"

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

    if summary_contract and summary_contract.get("kind") == "contract_strict_missing":
        period = summary_contract.get("period") or data.get("period") or "当前期间"
        entity = str(summary_contract.get("entity") or "").strip()
        entity_prefix = f"{entity} " if entity else ""
        reason = str(summary_contract.get("reason") or "合同/项目台账在请求期间没有足够记录").strip()
        continuity_candidates = summary_contract.get("continuity_candidates") or []
        if isinstance(continuity_candidates, list) and continuity_candidates:
            detail_lines = []
            total_received = 0.0
            entity_received = {}
            normalized_candidates = []
            for item in continuity_candidates:
                if not isinstance(item, dict):
                    continue
                candidate_entity = str(item.get("candidate_entity") or "").strip()
                received = float(first_float(item.get("candidate_received_amount"), 0) or 0)
                total_received += received
                if candidate_entity:
                    entity_received[candidate_entity] = entity_received.get(candidate_entity, 0.0) + received
                normalized_candidates.append(item)
            top_entity = ""
            if entity_received:
                top_entity = max(entity_received.items(), key=lambda pair: pair[1])[0]
            for item in normalized_candidates[:5]:
                candidate_entity = str(item.get("candidate_entity") or "").strip()
                contract_content = str(item.get("contract_content") or "").strip()
                received = float(first_float(item.get("candidate_received_amount"), 0) or 0)
                settlement = float(first_float(item.get("candidate_settlement_amount"), 0) or 0)
                invoice = float(first_float(item.get("candidate_invoice_amount"), 0) or 0)
                label = " - ".join(part for part in (candidate_entity, contract_content) if part)
                if label:
                    detail_lines.append(f"{label}：合同结算 {settlement:.2f} 元，到账 {received:.2f} 元，开票 {invoice:.2f} 元")
            detail_text = "；".join(detail_lines)
            hidden_count = max(len(normalized_candidates) - len(detail_lines), 0)
            if hidden_count > 0:
                detail_text = detail_text + f"；另有 {hidden_count} 条候选未展开"
            if detail_text:
                detail_text = "候选：" + detail_text + "。"
            guess_subject = f"疑似可按 {top_entity} 的同名项目参考" if top_entity else "疑似可按同名项目参考"
            return {
                "结论": f"{entity_prefix}{period} 原主体合同台账无记录；但发现历史同名项目在当前期间挂到其他主体名下，{guess_subject}，候选到账合计 {total_received:.2f} 元。",
                "原因": append_source_note(
                    f"这不是固定映射表判断，而是按同名合同/项目跨期间连续性给出的推断候选；{detail_text}",
                    summary_contract.get("source_note"),
                ),
                "建议": "可以先按这些候选给老板一个“疑似承继主体/项目连续性”的参考答案，并明确这是推断，不把它写成确定主体映射。",
            }
        return {
            "结论": f"{entity_prefix}{period} 合同口径当前不能直接回答：{reason}。",
            "原因": append_source_note(
                "系统已经按合同/项目台账优先探测，但没有足够记录支撑确定性金额；为避免把非老板口径冒充合同口径，本次未自动切到财务账或银行流水。",
                summary_contract.get("source_note"),
            ),
            "建议": "如果你要看非合同口径，请明确说“账上/科目余额/资产负债表/序时账”，或明确说“银行流水/实际到账/实际支出”。",
        }

    if summary_contract and summary_contract.get("kind") == "counterparty_receipts_with_subperiod":
        entity = summary_contract.get("entity") or "该主体"
        total_amount = float(summary_contract.get("total_amount", 0))
        sub_period = summary_contract.get("sub_period") or "当前子期间"
        sub_period_amount = float(summary_contract.get("sub_period_amount", 0))
        return {
            "结论": f"{entity}累计回款 {total_amount:.2f} 元，其中 {sub_period} 到账 {sub_period_amount:.2f} 元。",
            "原因": "累计回款和子期间到账是两个层次的金额，宿主应直接引用后端拆分结果，不能把子期间到账误当成累计值。",
            "建议": "如果还要继续判断这些回款对应哪些月份收入，请再追问应收配对或历史回款归属。",
        }

    if summary_contract and summary_contract.get("kind") == "contract_dimension":
        entity = summary_contract.get("entity") or "该合同主体"
        period = summary_contract.get("period") or "当前期间"
        asked_topic = summary_contract.get("asked_topic") or "generic"
        role = summary_contract.get("role") or ""
        cash_view = summary_contract.get("cash_view") or {}
        book_view = summary_contract.get("book_view") or {}
        contracts = summary_contract.get("contracts") or []
        source_tables = summary_contract.get("source_tables") or []

        def source_reason(default_reason):
            if summary_contract.get("source_summary") or summary_contract.get("source_note"):
                return append_source_note(default_reason, summary_contract.get("source_summary") or summary_contract.get("source_note"))
            if not source_tables:
                return append_source_note(default_reason)
            joined = " + ".join(str(item) for item in source_tables if item)
            if not joined:
                return append_source_note(default_reason)
            return append_source_note(f"{default_reason} 本次口径来自 {joined}。")

        if asked_topic == "content":
            contents = []
            seen = set()
            for item in contracts:
                content = str((item or {}).get("contract_content") or "").strip()
                if not content or content in seen:
                    continue
                seen.add(content)
                contents.append(content)
            joined = "；".join(contents) if contents else "暂无明确合同内容"
            return {
                "结论": f"{entity}匹配到的合同内容：{joined}。",
                "原因": source_reason("这是直接从合同台账命中的合同内容，不再通过普通往来或流水摘要二次猜测。"),
                "建议": "如果还要看某一份合同的营收、回款或成本，再补充年份或月份即可。",
            }

        if asked_topic == "profit":
            if role == "mixed_contract":
                received = float(first_float(cash_view.get("received_amount"), 0) or 0)
                paid = float(first_float(cash_view.get("cash_paid_amount"), 0) or 0)
                revenue = float(first_float(book_view.get("revenue_settlement"), 0) or 0)
                cost = float(first_float(book_view.get("cost_settlement"), 0) or 0)
                return {
                    "结论": f"{entity}{period}先看现金口径：净回款 {received - paid:.2f} 元；再看经营口径：合同利润 {revenue - cost:.2f} 元。",
                    "原因": source_reason("合同利润和净回款都直接来自合同维度结果，优先使用 fin_fund_income 与 fin_cost_settlements，不退回普通往来口径。"),
                    "建议": "如果要继续拆利润差异，可以再追问具体是哪些合同或哪几个月形成的成本。",
                }
            received = float(first_float(cash_view.get("received_amount"), 0) or 0)
            settlement = float(first_float(book_view.get("settlement_amount"), 0) or 0)
            invoice = float(first_float(book_view.get("invoice_amount"), 0) or 0)
            if role == "customer_contract":
                return {
                    "结论": f"{entity}{period}当前合同台账只匹配到收入/回款，暂不能直接给完整合同利润；现金口径到账 {received:.2f} 元，经营口径结算 {settlement:.2f} 元、开票 {invoice:.2f} 元。",
                    "原因": source_reason("老板口径下，合同利润必须优先基于 fin_fund_income + fin_cost_settlements；当前未匹配到合同成本时，宿主不能擅自用普通利润口径替代。"),
                    "建议": "如果要形成完整合同利润，请继续补该主体对应的合同成本或供应商合同匹配。",
                }
            paid = float(first_float(cash_view.get("cash_paid_amount"), 0) or 0)
            contract_cost = float(first_float(book_view.get("contract_cost"), 0) or 0)
            return {
                "结论": f"{entity}{period}这是供应商合同，只有成本没有营收；现金口径付款 {paid:.2f} 元，经营口径合同成本 {contract_cost:.2f} 元。",
                "原因": source_reason("供应商合同不应被宿主误总结成营收或利润。"),
                "建议": "如需完整利润，请改查对应客户合同或混合合同主体。",
            }

        if asked_topic in ("revenue", "receipts", "generic"):
            received = float(first_float(cash_view.get("received_amount"), 0) or 0)
            settlement = float(first_float(book_view.get("settlement_amount"), book_view.get("revenue_settlement"), 0) or 0)
            invoice = float(first_float(book_view.get("invoice_amount"), 0) or 0)
            if summary_contract.get("sub_period") and summary_contract.get("sub_period_receipts") is not None:
                return {
                    "结论": f"{entity}{period}累计合同回款 {received:.2f} 元，其中 {summary_contract.get('sub_period')} 到账 {float(summary_contract.get('sub_period_receipts') or 0):.2f} 元；经营口径合同结算 {settlement:.2f} 元，开票 {invoice:.2f} 元。",
                    "原因": source_reason("合同回款与子期间到账都直接引用合同维度拆分结果，不再混回普通往来累计口径。"),
                    "建议": "如果还要看对应合同内容或未回款部分，可以继续追问具体合同和月份。",
                }
            return {
                "结论": f"{entity}{period}先看现金口径：合同到账 {received:.2f} 元；再看经营口径：合同结算 {settlement:.2f} 元，开票 {invoice:.2f} 元。",
                "原因": source_reason("合同营收优先引用 fin_fund_income，不再退回普通往来收款或总账收入口径。"),
                "建议": "如果你还要看合同利润或合同成本，可以直接继续追问。",
            }

        if asked_topic in ("cost", "payments"):
            paid = float(first_float(cash_view.get("cash_paid_amount"), 0) or 0)
            contract_cost = float(first_float(book_view.get("contract_cost"), book_view.get("cost_settlement"), 0) or 0)
            return {
                "结论": f"{entity}{period}先看现金口径：合同付款 {paid:.2f} 元；再看经营口径：合同成本 {contract_cost:.2f} 元。",
                "原因": source_reason("合同成本优先引用 fin_cost_settlements，不再退回普通供应商付款统计。"),
                "建议": "如果还要判断是否已经开票或是否形成应付，请继续追问合同应付或发票状态。",
            }

    if summary_contract and summary_contract.get("kind") == "contract_aggregate":
        period = summary_contract.get("period") or "当前期间"
        requested = summary_contract.get("requested_metrics") or []
        if isinstance(requested, tuple):
            requested = list(requested)
        if not isinstance(requested, list):
            requested = []
        metric = str(summary_contract.get("metric") or "").strip()
        cash_view = summary_contract.get("cash_view") or {}
        book_view = summary_contract.get("book_view") or {}
        source_tables = summary_contract.get("source_tables") or []

        def source_reason(default_reason):
            if summary_contract.get("source_summary") or summary_contract.get("source_note"):
                return append_source_note(default_reason, summary_contract.get("source_summary") or summary_contract.get("source_note"))
            if not source_tables:
                return append_source_note(default_reason)
            joined = " + ".join(str(item) for item in source_tables if item)
            if not joined:
                return append_source_note(default_reason)
            return append_source_note(f"{default_reason} 本次口径来自 {joined}。")

        requested_set = {str(item).strip() for item in requested if str(item).strip()}
        if not requested_set and metric:
            requested_set.add(metric)

        if requested_set == {"应收"} or requested_set == {"应收账款"}:
            contract_summary = summary_contract.get("contract_summary") or {}
            received = float(first_float(cash_view.get("已到账"), cash_view.get("到账"), cash_view.get("回款"), contract_summary.get("revenue_received"), 0) or 0)
            settlement = float(first_float(book_view.get("营收"), contract_summary.get("revenue_settlement"), 0) or 0)
            receivable = float(first_float(book_view.get("合同应收"), contract_summary.get("receivable_amount"), 0) or 0)
            invoice_open = float(first_float(book_view.get("已开票未回款"), contract_summary.get("invoiced_unreceived_amount"), 0) or 0)
            return {
                "结论": f"{period}按合同/项目老板口径：合同应收 {receivable:.2f} 元。",
                "原因": source_reason(f"合同应收来自收入计算表的结算额与到账额差额；补充看，合同结算 {settlement:.2f} 元、已到账 {received:.2f} 元，其中已开票未回款 {invoice_open:.2f} 元。"),
                "建议": "如果要继续追明细，优先按客户/项目拆合同应收和已开票未回款。",
            }

        if requested_set == {"应付"} or requested_set == {"应付账款"}:
            contract_summary = summary_contract.get("contract_summary") or {}
            paid = float(first_float(cash_view.get("已付款"), cash_view.get("付款"), contract_summary.get("cost_paid"), 0) or 0)
            cost = float(first_float(book_view.get("合同成本"), contract_summary.get("cost_settlement"), 0) or 0)
            payable = float(first_float(book_view.get("合同应付"), contract_summary.get("payable_amount"), 0) or 0)
            invoice_open = float(first_float(book_view.get("已收票未付款"), contract_summary.get("invoiced_unpaid_amount"), 0) or 0)
            return {
                "结论": f"{period}按合同/项目老板口径：合同应付 {payable:.2f} 元。",
                "原因": source_reason(f"合同应付来自成本计算表的合同成本与已付款差额；补充看，合同成本 {cost:.2f} 元、已付款 {paid:.2f} 元，其中已收票未付款 {invoice_open:.2f} 元。"),
                "建议": "如果要继续追付款优先级，建议按供应商/项目拆合同应付和已收票未付款。",
            }

        if requested_set in ({"已开票未回款"}, {"已开票未付款"}, {"已开票未收款"}):
            contract_summary = summary_contract.get("contract_summary") or {}
            invoice = float(first_float(book_view.get("已开票"), contract_summary.get("invoice_amount"), 0) or 0)
            received = float(first_float(cash_view.get("已到账"), cash_view.get("到账"), contract_summary.get("revenue_received"), 0) or 0)
            invoice_open = float(first_float(book_view.get("已开票未回款"), contract_summary.get("invoiced_unreceived_amount"), 0) or 0)
            detail_lines = []
            for item in (contract_summary.get("invoice_open_items") or [])[:3]:
                if not isinstance(item, dict):
                    continue
                customer_name = str(item.get("customer_name") or "").strip()
                contract_content = str(item.get("contract_content") or "").strip()
                item_label = " - ".join(part for part in (customer_name, contract_content) if part)
                if not item_label:
                    continue
                item_invoice = float(first_float(item.get("invoice_amount"), 0) or 0)
                item_received = float(first_float(item.get("received_amount"), 0) or 0)
                item_open = float(first_float(item.get("open_amount"), 0) or 0)
                detail_lines.append(
                    f"{item_label} 已开票 {item_invoice:.2f} 元，已回款 {item_received:.2f} 元，未回款 {item_open:.2f} 元"
                )
            detail_text = ""
            if detail_lines:
                detail_text = " 明细：" + "；".join(detail_lines) + "。"
            return {
                "结论": f"{period}按合同/项目老板口径：已开票未回款 {invoice_open:.2f} 元。{detail_text}",
                "原因": source_reason(f"这是收入侧口径，按收入计算表的已开票金额减已到账金额统计；补充看，已开票 {invoice:.2f} 元、已到账 {received:.2f} 元。"),
                "建议": "如果要继续催收，建议按客户/项目拆出已开票未回款明细。",
            }

        if requested_set in ({"已收票未付款"}, {"收到发票未付款"}):
            contract_summary = summary_contract.get("contract_summary") or {}
            invoice = float(first_float(book_view.get("已收票"), contract_summary.get("invoice_amount"), 0) or 0)
            paid = float(first_float(cash_view.get("已付款"), cash_view.get("付款"), contract_summary.get("cost_paid"), 0) or 0)
            invoice_open = float(first_float(book_view.get("已收票未付款"), contract_summary.get("invoiced_unpaid_amount"), 0) or 0)
            return {
                "结论": f"{period}按合同/项目老板口径：已收票未付款 {invoice_open:.2f} 元。",
                "原因": source_reason(f"这是成本侧口径，按成本计算表的已收票/开票金额减已付款金额统计；补充看，已收票/开票 {invoice:.2f} 元、已付款 {paid:.2f} 元。"),
                "建议": "如果要继续排付款计划，建议按供应商/项目拆出已收票未付款明细。",
            }

        if requested_set == {"收入"}:
            received = float(first_float(cash_view.get("到账"), cash_view.get("回款"), 0) or 0)
            revenue = float(first_float(book_view.get("营收"), 0) or 0)
            invoice = float(first_float(book_view.get("已开票"), 0) or 0)
            return {
                "结论": f"{period}先看合同现金口径：合同到账 {received:.2f} 元；再看合同经营口径：合同营收 {revenue:.2f} 元，已开票 {invoice:.2f} 元。",
                "原因": source_reason("老板汇总问题已优先走合同收入汇总，不回退普通收入表或银行流水汇总。"),
                "建议": "如果要继续看合同利润或未回款部分，可以继续追问利润、应收或具体客户/项目。",
            }

        if requested_set == {"成本"}:
            paid = float(first_float(cash_view.get("付款"), 0) or 0)
            cost = float(first_float(book_view.get("合同成本"), 0) or 0)
            return {
                "结论": f"{period}先看合同现金口径：合同付款 {paid:.2f} 元；再看合同经营口径：合同成本 {cost:.2f} 元。",
                "原因": source_reason("老板汇总问题已优先走合同成本汇总，不回退普通供应商付款统计。"),
                "建议": "如果还要核对应付、发票或供应商分布，可以继续追问成本构成或合同应付。",
            }

        if requested_set == {"利润"}:
            cash_net = float(first_float(cash_view.get("净现金"), 0) or 0)
            received = float(first_float(cash_view.get("回款"), 0) or 0)
            paid = float(first_float(cash_view.get("付款"), 0) or 0)
            profit = float(first_float(book_view.get("利润"), 0) or 0)
            return {
                "结论": f"{period}先看合同现金口径：净现金 {cash_net:.2f} 元（回款 {received:.2f} 元，付款 {paid:.2f} 元）；再看合同经营口径：合同利润 {profit:.2f} 元。",
                "原因": source_reason("老板汇总问题已优先走合同收入+成本汇总，利润不再退回普通利润表口径。"),
                "建议": "如果要继续拆利润差异，可以再追问回款、付款或具体合同清单。",
            }

        if requested_set and requested_set.issubset({"收入", "营收", "销售额", "gmv", "GMV", "成本"}):
            received = float(first_float(cash_view.get("回款"), cash_view.get("到账"), 0) or 0)
            paid = float(first_float(cash_view.get("付款"), 0) or 0)
            cash_net = float(first_float(cash_view.get("净现金"), received - paid) or 0)
            revenue = float(first_float(book_view.get("营收"), 0) or 0)
            cost = float(first_float(book_view.get("合同成本"), 0) or 0)
            return {
                "结论": f"{period}先看合同现金口径：回款 {received:.2f} 元、付款 {paid:.2f} 元、净现金 {cash_net:.2f} 元；再看合同经营口径：营收 {revenue:.2f} 元、合同成本 {cost:.2f} 元。",
                "原因": source_reason("老板汇总问题已优先走合同汇总表；本次只返回已请求的收入/成本指标，不补未请求的利润占位值。"),
                "建议": "如果还要看合同利润，可以继续追问利润；如果要拆明细，可以按客户、供应商、项目或月份继续追问。",
            }

        received = float(first_float(cash_view.get("回款"), cash_view.get("到账"), 0) or 0)
        paid = float(first_float(cash_view.get("付款"), 0) or 0)
        cash_net = float(first_float(cash_view.get("净现金"), received - paid) or 0)
        revenue = float(first_float(book_view.get("营收"), 0) or 0)
        cost = float(first_float(book_view.get("合同成本"), 0) or 0)
        profit = float(first_float(book_view.get("利润"), 0) or 0)
        return {
            "结论": f"{period}先看合同现金口径：回款 {received:.2f} 元、付款 {paid:.2f} 元、净现金 {cash_net:.2f} 元；再看合同经营口径：营收 {revenue:.2f} 元、合同成本 {cost:.2f} 元、利润 {profit:.2f} 元。",
            "原因": source_reason("老板汇总问题已优先走合同汇总表，不回退普通核心指标口径。"),
            "建议": "如果还要继续拆客户、项目或月份，请继续追问更细的合同维度问题。",
        }

    if summary_supplier and summary_supplier.get("kind") == "supplier_payments_period_summary":
        period = summary_supplier.get("period") or data.get("period") or "当前期间"
        count = int(first_float(summary_supplier.get("count"), data.get("count"), 0) or 0)
        total = float(first_float(summary_supplier.get("total"), data.get("total"), 0) or 0)
        top = summary_supplier.get("top_supplier") or {}
        top_name = str(top.get("name") or "暂无").strip() or "暂无"
        top_amount = float(first_float(top.get("out_amount"), 0) or 0)
        evidence_phrase = "，并结合对手方证据补证识别供应商" if summary_supplier.get("supporting_evidence_used") else ""
        return {
            "结论": f"{period}外部供应商付款共 {count} 家，合计 {total:.2f} 元。",
            "原因": append_source_note(
                f"已按期间内银行实际付款统计{evidence_phrase}，并剔除员工、内部往来、税费和手续费等非供应商项；付款额最高的是 {top_name} {top_amount:.2f} 元。",
                summary_supplier.get("source_summary") or summary_supplier.get("source_note"),
            ),
            "建议": "优先核对前五大供应商付款、发票和应付冲销是否一致。",
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
            "原因": append_tax_note("默认先展示现金收付，再补经营确认结果，避免把到账/付款和经营利润混成一个口径。"),
            "建议": "如果要继续追差异，请直接问利润和现金流为什么不一致。",
        }
    if metric and "money_value" in data and "account_value" in data:
        return {
            "结论": f"{period}{metric}：现金口径 {float(data.get('money_value', 0)):.2f} 元，经营口径 {float(data.get('account_value', 0)):.2f} 元。",
            "原因": append_source_note(append_tax_note("两边差异通常来自确认时点、预提和冲回。")),
            "建议": "优先盯回款与大额支出节奏，避免下月利润波动。",
        }
    if metric and "account_value" in data:
        if tax_inclusion == "journal_entry_gross_amount_default":
            reason = "当前结果只落在经营口径，且经营口径来自序时账汇总。"
        else:
            reason = "当前结果只落在经营口径；若该问题走双视角，会先展示现金口径，再补经营口径。"
        return {
            "结论": f"{period}{metric}约 {float(data.get('account_value', 0)):.2f} 元。",
            "原因": append_source_note(append_tax_note(reason)),
            "建议": "如果要继续核对现金收付，请直接追问到账、付款或现金流。",
        }

    if "total" in data and "period" in data:
        return {
            "结论": f"{data.get('period')} 核心金额约 {float(data.get('total', 0)):.2f} 元。",
            "原因": append_source_note(msg),
            "建议": "建议结合回款、应收应付和税额一起看，不单看一个数字。",
        }

    return {
        "结论": msg,
        "原因": append_source_note("本次结果来自财务引擎规则计算。"),
        "建议": "如需更细，请指定对象 + 月份 + 指标。",
    }


def build_final_answer(payload):
    boss_reply = payload.get("boss_reply") or {}
    if not isinstance(boss_reply, dict):
        boss_reply = {}

    parts = []
    conclusion = str(boss_reply.get("结论") or "").strip()
    reason = str(boss_reply.get("原因") or "").strip()
    suggestion = str(boss_reply.get("建议") or "").strip()
    if conclusion:
        parts.append(conclusion)
    if reason:
        parts.append("原因：" + reason)
    if suggestion:
        parts.append("建议：" + suggestion)

    if not parts:
        message = str(payload.get("message") or "").strip()
        if message:
            parts.append(message)

    final_answer = "\n\n".join(parts).strip()
    data = payload.get("data") or {}
    source_note = str(data.get("source_note") or data.get("source_summary") or "").strip()
    if source_note and source_note not in final_answer:
        if final_answer:
            final_answer = final_answer + "\n\n" + source_note
        else:
            final_answer = source_note
    return final_answer


def build_structured_response(payload, query):
    payload = ensure_trace_fields(payload)
    payload["boss_reply"] = build_boss_reply(payload, query)
    summary_contract = build_host_summary_contract(payload, query)
    if summary_contract:
        payload["host_summary_contract"] = summary_contract
    summary_supplier = build_host_summary_supplier_payments(payload, query)
    if summary_supplier:
        payload["host_summary_supplier_payments"] = summary_supplier
    final_answer = build_final_answer(payload)
    if final_answer:
        payload["final_answer"] = final_answer
        payload["boss_reply_text"] = final_answer
    final_answer_source = "none"
    if final_answer:
        final_answer_source = "boss_reply" if isinstance(payload.get("boss_reply"), dict) else "message"
    bridge_meta = build_bridge_meta(query=query)
    bridge_meta["final_answer_available"] = bool(final_answer)
    bridge_meta["final_answer_source"] = final_answer_source
    payload["bridge_meta"] = bridge_meta
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
    return normalize_cli_command_response(proc.stdout, "finance-upload", "import")


def run_sync(arguments):
    directory_path = str((arguments or {}).get("directoryPath") or "").strip()
    if not directory_path:
        raise ValueError("directoryPath is required")
    cmd = [
        str(FINANCEQA_BIN),
        "sync",
        "--db", str(FINANCEQA_DB),
    ]
    append_optional_bool_flag(cmd, "--incremental", (arguments or {}).get("incremental"))
    append_optional_flag(cmd, "--company", (arguments or {}).get("company"))
    cmd.append(directory_path)
    proc = run_cmd(cmd)
    if proc.returncode != 0:
        raise RuntimeError((proc.stderr or proc.stdout or "").strip())
    return normalize_cli_command_response(proc.stdout, "finance-sync", "sync")


def run_dimensions(arguments):
    cmd, tool_operation = build_dimensions_command(arguments or {})
    proc = run_cmd(cmd)
    if proc.returncode != 0:
        raise RuntimeError((proc.stderr or proc.stdout or "").strip())
    return normalize_cli_command_response(proc.stdout, "finance-dimensions", tool_operation)


def handle_list_tools():
    return {"tools": TOOLS}


def handle_call_tool(name, arguments):
    ensure_runtime_ready()

    if name == "finance-query":
        query = (arguments or {}).get("query", "").strip()
        if not query:
            raise ValueError("query is required")
        payload = run_finance_query(query)
        payload = sanitize_bridge_payload(payload)
        return text_result(json.dumps(payload, ensure_ascii=False, indent=2))

    if name == "finance-host-data":
        payload = run_host_data(arguments or {})
        payload = sanitize_bridge_payload(payload)
        return text_result(json.dumps(payload, ensure_ascii=False, indent=2))

    if name == "finance-upload":
        file_path = (arguments or {}).get("filePath", "").strip()
        if not file_path:
            raise ValueError("filePath is required")
        payload = run_upload(file_path)
        return text_result(json.dumps(payload, ensure_ascii=False, indent=2))

    if name == "finance-sync":
        payload = run_sync(arguments or {})
        return text_result(json.dumps(payload, ensure_ascii=False, indent=2))

    if name == "finance-dimensions":
        payload = run_dimensions(arguments or {})
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
