#!/usr/bin/env python3
"""
Migrate SQLite tables/data to PostgreSQL with table prefix and comments.

Security:
- Credentials are read from environment variables only.
- No plaintext credentials are written to repository files.
"""

from __future__ import annotations

import argparse
import os
import sqlite3
from dataclasses import dataclass
from datetime import datetime
from typing import Dict, Iterable, List, Optional, Sequence, Tuple

import psycopg
from psycopg import sql


EXCLUDED_SQLITE_TABLES = {
    "meta_table_comments",
    "meta_column_comments",
}


@dataclass
class ColumnDef:
    name: str
    sqlite_type: str
    notnull: bool
    default_value: Optional[str]
    pk_position: int


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Migrate SQLite to PostgreSQL")
    parser.add_argument("--sqlite", required=True, help="Path to source sqlite db")
    parser.add_argument("--schema", default=os.getenv("PG_SCHEMA", "public"), help="Target PostgreSQL schema")
    parser.add_argument("--prefix", default="fin_", help="Target table prefix")
    parser.add_argument("--drop-existing", action="store_true", help="Drop target tables before creating")
    parser.add_argument("--batch-size", type=int, default=2000, help="Batch insert size")
    parser.add_argument(
        "--include-table",
        action="append",
        default=[],
        help="Only migrate selected source table (repeatable). If omitted, migrate all business tables.",
    )
    return parser.parse_args()


def sqlite_tables(conn: sqlite3.Connection, include: Sequence[str]) -> List[str]:
    rows = conn.execute(
        """
        SELECT name
        FROM sqlite_master
        WHERE type='table' AND name NOT LIKE 'sqlite_%'
        ORDER BY name
        """
    ).fetchall()
    names = [r[0] for r in rows if r[0] not in EXCLUDED_SQLITE_TABLES]
    if include:
        includes = set(include)
        names = [n for n in names if n in includes]
    return names


def sqlite_table_sql(conn: sqlite3.Connection, table: str) -> str:
    row = conn.execute(
        "SELECT sql FROM sqlite_master WHERE type='table' AND name=?",
        (table,),
    ).fetchone()
    if not row or not row[0]:
        raise RuntimeError(f"missing CREATE TABLE SQL for {table}")
    return row[0]


def sqlite_columns(conn: sqlite3.Connection, table: str) -> List[ColumnDef]:
    rows = conn.execute(f'PRAGMA table_info("{table}")').fetchall()
    cols: List[ColumnDef] = []
    for cid, name, col_type, notnull, dflt_value, pk in rows:
        cols.append(
            ColumnDef(
                name=name,
                sqlite_type=(col_type or "").strip(),
                notnull=bool(notnull),
                default_value=dflt_value,
                pk_position=int(pk or 0),
            )
        )
    return cols


def sqlite_unique_constraints(conn: sqlite3.Connection, table: str) -> List[List[str]]:
    constraints: List[List[str]] = []
    indexes = conn.execute(f'PRAGMA index_list("{table}")').fetchall()
    for idx in indexes:
        # SQLite PRAGMA index_list columns: seq, name, unique, origin, partial
        idx_name = idx[1]
        unique_flag = int(idx[2])
        origin = idx[3] if len(idx) > 3 else ""
        if unique_flag != 1:
            continue
        # Skip PK unique index; PK is handled separately.
        if origin == "pk":
            continue
        cols_info = conn.execute(f'PRAGMA index_info("{idx_name}")').fetchall()
        col_names = [r[2] for r in cols_info if r[2] is not None]
        if col_names:
            constraints.append(col_names)
    return constraints


def map_sqlite_type_to_pg(sqlite_type: str) -> str:
    t = (sqlite_type or "").strip().upper()
    if t == "":
        return "TEXT"
    if t.startswith("INTEGER") or t == "INT":
        return "BIGINT"
    if t.startswith("BOOLEAN"):
        return "BOOLEAN"
    if t.startswith("DECIMAL") or t.startswith("NUMERIC"):
        return t.replace("DECIMAL", "NUMERIC")
    if t.startswith("TIMESTAMP") or "DATETIME" in t:
        return "TIMESTAMP"
    if t.startswith("DATE"):
        return "DATE"
    if t.startswith("TIME"):
        return "TIME"
    if "CHAR" in t or "TEXT" in t or "CLOB" in t:
        return "TEXT"
    if "REAL" in t or "FLOA" in t or "DOUB" in t:
        return "DOUBLE PRECISION"
    return "TEXT"


def normalize_default(expr: Optional[str], pg_type: str) -> Optional[str]:
    if expr is None:
        return None
    e = expr.strip()
    if e == "":
        return None
    if pg_type.upper() == "BOOLEAN":
        if e in ("1", "TRUE", "true"):
            return "true"
        if e in ("0", "FALSE", "false"):
            return "false"
    if e.upper() == "CURRENT_TIMESTAMP":
        return "CURRENT_TIMESTAMP"
    # Keep numeric and quoted literals as-is.
    return e


def create_table_pg(
    pg_conn: psycopg.Connection,
    schema: str,
    target_table: str,
    columns: List[ColumnDef],
    unique_constraints: List[List[str]],
    drop_existing: bool,
    source_create_sql: str,
) -> None:
    if drop_existing:
        pg_conn.execute(
            sql.SQL("DROP TABLE IF EXISTS {}.{} CASCADE").format(
                sql.Identifier(schema), sql.Identifier(target_table)
            )
        )

    pk_cols = [c for c in columns if c.pk_position > 0]
    pk_single_int_col = None
    if len(pk_cols) == 1:
        c = pk_cols[0]
        if (c.sqlite_type or "").upper().startswith("INTEGER"):
            pk_single_int_col = c.name

    col_defs: List[sql.Composed] = []
    for c in columns:
        if pk_single_int_col == c.name:
            col_sql = sql.SQL("{} BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY").format(
                sql.Identifier(c.name)
            )
            col_defs.append(col_sql)
            continue

        dtype = map_sqlite_type_to_pg(c.sqlite_type)
        parts: List[sql.Composed] = [
            sql.SQL("{} {}").format(sql.Identifier(c.name), sql.SQL(dtype))
        ]
        if c.notnull:
            parts.append(sql.SQL("NOT NULL"))
        dflt = normalize_default(c.default_value, dtype)
        if dflt is not None:
            parts.append(sql.SQL("DEFAULT ") + sql.SQL(dflt))
        if c.pk_position > 0 and len(pk_cols) == 1:
            parts.append(sql.SQL("PRIMARY KEY"))
        col_defs.append(sql.SQL(" ").join(parts))

    constraints: List[sql.Composed] = []
    if len(pk_cols) > 1:
        ordered = sorted(pk_cols, key=lambda x: x.pk_position)
        constraints.append(
            sql.SQL("PRIMARY KEY ({})").format(
                sql.SQL(", ").join(sql.Identifier(c.name) for c in ordered)
            )
        )
    for uq_cols in unique_constraints:
        constraints.append(
            sql.SQL("UNIQUE ({})").format(
                sql.SQL(", ").join(sql.Identifier(n) for n in uq_cols)
            )
        )

    all_defs = col_defs + constraints
    create_stmt = sql.SQL("CREATE TABLE {}.{} ({})").format(
        sql.Identifier(schema),
        sql.Identifier(target_table),
        sql.SQL(", ").join(all_defs),
    )
    pg_conn.execute(create_stmt)

    pg_conn.execute(
        sql.SQL("COMMENT ON TABLE {}.{} IS {}").format(
            sql.Identifier(schema),
            sql.Identifier(target_table),
            sql.Literal(f"source_sqlite_ddl: {source_create_sql}"),
        )
    )


def fetch_table_comment_map(sqlite_conn: sqlite3.Connection) -> Dict[str, str]:
    rows = sqlite_conn.execute(
        "SELECT table_name, comment FROM meta_table_comments"
    ).fetchall()
    return {r[0]: r[1] for r in rows}


def fetch_column_comment_map(sqlite_conn: sqlite3.Connection) -> Dict[Tuple[str, str], str]:
    rows = sqlite_conn.execute(
        "SELECT table_name, column_name, comment FROM meta_column_comments"
    ).fetchall()
    return {(r[0], r[1]): r[2] for r in rows}


def apply_comments(
    pg_conn: psycopg.Connection,
    schema: str,
    source_table: str,
    target_table: str,
    columns: Sequence[ColumnDef],
    table_comments: Dict[str, str],
    column_comments: Dict[Tuple[str, str], str],
) -> None:
    tbl_comment = table_comments.get(source_table)
    if tbl_comment:
        pg_conn.execute(
            sql.SQL("COMMENT ON TABLE {}.{} IS {}").format(
                sql.Identifier(schema),
                sql.Identifier(target_table),
                sql.Literal(tbl_comment),
            )
        )
    for c in columns:
        key = (source_table, c.name)
        col_comment = column_comments.get(key)
        if col_comment:
            pg_conn.execute(
                sql.SQL("COMMENT ON COLUMN {}.{}.{} IS {}").format(
                    sql.Identifier(schema),
                    sql.Identifier(target_table),
                    sql.Identifier(c.name),
                    sql.Literal(col_comment),
                )
            )


def copy_data(
    sqlite_conn: sqlite3.Connection,
    pg_conn: psycopg.Connection,
    schema: str,
    source_table: str,
    target_table: str,
    columns: Sequence[ColumnDef],
    batch_size: int,
) -> int:
    col_names = [c.name for c in columns]
    bool_idx = {
        i
        for i, c in enumerate(columns)
        if map_sqlite_type_to_pg(c.sqlite_type).upper() == "BOOLEAN"
    }
    ts_idx = {
        i
        for i, c in enumerate(columns)
        if map_sqlite_type_to_pg(c.sqlite_type).upper() == "TIMESTAMP"
    }
    quoted_cols = ", ".join([f'"{n}"' for n in col_names])
    select_sql = f'SELECT {quoted_cols} FROM "{source_table}"'
    rows = sqlite_conn.execute(select_sql)

    placeholders = sql.SQL(", ").join(sql.Placeholder() for _ in col_names)
    insert_stmt = sql.SQL("INSERT INTO {}.{} ({}) VALUES ({})").format(
        sql.Identifier(schema),
        sql.Identifier(target_table),
        sql.SQL(", ").join(sql.Identifier(n) for n in col_names),
        placeholders,
    )

    total = 0
    batch: List[Tuple] = []
    for row in rows:
        values = list(row)
        for idx in bool_idx:
            if values[idx] is None:
                continue
            if isinstance(values[idx], bool):
                continue
            if values[idx] in (0, 1):
                values[idx] = bool(values[idx])
        for idx in ts_idx:
            values[idx] = normalize_timestamp_value(values[idx])
        batch.append(tuple(values))
        if len(batch) >= batch_size:
            with pg_conn.cursor() as cur:
                cur.executemany(insert_stmt, batch)
            total += len(batch)
            batch.clear()
    if batch:
        with pg_conn.cursor() as cur:
            cur.executemany(insert_stmt, batch)
        total += len(batch)
    return total


def normalize_timestamp_value(value):
    if value is None:
        return None
    if isinstance(value, datetime):
        return value
    if not isinstance(value, str):
        return value
    s = value.strip()
    if s == "":
        return None

    # Common SQLite-exported pattern: "2026-04-11 13:06:46.133463 +0000 UTC"
    if s.endswith(" UTC") and (" +" in s or " -" in s):
        s = s[: -len(" UTC")]
    # If offset exists, parse aware dt then drop tz (target type is TIMESTAMP w/o tz)
    for fmt in ("%Y-%m-%d %H:%M:%S.%f %z", "%Y-%m-%d %H:%M:%S %z"):
        try:
            dt = datetime.strptime(s, fmt)
            return dt.replace(tzinfo=None)
        except ValueError:
            pass
    # Already plain timestamp
    for fmt in ("%Y-%m-%d %H:%M:%S.%f", "%Y-%m-%d %H:%M:%S"):
        try:
            return datetime.strptime(s, fmt)
        except ValueError:
            pass
    return value


def sync_identity_sequence(
    pg_conn: psycopg.Connection,
    schema: str,
    target_table: str,
    columns: Sequence[ColumnDef],
) -> None:
    id_col = None
    for c in columns:
        if c.pk_position > 0 and (c.sqlite_type or "").upper().startswith("INTEGER"):
            id_col = c.name
            break
    if not id_col:
        return

    pg_conn.execute(
        sql.SQL(
            """
            SELECT setval(
                pg_get_serial_sequence(%s, %s),
                COALESCE((SELECT MAX({id_col}) FROM {schema}.{table}), 1),
                true
            )
            """
        ).format(
            id_col=sql.Identifier(id_col),
            schema=sql.Identifier(schema),
            table=sql.Identifier(target_table),
        ),
        (f"{schema}.{target_table}", id_col),
    )


def pg_dsn_from_env() -> str:
    # Preferred: PG_DSN. Fallback: PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE
    dsn = os.getenv("PG_DSN")
    if dsn:
        return dsn

    required = ["PGHOST", "PGPORT", "PGUSER", "PGPASSWORD", "PGDATABASE"]
    missing = [k for k in required if not os.getenv(k)]
    if missing:
        raise RuntimeError(f"missing postgres env vars: {', '.join(missing)}")

    return (
        f"host={os.environ['PGHOST']} "
        f"port={os.environ['PGPORT']} "
        f"user={os.environ['PGUSER']} "
        f"password={os.environ['PGPASSWORD']} "
        f"dbname={os.environ['PGDATABASE']}"
    )


def ensure_schema(pg_conn: psycopg.Connection, schema: str) -> None:
    # `public` schema always exists in standard PostgreSQL deployment.
    # Some managed DB users do not have CREATE SCHEMA privilege.
    if schema == "public":
        return
    exists = pg_conn.execute(
        "SELECT 1 FROM pg_namespace WHERE nspname = %s",
        (schema,),
    ).fetchone()
    if exists:
        return
    pg_conn.execute(
        sql.SQL("CREATE SCHEMA {}").format(sql.Identifier(schema))
    )


def main() -> None:
    args = parse_args()
    sqlite_conn = sqlite3.connect(args.sqlite)
    sqlite_conn.row_factory = sqlite3.Row

    tables = sqlite_tables(sqlite_conn, args.include_table)
    if not tables:
        raise RuntimeError("no sqlite tables selected to migrate")

    table_comments = fetch_table_comment_map(sqlite_conn)
    column_comments = fetch_column_comment_map(sqlite_conn)

    dsn = pg_dsn_from_env()
    with psycopg.connect(dsn, autocommit=False) as pg_conn:
        ensure_schema(pg_conn, args.schema)

        for src_table in tables:
            target_table = f"{args.prefix}{src_table}"
            cols = sqlite_columns(sqlite_conn, src_table)
            uniques = sqlite_unique_constraints(sqlite_conn, src_table)
            src_ddl = sqlite_table_sql(sqlite_conn, src_table)

            create_table_pg(
                pg_conn=pg_conn,
                schema=args.schema,
                target_table=target_table,
                columns=cols,
                unique_constraints=uniques,
                drop_existing=args.drop_existing,
                source_create_sql=src_ddl,
            )

            inserted = copy_data(
                sqlite_conn=sqlite_conn,
                pg_conn=pg_conn,
                schema=args.schema,
                source_table=src_table,
                target_table=target_table,
                columns=cols,
                batch_size=args.batch_size,
            )
            sync_identity_sequence(pg_conn, args.schema, target_table, cols)
            apply_comments(
                pg_conn=pg_conn,
                schema=args.schema,
                source_table=src_table,
                target_table=target_table,
                columns=cols,
                table_comments=table_comments,
                column_comments=column_comments,
            )
            print(f"[ok] {src_table} -> {args.schema}.{target_table}, rows={inserted}")

        pg_conn.commit()

    sqlite_conn.close()
    print("[done] migration committed")


if __name__ == "__main__":
    main()
