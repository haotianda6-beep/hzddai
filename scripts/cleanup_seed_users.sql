-- =============================================================================
-- 清理「返利极差 / VIP 链式压测」等测试用户（vipchain、v0cap、loadtest、@sim.nofx.test 等）
-- 会删除匹配到的 users 行及其交易员、流水、网格等关联数据（与 cmd/loadtest_invite -cleanup 思路一致，规则更宽）。
--
-- 【在服务器上执行】数据在容器挂载的 ./data/data.db，不是在 Cursor 本机：
--   cd /你的项目/hzddai
--   docker compose stop nofx
--   sqlite3 data/data.db < scripts/cleanup_seed_users.sql
--   docker compose up -d nofx
--
-- 另：以下 13 个与后台截图一致（极差链 / cap 测），按 id / email / display_name 精确匹配：
--   v0cap-test2-child, v0cap-test2-parent, vipchain-test-v0leaf, vipchain-test-v0..v5,
--   v1cap-test-child, v1cap-test-parent, v0cap-test-child, v0cap-test-parent
--
-- 压测专用邮箱后缀可执行（仅删 *@sim.nofx.test）：
--   docker compose exec nofx /app/loadtest_invite -cleanup -confirm
--
-- 返利 Python 里已同步的团队业绩不会随本脚本回滚，需到返利服务单独处理。
-- =============================================================================
-- 使用前务必停止 nofx：docker compose stop nofx
-- SQLite

BEGIN;

DROP TABLE IF EXISTS tmp_seed_delete;
CREATE TEMP TABLE tmp_seed_delete(id TEXT PRIMARY KEY);

INSERT OR IGNORE INTO tmp_seed_delete(id)
SELECT id FROM users WHERE
  lower(trim(ifnull(email,''))) IN (
    'v0cap-test2-child','v0cap-test2-parent','vipchain-test-v0leaf','vipchain-test-v0','vipchain-test-v1','vipchain-test-v2','vipchain-test-v3','vipchain-test-v4','vipchain-test-v5','v1cap-test-child','v1cap-test-parent','v0cap-test-child','v0cap-test-parent'
  )
  OR lower(trim(ifnull(display_name,''))) IN (
    'v0cap-test2-child','v0cap-test2-parent','vipchain-test-v0leaf','vipchain-test-v0','vipchain-test-v1','vipchain-test-v2','vipchain-test-v3','vipchain-test-v4','vipchain-test-v5','v1cap-test-child','v1cap-test-parent','v0cap-test-child','v0cap-test-parent'
  )
  OR lower(trim(ifnull(id,''))) IN (
    'v0cap-test2-child','v0cap-test2-parent','vipchain-test-v0leaf','vipchain-test-v0','vipchain-test-v1','vipchain-test-v2','vipchain-test-v3','vipchain-test-v4','vipchain-test-v5','v1cap-test-child','v1cap-test-parent','v0cap-test-child','v0cap-test-parent'
  )
  OR lower(ifnull(email,'')) LIKE '%@sim.nofx.test'
  OR lower(ifnull(email,'')) LIKE '%vipchain-test%'
  OR lower(ifnull(email,'')) LIKE '%vipchain%test%'
  OR lower(ifnull(email,'')) LIKE '%v0cap-test%'
  OR lower(ifnull(email,'')) LIKE '%v1cap-test%'
  OR lower(ifnull(email,'')) LIKE '%v0cap-test2%'
  OR lower(ifnull(email,'')) LIKE 'loadtest%'
  OR lower(ifnull(display_name,'')) LIKE '%vipchain-test%'
  OR lower(ifnull(display_name,'')) LIKE '%vipchain%test%'
  OR lower(ifnull(display_name,'')) LIKE '%v0cap-test%'
  OR lower(ifnull(display_name,'')) LIKE '%v1cap-test%'
  OR lower(ifnull(display_name,'')) LIKE '%v0cap-test2%'
  OR ifnull(display_name,'') LIKE '%（测）'
  OR lower(ifnull(id,'')) LIKE '%vipchain-test%'
  OR lower(ifnull(id,'')) LIKE '%v0cap-test%'
  OR lower(ifnull(id,'')) LIKE '%v1cap-test%';

DELETE FROM wallet_ledgers WHERE user_id IN (SELECT id FROM tmp_seed_delete);
DELETE FROM user_notifications WHERE user_id IN (SELECT id FROM tmp_seed_delete);
DELETE FROM strategy_market_entitlements WHERE user_id IN (SELECT id FROM tmp_seed_delete);
DELETE FROM ai_platform_usage_ledger WHERE user_id IN (SELECT id FROM tmp_seed_delete);
DELETE FROM comkun_official_api_ledger WHERE user_id IN (SELECT id FROM tmp_seed_delete);
DELETE FROM comkun_official_api_credentials WHERE user_id IN (SELECT id FROM tmp_seed_delete);

DELETE FROM trader_equity_snapshots WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete));
DELETE FROM trader_positions WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete));
DELETE FROM trader_orders WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete));
DELETE FROM trader_fills WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete));
DELETE FROM decision_records WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete));
DELETE FROM comkun_follow_broadcast_consumptions WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete));
DELETE FROM comkun_follow_trader_balances WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete));
DELETE FROM ai_charges WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete));

DELETE FROM grid_events WHERE instance_id IN (
  SELECT gi.id FROM grid_instances gi
  INNER JOIN grid_configs gc ON gc.id = gi.config_id
  WHERE gc.user_id IN (SELECT id FROM tmp_seed_delete)
);
DELETE FROM grid_levels WHERE instance_id IN (
  SELECT gi.id FROM grid_instances gi
  INNER JOIN grid_configs gc ON gc.id = gi.config_id
  WHERE gc.user_id IN (SELECT id FROM tmp_seed_delete)
);
DELETE FROM grid_regime_assessments WHERE instance_id IN (
  SELECT gi.id FROM grid_instances gi
  INNER JOIN grid_configs gc ON gc.id = gi.config_id
  WHERE gc.user_id IN (SELECT id FROM tmp_seed_delete)
);
DELETE FROM grid_instances WHERE config_id IN (
  SELECT id FROM grid_configs WHERE user_id IN (SELECT id FROM tmp_seed_delete)
);
DELETE FROM grid_configs WHERE user_id IN (SELECT id FROM tmp_seed_delete);

DELETE FROM traders WHERE user_id IN (SELECT id FROM tmp_seed_delete);
DELETE FROM ai_models WHERE user_id IN (SELECT id FROM tmp_seed_delete);
DELETE FROM exchanges WHERE user_id IN (SELECT id FROM tmp_seed_delete);
DELETE FROM strategies WHERE user_id IN (SELECT id FROM tmp_seed_delete);

UPDATE outbound_proxy_pool SET assigned_user_id = '', assigned_exchange_id = '', assigned_at = NULL
WHERE assigned_user_id IN (SELECT id FROM tmp_seed_delete);

UPDATE users SET invited_by_user_id = '' WHERE invited_by_user_id IN (SELECT id FROM tmp_seed_delete);

DELETE FROM users WHERE id IN (SELECT id FROM tmp_seed_delete);

COMMIT;
