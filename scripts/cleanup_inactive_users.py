#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
import secrets
import sqlite3
import time
import urllib.parse
import urllib.request
from datetime import datetime, timedelta, timezone
from pathlib import Path

ALPHABET = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
USER_TABLES = (
    "ai_models",
    "ai_platform_usage_ledger",
    "comkun_official_api_credentials",
    "comkun_official_api_ledger",
    "grid_configs",
    "mirror_execution_intents",
    "outbound_proxy_fault_events",
    "partner_rebate_outbox",
    "strategies",
    "strategy_market_entitlements",
    "user_notifications",
    "wallet_ledgers",
)
TRADER_TABLES = (
    "ai_charges",
    "ai_platform_usage_ledger",
    "comkun_follow_broadcast_consumptions",
    "comkun_follow_trader_balances",
    "decision_records",
    "grid_configs",
    "mirror_execution_intents",
    "mt4_follow_ticket_mappings",
    "outbound_proxy_fault_events",
    "trader_equity_snapshots",
    "trader_fills",
    "trader_orders",
    "trader_positions",
)


def parse_args():
    parser = argparse.ArgumentParser(description="Safely remove inactive COMKUN accounts")
    parser.add_argument("--db", default="/root/hzddai/data/data.db")
    parser.add_argument("--backup-dir", default="/root/hz-backups/inactive-users")
    parser.add_argument("--env-file", default="/root/hzddai/.env")
    parser.add_argument("--mode", choices=("initial", "scheduled"), default="scheduled")
    parser.add_argument("--days", type=int, default=30)
    parser.add_argument("--apply", action="store_true")
    return parser.parse_args()


def env_values(path: str) -> dict[str, str]:
    values = dict(os.environ)
    try:
        for raw in Path(path).read_text(encoding="utf-8").splitlines():
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            values.setdefault(key.strip(), value.strip().strip("'\""))
    except FileNotFoundError:
        pass
    return values


def exemptions(env: dict[str, str]) -> list[str]:
    raw = ",".join(
        (
            env.get("COMKUN_ADMIN_EMAILS", "haotianda6@gmail.com"),
            env.get("COMKUN_FINANCE_EMAILS", "1013018910@qq.com"),
        )
    )
    return sorted({part.strip().lower() for part in raw.split(",") if part.strip()})


def ensure_login_column(conn: sqlite3.Connection, apply: bool) -> bool:
    columns = {row[1] for row in conn.execute("PRAGMA table_info(users)")}
    if "last_login_at" in columns:
        return True
    if not apply:
        return False
    conn.execute("ALTER TABLE users ADD COLUMN last_login_at datetime")
    conn.execute("CREATE INDEX IF NOT EXISTS idx_users_last_login_at ON users(last_login_at)")
    conn.commit()
    return True


def candidates(conn: sqlite3.Connection, mode: str, cutoff: str, exempt: list[str], has_login: bool):
    placeholders = ",".join("?" for _ in exempt) or "''"
    activity_filter = (
        "(trader_count=0 OR julianday(last_activity)<julianday(?))"
        if mode == "initial"
        else "(last_login_at IS NULL OR julianday(last_login_at)<julianday(?))"
    )
    login_column = "u.last_login_at" if has_login else "NULL"
    sql = f"""
WITH last_decision AS (
  SELECT trader_id, MAX(timestamp) ts FROM decision_records GROUP BY trader_id
), last_equity AS (
  SELECT trader_id, MAX(timestamp) ts FROM trader_equity_snapshots GROUP BY trader_id
), activity AS (
  SELECT t.user_id,t.id,t.is_running,
    MAX(COALESCE(datetime(t.updated_at),''),COALESCE(datetime(d.ts),''),COALESCE(datetime(e.ts),'')) last_activity
  FROM traders t LEFT JOIN last_decision d ON d.trader_id=t.id
  LEFT JOIN last_equity e ON e.trader_id=t.id
), summary AS (
  SELECT u.id,u.email,u.balance_usdt,{login_column} last_login_at,COUNT(a.id) trader_count,
    COALESCE(SUM(a.is_running),0) running_count,MAX(a.last_activity) last_activity
  FROM users u LEFT JOIN activity a ON a.user_id=u.id GROUP BY u.id
), protected AS (
  SELECT DISTINCT t.user_id FROM traders t JOIN trader_positions p ON p.trader_id=t.id WHERE p.status='OPEN'
  UNION SELECT DISTINCT t.user_id FROM traders t JOIN trader_orders o ON o.trader_id=t.id
    WHERE o.status NOT IN ('FILLED','CANCELED','REJECTED','EXPIRED')
)
SELECT s.id,s.email,s.balance_usdt,s.trader_count,s.last_activity
FROM summary s LEFT JOIN protected p ON p.user_id=s.id
WHERE lower(s.email) NOT IN ({placeholders}) AND s.running_count=0 AND p.user_id IS NULL
  AND {activity_filter}
ORDER BY s.id
"""
    return conn.execute(sql, [*exempt, cutoff]).fetchall()


def backup_database(conn: sqlite3.Connection, directory: Path, stamp: str, db_path: Path) -> Path:
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    target = directory / f"data-pre-inactive-cleanup-{stamp}.db"
    incomplete = target.with_suffix(".db.incomplete")
    with sqlite3.connect(incomplete) as backup:
        conn.backup(backup, pages=8192)
    incomplete.replace(target)
    target.chmod(0o600)
    for old in directory.glob("data-pre-inactive-cleanup-*.db"):
        if old != target and datetime.fromtimestamp(old.stat().st_mtime, timezone.utc) < datetime.now(timezone.utc) - timedelta(days=7):
            old.unlink()
    return target


def ids(conn: sqlite3.Connection, table: str, column: str, values: list[str]) -> list[str]:
    if not values:
        return []
    marks = ",".join("?" for _ in values)
    return [row[0] for row in conn.execute(f"SELECT id FROM {table} WHERE {column} IN ({marks})", values)]


def delete_where_in(conn, table: str, column: str, values: list[str]):
    if values:
        marks = ",".join("?" for _ in values)
        conn.execute(f"DELETE FROM {table} WHERE {column} IN ({marks})", values)


def apply_deletion(conn: sqlite3.Connection, rows, mode: str) -> None:
    user_ids = [row[0] for row in rows]
    trader_ids = ids(conn, "traders", "user_id", user_ids)
    exchange_ids = ids(conn, "exchanges", "user_id", user_ids)
    config_ids = ids(conn, "grid_configs", "user_id", user_ids)
    instance_ids = ids(conn, "grid_instances", "config_id", config_ids)
    level_ids = ids(conn, "grid_levels", "instance_id", instance_ids)
    delete_where_in(conn, "grid_events", "level_id", level_ids)
    for table in ("grid_events", "grid_levels", "grid_regime_assessments"):
        delete_where_in(conn, table, "instance_id", instance_ids)
    delete_where_in(conn, "grid_instances", "config_id", config_ids)
    for table in TRADER_TABLES:
        delete_where_in(conn, table, "trader_id", trader_ids)
    for table in USER_TABLES:
        delete_where_in(conn, table, "user_id", user_ids)
    conn.execute(
        f"UPDATE outbound_proxy_pool SET assigned_user_id=NULL,assigned_exchange_id=NULL,assigned_at=NULL WHERE assigned_user_id IN ({','.join('?' for _ in user_ids)})",
        user_ids,
    )
    delete_where_in(conn, "exchanges", "user_id", user_ids)
    delete_where_in(conn, "traders", "user_id", user_ids)
    marks = ",".join("?" for _ in user_ids)
    conn.execute(f"UPDATE users SET invited_by_user_id='' WHERE invited_by_user_id IN ({marks})", user_ids)
    delete_where_in(conn, "users", "id", user_ids)
    now = datetime.now(timezone.utc).isoformat()
    if mode == "initial":
        seen: set[str] = set()
        for (user_id,) in conn.execute("SELECT id FROM users ORDER BY id"):
            code = ""
            while not code or code in seen:
                code = "".join(secrets.choice(ALPHABET) for _ in range(8))
            seen.add(code)
            conn.execute(
                "UPDATE users SET invited_by_user_id='',invite_code=?,last_login_at=?,updated_at=? WHERE id=?",
                (code, now, now, user_id),
            )
    remaining = conn.execute(f"SELECT COUNT(*) FROM users WHERE id IN ({marks})", user_ids).fetchone()[0]
    if remaining:
        raise RuntimeError("账号删除验证失败")


def sync_authoritative(conn: sqlite3.Connection, env: dict[str, str]) -> None:
    base = env.get("AGENT_REBATE_BASE_URL", "").rstrip("/")
    secret = env.get("AGENT_REBATE_PLATFORM_SECRET", "")
    if not base or not secret:
        raise RuntimeError("AGENT_REBATE_BASE_URL/SECRET未配置")
    parsed = urllib.parse.urlsplit(base)
    if parsed.hostname == "host.docker.internal":
        netloc = "127.0.0.1"
        if parsed.port:
            netloc += f":{parsed.port}"
        base = urllib.parse.urlunsplit((parsed.scheme, netloc, parsed.path, "", ""))
    users = conn.execute(
        "SELECT id,COALESCE(NULLIF(display_name,''),substr(email,1,instr(email,'@')-1)),invited_by_user_id FROM users ORDER BY created_at"
    ).fetchall()
    payload = {
        "replace_missing": True,
        "users": [
            {"external_uid": row[0], "nickname": row[1] or "", "parent_external_uid": row[2] or None}
            for row in users
        ],
    }
    last_error: Exception | None = None
    for attempt in range(5):
        request = urllib.request.Request(
            base + "/api/platform/user-sync",
            data=json.dumps(payload).encode(),
            headers={"Content-Type": "application/json", "X-Platform-Secret": secret},
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                if response.status < 300:
                    return
                raise RuntimeError(f"返佣用户权威同步失败: HTTP {response.status}")
        except Exception as exc:
            last_error = exc
            if attempt < 4:
                time.sleep(2**attempt)
    raise RuntimeError("返佣用户权威同步重试5次仍失败") from last_error


def main():
    args = parse_args()
    env = env_values(args.env_file)
    cutoff = (datetime.now(timezone.utc) - timedelta(days=args.days)).isoformat()
    conn = sqlite3.connect(f"file:{args.db}?mode=rw", uri=True, timeout=30)
    conn.execute("PRAGMA busy_timeout=30000")
    has_login = ensure_login_column(conn, args.apply)
    rows = candidates(conn, args.mode, cutoff, exemptions(env), has_login)
    report = {
        "mode": args.mode,
        "cutoff": cutoff,
        "candidate_count": len(rows),
        "candidate_balance_usdt": round(sum(float(row[2] or 0) for row in rows), 8),
        "candidate_ids_sha256": [hashlib.sha256(row[0].encode()).hexdigest() for row in rows],
        "applied": args.apply,
    }
    stamp = datetime.now().strftime("%Y%m%dT%H%M%S")
    if args.apply and rows:
        backup = backup_database(conn, Path(args.backup_dir), stamp, Path(args.db))
        report["backup"] = str(backup)
        conn.execute("BEGIN IMMEDIATE")
        try:
            apply_deletion(conn, rows, args.mode)
            conn.commit()
        except Exception:
            conn.rollback()
            raise
    if args.apply:
        sync_authoritative(conn, env)
        report_path = Path(args.backup_dir) / f"cleanup-report-{stamp}.json"
        report_path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
        report_path.chmod(0o600)
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()
