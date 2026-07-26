#!/usr/bin/env python3
"""Restart bn-scraper-01..04 when browser POST feed is stale (Fred 现网)."""
from __future__ import annotations

import json
import os
import subprocess
import sys
import time
from datetime import datetime

BASE = "/root/hzddai"
# 与 bn_dom_scraper.API_STALE_POST_SEC 对齐：断流超过此秒数即重启
STALE_RESTART_SEC = 180
# 无 health 文件时，用 intercept dump 的 mtime 兜底
INTERCEPT_DIR = "/root/biangen"

ROUTES = (
    ("01", 18765, "bn-scraper-01"),
    ("02", 18766, "bn-scraper-02"),
    ("03", 18767, "bn-scraper-03"),
    ("04", 18768, "bn-scraper-04"),
)


def _read_health(port: int) -> dict | None:
    path = os.path.join(BASE, f".bn_scraper_health_{port}.json")
    if not os.path.isfile(path):
        return None
    try:
        with open(path, encoding="utf-8") as f:
            return json.load(f)
    except (OSError, json.JSONDecodeError):
        return None


def _intercept_mtime(port: int) -> float:
    path = os.path.join(INTERCEPT_DIR, f"last_api_intercept_{port}.txt")
    if os.path.isfile(path):
        return os.path.getmtime(path)
    return 0.0


def _last_post_age(port: int) -> tuple[float, str]:
    h = _read_health(port)
    now = time.time()
    if h:
        lp = float(h.get("last_post_at") or 0)
        if lp > 0:
            return now - lp, "health"
        started = float(h.get("started_at") or 0)
        if started > 0:
            return now - started, "never_post"
    im = _intercept_mtime(port)
    if im > 0:
        return now - im, "intercept"
    return 0.0, "missing"


def _restart(service: str, reason: str) -> None:
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{ts}] RESTART {service}: {reason}", flush=True)
    subprocess.run(
        ["systemctl", "restart", service],
        check=False,
        timeout=30,
    )


def main() -> int:
    any_restart = False
    for label, port, service in ROUTES:
        age, src = _last_post_age(port)
        if src == "missing":
            continue
        if src == "never_post" and age >= STALE_RESTART_SEC:
            _restart(
                service,
                f"route {label}: no browser POST since start ({age:.0f}s)",
            )
            any_restart = True
            continue
        if src in ("health", "intercept") and age >= STALE_RESTART_SEC:
            _restart(
                service,
                f"route {label}: stale {age:.0f}s (via {src}, port {port})",
            )
            any_restart = True
    return 1 if any_restart else 0


if __name__ == "__main__":
    sys.exit(main())
