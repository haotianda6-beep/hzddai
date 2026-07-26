#!/usr/bin/env python3
"""Restart okx-scraper-01..08 when browser POST feed is stale."""
from __future__ import annotations

import json
import os
import subprocess
import time
from datetime import datetime

BASE = "/root/hzddai"
INTERCEPT_DIR = "/root/biangen"

STALE_RESTART_SEC = 180

ROUTES = (
    ("01", 18765, "okx-scraper-01"),
    ("02", 18766, "okx-scraper-02"),
    ("03", 18767, "okx-scraper-03"),
    ("04", 18768, "okx-scraper-04"),
    ("05", 18769, "okx-scraper-05"),
    ("06", 18770, "okx-scraper-06"),
    ("07", 18771, "okx-scraper-07"),
    ("08", 18772, "okx-scraper-08"),
)


def _read_health(port: int) -> dict | None:
    path = os.path.join(BASE, f".okx_scraper_health_{port}.json")
    if not os.path.isfile(path):
        return None
    try:
        with open(path, encoding="utf-8") as f:
            return json.load(f)
    except (OSError, json.JSONDecodeError):
        return None


def _intercept_mtime(port: int) -> float:
    for name in (
        f"last_okx_api_intercept_{port}.txt",
        f"last_api_intercept_{port}.txt",
    ):
        path = os.path.join(INTERCEPT_DIR, name)
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
        updated = float(h.get("updated_at") or 0)
        if updated > 0:
            return now - updated, "never_post"
    im = _intercept_mtime(port)
    if im > 0:
        return now - im, "intercept"
    return 0.0, "missing"


def _restart(service: str, reason: str) -> None:
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{ts}] RESTART {service}: {reason}", flush=True)
    subprocess.run(["systemctl", "restart", service], check=False, timeout=30)


def main() -> int:
    for label, port, service in ROUTES:
        age, src = _last_post_age(port)
        if src == "missing":
            continue
        if src in ("never_post", "health", "intercept") and age >= STALE_RESTART_SEC:
            _restart(service, f"route {label}: stale {age:.0f}s via {src}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
