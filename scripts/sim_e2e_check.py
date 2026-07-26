#!/usr/bin/env python3
"""四路镜像 + 程序化马丁：只读/安全模拟（ping 不写假仓位，避免触发跟单）。"""
from __future__ import annotations

import json
import os
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime

BASE = "/root/hzddai"
DB = os.path.join(BASE, "data", "data.db")

ROUTES = (
    ("01", 18765, "bn-scraper-01", "bn-screen-mirror-ec92c8f5"),
    ("02", 18766, "bn-scraper-02", "bn-screen-mirror-test02-5a3b7c1d"),
    ("03", 18767, "bn-scraper-03", "bn-screen-mirror-test03-8c402b25"),
    ("04", 18768, "bn-scraper-04", "bn-screen-mirror-test04-2560c95f"),
)

MARTINGALE_SID = "mt5-xau-martingale-bn-draft-v1"
POST_STALE_WARN = 90
BC_STALE_WARN = 90


def ok(msg: str) -> None:
    print(f"  ✓ {msg}")


def warn(msg: str) -> None:
    print(f"  ⚠ {msg}")


def fail(msg: str) -> None:
    print(f"  ✗ {msg}")


def section(title: str) -> None:
    print(f"\n=== {title} ===")


def http_json(url: str, data: dict | None = None, headers: dict | None = None, timeout: int = 5):
    hdrs = {"Content-Type": "application/json"}
    if headers:
        hdrs.update(headers)
    body = None if data is None else json.dumps(data).encode()
    req = urllib.request.Request(url, data=body, headers=hdrs, method="POST" if data else "GET")
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.status, json.loads(resp.read().decode())


def check_go_tests() -> bool:
    section("Go 单元测试（马丁）")
    r = subprocess.run(
        ["go", "test", "./trader", "-run", "Martingale", "-count=1"],
        cwd=BASE,
        capture_output=True,
        text=True,
        timeout=60,
    )
    if r.returncode == 0:
        ok(r.stdout.strip() or "pass")
        return True
    fail(r.stderr or r.stdout or "go test failed")
    return False


def check_scraper_routes() -> int:
    section("四路爬虫（health + 安全 ping）")
    issues = 0
    now = time.time()
    for label, port, service, sid in ROUTES:
        print(f"\n--- 路 {label} {service} :{port} ---")
        # systemd
        r = subprocess.run(
            ["systemctl", "is-active", service],
            capture_output=True,
            text=True,
            timeout=8,
        )
        if r.stdout.strip() != "active":
            fail(f"{service} 未 active")
            issues += 1
            continue
        ok(f"{service} active")

        # health
        try:
            _, h = http_json(f"http://127.0.0.1:{port}/health")
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as e:
            fail(f"/health 不可达: {e}")
            issues += 1
            continue

        stale = float(h.get("stale_sec") or 0)
        pos = int(h.get("positions") or 0)
        api_mode = h.get("api_mode")
        if stale >= POST_STALE_WARN:
            warn(f"POST 偏旧 stale_sec={stale:.0f}s（阈值提醒 {POST_STALE_WARN}s）")
            issues += 1
        else:
            ok(f"POST 新鲜 stale={stale:.1f}s pos={pos} api_mode={api_mode}")

        # DB 最近广播
        conn = sqlite3.connect(DB, timeout=10)
        row = conn.execute(
            "SELECT id, created_at FROM comkun_master_broadcasts "
            "WHERE source_strategy_id=? ORDER BY id DESC LIMIT 1",
            (sid,),
        ).fetchone()
        conn.close()
        if not row:
            fail("DB 无广播记录")
            issues += 1
        else:
            bid, created = row
            ok(f"最近广播 bid={bid} at {created}")

        # 安全 ping（不携带仓位）
        try:
            status, resp = http_json(
                f"http://127.0.0.1:{port}/data",
                {"heartbeat_only": True, "source": f"sim-e2e-{label}-ping"},
            )
            if status == 200 and resp.get("ping"):
                ok("POST /data ping 成功")
            else:
                warn(f"ping 响应异常: {resp}")
                issues += 1
        except Exception as e:
            fail(f"ping POST 失败: {e}")
            issues += 1

        # ping 后 health 应变新
        try:
            _, h2 = http_json(f"http://127.0.0.1:{port}/health")
            stale2 = float(h2.get("stale_sec") or 99)
            if stale2 < 3:
                ok(f"ping 后 stale={stale2:.1f}s")
            else:
                warn(f"ping 后仍偏旧 stale={stale2:.1f}s")
        except Exception as e:
            warn(f"ping 后 health 检查失败: {e}")

    return issues


def check_martingale_config() -> int:
    section("程序化马丁策略配置")
    issues = 0
    if not os.path.isfile(DB):
        fail("data.db 不存在")
        return 1
    conn = sqlite3.connect(DB, timeout=10)
    row = conn.execute(
        "SELECT name, config FROM strategies WHERE id=?",
        (MARTINGALE_SID,),
    ).fetchone()
    conn.close()
    if not row:
        fail(f"策略 {MARTINGALE_SID} 不存在")
        return 1
    name, cfg_s = row
    cfg = json.loads(cfg_s)
    st = cfg.get("strategy_type")
    mp = cfg.get("martingale_program") or {}
    ok(f"策略「{name}」type={st}")
    if st != "program_martingale":
        fail(f"strategy_type 应为 program_martingale，现为 {st}")
        issues += 1
    if not mp:
        fail("缺少 martingale_program")
        issues += 1
    else:
        for k in ("symbol", "leverage", "max_layers", "margin_budget_pct"):
            if mp.get(k) is None:
                fail(f"martingale_program 缺 {k}")
                issues += 1
        if mp.get("close_on_trend_neutral"):
            warn("仍含 close_on_trend_neutral=true（应已关闭震荡平仓）")
            issues += 1
        ok(
            f"XAU={mp.get('symbol')} lev={mp.get('leverage')} "
            f"layers={mp.get('max_layers')} budget={mp.get('margin_budget_pct')}"
        )
    return issues


def check_martingale_traders() -> int:
    section("马丁交易员 / 决策记录")
    issues = 0
    conn = sqlite3.connect(DB, timeout=10)
    traders = conn.execute(
        "SELECT id, name, ai_model_id, strategy_id, is_running FROM traders "
        "WHERE strategy_id=? OR strategy_id LIKE ?",
        (MARTINGALE_SID, "%martingale%"),
    ).fetchall()
    if not traders:
        warn("未找到绑定马丁策略的运行中交易员（仅检查配置层）")
    for tid, tname, model, sid, running in traders:
        print(f"\n  交易员 {tname} ({tid[:12]}…) running={running} model={model}")
        if model != "comkun_ai":
            warn("未绑定 COMKUN-AI，周期不会执行马丁")
            issues += 1
        else:
            ok("已绑 COMKUN-AI")
        dec = conn.execute(
            "SELECT cycle_number, system_prompt, length(cot_trace), length(execution_log), "
            "substr(execution_log,1,80), timestamp "
            "FROM decision_records WHERE trader_id=? ORDER BY id DESC LIMIT 1",
            (tid,),
        ).fetchone()
        if not dec:
            warn("尚无决策记录")
            continue
        cyc, sp, cot_len, ex_len, ex_preview, ts = dec
        ok(f"最近周期 #{cyc} @ {ts}")
        if sp != "COMKUN_FOLLOW_INFO_ONLY":
            warn(f"system_prompt={sp!r}（期望 COMKUN_FOLLOW_INFO_ONLY 以隐藏思考）")
            issues += 1
        else:
            ok("思考过程已标记为未公开（COMKUN_FOLLOW_INFO_ONLY）")
        if cot_len and cot_len > 0:
            warn(f"cot_trace 仍有 {cot_len} 字符（旧记录或写入未生效）")
        else:
            ok("cot_trace 为空")
        if ex_len and ex_len > 0:
            ok(f"execution_log 有内容（内部排障用，前端不展示）")
    conn.close()
    return issues


def check_nofx_container() -> int:
    section("nofx 容器")
    issues = 0
    r = subprocess.run(
        ["docker", "inspect", "-f", "{{.State.Status}}", "nofx-trading"],
        capture_output=True,
        text=True,
        timeout=15,
    )
    st = (r.stdout or "").strip()
    if st == "running":
        ok("nofx-trading running")
    else:
        fail(f"nofx-trading status={st!r}")
        issues += 1
    return issues


def main() -> int:
    print(f"模拟巡检 @ {datetime.now().isoformat(timespec='seconds')}")
    total = 0
    if not check_go_tests():
        total += 1
    total += check_nofx_container()
    total += check_scraper_routes()
    total += check_martingale_config()
    total += check_martingale_traders()

    section("汇总")
    if total == 0:
        ok("未发现阻断项（⚠ 为提醒，可观察）")
        return 0
    fail(f"共 {total} 项需关注（含 ⚠）")
    return 1


if __name__ == "__main__":
    sys.exit(main())
