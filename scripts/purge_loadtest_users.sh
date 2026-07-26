#!/usr/bin/env bash
# 删除主站 hzddai 中压测工具生成的用户（邮箱后缀 @sim.nofx.test）及关联数据。
# 需在 /root/hzddai 下执行；依赖与主程序相同的 .env / 数据库路径。
set -euo pipefail
cd "$(dirname "$0")/.."
exec go run ./cmd/loadtest_invite -cleanup -confirm
