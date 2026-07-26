// 代理邀请压测：生成一名「合伙人」+ N 名随机充值的下级客户，便于在主站管理后台 /admin 看邀请用户列表；返佣子系统见 agent_rebate_backend。
//
// 用法（需在 /root/hzddai 下、与主程序相同 .env / 数据库）：
//
//	# 生成（默认 1000 人，每人随机 10~5000 USDT 充值）
//	go run ./cmd/loadtest_invite -seed
//
//	# 森林模式：指定管理员邮箱为总代理根，创建多名「下级代理」，1000 客户随机挂到各代理下，
//	# 并按代理维度控制团队充值总额，便于在返利侧体现 VIP1～VIP5（需先同步返利后再跑任务回算）。
//	go run ./cmd/loadtest_invite -seed -forest -admin-email='你的管理员邮箱'
//
//	# 仅删除本工具创建的邮箱（*@sim.nofx.test），需二次确认
//	go run ./cmd/loadtest_invite -cleanup -confirm
//
// Docker 内（镜像含 loadtest_invite 二进制）：
//
//	docker compose exec nofx /app/loadtest_invite -seed
//	docker compose exec nofx /app/loadtest_invite -cleanup -confirm
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"nofx/auth"
	"nofx/config"
	"nofx/crypto"
	"nofx/store"
)

const simEmailSuffix = "@sim.nofx.test"

// loadtest 森林模式代理昵称：区域 + 序号，便于后台区分
var forestRegionTags = []string{"华东", "华南", "华北", "西南", "华中", "西北", "东北", "台港澳", "京津", "沪浙"}

func main() {
	seed := flag.Bool("seed", false, "创建压测代理 + N 名下级并随机充值")
	cleanup := flag.Bool("cleanup", false, "删除所有 "+simEmailSuffix+" 用户及相关数据")
	confirm := flag.Bool("confirm", false, "与 -cleanup 同时使用才执行删除")
	n := flag.Int("n", 1000, "下级客户数量（-seed）")
	minUSDT := flag.Float64("min", 10, "单次充值最小 USDT（含）")
	maxUSDT := flag.Float64("max", 5000, "单次充值最大 USDT（含）")
	forest := flag.Bool("forest", false, "管理员为总代理 + 多下级代理 + 随机分配客户（需 -admin-email）")
	adminEmail := flag.String("admin-email", "", "forest：管理员账号邮箱（必须在 users 表中已存在）")
	agents := flag.Int("agents", 10, "forest：下级代理人数（每位代理挂若干随机客户）")
	flag.Parse()

	_ = godotenv.Load()
	config.Init()
	cfg := config.Get()

	cs, err := crypto.NewCryptoService()
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}
	crypto.SetGlobalCryptoService(cs)

	dbType := store.DBTypeSQLite
	if cfg.DBType == "postgres" {
		dbType = store.DBTypePostgres
	}
	st, err := store.NewWithConfig(store.DBConfig{
		Type:     dbType,
		Path:     cfg.DBPath,
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		DBName:   cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	})
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	switch {
	case *seed && *cleanup:
		log.Fatal("不要同时使用 -seed 与 -cleanup")
	case *seed:
		if *n < 1 || *n > 50000 {
			log.Fatal("-n 建议在 1~50000")
		}
		if *maxUSDT < *minUSDT || *minUSDT < 0 {
			log.Fatal("无效的 -min / -max")
		}
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		if *forest {
			em := strings.TrimSpace(*adminEmail)
			if em == "" {
				log.Fatal("-forest 必须同时指定 -admin-email（数据库里已有的管理员邮箱）")
			}
			if *agents < 1 || *agents > 200 {
				log.Fatal("-agents 建议在 1~200")
			}
			if err := runForestSeed(st, rng, em, *agents, *n, *minUSDT, *maxUSDT); err != nil {
				log.Fatalf("forest seed: %v", err)
			}
			fmt.Println("✅ 森林压测已写入：总代理=" + em + "，下级代理 " + fmt.Sprintf("%d", *agents) + " 名，客户 " + fmt.Sprintf("%d", *n) + " 名随机分配。")
			fmt.Println("   主站「邀请裂变」可看伞下树；返利侧需用户同步 + 任务回算后查看 VIP。")
		} else {
			if err := runSeed(st, rng, *n, *minUSDT, *maxUSDT); err != nil {
				log.Fatalf("seed: %v", err)
			}
			fmt.Println("✅ 压测数据已写入。请到主站 Web /admin → 邀请概览查看。")
		}
	case *cleanup:
		if !*confirm {
			log.Fatal("清理请加 -confirm，避免误删")
		}
		if err := runCleanup(st.GormDB()); err != nil {
			log.Fatalf("cleanup: %v", err)
		}
		fmt.Println("✅ 已删除所有 " + simEmailSuffix + " 用户及关联数据。")
	default:
		fmt.Println("请指定 -seed 或 -cleanup，见文件头注释。")
	}
}

func runSeed(st *store.Store, rng *rand.Rand, n int, minU, maxU float64) error {
	passHash, err := auth.HashPassword("sim-loadtest-disabled")
	if err != nil {
		return err
	}

	return st.GormDB().Transaction(func(tx *gorm.DB) error {
		us := store.NewUserStore(tx)
		bs := store.NewBillingStore(tx)

		agentEmail := "loadtest_agent" + simEmailSuffix
		var exist store.User
		var agentID string
		if err := tx.Where("email = ?", agentEmail).First(&exist).Error; err == nil {
			agentID = exist.ID
			fmt.Printf("已存在压测代理 %s，复用 id=%s\n", agentEmail, agentID)
		} else if err != gorm.ErrRecordNotFound {
			return err
		} else {
			agentID = uuid.New().String()
			agent := &store.User{
				ID:              agentID,
				Email:           agentEmail,
				PasswordHash:    passHash,
				DisplayName:     "压测代理（可删）",
				BalanceUSDT:     0,
				InvitedByUserID: "",
			}
			if err := us.Create(agent); err != nil {
				return fmt.Errorf("创建代理: %w", err)
			}
			fmt.Printf("已创建压测代理 %s id=%s\n", agentEmail, agentID)
		}

		for i := 0; i < n; i++ {
			uid := uuid.New().String()
			email := fmt.Sprintf("loadtest_c_%05d%s", i+1, simEmailSuffix)
			u := &store.User{
				ID:              uid,
				Email:           email,
				PasswordHash:    passHash,
				DisplayName:     fmt.Sprintf("压测客户%d", i+1),
				BalanceUSDT:     0,
				InvitedByUserID: agentID,
			}
			if err := us.Create(u); err != nil {
				return fmt.Errorf("创建用户 %s: %w", email, err)
			}
			amt := randomAmount(rng, minU, maxU)
			newBal, ok, err := us.AddBalanceDelta(tx, uid, amt)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("充值失败（余额逻辑）: %s", email)
			}
			if _, err := bs.AppendLedger(tx, uid, amt, newBal, "recharge", ""); err != nil {
				return err
			}
		}
		return nil
	})
}

func randomAmount(rng *rand.Rand, minU, maxU float64) float64 {
	if maxU <= minU {
		return round2(minU)
	}
	x := minU + rng.Float64()*(maxU-minU)
	return round2(x)
}

// amountsForTarget 生成 count 笔充值，总和约为 targetTotal，单笔夹在 [minU,maxU]（最后一笔补齐）。
func amountsForTarget(rng *rand.Rand, count int, targetTotal float64, minU, maxU float64) []float64 {
	if count <= 0 {
		return nil
	}
	floor := float64(count) * minU
	if targetTotal < floor {
		targetTotal = floor
	}
	raw := make([]float64, count)
	sum := 0.0
	for i := 0; i < count; i++ {
		raw[i] = randomAmount(rng, minU, maxU)
		sum += raw[i]
	}
	if sum <= 0 {
		for i := range raw {
			raw[i] = minU
		}
		sum = float64(count) * minU
	}
	scale := targetTotal / sum
	out := make([]float64, count)
	rs := 0.0
	for i := 0; i < count-1; i++ {
		x := round2(raw[i] * scale)
		if x < minU {
			x = minU
		}
		if x > maxU {
			x = maxU
		}
		out[i] = x
		rs += x
	}
	out[count-1] = round2(targetTotal - rs)
	if out[count-1] < minU {
		out[count-1] = minU
	}
	return out
}

func runForestSeed(st *store.Store, rng *rand.Rand, adminEmail string, agentsN, nCustomers int, minU, maxU float64) error {
	passHash, err := auth.HashPassword("sim-loadtest-disabled")
	if err != nil {
		return err
	}
	admin, err := store.NewUserStore(st.GormDB()).GetByEmail(adminEmail)
	if err != nil {
		return fmt.Errorf("管理员邮箱不存在或查询失败: %s (%w)", adminEmail, err)
	}

	// 每位下级代理「团队充值」目标（客户充值之和），对齐返利 VIP 阈值区间（5000/15000/50000/150000/500000）
	tierTargets := []float64{
		6000, 18000, 60000, 200000, 520000,
		4000, 12000, 45000, 130000, 400000,
	}

	return st.GormDB().Transaction(func(tx *gorm.DB) error {
		us := store.NewUserStore(tx)
		bs := store.NewBillingStore(tx)

		agentIDs := make([]string, agentsN)
		for i := 0; i < agentsN; i++ {
			email := fmt.Sprintf("loadtest_sub_%03d%s", i, simEmailSuffix)
			var exist store.User
			if err := tx.Where("email = ?", email).First(&exist).Error; err == nil {
				agentIDs[i] = exist.ID
				fmt.Printf("已存在下级代理 %s，复用 id=%s\n", email, exist.ID)
			} else if err != gorm.ErrRecordNotFound {
				return err
			} else {
				aid := uuid.New().String()
				tag := forestRegionTags[i%len(forestRegionTags)]
				agent := &store.User{
					ID:              aid,
					Email:           email,
					PasswordHash:    passHash,
					DisplayName:     fmt.Sprintf("%s·合伙%02d（测）", tag, i+1),
					BalanceUSDT:     0,
					InvitedByUserID: admin.ID,
				}
				if err := us.Create(agent); err != nil {
					return fmt.Errorf("创建下级代理 %s: %w", email, err)
				}
				agentIDs[i] = aid
				fmt.Printf("已创建下级代理 %s id=%s（上级为管理员 %s）\n", email, aid, adminEmail)
			}
		}

		assignment := make([]int, nCustomers)
		counts := make([]int, agentsN)
		for i := 0; i < nCustomers; i++ {
			a := rng.Intn(agentsN)
			assignment[i] = a
			counts[a]++
		}

		amountsPerAgent := make([][]float64, agentsN)
		for a := 0; a < agentsN; a++ {
			tgt := tierTargets[a%len(tierTargets)]
			amountsPerAgent[a] = amountsForTarget(rng, counts[a], tgt, minU, maxU)
		}

		cursors := make([]int, agentsN)
		for i := 0; i < nCustomers; i++ {
			ag := assignment[i]
			amt := amountsPerAgent[ag][cursors[ag]]
			cursors[ag]++

			uid := uuid.New().String()
			email := fmt.Sprintf("loadtest_fc_%05d%s", i+1, simEmailSuffix)
			u := &store.User{
				ID:              uid,
				Email:           email,
				PasswordHash:    passHash,
				DisplayName:     fmt.Sprintf("伞下·%05d（测）", i+1),
				BalanceUSDT:     0,
				InvitedByUserID: agentIDs[ag],
			}
			if err := us.Create(u); err != nil {
				return fmt.Errorf("创建客户 %s: %w", email, err)
			}
			newBal, ok, err := us.AddBalanceDelta(tx, uid, amt)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("充值失败（余额逻辑）: %s", email)
			}
			if _, err := bs.AppendLedger(tx, uid, amt, newBal, "recharge", ""); err != nil {
				return err
			}
		}
		return nil
	})
}

func round2(x float64) float64 {
	return float64(int64(x*100+0.5)) / 100
}

func runCleanup(db *gorm.DB) error {
	suffix := "%" + simEmailSuffix

	var nUsers int64
	if err := db.Model(&store.User{}).Where("email LIKE ?", suffix).Count(&nUsers).Error; err != nil {
		return err
	}
	fmt.Printf("将删除约 %d 个用户（邮箱后缀 %s）…\n", nUsers, simEmailSuffix)
	if nUsers == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		exec := func(sql string) error {
			return tx.Exec(sql, suffix).Error
		}

		// 交易员子表 → traders → 用户级资源 → users
		steps := []string{
			`DELETE FROM decision_records WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM trader_equity_snapshots WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM trader_orders WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM trader_fills WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM trader_positions WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM ai_charges WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM comkun_follow_broadcast_consumptions WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM comkun_follow_trader_balances WHERE trader_id IN (SELECT id FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,

			// grid_instances 通过 config_id 关联 grid_configs.user_id（表结构无 user_id 列）
			`DELETE FROM grid_events WHERE instance_id IN (SELECT gi.id FROM grid_instances gi INNER JOIN grid_configs gc ON gc.id = gi.config_id WHERE gc.user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM grid_levels WHERE instance_id IN (SELECT gi.id FROM grid_instances gi INNER JOIN grid_configs gc ON gc.id = gi.config_id WHERE gc.user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM grid_regime_assessments WHERE instance_id IN (SELECT gi.id FROM grid_instances gi INNER JOIN grid_configs gc ON gc.id = gi.config_id WHERE gc.user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM grid_instances WHERE config_id IN (SELECT id FROM grid_configs WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?))`,
			`DELETE FROM grid_configs WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,

			`DELETE FROM traders WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,

			`DELETE FROM wallet_ledgers WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,
			`DELETE FROM strategy_market_entitlements WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,
			`DELETE FROM ai_platform_usage_ledger WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,
			`DELETE FROM user_notifications WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,
			`DELETE FROM ai_models WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,
			`DELETE FROM exchanges WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,
			`DELETE FROM strategies WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)`,

			`UPDATE outbound_proxy_pool SET assigned_user_id = '' WHERE assigned_user_id IN (SELECT id FROM users WHERE email LIKE ?)`,

			`DELETE FROM users WHERE email LIKE ?`,
		}

		for _, q := range steps {
			if err := exec(q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	})
}
