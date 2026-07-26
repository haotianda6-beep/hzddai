import useSWR from 'swr'
import { Radio, ListOrdered } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api } from '../../../lib/api'
import type {
  ComkunMasterBoardOrder,
  ComkunMasterBoardPayload,
  ComkunMasterBoardPosition,
  ScreenMonitorMasterState,
} from '../../../lib/api/strategies'

type ScreenInstruction = {
  type: string
  symbol: string
  side?: string
  entry_price?: number
  quantity?: number
  added_quantity?: number
  reduced_quantity?: number
  remaining_quantity?: number
  leverage?: number
  margin_used?: number
  prev_margin?: number
  margin_change?: number
  margin_change_pct?: number
  qty_change?: number
  reduced_margin?: number
  remaining_margin?: number
  prev_side?: string
  timestamp?: number
  reason?: string
}

type Tab = 'broadcasts' | 'orders' | 'followers'

function fmtMs(ms: number): string {
  if (!ms) return '—'
  try {
    const d = new Date(ms)
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch {
    return String(ms)
  }
}

function fmtBoardPrice(n: number | undefined | null): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  const abs = Math.abs(n)
  const maxFrac = abs >= 1000 ? 2 : abs >= 1 ? 4 : 8
  return n.toLocaleString(undefined, { maximumFractionDigits: maxFrac })
}

function fmtBoardPnL(n: number | undefined | null): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  const abs = Math.abs(n)
  const maxFrac = abs >= 1 ? 2 : 4
  return n.toLocaleString(undefined, { maximumFractionDigits: maxFrac, signDisplay: 'always' })
}

function positionDirZh(p: ComkunMasterBoardPosition): string {
  const s = (p.side || '').toLowerCase()
  if (s === 'long' || s === 'buy') return '开多'
  if (s === 'short' || s === 'sell') return '开空'
  return p.side ? String(p.side) : '—'
}

function orderIntentZh(o: ComkunMasterBoardOrder): string {
  const ps = (o.position_side || '').toUpperCase()
  const sd = (o.side || '').toUpperCase()
  const typ = (o.type || '').toUpperCase()
  const bracket = typ.includes('TAKE_PROFIT') || typ.includes('STOP') || typ.includes('TRAILING')
  if (bracket) {
    if (ps === 'LONG' && sd === 'SELL') return '平多'
    if (ps === 'SHORT' && sd === 'BUY') return '平空'
  }
  if (sd === 'BUY' || ps === 'LONG') return '开多'
  if (sd === 'SELL' || ps === 'SHORT') return '开空'
  return o.side || '—'
}

function orderTypeBriefZh(t: string): string {
  const u = (t || '').toUpperCase()
  if (u.includes('LIMIT')) return '限价'
  if (u.includes('MARKET') && !u.includes('STOP') && !u.includes('TAKE_PROFIT')) return '市价'
  if (u.includes('TAKE_PROFIT')) return '止盈'
  if (u.includes('TRAILING')) return '追踪'
  if (u.includes('STOP')) return '条件'
  return t ? t.slice(0, 8) : '委托'
}

function orderTriggerPriceZh(o: ComkunMasterBoardOrder): number | undefined {
  if (o.price > 0) return o.price
  if (o.stop_price > 0) return o.stop_price
  return undefined
}

export function ComkunFollowListingDataBoard({ strategyId }: { strategyId: string }) {
  const [tab, setTab] = useState<Tab>('broadcasts')
  const [lastFetchAt, setLastFetchAt] = useState<number | null>(null)
  const { data: payload, error, isLoading } = useSWR<ComkunMasterBoardPayload>(
    strategyId ? ['comkun-master-board', strategyId] : null,
    () => api.getComkunMasterBoard(strategyId),
    // 主控接口会对每个跟单用户拉交易所持仓/挂单，刷新过频易触发币安 IP 限频（-1003）
    { refreshInterval: 20000, revalidateOnFocus: true }
  )

  useEffect(() => {
    if (payload && !error) setLastFetchAt(Date.now())
  }, [payload, error])

  const tabBtn = (active: boolean) =>
    active
      ? 'rounded-lg bg-primary-container/20 px-3 py-1.5 text-xs font-bold text-primary-container ring-1 ring-primary-container/35'
      : 'rounded-lg px-3 py-1.5 text-xs font-medium text-on-surface-variant hover:bg-surface-container-highest'

  const loadErr = error ? `加载失败：${error instanceof Error ? error.message : String(error)}` : null

  return (
    <div className="relative z-[1] space-y-3 rounded-xl border border-outline-variant/25 bg-surface-container-lowest/90 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Radio className="h-4 w-4 text-primary" aria-hidden />
          <span className="text-xs font-semibold tracking-wide text-on-surface-variant">策略源数据看板</span>
        </div>
        <div className="flex gap-1 rounded-lg border border-outline-variant/20 bg-surface-container-low p-0.5">
          <button type="button" className={tabBtn(tab === 'broadcasts')} onClick={() => setTab('broadcasts')}>
            广播记录
          </button>
          <button type="button" className={tabBtn(tab === 'orders')} onClick={() => setTab('orders')}>
            源账户订单
          </button>
          <button type="button" className={tabBtn(tab === 'followers')} onClick={() => setTab('followers')}>
            用户账户
          </button>
        </div>
      </div>

      <div className="rounded-lg border border-primary/15 bg-surface-container-low/80 p-3 text-[11px] leading-relaxed text-on-surface-variant">
        <p className="font-semibold text-on-surface">同步说明（简版）</p>
        <ul className="mt-1.5 list-disc space-y-1 pl-4">
          <li>
            客户<strong className="text-on-surface">购买/复制</strong>本策略后，会得到自动同步配置：每轮扫描读取
            <strong className="text-on-surface">本策略 ID</strong> 对应的<strong className="text-on-surface">策略源广播</strong>
            ，并在<strong className="text-on-surface">客户自己的交易所账户</strong>上按<strong>保证金占策略源权益比例</strong>尝试对齐（不是用你的 API 密钥代下）。
          </li>
          <li>
            策略源使用<strong>币安等实盘 + COMKUN-AI（主广播 / 虚拟 Token）</strong>时，同步轮次结束会<strong>自动写入广播</strong>。若需补发或脚本集成，仍可用{' '}
            <code className="rounded bg-surface-container-highest px-1">POST /api/admin/comkun/master-broadcast</code>，且{' '}
            <code className="rounded bg-surface-container-highest px-1">source_strategy_id</code> 等于本策略 ID。策略源交易员须绑定<strong>本策略</strong>，下方「源账户订单」才与源账户一致。
          </li>
          <li>
            若主控策略开启了<strong className="text-on-surface">「SOL 手动广播模式」</strong>，广播里只会带
            <strong className="text-on-surface">SOLUSDT</strong> 的持仓与挂单；其它合约主控即便有仓位也
            <strong className="text-on-surface">不会同步给跟单</strong>。
          </li>
        </ul>
      </div>

      {payload?.master_trader_id ? (
        <p className="text-[11px] text-on-surface-variant">
          已绑定策略源交易员：<span className="font-mono text-on-surface">{payload.master_trader_name || '—'}</span>（
          <span className="font-mono">{payload.master_trader_id.slice(0, 8)}…</span>）
        </p>
      ) : (
        <p className="text-[11px] text-amber-200/90">
          尚未找到绑定本策略的交易员：请在「AI 交易员」里创建/编辑交易员，并把<strong>策略</strong>选为本条同步模板，保存后即可在此看到源账户订单流水。
        </p>
      )}

      {loadErr && <p className="text-xs text-error">{loadErr}</p>}
      {isLoading && !payload && <p className="text-xs text-on-surface-variant">加载中…</p>}

      {tab === 'broadcasts' && (
        <>
          {/* 屏幕 OCR 识别的最新持仓数据 */}
          {(() => {
            const latestWithState = (payload?.broadcasts ?? []).find(b => b.master_state?.positions?.length);
            if (!latestWithState) return null;
            const ms = latestWithState.master_state as ScreenMonitorMasterState;
            return (
              <div className="mb-3 rounded-xl border border-primary/25 bg-primary/5 p-4">
                <div className="mb-2 flex items-center gap-2">
                  <span className="material-symbols-outlined text-sm text-primary">screenshot_monitor</span>
                  <span className="text-xs font-semibold text-primary">
                    屏幕监控 · 最新扫描 #{latestWithState.id}
                  </span>
                  <span className="rounded bg-surface-container-high px-1.5 py-0.5 font-mono text-[10px] text-on-surface-variant">
                    {latestWithState.created_at?.replace('T', ' ').slice(0, 19) ?? '—'}
                  </span>
                  <span className="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] text-amber-400">
                    {ms.positions.length} 个持仓
                  </span>
                </div>
                <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                  {ms.positions.map((p, i) => {
                    const isLong = p.side?.toLowerCase() === 'long';
                    const isShort = p.side?.toLowerCase() === 'short';
                    const sideColor = isLong ? 'text-emerald-400' : isShort ? 'text-rose-400' : 'text-on-surface-variant';
                    const sideBg = isLong ? 'bg-emerald-400/10 border-emerald-400/30' : isShort ? 'bg-rose-400/10 border-rose-400/30' : 'bg-surface-container-low border-outline-variant/20';
                    const pnlColor = (p.unrealized_pnl ?? 0) > 0 ? 'text-emerald-300' : (p.unrealized_pnl ?? 0) < 0 ? 'text-rose-300' : 'text-on-surface-variant';
                    return (
                      <div key={i} className={`rounded-lg border p-3 font-mono text-[11px] leading-relaxed ${sideBg}`}>
                        <div className="flex items-center justify-between mb-1">
                          <span className="font-bold text-on-surface text-xs">{p.symbol}</span>
                          <span className={`text-[10px] font-semibold ${sideColor}`}>
                            {isLong ? '多' : isShort ? '空' : p.side}
                          </span>
                        </div>
                        <div className="space-y-0.5 text-on-surface-variant">
                          <div className="flex justify-between">
                            <span>入场价</span>
                            <span className="tabular-nums text-on-surface">{p.entry_price?.toFixed?.(p.entry_price >= 100 ? 2 : p.entry_price >= 1 ? 4 : 8) ?? '—'}</span>
                          </div>
                          <div className="flex justify-between">
                            <span>保证金</span>
                            <span className="tabular-nums text-on-surface">{p.margin_used?.toFixed?.(2) ?? '—'} USDT</span>
                          </div>
                          <div className="flex justify-between">
                            <span>杠杆</span>
                            <span className="tabular-nums text-primary">{p.leverage}x</span>
                          </div>
                          <div className="flex justify-between">
                            <span>数量</span>
                            <span className="tabular-nums text-on-surface">{p.quantity}</span>
                          </div>
                          {(p.unrealized_pnl !== undefined && p.unrealized_pnl !== null) && (
                            <div className="flex justify-between border-t border-outline-variant/15 pt-0.5 mt-0.5">
                              <span>浮盈亏</span>
                              <span className={`tabular-nums ${pnlColor}`}>
                                {p.unrealized_pnl >= 0 ? '+' : ''}{p.unrealized_pnl?.toFixed?.(2)} USDT
                              </span>
                            </div>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            )
          })()}
          {/* JSON 开仓/平仓/补仓指令（真实广播数据） */}
          {(() => {
            const latestWithState = (payload?.broadcasts ?? []).find(b => {
              const ms = b.master_state as ScreenMonitorMasterState | undefined;
              return ms?.positions?.length || ms?.instructions?.length;
            });
            if (!latestWithState) return null;
            const ms = latestWithState.master_state as ScreenMonitorMasterState & { instructions?: ScreenInstruction[] };
            const instructions = ms.instructions || [];
            const hasInstructions = instructions.length > 0;
            // Build display JSON: positions snapshot + order instructions
            const displayJson = {
              scan_id: latestWithState.id,
              scan_time: latestWithState.created_at,
              positions_snapshot: ms.positions?.map(p => ({
                symbol: p.symbol,
                side: p.side,
                entry_price: p.entry_price,
                quantity: p.quantity,
                leverage: p.leverage,
                margin_used: p.margin_used,
              })) || [],
              order_instructions: instructions,
              summary: {
                opens: instructions.filter(i => i.type === 'OPEN').length,
                adds: instructions.filter(i => i.type === 'ADD').length,
                closes: instructions.filter(i => i.type === 'CLOSE').length,
                partials: instructions.filter(i => i.type === 'PARTIAL_CLOSE').length,
                flips: instructions.filter(i => i.type === 'FLIP').length,
              },
            };
            const jsonOutput = JSON.stringify(displayJson, null, 2);
            const instColors: Record<string, string> = {
              OPEN: 'text-emerald-400',
              ADD: 'text-amber-400',
              CLOSE: 'text-rose-400',
              PARTIAL_CLOSE: 'text-orange-400',
              FLIP: 'text-purple-400',
            };
            return (
              <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-400/5 p-4">
                <div className="mb-3 flex items-center gap-2 flex-wrap">
                  <span className="material-symbols-outlined text-sm text-amber-400">terminal</span>
                  <span className="text-xs font-semibold text-amber-400">
                    {hasInstructions ? '实时开仓/平仓/补仓指令' : '持仓快照（无新指令）'}
                  </span>
                  {hasInstructions ? (
                    <span className="rounded bg-emerald-400/15 px-1.5 py-0.5 text-[10px] text-emerald-300">
                      待执行
                    </span>
                  ) : (
                    <span className="rounded bg-surface-container-high px-1.5 py-0.5 text-[10px] text-on-surface-variant">
                      无变化
                    </span>
                  )}
                  {hasInstructions && instructions.map((inst, i) => (
                    <span key={i} className={`rounded px-1.5 py-0.5 text-[10px] font-mono font-semibold ${instColors[inst.type] || 'text-on-surface-variant'} bg-surface-container-high/80`}>
                      {inst.type}:{inst.symbol}
                    </span>
                  ))}
                </div>
                <pre className="max-h-[500px] overflow-auto rounded-lg bg-[#0d1117] p-4 text-[11px] leading-relaxed text-[#c9d1d9] font-mono whitespace-pre-wrap break-all">{jsonOutput}</pre>
                <p className="mt-2 text-[10px] text-amber-300/60">
                  以上为监控脚本生成的实时指令。跟单端只执行 type=OPEN 的新开仓指令，忽略历史仓位。
                  补仓(ADD)需用户确认后跟随。平仓(CLOSE)及时同步。
                </p>
              </div>
            )
          })()}
          <div className="max-h-[280px] overflow-auto rounded-lg border border-outline-variant/15">
          {!isLoading && (payload?.broadcasts?.length ?? 0) === 0 && (
            <p className="p-4 text-xs text-on-surface-variant">暂无广播。源账户推送后此处按时间倒序显示；决策条数来自广播里的 JSON。</p>
          )}
          <table className="w-full min-w-[520px] text-left text-[11px]">
            <thead className="sticky top-0 bg-surface-container-highest/95 text-on-surface-variant">
              <tr>
                <th className="px-2 py-2">#</th>
                <th className="px-2 py-2">时间</th>
                <th className="px-2 py-2">主权益</th>
                <th className="px-2 py-2">决策条数</th>
                <th className="px-2 py-2">分析摘要</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-outline-variant/10 text-on-surface">
              {(payload?.broadcasts ?? []).map((b) => (
                <tr key={b.id} className="align-top hover:bg-surface-container-low/60">
                  <td className="px-2 py-2 font-mono text-on-surface-variant">{b.id}</td>
                  <td className="whitespace-nowrap px-2 py-2 text-on-surface-variant">{b.created_at.replace('T', ' ').slice(0, 19)}</td>
                  <td className="px-2 py-2 tabular-nums">{b.master_account_equity?.toFixed?.(2) ?? b.master_account_equity}</td>
                  <td className="px-2 py-2 tabular-nums">{b.decision_count}</td>
                  <td className="max-w-[280px] px-2 py-2 text-on-surface-variant">
                    {b.analysis_preview || '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        </>
      )}

      {tab === 'orders' && (
        <div className="max-h-[280px] overflow-auto rounded-lg border border-outline-variant/15">
          {!isLoading && (payload?.orders?.length ?? 0) === 0 && (
            <p className="p-4 text-xs text-on-surface-variant">
              暂无订单记录。策略源交易员产生交易所挂单后会出现在此（来自系统订单表）。
            </p>
          )}
          <table className="w-full min-w-[640px] text-left text-[11px]">
            <thead className="sticky top-0 bg-surface-container-highest/95 text-on-surface-variant">
              <tr>
                <th className="px-2 py-2">时间</th>
                <th className="px-2 py-2">合约</th>
                <th className="px-2 py-2">方向</th>
                <th className="px-2 py-2">类型</th>
                <th className="px-2 py-2">状态</th>
                <th className="px-2 py-2">数量</th>
                <th className="px-2 py-2">已成交</th>
                <th className="px-2 py-2">均价</th>
                <th className="px-2 py-2">杠杆</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-outline-variant/10 text-on-surface">
              {(payload?.orders ?? []).map((o) => (
                <tr key={o.id} className="hover:bg-surface-container-low/60">
                  <td className="whitespace-nowrap px-2 py-2 text-on-surface-variant">{fmtMs(o.created_at)}</td>
                  <td className="px-2 py-2 font-mono">{o.symbol}</td>
                  <td className="px-2 py-2">{o.side}</td>
                  <td className="px-2 py-2">{o.type}</td>
                  <td className="px-2 py-2">{o.status}</td>
                  <td className="px-2 py-2 tabular-nums">{o.quantity}</td>
                  <td className="px-2 py-2 tabular-nums">{o.filled_quantity}</td>
                  <td className="px-2 py-2 tabular-nums">{o.avg_fill_price}</td>
                  <td className="px-2 py-2 tabular-nums">{o.leverage}x</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'followers' && (
        <div className="max-h-[340px] overflow-auto rounded-lg border border-outline-variant/15">
          {!isLoading && (payload?.followers?.length ?? 0) === 0 && (
            <p className="p-4 text-xs text-on-surface-variant">
              暂无跟单用户记录。请确认：客户策略已开启「合规跟单」、且策略里的
              <strong className="text-on-surface"> 广播源策略 ID </strong>
              与当前这条主控策略 ID 完全一致；客户已创建交易员并启动。请点开本页顶部「
              <strong className="text-on-surface">用户账户</strong>」标签查看列表。
            </p>
          )}
          <table className="w-full min-w-[860px] text-left text-[11px]">
            <thead className="sticky top-0 bg-surface-container-highest/95 text-on-surface-variant">
              <tr>
                <th className="px-2 py-2">交易员 / 跟单策略</th>
                <th className="px-2 py-2">状态</th>
                <th className="px-2 py-2">持仓</th>
                <th className="px-2 py-2">挂单</th>
                <th className="px-2 py-2">提示</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-outline-variant/10 text-on-surface">
              {(payload?.followers ?? []).map((f) => (
                <tr key={f.trader_id} className="align-top hover:bg-surface-container-low/60">
                  <td className="px-2 py-2">
                    <div className="font-semibold">{f.trader_name || '—'}</div>
                    <div className="font-mono text-[10px] text-on-surface-variant">{f.trader_id.slice(0, 10)}…</div>
                    {(f.strategy_name || f.user_label || f.user_email_masked) && (
                      <div className="mt-1 text-[10px] leading-snug text-on-surface-variant">
                        {f.strategy_name && (
                          <div>
                            策略：<span className="font-medium text-on-surface">{f.strategy_name}</span>
                          </div>
                        )}
                        {(f.user_label || f.user_email_masked) && (
                          <div>
                            用户：{f.user_label || f.user_email_masked}
                          </div>
                        )}
                      </div>
                    )}
                  </td>
                  <td className="px-2 py-2">
                    <span className={f.is_running ? 'text-primary' : 'text-on-surface-variant'}>
                      {f.is_running ? '运行中' : '已停止'}
                    </span>
                  </td>
                  <td className="px-2 py-2">
                    {f.positions?.length ? (
                      <div className="space-y-1">
                        {f.positions.map((p) => {
                          const upnl = p.unrealized_pnl
                          const upnlClass =
                            upnl === undefined || Number.isNaN(upnl)
                              ? 'text-on-surface-variant'
                              : upnl > 0
                                ? 'text-emerald-400'
                                : upnl < 0
                                  ? 'text-rose-400'
                                  : 'text-on-surface'
                          return (
                            <div key={p.id} className="rounded bg-surface-container-low px-2 py-1 font-mono text-[10px] leading-relaxed">
                              <span className="text-on-surface">{p.symbol}</span>
                              <span className="ml-1.5 text-primary">{positionDirZh(p)}</span>
                              <span className="ml-1.5 tabular-nums text-on-surface-variant">入场 {fmtBoardPrice(p.entry_price)}</span>
                              <span className="ml-1.5 tabular-nums text-on-surface-variant">{p.leverage}x</span>
                              <span className={`ml-1.5 tabular-nums ${upnlClass}`}>浮盈亏 {fmtBoardPnL(upnl)}</span>
                            </div>
                          )
                        })}
                      </div>
                    ) : (
                      <span className="text-on-surface-variant">—</span>
                    )}
                  </td>
                  <td className="px-2 py-2">
                    {f.orders?.length ? (
                      <div className="space-y-1">
                        {f.orders.map((o) => {
                          const trig = orderTriggerPriceZh(o)
                          const m = o.estimated_margin
                          return (
                            <div key={o.id} className="rounded bg-surface-container-low px-2 py-1 font-mono text-[10px] leading-relaxed">
                              <span className="text-on-surface">{o.symbol}</span>
                              <span className="ml-1.5 text-primary">
                                {orderTypeBriefZh(o.type)} · {orderIntentZh(o)}
                              </span>
                              <span className="ml-1.5 tabular-nums text-on-surface-variant">@{fmtBoardPrice(trig)}</span>
                              <span className="ml-1.5 tabular-nums text-on-surface-variant">
                                保证金≈{m !== undefined && m > 0 ? fmtBoardPrice(m) : '—'}
                              </span>
                              <span className="ml-1.5 tabular-nums text-emerald-300/90">止盈 {fmtBoardPrice(o.take_profit_price)}</span>
                              <span className="ml-1.5 tabular-nums text-rose-300/90">止损 {fmtBoardPrice(o.stop_loss_price)}</span>
                            </div>
                          )
                        })}
                      </div>
                    ) : (
                      <span className="text-on-surface-variant">—</span>
                    )}
                  </td>
                  <td className="max-w-[200px] px-2 py-2 text-on-surface-variant">
                    {f.status_hint ? (
                      <span className="text-amber-200/95">{f.status_hint}</span>
                    ) : (
                      '—'
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[10px] text-on-surface-variant/80">
        <ListOrdered className="h-3 w-3 shrink-0" aria-hidden />
        <span>约每 5 秒自动刷新。</span>
        {lastFetchAt ? (
          <span className="tabular-nums">
            上次更新：{new Date(lastFetchAt).toLocaleString('zh-CN', { hour12: false })}
          </span>
        ) : null}
      </div>
    </div>
  )
}
