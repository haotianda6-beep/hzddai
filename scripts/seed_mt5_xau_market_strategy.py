#!/usr/bin/env python3
"""Seed/update MT5 XAU martingale migration draft on strategy market (haotianda6)."""
from __future__ import annotations

import json
import sqlite3
from datetime import datetime, timezone

DB = "/root/hzddai/data/data.db"
OWNER_EMAIL = "haotianda6@gmail.com"
STRATEGY_ID = "mt5-xau-martingale-bn-draft-v1"

# 构建器默认：20x 杠杆，相对首轮币安对标整体缩小 50%
CONFIG = {
    "strategy_type": "program_martingale",
    "language": "zh",
    "strategy_prompt": """# XAU 程序化马丁（COMKUN-AI · 无 LLM）

- **类型**：program_martingale（非网格）
- **杠杆**：20x
- **规模**：相对 MT5 对标再 ×50%（margin_budget_pct=6%）
- **大趋势**：4h EMA20 vs EMA50
- **补仓**：最多 7 层，按 layer_weights 占净值保证金比例拆分
- **平仓**：basket 止盈或止损 ROE 达阈值时全平（不因 4h 震荡/趋势反转自动平）
""",
    "coin_source": {
        "source_type": "static",
        "static_coins": ["XAUUSDT"],
        "use_ai500": False,
        "use_oi_top": False,
        "use_oi_low": False,
        "use_hyper_all": False,
        "use_hyper_main": False,
    },
    "indicators": {
        "klines": {
            "primary_timeframe": "5m",
            "primary_count": 30,
            "longer_timeframe": "1h",
            "longer_count": 24,
            "enable_multi_timeframe": True,
            "selected_timeframes": ["5m", "15m", "1h"],
        },
        "enable_raw_klines": True,
        "enable_ema": False,
        "enable_macd": False,
        "enable_rsi": False,
        "enable_atr": True,
        "enable_boll": False,
        "enable_volume": True,
        "enable_oi": False,
        "enable_funding_rate": False,
        "enable_quant_data": False,
        "enable_quant_oi": False,
        "enable_quant_netflow": False,
        "enable_oi_ranking": False,
        "enable_netflow_ranking": False,
        "enable_price_ranking": False,
    },
    "risk_control": {
        "max_positions": 1,
        "btc_eth_max_leverage": 20,
        "altcoin_max_leverage": 20,
        "btc_eth_max_position_value_ratio": 0.075,
        "altcoin_max_position_value_ratio": 0.075,
        "max_margin_usage": 0.06,
        "min_position_size": 12,
        "min_risk_reward_ratio": 0,
        "min_confidence": 0,
    },
    "prompt_sections": {},
    "grid_config": None,
    "martingale_program": {
        "symbol": "XAUUSDT",
        "leverage": 20,
        "max_layers": 7,
        "layer_weights": [0.0298, 0.0472, 0.0754, 0.1197, 0.1904, 0.3030, 0.4845],
        "margin_budget_pct": 0.06,
        "add_step_pct": 0.006,
        "basket_take_profit_roe": 0.025,
        "trend_min_sep_pct": 0.0008,
        "allow_short": True,
        "max_basket_loss_roe": 0.12,
        "daily_loss_limit_pct": 8,
        "budget_use_available_only": False,
        "min_layer_margin_usdt": 0.46,
    },
    "market_sale_price_usdt": 0,
    "comkun_follow_listing_template": False,
    "comkun_market_follow": False,
}

DESCRIPTION = (
    "币安 XAUUSDT 程序化马丁：4h 趋势开仓 + 7 层按比例补仓，COMKUN-AI 程序执行（无 LLM）。"
)


def main() -> int:
    conn = sqlite3.connect(DB)
    row = conn.execute("SELECT id FROM users WHERE email = ?", (OWNER_EMAIL,)).fetchone()
    if not row:
        print(f"user not found: {OWNER_EMAIL}")
        return 1
    exists = conn.execute(
        "SELECT market_sale_price_usdt FROM strategies WHERE id = ?", (STRATEGY_ID,)
    ).fetchone()
    cfg = dict(CONFIG)
    if exists and exists[0]:
        cfg["market_sale_price_usdt"] = float(exists[0])

    cfg_json = json.dumps(cfg, ensure_ascii=False)
    now = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S.%f+00:00")
    if exists:
        conn.execute(
            """UPDATE strategies SET description=?, config=?,
               market_revision=market_revision+1, updated_at=? WHERE id=?""",
            (DESCRIPTION, cfg_json, now, STRATEGY_ID),
        )
        print(f"updated strategy {STRATEGY_ID}")
    else:
        uid = row[0]
        conn.execute(
            """INSERT INTO strategies (
                id, user_id, name, description, is_active, is_default, config,
                is_public, config_visible, market_access, market_revision,
                show_after_rename, created_at, updated_at
            ) VALUES (?, ?, ?, ?, 0, 0, ?, 0, 0, 'subscription', 1, 0, ?, ?)""",
            (STRATEGY_ID, uid, "只做XAU（调试中）", DESCRIPTION, cfg_json, now, now),
        )
        print(f"created strategy {STRATEGY_ID}")
    conn.commit()
    conn.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
