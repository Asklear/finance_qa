#!/usr/bin/env python3
"""Prepare and verify the FinanceQA tenant_uhub -> shadow schema migration.

The default commands are read-only. This script intentionally separates SQL
plan generation from execution so review can happen before any table changes.
"""

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Dict, List, Optional, Sequence, Tuple


SOURCE_SCHEMA = "tenant_uhub"
TARGET_SCHEMA = "tenant_uhub_etl_shadow"

MIGRATION_TABLES = sorted(
    [
        "contract_categories",
        "contract_duplicate_logs",
        "contract_invoices",
        "contract_main",
        "contract_pages",
        "feishu_sync_sources",
        "fin_bank_statement",
        "fin_balance_detail",
        "fin_balance_sheet",
        "fin_contracts",
        "fin_cost_settlement_group_members",
        "fin_cost_settlement_groups",
        "fin_cost_settlements",
        "fin_dimension_members",
        "fin_dimensions",
        "fin_file_mappings",
        "fin_fund_income",
        "fin_fund_income_group_members",
        "fin_fund_income_groups",
        "fin_income_statement",
        "fin_journal",
        "fin_mapping_rules",
        "fin_revenue_settlements",
        "fin_table_idempotency_policies",
    ]
)

EXCLUDED_FINANCIAL_TABLES = sorted(
    ["financial_documents", "financial_links", "financial_rows"]
)

DB_ENV_KEYS = {
    "PGHOST",
    "PGPORT",
    "PGUSER",
    "PGPASSWORD",
    "PGDATABASE",
    "FINANCEQA_PG_SCHEMA",
}


def quote_ident(value: str) -> str:
    return '"' + value.replace('"', '""') + '"'


def qualified(schema: str, name: str) -> str:
    return f"{quote_ident(schema)}.{quote_ident(name)}"


def sql_values(items: Sequence[str]) -> str:
    return ",".join("('" + item.replace("'", "''") + "')" for item in items)


def sql_in(items: Sequence[str]) -> str:
    return ",".join("'" + item.replace("'", "''") + "'" for item in items)


def parse_env_file(path: Path) -> Dict[str, str]:
    values: Dict[str, str] = {}
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def merge_financeqa_env(
    finance_env: Dict[str, str], shadow_env: Dict[str, str]
) -> Tuple[Dict[str, str], List[str], List[str]]:
    merged = dict(finance_env)
    target_schema = shadow_env.get("DB_SEARCH_PATH", TARGET_SCHEMA).split(",", 1)[0].strip()
    replacements = {
        "PGHOST": shadow_env.get("DB_HOST", ""),
        "PGPORT": shadow_env.get("DB_PORT", "5432"),
        "PGUSER": shadow_env.get("DB_USER", ""),
        "PGPASSWORD": shadow_env.get("DB_PASSWORD", ""),
        "PGDATABASE": shadow_env.get("DB_NAME", ""),
        "FINANCEQA_PG_SCHEMA": target_schema,
    }
    merged.update(replacements)

    changed: List[str] = []
    forbidden: List[str] = []
    for key in sorted(set(finance_env) | set(merged)):
        if finance_env.get(key) == merged.get(key):
            continue
        if key in DB_ENV_KEYS:
            changed.append(key)
        else:
            forbidden.append(key)
    return merged, changed, forbidden


def psql_env_from_financeqa(env_values: Dict[str, str]) -> Dict[str, str]:
    env = os.environ.copy()
    env.update(
        {
            "PGHOST": env_values.get("PGHOST", ""),
            "PGPORT": env_values.get("PGPORT", "5432"),
            "PGUSER": env_values.get("PGUSER", ""),
            "PGDATABASE": env_values.get("PGDATABASE", ""),
            "PGPASSWORD": env_values.get("PGPASSWORD", ""),
        }
    )
    return env


def psql_env_from_shadow(env_values: Dict[str, str]) -> Dict[str, str]:
    env = os.environ.copy()
    env.update(
        {
            "PGHOST": env_values.get("DB_HOST", ""),
            "PGPORT": env_values.get("DB_PORT", "5432"),
            "PGUSER": env_values.get("DB_USER", ""),
            "PGDATABASE": env_values.get("DB_NAME", ""),
            "PGPASSWORD": env_values.get("DB_PASSWORD", ""),
        }
    )
    return env


def run_psql(env: Dict[str, str], sql: str) -> List[str]:
    proc = subprocess.run(
        ["psql", "-XAt", "-v", "ON_ERROR_STOP=1"],
        input=sql,
        universal_newlines=True,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr.strip())
    return [line for line in proc.stdout.splitlines() if line.strip()]


def rewrite_fk_definition(definition: str, source_schema: str, target_schema: str) -> str:
    rewritten = definition
    rewritten = rewritten.replace(f"REFERENCES {source_schema}.", f"REFERENCES {target_schema}.")
    rewritten = rewritten.replace(
        f'REFERENCES "{source_schema}".', f'REFERENCES "{target_schema}".'
    )
    for table_name in MIGRATION_TABLES:
        rewritten = re.sub(
            r'REFERENCES\s+"?' + re.escape(table_name) + r'"?\s*\(',
            f"REFERENCES {target_schema}.{table_name}(",
            rewritten,
        )
    return rewritten


def sequence_name(table_name: str, column_name: str) -> str:
    return f"{table_name}_{column_name}_seq"


def sequence_repair_sql(target_schema: str, table_name: str, column_name: str) -> List[str]:
    seq_name = sequence_name(table_name, column_name)
    seq_qualified = qualified(target_schema, seq_name)
    table_qualified = qualified(target_schema, table_name)
    column_ident = quote_ident(column_name)
    seq_regclass = f"'{seq_qualified}'::regclass"
    return [
        f"CREATE SEQUENCE IF NOT EXISTS {seq_qualified};",
        f"ALTER SEQUENCE {seq_qualified} OWNED BY {table_qualified}.{column_ident};",
        (
            f"ALTER TABLE {table_qualified} ALTER COLUMN {column_ident} "
            f"SET DEFAULT nextval({seq_regclass});"
        ),
        (
            f"SELECT setval({seq_regclass}, "
            f"COALESCE((SELECT MAX({column_ident}) FROM {table_qualified}), 1), "
            f"(SELECT MAX({column_ident}) IS NOT NULL FROM {table_qualified}));"
        ),
    ]


def validate_write_intent(apply: bool, yes: bool) -> None:
    if apply and not yes:
        raise SystemExit("--apply requires --yes-i-know-this-writes")


def expected_values_sql() -> str:
    return sql_values(MIGRATION_TABLES)


def verify_sql(source_schema: str, target_schema: str) -> str:
    return f"""
WITH expected(table_name) AS (VALUES {expected_values_sql()}),
source AS (
  SELECT table_name
  FROM information_schema.tables
  WHERE table_schema='{source_schema}' AND table_type='BASE TABLE'
),
relevant AS (
  SELECT table_name
  FROM source
  WHERE (table_name LIKE 'fin_%' OR table_name LIKE 'contract_%' OR table_name IN ('feishu_sync_sources','meta_table_comments','meta_column_comments','dimensions','dimension_members','mapping_rules','table_idempotency_policies','fin_table_idempotency_policies'))
    AND table_name NOT LIKE 'financial_%'
)
SELECT 'SOURCE_EXPECTED_MISSING|' || e.table_name FROM expected e LEFT JOIN source s USING(table_name) WHERE s.table_name IS NULL
UNION ALL
SELECT 'SOURCE_RELEVANT_EXTRA|' || r.table_name FROM relevant r LEFT JOIN expected e USING(table_name) WHERE e.table_name IS NULL
UNION ALL
SELECT 'SOURCE_EXPECTED_PRESENT_COUNT|' || count(*)::text FROM expected e JOIN source s USING(table_name)
ORDER BY 1;
"""


def target_overlap_sql(target_schema: str) -> str:
    return f"""
WITH expected(table_name) AS (VALUES {expected_values_sql()}),
target AS (
  SELECT t.table_name, c.oid::text AS oid
  FROM information_schema.tables t
  JOIN pg_namespace n ON n.nspname=t.table_schema
  JOIN pg_class c ON c.relnamespace=n.oid AND c.relname=t.table_name
  WHERE t.table_schema='{target_schema}' AND t.table_type='BASE TABLE'
)
SELECT 'TARGET_EXPECTED_OVERLAP|' || e.table_name || '|' || target.oid FROM expected e JOIN target USING(table_name)
UNION ALL
SELECT 'TARGET_FINANCIAL_PRESENT|' || target.table_name || '|' || target.oid FROM target WHERE target.table_name IN ({sql_in(EXCLUDED_FINANCIAL_TABLES)})
ORDER BY 1;
"""


def source_nextval_sql(source_schema: str) -> str:
    return f"""
SELECT table_name || '|' || column_name || '|' || column_default
FROM information_schema.columns
WHERE table_schema='{source_schema}'
  AND table_name IN ({sql_in(MIGRATION_TABLES)})
  AND column_default LIKE 'nextval%'
ORDER BY table_name, ordinal_position;
"""


def source_fk_sql(source_schema: str) -> str:
    return f"""
WITH expected(table_name) AS (VALUES {expected_values_sql()})
SELECT src.relname || '|' || con.conname || '|' || pg_get_constraintdef(con.oid)
FROM pg_constraint con
JOIN pg_class src ON src.oid=con.conrelid
JOIN pg_namespace nsrc ON nsrc.oid=src.relnamespace
JOIN pg_class dst ON dst.oid=con.confrelid
JOIN pg_namespace ndst ON ndst.oid=dst.relnamespace
JOIN expected es ON es.table_name=src.relname
JOIN expected ed ON ed.table_name=dst.relname
WHERE con.contype='f'
  AND nsrc.nspname='{source_schema}'
  AND ndst.nspname='{source_schema}'
ORDER BY src.relname, con.conname;
"""


def runtime_privilege_sql(source_schema: str, target_schema: str) -> str:
    return f"""
SELECT 'RUNTIME_USER|' || current_user
UNION ALL SELECT 'RUNTIME_TARGET_USAGE|' || has_schema_privilege(current_user, '{target_schema}', 'USAGE')::text
UNION ALL SELECT 'RUNTIME_TARGET_CREATE|' || has_schema_privilege(current_user, '{target_schema}', 'CREATE')::text
UNION ALL SELECT 'RUNTIME_SOURCE_USAGE|' || has_schema_privilege(current_user, '{source_schema}', 'USAGE')::text;
"""


def generated_plan_sql(
    source_schema: str,
    target_schema: str,
    nextval_rows: Sequence[str],
    fk_rows: Sequence[str],
) -> str:
    lines: List[str] = [
        "-- Generated by tests/scripts/prepare_financeqa_shadow_migration.py",
        "-- Review before executing. Excludes financial_* tables by design.",
        "BEGIN;",
        "SET LOCAL lock_timeout = '5s';",
        "SET LOCAL statement_timeout = '10min';",
        "",
        "DO $$",
        "DECLARE existing text;",
        "BEGIN",
        "  SELECT string_agg(table_name, ', ' ORDER BY table_name) INTO existing",
        "  FROM information_schema.tables",
        f"  WHERE table_schema = '{target_schema}'",
        f"    AND table_name IN ({sql_in(MIGRATION_TABLES)});",
        "  IF existing IS NOT NULL THEN",
        "    RAISE EXCEPTION 'target FinanceQA tables already exist: %', existing;",
        "  END IF;",
        "END $$;",
        "",
    ]

    for table_name in MIGRATION_TABLES:
        lines.append(
            f"CREATE TABLE {qualified(target_schema, table_name)} "
            f"(LIKE {qualified(source_schema, table_name)} INCLUDING ALL);"
        )
    lines.append("")

    for row in nextval_rows:
        table_name, column_name, _default = row.split("|", 2)
        lines.extend(sequence_repair_sql(target_schema, table_name, column_name)[:-1])
    lines.append("")

    for table_name in MIGRATION_TABLES:
        lines.append(
            f"INSERT INTO {qualified(target_schema, table_name)} "
            f"SELECT * FROM {qualified(source_schema, table_name)};"
        )
    lines.append("")

    for row in nextval_rows:
        table_name, column_name, _default = row.split("|", 2)
        lines.append(sequence_repair_sql(target_schema, table_name, column_name)[-1])
    lines.append("")

    for row in fk_rows:
        table_name, constraint_name, definition = row.split("|", 2)
        rewritten = rewrite_fk_definition(definition, source_schema, target_schema)
        lines.append(
            f"ALTER TABLE {qualified(target_schema, table_name)} "
            f"ADD CONSTRAINT {quote_ident(constraint_name)} {rewritten};"
        )
    lines.extend(["", "COMMIT;", ""])
    return "\n".join(lines)


def print_env_preview(finance_env: Dict[str, str], runtime_env: Dict[str, str]) -> int:
    merged, changed, forbidden = merge_financeqa_env(finance_env, runtime_env)
    print("ENV_ALLOWED_CHANGED|" + ",".join(changed))
    print("ENV_FORBIDDEN_CHANGED|" + ",".join(forbidden))
    for key in ["PGHOST", "PGPORT", "PGUSER", "PGDATABASE", "FINANCEQA_PG_SCHEMA"]:
        print(f"ENV_PROPOSED|{key}|{merged.get(key, '')}")
    print("ENV_PROPOSED|PGPASSWORD|***REDACTED***" if merged.get("PGPASSWORD") else "ENV_PROPOSED|PGPASSWORD|")
    return 0 if not forbidden else 1


def command_verify(args: argparse.Namespace) -> int:
    finance_env = parse_env_file(Path(args.finance_env))
    runtime_env = parse_env_file(Path(args.runtime_env))
    readonly_env = parse_env_file(Path(args.readonly_env)) if args.readonly_env else {}

    source_lines = run_psql(psql_env_from_financeqa(finance_env), verify_sql(args.source_schema, args.target_schema))
    for line in source_lines:
        print(line)
    fixed_ok = (
        f"SOURCE_EXPECTED_PRESENT_COUNT|{len(MIGRATION_TABLES)}" in source_lines
        and not any(line.startswith("SOURCE_EXPECTED_MISSING|") for line in source_lines)
        and not any(line.startswith("SOURCE_RELEVANT_EXTRA|") for line in source_lines)
    )
    print(f"RESULT|fixed_table_list|{'PASS' if fixed_ok else 'FAIL'}")

    target_lines = run_psql(psql_env_from_shadow(runtime_env), target_overlap_sql(args.target_schema))
    for line in target_lines:
        print(line)
    target_ok = not any(line.startswith("TARGET_EXPECTED_OVERLAP|") for line in target_lines)
    financial_ok = len([line for line in target_lines if line.startswith("TARGET_FINANCIAL_PRESENT|")]) == 3
    print(f"RESULT|target_expected_overlap|{'PASS' if target_ok else 'FAIL'}")
    print(f"RESULT|financial_exclusion_needed|{'PASS' if financial_ok else 'FAIL'}")

    nextval_lines = run_psql(psql_env_from_financeqa(finance_env), source_nextval_sql(args.source_schema))
    for line in nextval_lines:
        print("SOURCE_NEXTVAL|" + line)
    print(f"RESULT|sequence_repair_required|{'PASS' if nextval_lines else 'FAIL'}|count={len(nextval_lines)}")

    runtime_lines = run_psql(psql_env_from_shadow(runtime_env), runtime_privilege_sql(args.source_schema, args.target_schema))
    for line in runtime_lines:
        print(line)
    print(
        "RESULT|runtime_target_write_prereq|"
        + ("PASS" if "RUNTIME_TARGET_USAGE|true" in runtime_lines and "RUNTIME_TARGET_CREATE|true" in runtime_lines else "FAIL")
    )
    print(
        "RESULT|runtime_source_read_prereq|"
        + ("PASS" if "RUNTIME_SOURCE_USAGE|true" in runtime_lines else "FAIL_NEEDS_TEMP_GRANT")
    )

    if readonly_env:
        readonly_lines = run_psql(
            psql_env_from_shadow(readonly_env),
            f"""
SELECT 'READONLY_USER|' || current_user
UNION ALL SELECT 'READONLY_TARGET_USAGE|' || has_schema_privilege(current_user, '{args.target_schema}', 'USAGE')::text
UNION ALL SELECT 'READONLY_TARGET_CREATE|' || has_schema_privilege(current_user, '{args.target_schema}', 'CREATE')::text;
""",
        )
        for line in readonly_lines:
            print(line)
        print(
            "RESULT|readonly_is_readonly|"
            + ("PASS" if "READONLY_TARGET_USAGE|true" in readonly_lines and "READONLY_TARGET_CREATE|false" in readonly_lines else "FAIL")
        )

    env_status = print_env_preview(finance_env, runtime_env)
    return 0 if fixed_ok and target_ok and financial_ok and nextval_lines and env_status == 0 else 1


def command_plan_sql(args: argparse.Namespace) -> int:
    finance_env = parse_env_file(Path(args.finance_env))
    nextval_rows = run_psql(psql_env_from_financeqa(finance_env), source_nextval_sql(args.source_schema))
    fk_rows = run_psql(psql_env_from_financeqa(finance_env), source_fk_sql(args.source_schema))
    plan = generated_plan_sql(args.source_schema, args.target_schema, nextval_rows, fk_rows)
    if args.out:
        Path(args.out).write_text(plan)
        print(f"WROTE_SQL_PLAN|{args.out}")
    else:
        print(plan)
    print(f"PLAN_TABLE_COUNT|{len(MIGRATION_TABLES)}")
    print(f"PLAN_NEXTVAL_REPAIR_COUNT|{len(nextval_rows)}")
    print(f"PLAN_FK_COUNT|{len(fk_rows)}")
    print("PLAN_EXCLUDES|financial_documents,financial_links,financial_rows")
    return 0


def command_env_preview(args: argparse.Namespace) -> int:
    finance_env = parse_env_file(Path(args.finance_env))
    runtime_env = parse_env_file(Path(args.runtime_env))
    return print_env_preview(finance_env, runtime_env)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-schema", default=SOURCE_SCHEMA)
    parser.add_argument("--target-schema", default=TARGET_SCHEMA)
    sub = parser.add_subparsers(dest="command")
    sub.required = True

    verify = sub.add_parser("verify", help="Run read-only preflight checks")
    verify.add_argument("--finance-env", required=True)
    verify.add_argument("--runtime-env", required=True)
    verify.add_argument("--readonly-env")
    verify.set_defaults(func=command_verify)

    plan = sub.add_parser("plan-sql", help="Generate SQL migration plan without executing it")
    plan.add_argument("--finance-env", required=True)
    plan.add_argument("--out")
    plan.set_defaults(func=command_plan_sql)

    env_preview = sub.add_parser("env-preview", help="Preview FinanceQA env DB-key merge")
    env_preview.add_argument("--finance-env", required=True)
    env_preview.add_argument("--runtime-env", required=True)
    env_preview.set_defaults(func=command_env_preview)

    return parser


def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    validate_write_intent(getattr(args, "apply", False), getattr(args, "yes_i_know_this_writes", False))
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
