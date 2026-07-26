//+------------------------------------------------------------------+
//|                                           HZ-Sender-策略主控.mq4 |
//|                                     极致优化版：支持开平仓、止损止盈修改 |
//+------------------------------------------------------------------+
#property copyright "HZ"
#property link      "https://kunai.vip"
#property version   "1.04"
#property strict
#property description "【策略独立通道】实时发送订单信号，每10秒校准完整持仓，并每分钟同步黄金与美元指数行情。全局变量名与原发送端不同，可与原发送端共存同一MT4。"

//====================================================================
// 参数区精简
// 说明：为了不让客户看到一堆可改参数，这里全部改为内部固定值（不出现在参数窗口）。
// 如果你将来要换服务器地址，直接改下面的 WebhookURL 再编译即可。
//====================================================================
string WebhookURL = "https://kunai.vip/api/send_signal_strategy"; // 中转服务器发送地址
string MarketWebhookURL = "https://kunai.vip/api/news/gold-market-snapshot";
string StrategySignalSecret = "hongzhongdada";
string GoldMarketSymbol = "XAUUSDc";
string DollarMarketSymbol = "DOLLAR";
// 主控是美分账户时，MT4 里 AccountEquity 往往是「等值美元净值」的约 100 倍；跟单公式用手数÷净值，不折算会把跟单手数算得很小。
// 美元主控若改回「按显示净值」跟单，请改回 1；典型美分主控为 100。
double MasterEquityUsdDivisor = 100.0;
// 定时器间隔(毫秒)：越小响应越快，CPU 占用越高；券商一般允许 ≥5ms
int    TimerIntervalMs = 100;
int    MarketSnapshotSeconds = 60;
int    FullOrderSyncSeconds = 10;
// 策略主控：不自动改止损/止盈，仅扫描账户订单变化并转发（由策略 EA 自行管理 SL/TP）

// 结构体：用于记录每个订单的完整状态
struct OrderInfo {
    int ticket;
    double sl;
    double tp;
    double volume; // 新增：记录订单手数，用于判断部分平仓
    double open_price; // 新增：记录开仓价，用于判断挂单价格被拖动修改
    int status; // 0=持仓中, 1=已平仓
    int last_type; // 上次 OrderType()，用于发现「挂单→成交持仓」再发 OPEN，让币安挂上止损止盈
};

OrderInfo known_orders[];

// 全局变量：用于标记是否是初次启动，初次启动时的现有持仓只记录不发送
bool is_first_run = true;

// 全终端只允许一个发送端实例（与接收端类似的心跳）
int g_hz_sender_inst = 0;

// 与接收端对齐的实际定时器毫秒数
int g_hz_sender_timer_ms = 10;

// 左上角 POST 实时监控（失败时第一行立刻显示摘要，整块高亮告警）
string g_hzdd_snd_err = "";
int    g_hzdd_snd_http = 0;
uint   g_hzdd_snd_last_post_tick = 0;
datetime g_hzdd_market_next_send = 0;
datetime g_hzdd_order_sync_next_send = 0;
// 图表面板「购买量」展示用（55~78，每次加载 EA 随机一次；与 MQL5 官网策略市场后台统计无关）
int    g_hz_chart_purchase_qty = 0;

void HzSenderInitChartPurchaseQty()
{
    MathSrand((int)TimeLocal() + (int)GetTickCount() + AccountNumber() + (int)ChartID());
    g_hz_chart_purchase_qty = 55 + (MathRand() % 24);
}

string HzddSndTruncateStr(string s, int maxLen)
{
    if(StringLen(s) <= maxLen) return s;
    return StringSubstr(s, 0, maxLen - 2) + "..";
}

void HzddSndSetErr(string s)
{
    g_hzdd_snd_err = HzddSndTruncateStr(s, 100);
}

void HzddSndClearErr()
{
    g_hzdd_snd_err = "";
}

string HzddSndTfShort()
{
    int p = Period();
    if(p == PERIOD_M1)  return "M1";
    if(p == PERIOD_M5)  return "M5";
    if(p == PERIOD_M15) return "M15";
    if(p == PERIOD_M30) return "M30";
    if(p == PERIOD_H1)  return "H1";
    if(p == PERIOD_H4)  return "H4";
    if(p == PERIOD_D1)  return "D1";
    if(p == PERIOD_W1)  return "W1";
    if(p == PERIOD_MN1) return "MN";
    return IntegerToString(p);
}

void ClearHzddSenderPanel()
{
    string n0 = "HZDD_SND_BG";
    string n1 = "HZDD_SND_TITLE";
    string n2 = "HZDD_SND_L1";
    string n3 = "HZDD_SND_L2";
    string n4 = "HZDD_SND_L3";
    string n5 = "HZDD_SND_MON";
    if(ObjectFind(0, n0) >= 0) ObjectDelete(0, n0);
    if(ObjectFind(0, n1) >= 0) ObjectDelete(0, n1);
    if(ObjectFind(0, n2) >= 0) ObjectDelete(0, n2);
    if(ObjectFind(0, n3) >= 0) ObjectDelete(0, n3);
    if(ObjectFind(0, n4) >= 0) ObjectDelete(0, n4);
    if(ObjectFind(0, n5) >= 0) ObjectDelete(0, n5);
}

// 发送端：正常=绿色底（与接收端深蓝区分）；报错=整块切红底+黄标题，一眼能看出异常
void DrawHzddSenderPanel()
{
    string obj_bg = "HZDD_SND_BG";
    string obj_title = "HZDD_SND_TITLE";
    string obj_l1 = "HZDD_SND_L1";
    string obj_l2 = "HZDD_SND_L2";
    string obj_l3 = "HZDD_SND_L3";
    string obj_mon = "HZDD_SND_MON";

    bool hasErr = (StringLen(g_hzdd_snd_err) > 0);

    int px = 6;
    int py = 20;
    // 简洁小面板：宽度收窄，避免标题右侧大片空白
    int bw = 240;
    int bh = 126;

    if(ObjectFind(0, obj_bg) < 0)
    {
        ObjectCreate(0, obj_bg, OBJ_RECTANGLE_LABEL, 0, 0, 0);
        ObjectSetInteger(0, obj_bg, OBJPROP_CORNER, CORNER_LEFT_UPPER);
        ObjectSetInteger(0, obj_bg, OBJPROP_SELECTABLE, false);
        ObjectSetInteger(0, obj_bg, OBJPROP_BORDER_TYPE, BORDER_FLAT);
        ObjectSetInteger(0, obj_bg, OBJPROP_BACK, false);
    }
    ObjectSetInteger(0, obj_bg, OBJPROP_XDISTANCE, px);
    ObjectSetInteger(0, obj_bg, OBJPROP_YDISTANCE, py);
    ObjectSetInteger(0, obj_bg, OBJPROP_XSIZE, bw);
    ObjectSetInteger(0, obj_bg, OBJPROP_YSIZE, bh);
    if(hasErr)
    {
        ObjectSetInteger(0, obj_bg, OBJPROP_BGCOLOR, C'95,8,14');
        ObjectSetInteger(0, obj_bg, OBJPROP_COLOR, C'255,100,100');
    }
    else
    {
        ObjectSetInteger(0, obj_bg, OBJPROP_BGCOLOR, C'18,42,28');
        ObjectSetInteger(0, obj_bg, OBJPROP_COLOR, C'60,160,95');
    }

    if(ObjectFind(0, obj_title) < 0)
    {
        ObjectCreate(0, obj_title, OBJ_LABEL, 0, 0, 0);
        ObjectSetInteger(0, obj_title, OBJPROP_CORNER, CORNER_LEFT_UPPER);
        ObjectSetInteger(0, obj_title, OBJPROP_SELECTABLE, false);
        ObjectSetString(0, obj_title, OBJPROP_FONT, "Microsoft YaHei");
        ObjectSetInteger(0, obj_title, OBJPROP_FONTSIZE, 13);
    }
    ObjectSetInteger(0, obj_title, OBJPROP_XDISTANCE, px + 14);
    ObjectSetInteger(0, obj_title, OBJPROP_YDISTANCE, py + 6);
    if(hasErr)
    {
        ObjectSetInteger(0, obj_title, OBJPROP_COLOR, C'255,230,120');
        ObjectSetString(0, obj_title, OBJPROP_TEXT, "HZDD-发送端 · 异常告警");
    }
    else
    {
        ObjectSetInteger(0, obj_title, OBJPROP_COLOR, C'220,255,160');
        ObjectSetString(0, obj_title, OBJPROP_TEXT, "HZDD-发送端 · 实时监控");
    }

    if(ObjectFind(0, obj_l1) < 0)
    {
        ObjectCreate(0, obj_l1, OBJ_LABEL, 0, 0, 0);
        ObjectSetInteger(0, obj_l1, OBJPROP_CORNER, CORNER_LEFT_UPPER);
        ObjectSetInteger(0, obj_l1, OBJPROP_SELECTABLE, false);
        ObjectSetString(0, obj_l1, OBJPROP_FONT, "Microsoft YaHei");
        ObjectSetInteger(0, obj_l1, OBJPROP_FONTSIZE, 9);
    }
    ObjectSetInteger(0, obj_l1, OBJPROP_XDISTANCE, px + 14);
    ObjectSetInteger(0, obj_l1, OBJPROP_YDISTANCE, py + 30);
    if(hasErr)
    {
        ObjectSetInteger(0, obj_l1, OBJPROP_COLOR, clrWhite);
        ObjectSetString(0, obj_l1, OBJPROP_TEXT, "【上行】" + g_hzdd_snd_err);
    }
    else
    {
        ObjectSetInteger(0, obj_l1, OBJPROP_COLOR, C'190,235,205');
        ObjectSetString(0, obj_l1, OBJPROP_TEXT, "运行正常 · 实时监控中");
    }

    if(ObjectFind(0, obj_l2) < 0)
    {
        ObjectCreate(0, obj_l2, OBJ_LABEL, 0, 0, 0);
        ObjectSetInteger(0, obj_l2, OBJPROP_CORNER, CORNER_LEFT_UPPER);
        ObjectSetInteger(0, obj_l2, OBJPROP_SELECTABLE, false);
        ObjectSetString(0, obj_l2, OBJPROP_FONT, "Microsoft YaHei");
        ObjectSetInteger(0, obj_l2, OBJPROP_FONTSIZE, 9);
    }
    ObjectSetInteger(0, obj_l2, OBJPROP_XDISTANCE, px + 14);
    ObjectSetInteger(0, obj_l2, OBJPROP_YDISTANCE, py + 48);
    string pur2 = "购买量：" + IntegerToString(g_hz_chart_purchase_qty);
    if(hasErr)
    {
        ObjectSetInteger(0, obj_l2, OBJPROP_COLOR, C'255,220,180');
        ObjectSetString(0, obj_l2, OBJPROP_TEXT, pur2);
    }
    else
    {
        ObjectSetInteger(0, obj_l2, OBJPROP_COLOR, C'175,235,200');
        ObjectSetString(0, obj_l2, OBJPROP_TEXT, pur2);
    }

    if(ObjectFind(0, obj_l3) < 0)
    {
        ObjectCreate(0, obj_l3, OBJ_LABEL, 0, 0, 0);
        ObjectSetInteger(0, obj_l3, OBJPROP_CORNER, CORNER_LEFT_UPPER);
        ObjectSetInteger(0, obj_l3, OBJPROP_SELECTABLE, false);
        ObjectSetString(0, obj_l3, OBJPROP_FONT, "Microsoft YaHei");
        ObjectSetInteger(0, obj_l3, OBJPROP_FONTSIZE, 1);
        ObjectSetInteger(0, obj_l3, OBJPROP_COLOR, clrNONE);
    }
    ObjectSetString(0, obj_l3, OBJPROP_TEXT, "");

    if(ObjectFind(0, obj_mon) < 0)
    {
        ObjectCreate(0, obj_mon, OBJ_LABEL, 0, 0, 0);
        ObjectSetInteger(0, obj_mon, OBJPROP_CORNER, CORNER_LEFT_UPPER);
        ObjectSetInteger(0, obj_mon, OBJPROP_SELECTABLE, false);
        ObjectSetString(0, obj_mon, OBJPROP_FONT, "Microsoft YaHei");
        ObjectSetInteger(0, obj_mon, OBJPROP_FONTSIZE, 9);
    }
    ObjectSetInteger(0, obj_mon, OBJPROP_XDISTANCE, px + 14);
    ObjectSetInteger(0, obj_mon, OBJPROP_YDISTANCE, py + 66);
    uint lag = 0;
    if(g_hzdd_snd_last_post_tick > 0)
        lag = GetTickCount() - g_hzdd_snd_last_post_tick;
    string mon = "POST " + IntegerToString(g_hzdd_snd_http);
    if(g_hzdd_snd_http == 200)
        mon = mon + "  ·  " + IntegerToString(lag) + "ms";
    else if(g_hzdd_snd_http == 0)
        mon = mon + "  ·  等待…";
    else
        mon = mon + "  ·  网络/白名单";
    if(hasErr)
        ObjectSetInteger(0, obj_mon, OBJPROP_COLOR, C'255,255,160');
    else
        ObjectSetInteger(0, obj_mon, OBJPROP_COLOR, C'175,235,200');
    ObjectSetString(0, obj_mon, OBJPROP_TEXT, mon);

    ChartRedraw(0);
}

void HzSenderEnsureInstanceId()
{
    if(g_hz_sender_inst == 0)
        g_hz_sender_inst = (int)(GetTickCount() + MathRand());
}

void HzSenderUpdateHeartbeat()
{
    HzSenderEnsureInstanceId();
    GlobalVariableSet("HZ_STRAT_SND_INST", g_hz_sender_inst);
    GlobalVariableSet("HZ_STRAT_SND_TIME", TimeCurrent());
}

bool HzSenderIsMaster()
{
    HzSenderEnsureInstanceId();
    if(!GlobalVariableCheck("HZ_STRAT_SND_INST") || !GlobalVariableCheck("HZ_STRAT_SND_TIME"))
        return true;
    int rid = (int)GlobalVariableGet("HZ_STRAT_SND_INST");
    datetime rt = (datetime)GlobalVariableGet("HZ_STRAT_SND_TIME");
    if(rid == g_hz_sender_inst)
        return true;
    if(TimeCurrent() - rt > 3)
        return true;
    return false;
}

//+------------------------------------------------------------------+
//| EA初始化函数                                                       |
//+------------------------------------------------------------------+
int OnInit()
{
    HzSenderInitChartPurchaseQty();
    HzSenderEnsureInstanceId();
    Print("🚀 【发送端】启动 | 目标: ", WebhookURL, " | 实例ID: ", g_hz_sender_inst);
    Print("📌 说明：只需本【一个】图表挂载 EA；定时器会监控【全账户】所有品种/周期上的订单。");
    Print("📌 你在【没挂本EA】的图表里市价/限价下单，一样会被扫到并发服务器（与图表无关，只看账户订单池）。");
    if(!SymbolSelect(GoldMarketSymbol, true))
        Print("⚠️ 【行情同步】无法订阅黄金品种: ", GoldMarketSymbol);
    if(!SymbolSelect(DollarMarketSymbol, true))
        Print("⚠️ 【行情同步】无法订阅美元指数品种: ", DollarMarketSymbol);

    // 若 3 秒内已有其它发送端在跑，发击杀令（由旧实例在 OnTimer 里卸载）
    if(GlobalVariableCheck("HZ_STRAT_SND_INST") && GlobalVariableCheck("HZ_STRAT_SND_TIME"))
    {
        int old_id = (int)GlobalVariableGet("HZ_STRAT_SND_INST");
        datetime old_t = (datetime)GlobalVariableGet("HZ_STRAT_SND_TIME");
        if(TimeCurrent() - old_t < 3)
        {
            GlobalVariableSet("HZ_STRAT_SND_KILL_" + IntegerToString(old_id), 1);
            Print("⚔️ 检测到另一发送端实例，已请求旧实例退出，本实例将接管。");
        }
    }
    HzSenderUpdateHeartbeat();

    int ms = TimerIntervalMs;
    if(ms < 5)  ms = 5;
    if(ms > 500) ms = 500;
    g_hz_sender_timer_ms = ms;
    EventSetMillisecondTimer(ms);
    Comment("");
    DrawHzddSenderPanel();

    return(INIT_SUCCEEDED);
}

//+------------------------------------------------------------------+
//| EA退出函数                                                         |
//+------------------------------------------------------------------+
void OnDeinit(const int reason)
{
    EventKillTimer();
    Comment("");
    ClearHzddSenderPanel();
    Print("🛑 【发送端】已安全停止运行。");
}

//+------------------------------------------------------------------+
//| 查找订单在数组中的索引                                               |
//+------------------------------------------------------------------+
int FindOrderIndex(int ticket)
{
    for(int i = 0; i < ArraySize(known_orders); i++)
    {
        if(known_orders[i].ticket == ticket) return i;
    }
    return -1;
}

//+------------------------------------------------------------------+
//| 定时器函数 (每 100 毫秒执行一次，实现极速毫秒级响应)                    |
//+------------------------------------------------------------------+
void OnTimer()
{
    HzSenderEnsureInstanceId();
    if(GlobalVariableCheck("HZ_STRAT_SND_KILL_" + IntegerToString(g_hz_sender_inst)))
    {
        Print("💀 【发送端】收到新实例击杀信号，本实例退出（全终端只保留一个发送端）。");
        GlobalVariableDel("HZ_STRAT_SND_KILL_" + IntegerToString(g_hz_sender_inst));
        ExpertRemove();
        return;
    }
    if(!HzSenderIsMaster())
    {
        Print("🚨 【发送端】已有其它发送端为主，本实例自动卸载。请只保留一个图表挂载发送端 EA。");
        ExpertRemove();
        return;
    }
    HzSenderUpdateHeartbeat();

    // ==========================================
    // 1. 检查是否有新开仓，或者订单被修改 (修改止损止盈)
    // 全账户扫描：OrdersTotal/MODE_TRADES 与当前图表品种无关，你在任意图表、任意周期下的单都会被监听到
    // ==========================================
    // 记录本次循环找到的已知订单的索引，用于稍后清理已消失的订单
    int found_indices[];
    ArrayResize(found_indices, ArraySize(known_orders));
    for(int k=0; k<ArraySize(found_indices); k++) found_indices[k] = 0;

    for(int i = 0; i < OrdersTotal(); i++)
    {
        if(OrderSelect(i, SELECT_BY_POS, MODE_TRADES))
        {
            // 确保只处理当前图表或指定 MagicNumber 的订单，如果是全局跟单，去掉 MagicNumber 限制
            // 这里我们处理所有当前账号的订单
            int t = OrderTicket();
            double sl = OrderStopLoss();
            double tp = OrderTakeProfit();

            int idx = FindOrderIndex(t);

            if(idx == -1) // 这是一个全新的订单（或者是启动时扫描到的历史存量订单）！
            {
                // 加入已知列表
                int size = ArraySize(known_orders);
                ArrayResize(known_orders, size + 1);
                known_orders[size].ticket = t;
                known_orders[size].sl = sl;
                known_orders[size].tp = tp;
                known_orders[size].volume = OrderLots();
                known_orders[size].open_price = OrderOpenPrice();
                known_orders[size].status = 0;
                known_orders[size].last_type = OrderType();

                // 标记为已找到
                ArrayResize(found_indices, size + 1);
                found_indices[size] = 1;

                // 如果是 EA 刚挂上去的第一次扫描，我们只把现有订单记录到内存中，绝对不发送给服务器！
                // 这样跟单端就不会收到这些历史存量单了。
                if(!is_first_run)
                {
                    // ===============================================
                    // 修复无限开仓的Bug: 发送前先判断是不是已经发过了
                    // 只有 status == 0 且从未发过，或者这是一笔新单，才发 OPEN
                    // ===============================================
                    SendOrderState(t, "OPEN", sl, tp);
                    Print("🔥 【发送端】检测到新开仓，已发送信号！订单号: ", t);
                }
                else
                {
                    Print("🧹 【发送端】初次扫描到历史存量单，仅记录不发送: ", t);
                }
            }
            else // 订单已存在，检查是否修改了止盈止损！或部分平仓，或挂单价格被拖动
            {
                found_indices[idx] = 1; // 标记为已找到
                bool modified = false;
                int cur_type = OrderType();
                // 【挂单→持仓】限价/突破单成交后必须再发 OPEN，否则中转站只处理「首次开仓」不会在币安挂止损止盈
                if(known_orders[idx].last_type > OP_SELL && (cur_type == OP_BUY || cur_type == OP_SELL))
                {
                    modified = true;
                    Print("📌 【发送端】挂单已成交转为持仓，已通知服务器挂币安止损止盈！订单号: ", t);
                }
                known_orders[idx].last_type = cur_type;

                // 【核心优化2】止损止盈修改追踪
                double sym_point = MarketInfo(OrderSymbol(), MODE_POINT);
                if(MathAbs(known_orders[idx].sl - sl) > sym_point * 0.5 || MathAbs(known_orders[idx].tp - tp) > sym_point * 0.5)
                {
                    known_orders[idx].sl = sl;
                    known_orders[idx].tp = tp;
                    modified = true;
                    Print("✏️ 【发送端】检测到订单修改，已同步最新止盈止损！订单号: ", t);
                }

                // 【核心优化3】部分平仓追踪
                if(MathAbs(known_orders[idx].volume - OrderLots()) > 0.001)
                {
                    known_orders[idx].volume = OrderLots();
                    modified = true;
                    Print("✂️ 【发送端】检测到部分平仓，已同步最新手数！订单号: ", t, " 剩余手数: ", OrderLots());
                }

                // 【核心优化4】挂单价格拖动追踪 (仅对挂单有效)
                if(OrderType() > OP_SELL)
                {
                    // 浮点数比较，如果价格变化大于点值的0.5倍
                    if(MathAbs(known_orders[idx].open_price - OrderOpenPrice()) > sym_point * 0.5)
                    {
                        known_orders[idx].open_price = OrderOpenPrice();
                        modified = true;
                        Print("📍 【发送端】检测到挂单价格被拖动修改！订单号: ", t, " 新价格: ", OrderOpenPrice());
                    }
                }

                if(modified)
                {
                    // 发送带有最新 SL/TP/Price 和 Volume 的 OPEN 信号，接收端会自动进行同步
                    SendOrderState(t, "OPEN", sl, tp);
                }
            }
        }
    }

    // ==========================================
    // 2. 检查是否有已知订单被平仓
    // ==========================================
    // 从后往前遍历，以便在需要时可以直接从数组中删除元素，防止数组无限膨胀
    for(int i = ArraySize(known_orders) - 1; i >= 0; i--)
    {
        if(known_orders[i].status == 0) // 如果记录中它是持仓状态
        {
            // 如果在本次活跃订单循环中没有找到它，说明它已经被平仓了
            if(found_indices[i] == 0)
            {
                known_orders[i].status = 1; // 标记为已平仓
                // 只有当不是初次扫描时，才去历史记录里捞出它的最终状态并发送平仓信号
                if(!is_first_run)
                {
                    if(OrderSelect(known_orders[i].ticket, SELECT_BY_TICKET, MODE_HISTORY))
                    {
                        SendOrderState(known_orders[i].ticket, "CLOSE", OrderStopLoss(), OrderTakeProfit());
                    }
                    else
                    {
                        // 如果历史记录里也找不到（可能是被删除了的挂单），直接用缓存的数据发 CLOSE
                        SendOrderState(known_orders[i].ticket, "CLOSE", known_orders[i].sl, known_orders[i].tp);
                    }
                }
            }
        }
    }

    // 第一次扫描结束后，把标记设为 false，以后的新单和变动都会正常发送
    if(is_first_run)
    {
        is_first_run = false;
        Print("✅ 【发送端】初次历史存量订单扫描完毕！开始实时监听新动作...");
    }

    // 事件信号保证低延迟；完整快照负责清掉断网、重启或高频连发期间遗留的旧票号。
    if(TimeCurrent() >= g_hzdd_order_sync_next_send &&
       (g_hzdd_snd_last_post_tick == 0 || GetTickCount() - g_hzdd_snd_last_post_tick > 2000))
    {
        if(SendFullOrderSnapshot())
            g_hzdd_order_sync_next_send = TimeCurrent() + FullOrderSyncSeconds;
        else
            g_hzdd_order_sync_next_send = TimeCurrent() + 2;
    }

    // 行情遥测优先级低于订单信号；刚发送过交易动作时延后，避免阻塞高频开平仓扫描。
    if(TimeCurrent() >= g_hzdd_market_next_send &&
       (g_hzdd_snd_last_post_tick == 0 || GetTickCount() - g_hzdd_snd_last_post_tick > 2000))
    {
        SendGoldMarketSnapshot();
        g_hzdd_market_next_send = TimeCurrent() + MarketSnapshotSeconds;
    }

    DrawHzddSenderPanel();
}

//+------------------------------------------------------------------+
//| 统一封装：发送订单当前状态到服务器                                    |
//+------------------------------------------------------------------+
int HzddPostJsonTo(string url, int timeout_ms, string json, char &result[], string &result_headers)
{
    char post[];
    StringToCharArray(json, post, 0, WHOLE_ARRAY, CP_UTF8);
    int post_size = ArraySize(post);
    if(post_size > 0 && post[post_size - 1] == 0)
        ArrayResize(post, post_size - 1);

    ResetLastError();
    return WebRequest("POST", url, "Content-Type: application/json\r\n", timeout_ms,
        post, result, result_headers);
}

int HzddPostJson(string json, char &result[], string &result_headers)
{
    return HzddPostJsonTo(WebhookURL, 5000, json, result, result_headers);
}

bool SendFullOrderSnapshot()
{
    static int last_status = -999;
    string orders = "[";
    int active = 0;
    int total = OrdersTotal();
    for(int i = 0; i < total; i++)
    {
        if(!OrderSelect(i, SELECT_BY_POS, MODE_TRADES))
        {
            if(last_status != -2)
                Print("⚠️ 【持仓校准】订单池读取不完整，本轮不发送，2秒后重试。");
            last_status = -2;
            return false;
        }
        int type = OrderType();
        if(type != OP_BUY && type != OP_SELL)
            continue;
        if(active > 0)
            orders = orders + ",";
        string side = (type == OP_BUY ? "BUY" : "SELL");
        orders = orders + StringFormat(
            "{\"ticket\":%d,\"symbol\":\"%s\",\"side\":\"%s\",\"mt4_type\":%d,\"volume\":%.2f,\"price\":%.5f,\"sl\":%.5f,\"tp\":%.5f}",
            OrderTicket(), OrderSymbol(), side, type, OrderLots(), OrderOpenPrice(), OrderStopLoss(), OrderTakeProfit()
        );
        active++;
    }
    orders = orders + "]";

    double master_eq = AccountEquity();
    double master_margin_used = AccountMargin();
    if(MasterEquityUsdDivisor > 0.0)
    {
        master_eq = master_eq / MasterEquityUsdDivisor;
        master_margin_used = master_margin_used / MasterEquityUsdDivisor;
    }
    if(master_eq < 1.0) master_eq = 1.0;
    if(master_margin_used < 0.0) master_margin_used = 0.0;

    string json = StringFormat(
        "{\"action\":\"SYNC\",\"sync_complete\":true,\"master_equity\":%.2f,\"master_margin_used\":%.2f,\"client_timestamp\":%d,\"secret\":\"%s\",\"orders\":%s}",
        master_eq, master_margin_used, TimeCurrent(), StrategySignalSecret, orders
    );
    char result[];
    string result_headers;
    g_hzdd_snd_last_post_tick = GetTickCount();
    int res = HzddPostJson(json, result, result_headers);
    g_hzdd_snd_http = res;
    if(res == -1 && GetLastError() != 4060)
    {
        Sleep(250);
        ArrayResize(result, 0);
        result_headers = "";
        res = HzddPostJson(json, result, result_headers);
        g_hzdd_snd_http = res;
    }

    if(res == 200)
    {
        HzddSndClearErr();
        if(last_status != 200)
            Print("✅ 【持仓校准】完整快照同步成功，当前市价持仓: ", active);
        last_status = res;
        return true;
    }

    int err = GetLastError();
    if(err == 4060)
        HzddSndSetErr("WebRequest 白名单未放行 https://kunai.vip 错误码" + IntegerToString(err));
    else
        HzddSndSetErr("持仓校准失败，将在2秒后重试 错误码" + IntegerToString(err));
    if(last_status != res)
        Print("❌ 【持仓校准】完整快照同步失败 HTTP ", res, " 错误码: ", err);
    last_status = res;
    return false;
}

void SendGoldMarketSnapshot()
{
    static int last_status = -999;
    double xau_bid = MarketInfo(GoldMarketSymbol, MODE_BID);
    double xau_ask = MarketInfo(GoldMarketSymbol, MODE_ASK);
    double dollar_bid = MarketInfo(DollarMarketSymbol, MODE_BID);
    double dollar_ask = MarketInfo(DollarMarketSymbol, MODE_ASK);
    if(xau_bid <= 0 || xau_ask <= 0 || dollar_bid <= 0 || dollar_ask <= 0)
    {
        if(last_status != 0)
            Print("⚠️ 【行情同步】报价未就绪，请确认市场报价中已显示 ", GoldMarketSymbol, " 和 ", DollarMarketSymbol);
        last_status = 0;
        return;
    }

    string json = StringFormat(
        "{\"secret\":\"%s\",\"client_timestamp\":%d,\"xau_symbol\":\"%s\",\"xau_bid\":%.5f,\"xau_ask\":%.5f,\"dollar_symbol\":\"%s\",\"dollar_bid\":%.5f,\"dollar_ask\":%.5f}",
        StrategySignalSecret, TimeCurrent(), GoldMarketSymbol, xau_bid, xau_ask,
        DollarMarketSymbol, dollar_bid, dollar_ask
    );
    char result[];
    string result_headers;
    int res = HzddPostJsonTo(MarketWebhookURL, 1200, json, result, result_headers);
    if(res != last_status)
    {
        if(res == 200)
            Print("✅ 【行情同步】黄金、美元指数行情已接入新闻监控。");
        else
            Print("⚠️ 【行情同步】上报失败 HTTP ", res, "，不影响订单信号发送。");
    }
    last_status = res;
}

void SendOrderState(int ticket, string action, double sl, double tp)
{
    // 同一票号、同一订单状态在极短时间内重复发送时忽略（高频定时器 + 多条件触发易连发两次），减轻跟单端重复 OPEN
    static int deb_ticket = -1;
    static int deb_type = -99;
    static uint deb_time = 0;
    static double deb_vol = 0, deb_sl = 0, deb_tp = 0, deb_op = 0;
    static string deb_action = "";
    double op = OrderOpenPrice();
    double vol = OrderLots();
    int otype = OrderType();
    double master_eq = AccountEquity();
    double master_margin_used = AccountMargin();
    if(MasterEquityUsdDivisor > 0.0)
    {
        master_eq = master_eq / MasterEquityUsdDivisor;
        master_margin_used = master_margin_used / MasterEquityUsdDivisor;
    }
    if(master_eq < 1.0) master_eq = 1.0;
    if(master_margin_used < 0.0) master_margin_used = 0.0;
    if(ticket == deb_ticket && deb_action == action && deb_type == otype &&
       MathAbs(vol - deb_vol) < 0.0001 && MathAbs(sl - deb_sl) < 0.0000001 && MathAbs(tp - deb_tp) < 0.0000001 &&
       MathAbs(op - deb_op) < 0.0000001 && (GetTickCount() - deb_time) < 80)
    {
        return;
    }
    deb_ticket = ticket;
    deb_action = action;
    deb_type = otype;
    deb_vol = vol;
    deb_sl = sl;
    deb_tp = tp;
    deb_op = op;
    deb_time = GetTickCount();

    string side = "UNKNOWN";
    int type = OrderType();

    if(type == OP_BUY) side = "BUY";
    else if(type == OP_SELL) side = "SELL";
    else if(type == OP_BUYLIMIT) side = "BUY_LIMIT";
    else if(type == OP_SELLLIMIT) side = "SELL_LIMIT";
    else if(type == OP_BUYSTOP) side = "BUY_STOP";
    else if(type == OP_SELLSTOP) side = "SELL_STOP";

    // seq 必须每笔唯一：同一毫秒连开多单时 GetTickCount 相同，故叠加递增计数器
    static uint g_seq_monotonic = 0;
    g_seq_monotonic++;
    uint seq = (uint)GetTickCount() + (uint)ticket * 1000003 + g_seq_monotonic * 13;

    // mt4_type：与 MT4 的 OrderType() 一致，中转站用它判断市价/限价/突破单，避免只靠 side 字符串误判
    int mt4_type = OrderType();
    // 当前图表品种的买一卖一：中转站用 (bid+ask)/2 与币安比价算「基差」，不能用工单上的限价当基准，否则限价单在币安上价格会算错
    double mt4_bid = MarketInfo(OrderSymbol(), MODE_BID);
    double mt4_ask = MarketInfo(OrderSymbol(), MODE_ASK);
    double close_price = 0.0;
    if(action == "CLOSE")
        close_price = OrderClosePrice();
    // 修复数据解析失败的问题：确保浮点数转换不会因为逗号问题导致JSON不合法
    // master_margin_used：主控实际占用保证金；跟单端按主控风险占比换算，避免美分账户马丁仓位放大。
    string json = StringFormat(
        "{\"action\":\"%s\",\"symbol\":\"%s\",\"side\":\"%s\",\"mt4_type\":%d,\"volume\":%.2f,\"price\":%.5f,\"close_price\":%.5f,\"mt4_bid\":%.5f,\"mt4_ask\":%.5f,\"sl\":%.5f,\"tp\":%.5f,\"master_equity\":%.2f,\"master_margin_used\":%.2f,\"ticket\":%d,\"position_id\":%d,\"secret\":\"%s\",\"client_timestamp\":%d,\"seq\":%u}",
        action, OrderSymbol(), side, mt4_type, OrderLots(), OrderOpenPrice(), close_price, mt4_bid, mt4_ask, sl, tp, master_eq, master_margin_used, ticket, ticket, StrategySignalSecret, TimeCurrent(), seq
    );

    char result[];
    string result_headers;

    // MT4 的 WebRequest 是同步调用；给 TLS/跨网链路留出足够时间，避免偶发慢响应被误判为掉线。
    g_hzdd_snd_last_post_tick = GetTickCount();
    int res = HzddPostJson(json, result, result_headers);
    g_hzdd_snd_http = res;

    // 网络类失败自动重试一次；白名单错误直接提示，避免无意义地阻塞发送端。
    if(res == -1)
    {
        int first_err = GetLastError();
        if(first_err != 4060)
        {
            Sleep(250);
            ArrayResize(result, 0);
            result_headers = "";
            res = HzddPostJson(json, result, result_headers);
            g_hzdd_snd_http = res;
        }
    }

    if(res == 200)
    {
        HzddSndClearErr();
        Print("📡 【发送端】信号发送成功: ", action, " -> 订单号: ", ticket);
        string res_str = CharArrayToString(result, 0, WHOLE_ARRAY, CP_UTF8);
        Print("📥 【发送端】服务器返回: ", res_str);
    }
    else if(res == -1)
    {
        int err = GetLastError();
        if(err == 4060)
            HzddSndSetErr("WebRequest 白名单未放行 https://kunai.vip 错误码" + IntegerToString(err));
        else
            HzddSndSetErr("请求超时或网络中断，请检查网络 错误码" + IntegerToString(err));
        Print("❌ 【发送端】发送失败！WebRequest返回码: ", res, " 详细错误码: ", err);
    }
    else
    {
        int err = GetLastError();
        HzddSndSetErr("POST 失败 HTTP " + IntegerToString(res) + " 错误码" + IntegerToString(err));
        Print("❌ 【发送端】发送失败！WebRequest返回码: ", res, " 详细错误码: ", err);
    }
}
