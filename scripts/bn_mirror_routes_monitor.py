#!/usr/bin/env python3
"""
四路网页镜像跟单（01-04）巡检 + 自动修复。
每 15 分钟由 systemd timer 触发；发现异常立即处理（重启 scraper / 回收卡死消费 / 必要时拉起 nofx）。

与 bn_scraper_watchdog.py 分工：watchdog 约 60s 盯 POST 断流；本脚本做全链路体检。
"""
from __future__ import annotations

import json
import os
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone

BASE = "/root/hzddai"
DB_PATH = os.path.join(BASE, "data", "data.db")
LOG_PATH = os.path.join(BASE, "logs", "bn_mirror_monitor.log")
INTERCEPT_DIR = "/root/biangen"

# 与 bn_dom_scraper.API_STALE_POST_SEC(120) 略放宽
POST_STALE_SEC = 150
# 正常约 8s 一跳广播；超过此秒无新 bid 视为停播
BROADCAST_STALE_SEC = 90
# 进程启动后一直无 POST
NEVER_POST_SEC = 200
# 跟单消费卡在 processing
CONSUMPTION_STALE_SEC = 180

ROUTES = (
    {
        "label": "01",
        "port": 18765,
        "service": "bn-scraper-01",
        "strategy_id": "bn-screen-mirror-ec92c8f5",
        "follower_id": "80ebc973_comkun_ai_1778954345",
    },
    {
        "label": "02",
        "port": 18766,
        "service": "bn-scraper-02",
        "strategy_id": "bn-screen-mirror-test02-5a3b7c1d",
        "follower_id": "c5d3f195_9ed03623-b679-43fc-bace-e3cd14c00903_comkun_ai_1779171517",
    },
    {
        "label": "03",
        "port": 18767,
        "service": "bn-scraper-03",
        "strategy_id": "bn-screen-mirror-test03-8c402b25",
        "follower_id": "13495fa3_d562e45b-35a7-4f2e-bfc7-fcb69e0e5a86_comkun_ai_1779253249",
    },
    {
        "label": "04",
        "port": 18768,
        "service": "bn-scraper-04",
        "strategy_id": "bn-screen-mirror-test04-2560c95f",
        "follower_id": "4a8928b0_9bdac5bb-7d2d-480a-9e45-3dfe6ca90777_comkun_ai_1779254267",
    },
)


def _log(msg: str) -> None:
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    line = f"[{ts}] {msg}"
    print(line, flush=True)
    try:
        os.makedirs(os.path.dirname(LOG_PATH), exist_ok=True)
        with open(LOG_PATH, "a", encoding="utf-8") as f:
            f.write(line + "\n")
    except OSError:
        pass


def _run(cmd: list[str], timeout: int = 60) -> int:
    try:
        r = subprocess.run(cmd, timeout=timeout, check=False)
        return r.returncode
    except subprocess.TimeoutExpired:
        _log(f"TIMEOUT: {' '.join(cmd)}")
        return 124


def _systemd_active(unit: str) -> bool:
    r = subprocess.run(
        ["systemctl", "is-active", unit],
        capture_output=True,
        text=True,
        timeout=10,
    )
    return r.stdout.strip() == "active"


def _read_health(port: int) -> dict | None:
    path = os.path.join(BASE, f".bn_scraper_health_{port}.json")
    if not os.path.isfile(path):
        return None
    try:
        with open(path, encoding="utf-8") as f:
            return json.load(f)
    except (OSError, json.JSONDecodeError):
        return None


def _fetch_health_http(port: int) -> dict | None:
    url = f"http://127.0.0.1:{port}/health"
    try:
        with urllib.request.urlopen(url, timeout=4) as resp:
            return json.loads(resp.read().decode())
    except (urllib.error.URLError, json.JSONDecodeError, TimeoutError):
        return None


def _parse_db_time(s: str | None) -> float | None:
    if not s:
        return None
    s = str(s).strip().replace(" ", "T", 1)
    try:
        if s.endswith("Z"):
            dt = datetime.fromisoformat(s.replace("Z", "+00:00"))
        else:
            dt = datetime.fromisoformat(s)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return dt.timestamp()
    except ValueError:
        return None


def _last_broadcast_age(strategy_id: str) -> tuple[float | None, int | None]:
    if not os.path.isfile(DB_PATH):
        return None, None
    try:
        conn = sqlite3.connect(DB_PATH, timeout=10)
        row = conn.execute(
            "SELECT id, created_at FROM comkun_master_broadcasts "
            "WHERE source_strategy_id = ? ORDER BY id DESC LIMIT 1",
            (strategy_id,),
        ).fetchone()
        conn.close()
    except sqlite3.Error as e:
        _log(f"DB error {strategy_id}: {e}")
        return None, None
    if not row:
        return None, None
    bid, created = row
    ts = _parse_db_time(created)
    if ts is None:
        return None, bid
    return time.time() - ts, bid


def _reclaim_stale_consumption(follower_id: str) -> bool:
    if not os.path.isfile(DB_PATH):
        return False
    try:
        conn = sqlite3.connect(DB_PATH, timeout=10)
        row = conn.execute(
            "SELECT id, broadcast_id, updated_at FROM comkun_follow_broadcast_consumptions "
            "WHERE trader_id = ? AND status = 'processing' ORDER BY id DESC LIMIT 1",
            (follower_id,),
        ).fetchone()
        if not row:
            conn.close()
            return False
        cid, bid, updated = row
        age = time.time() - (_parse_db_time(updated) or time.time())
        if age < CONSUMPTION_STALE_SEC:
            conn.close()
            return False
        conn.execute(
            "UPDATE comkun_follow_broadcast_consumptions "
            "SET status = 'failed', error = 'monitor_stale_processing_reclaim', "
            "updated_at = CURRENT_TIMESTAMP WHERE id = ?",
            (cid,),
        )
        conn.commit()
        conn.close()
        _log(f"RECLAIM consumption id={cid} trader={follower_id} bid={bid} stuck {age:.0f}s")
        return True
    except sqlite3.Error as e:
        _log(f"reclaim failed {follower_id}: {e}")
        return False


def _restart_scraper(service: str, reason: str) -> None:
    _log(f"RESTART {service}: {reason}")
    _run(["systemctl", "restart", service])


def _ensure_nofx() -> None:
    r = subprocess.run(
        ["docker", "inspect", "-f", "{{.State.Health.Status}}", "nofx-trading"],
        capture_output=True,
        text=True,
        timeout=15,
    )
    status = (r.stdout or "").strip()
    if status == "healthy":
        return
    _log(f"nofx-trading health={status!r} → recreate")
    _run(
        ["bash", "-lc", f"cd {BASE} && docker compose up -d --force-recreate nofx"],
        timeout=300,
    )


def _check_route(route: dict) -> list[str]:
    """返回需执行的修复原因列表（空=正常）。"""
    label = route["label"]
    port = route["port"]
    service = route["service"]
    sid = route["strategy_id"]
    issues: list[str] = []

    if not _systemd_active(service):
        issues.append(f"systemd {service} not active")

    http_h = _fetch_health_http(port)
    file_h = _read_health(port) or {}
    h = http_h or file_h

    now = time.time()
    last_post = float(h.get("last_post_at") or 0)
    started = float(h.get("started_at") or 0)
    stale_sec = h.get("stale_sec")
    if stale_sec is None and last_post > 0:
        stale_sec = now - last_post

    if last_post <= 0:
        if started > 0 and (now - started) >= NEVER_POST_SEC:
            issues.append(f"never_post {(now - started):.0f}s")
        elif started > 0:
            pass  # 刚启动，本轮仅记录
        else:
            issues.append("no last_post_at")
    elif stale_sec is not None and float(stale_sec) >= POST_STALE_SEC:
        issues.append(f"post_stale {float(stale_sec):.0f}s")

    if http_h is None:
        issues.append("health HTTP unreachable")

    bc_age, last_bid = _last_broadcast_age(sid)
    if bc_age is None:
        issues.append("no broadcast in DB")
    elif bc_age >= BROADCAST_STALE_SEC:
        issues.append(f"broadcast_stale {bc_age:.0f}s bid={last_bid}")

    # 有 POST 但长期不广播（例如只 ping、拦截挂了）
    if last_post > 0 and bc_age is not None and bc_age >= BROADCAST_STALE_SEC:
        if float(stale_sec or 0) < POST_STALE_SEC:
            issues.append("ping_only_or_stuck_loop")

    pos_count = int(h.get("positions") or h.get("position_count") or 0)
    state_path = os.path.join(BASE, f".bn_position_state_{port}.json")
    if pos_count == 0 and os.path.isfile(state_path):
        try:
            with open(state_path, encoding="utf-8") as f:
                cached = len(json.load(f).get("positions") or {})
            if cached > 0 and bc_age is not None and bc_age >= BROADCAST_STALE_SEC:
                issues.append(f"cache_has_{cached}_pos_but_live_empty")
        except (OSError, json.JSONDecodeError):
            pass

    intercept = os.path.join(INTERCEPT_DIR, f"last_api_intercept_{port}.txt")
    if os.path.isfile(intercept):
        im_age = now - os.path.getmtime(intercept)
        if im_age >= POST_STALE_SEC and not any("post_stale" in x or "never_post" in x for x in issues):
            issues.append(f"intercept_stale {im_age:.0f}s")

    if issues:
        _log(
            f"ROUTE {label} port={port} pos={pos_count} "
            f"post_age={stale_sec} bc_age={bc_age} issues={issues}"
        )
    else:
        _log(
            f"OK route {label} pos={pos_count} post_stale={stale_sec} "
            f"bc_age={bc_age:.0f}s bid={last_bid}"
        )
    return issues


def main() -> int:
    _log("=== monitor run start ===")
    _ensure_nofx()

    any_fix = False
    for route in ROUTES:
        issues = _check_route(route)
        if route["follower_id"]:
            if _reclaim_stale_consumption(route["follower_id"]):
                any_fix = True

        if not issues:
            continue

        any_fix = True
        reason = "; ".join(issues)
        _restart_scraper(route["service"], reason)

    _log("=== monitor run end ===")
    return 1 if any_fix else 0


if __name__ == "__main__":
    sys.exit(main())
