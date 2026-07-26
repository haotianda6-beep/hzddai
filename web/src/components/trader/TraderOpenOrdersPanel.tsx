import useSWR from 'swr'
import { Loader2 } from 'lucide-react'
import { api } from '../../lib/api'
import { API_BASE, getAuthHeaders } from '../../lib/api/helpers'
import type { ExchangeOpenOrder } from '../../types'
import type { Language } from '../../i18n/translations'
import type { ComkunMasterBoardPayload } from '../../lib/api/strategies'

function isLimitType(t: unknown): boolean {
  if (t == null || typeof t !== 'string') return false
  const u = t.toUpperCase()
  if (u === 'LIMIT' || u === 'LIMIT_MAKER') return true
  return (u.includes('STOP') && u.includes('LIMIT')) || (u.includes('TAKE_PROFIT') && u.includes('LIMIT'))
}

/** 与后端 kernel.IsConditionalTakeProfitOrderType 对齐 */
function isTakeProfitKind(t: unknown): boolean {
  const u = String(t || '').toUpperCase()
  return u.includes('TAKE_PROFIT')
}

/** 与后端 kernel.IsConditionalStopLossOrderType 对齐 */
function isStopLossKind(t: unknown): boolean {
  const u = String(t || '').toUpperCase()
  if (u.includes('TAKE_PROFIT')) return false
  return u.includes('STOP') || u.includes('TRAILING')
}

function inferredPosSide(o: ExchangeOpenOrder): string {
  const ps = (o.position_side || '').toUpperCase()
  if (ps && ps !== 'BOTH') return ps
  return (o.side || '').toUpperCase() === 'SELL' ? 'SHORT' : 'LONG'
}

/** 与 kernel.FindSLTPFromPendingOrders 对齐：从全量挂单中解析某持仓方向的 SL/TP 触发价 */
function findSLTPForSymbol(rows: ExchangeOpenOrder[], sym: string, posSide: string): { sl: number; tp: number } {
  const su = (sym || '').toUpperCase().trim()
  const ps = (posSide || '').toUpperCase().trim()
  let sl = 0
  let tp = 0
  for (const o of rows) {
    if ((o.symbol || '').toUpperCase().trim() !== su) continue
    const typ = (o.type || '').toUpperCase()
    const os = (o.side || '').toUpperCase()
    const ops = (o.position_side || '').toUpperCase()
    if (ops && ops !== 'BOTH' && ops !== ps) continue
    const trigger = (o.stop_price ?? 0) > 0 ? o.stop_price! : (o.price ?? 0)
    if (trigger <= 0) continue
    if (ps === 'LONG') {
      if (os === 'SELL' && isStopLossKind(typ)) sl = trigger
      if (os === 'SELL' && isTakeProfitKind(typ)) tp = trigger
    } else if (ps === 'SHORT') {
      if (os === 'BUY' && isStopLossKind(typ)) sl = trigger
      if (os === 'BUY' && isTakeProfitKind(typ)) tp = trigger
    }
  }
  return { sl, tp }
}

function fmtNum(n: number, maxFrac = 6): string {
  if (n === undefined || n === null || !Number.isFinite(n)) return '—'
  if (Math.abs(n) >= 1000) return n.toFixed(2)
  if (Math.abs(n) >= 1) return n.toFixed(4)
  return n.toFixed(maxFrac)
}

/** 本行即为交易所返回的止盈/止损条件单时展示 */
function directTPSLNote(o: ExchangeOpenOrder, zh: boolean): string {
  const typ = o.type || ''
  const trig = (o.stop_price ?? 0) > 0 ? o.stop_price! : (o.price ?? 0)
  if (trig <= 0) return ''
  if (isTakeProfitKind(typ)) {
    return zh ? `止盈（交易所）触发 ${fmtNum(trig)}` : `Take-profit @ ${fmtNum(trig)}`
  }
  if (isStopLossKind(typ)) {
    return zh ? `止损（交易所）触发 ${fmtNum(trig)}` : `Stop-loss @ ${fmtNum(trig)}`
  }
  return ''
}

function sortOpenOrders(a: ExchangeOpenOrder, b: ExchangeOpenOrder): number {
  const sa = (a.symbol || '').localeCompare(b.symbol || '')
  if (sa !== 0) return sa
  const aLim = isLimitType(a.type) ? 0 : 1
  const bLim = isLimitType(b.type) ? 0 : 1
  if (aLim !== bLim) return aLim - bLim
  return String(a.order_id).localeCompare(String(b.order_id))
}

export function TraderOpenOrdersPanel({
  traderId,
  language,
  refreshMs = 15000,
}: {
  traderId: string
  language: Language
  refreshMs?: number
}) {
  const zh = language === 'zh'
  const { data, error, isLoading, isValidating } = useSWR<ExchangeOpenOrder[]>(
    traderId ? ['exchange-open-orders', traderId] : null,
    () => api.getOpenOrders(traderId, undefined, true),
    { refreshInterval: refreshMs, revalidateOnFocus: true }
  )

  const rows = (data ?? []).filter(
    (o): o is ExchangeOpenOrder => o != null && typeof o === 'object'
  )
  const sorted = [...rows].sort(sortOpenOrders)

  const title = zh ? '当前交易所挂单（未成交）' : 'Open orders (exchange)'
  const sub = zh
    ? '含限价、止盈止损市价/条件单；触发价来自交易所。限价单与附带止盈止损在交易所是不同订单，本表各占一行。'
    : 'Limits and TP/SL from the exchange. Attached TP/SL are separate rows from the limit order.'
  const empty = zh ? '暂无未成交挂单。' : 'No pending orders.'
  const errMsg = zh ? '加载挂单失败' : 'Failed to load open orders'
  const cols = zh
    ? ['合约', '方向', '持仓侧', '类型', '止盈/止损说明', '委托价', '触发价', '数量', '状态', '订单号']
    : ['Symbol', 'Side', 'Pos.', 'Type', 'TP / SL', 'Price', 'Trigger', 'Qty', 'Status', 'Order ID']

  function tpslCell(o: ExchangeOpenOrder): string {
    const direct = directTPSLNote(o, zh)
    if (direct) return direct
    if (isLimitType(o.type)) {
      const ps = inferredPosSide(o)
      const { sl, tp } = findSLTPForSymbol(rows, o.symbol ?? '', ps)
      if (sl > 0 || tp > 0) {
        const parts: string[] = []
        if (sl > 0) parts.push(zh ? `同品种止损 ${fmtNum(sl)}` : `SL ${fmtNum(sl)}`)
        if (tp > 0) parts.push(zh ? `同品种止盈 ${fmtNum(tp)}` : `TP ${fmtNum(tp)}`)
        return zh ? `（来自其他挂单）${parts.join(' · ')}` : `(from other orders) ${parts.join(' · ')}`
      }
      return zh ? '—（无同向条件单）' : '—'
    }
    return '—'
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h3 className="text-sm font-bold text-[#EAECEF]">{title}</h3>
          <p className="mt-0.5 text-[11px] text-[#848E9C]">{sub}</p>
        </div>
        {isValidating && !isLoading ? (
          <Loader2 className="h-4 w-4 animate-spin text-[#d4ff33]/70" aria-hidden />
        ) : null}
      </div>

      {error ? (
        <div className="rounded-lg border border-[#F6465D]/30 bg-[#F6465D]/10 px-3 py-2 text-xs text-[#F6465D]">
          {errMsg}: {error instanceof Error ? error.message : String(error)}
        </div>
      ) : null}

      {isLoading && !data ? (
        <div className="flex items-center gap-2 py-8 text-sm text-[#848E9C]">
          <Loader2 className="h-5 w-5 animate-spin text-[#d4ff33]" />
          {zh ? '正在拉取挂单…' : 'Loading…'}
        </div>
      ) : rows.length === 0 ? (
        <div className="rounded-lg border border-dashed border-[#46484d]/55 bg-nofx-bg-tertiary/20 py-10 text-center text-sm text-[#848E9C]">{empty}</div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-[#2b3139]">
          <table className="w-full min-w-[960px] text-left text-xs">
            <thead className="border-b border-[#2b3139] bg-nofx-bg-tertiary/60 text-[#848E9C]">
              <tr>
                {cols.map((c) => (
                  <th key={c} className="whitespace-nowrap px-2 py-2 font-semibold">
                    {c}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-[#2b3139]/80 text-[#EAECEF]">
              {sorted.map((o, idx) => {
                const isTP = isTakeProfitKind(o.type)
                const isSL = isStopLossKind(o.type)
                const isCond = isTP || isSL
                const typeBadge =
                  isTP
                    ? 'bg-[#0ECB81]/15 text-[#0ECB81]'
                    : isSL
                      ? 'bg-[#F6465D]/15 text-[#F6465D]'
                      : 'bg-[#d4ff33]/15 text-[#d4ff33]'
                return (
                  <tr
                    key={`${String(o.order_id ?? idx)}-${idx}`}
                    className={`hover:bg-white/[0.03] ${isCond ? 'bg-white/[0.02]' : ''}`}
                  >
                    <td className="px-2 py-2 font-mono">{o.symbol ?? '—'}</td>
                    <td className="px-2 py-2">{o.side ?? '—'}</td>
                    <td className="px-2 py-2">{o.position_side || '—'}</td>
                    <td className="px-2 py-2">
                      <span className={`rounded px-1.5 py-0.5 ${typeBadge}`}>{o.type ?? '—'}</span>
                    </td>
                    <td className="max-w-[220px] whitespace-normal break-words px-2 py-2 text-[#b7bdc6]">
                      {tpslCell(o)}
                    </td>
                    <td className="px-2 py-2 tabular-nums">{fmtNum(o.price)}</td>
                    <td className="px-2 py-2 tabular-nums font-medium text-[#EAECEF]">{fmtNum(o.stop_price)}</td>
                    <td className="px-2 py-2 tabular-nums">{fmtNum(o.quantity)}</td>
                    <td className="px-2 py-2 text-[#848E9C]">{o.status ?? '—'}</td>
                    <td className="max-w-[100px] truncate px-2 py-2 font-mono text-[10px] text-[#5e6673]">
                      {o.order_id ?? '—'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      <p className="text-[10px] leading-relaxed text-[#5e6673]">
        {zh
          ? '说明：你在限价单上「附带」的止盈止损，交易所会拆成独立的条件单行（类型多为 STOP_MARKET / TAKE_PROFIT_MARKET），不会写在限价那一行的触发价里。限价行若写「同品种止损/止盈」，是本页从其它行汇总的提示。若账户为统一/组合保证金且 API 只走 U 本位 fapi，部分单可能需交易所侧支持才能出现在本表。'
          : 'Bracket TP/SL appear as separate conditional rows, not on the limit row’s trigger. Limit-row hints aggregate sibling orders. Some portfolio-margin orders may not appear via standard fapi.'}
      </p>
    </div>
  )
}

export function SourceUserAccountsPanel({
  strategyId,
  language,
}: {
  strategyId?: string
  language: Language
}) {
  const zh = language === 'zh'
  const { data, isLoading, isValidating } = useSWR<ComkunMasterBoardPayload | null>(
    strategyId ? ['source-user-accounts', strategyId] : null,
    async () => {
      const res = await fetch(
        `${API_BASE}/strategies/${encodeURIComponent(strategyId!)}/comkun-master-board`,
        { headers: getAuthHeaders() }
      )
      // 普通用户没有策略源看板权限时静默隐藏，订单页只显示自己的挂单。
      if (res.status === 403 || res.status === 404) return null
      if (!res.ok) return null
      return (await res.json()) as ComkunMasterBoardPayload
    },
    { refreshInterval: 15000, revalidateOnFocus: true, shouldRetryOnError: false }
  )

  if (!strategyId || data === null) return null

  const followers = data?.followers ?? []
  const hasRows = followers.some((f) => (f.positions?.length ?? 0) > 0 || (f.orders?.length ?? 0) > 0)

  return (
    <div className="mt-5 space-y-3 border-t border-[#2b3139]/80 pt-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-bold text-[#EAECEF]">
            {zh ? '用户账户持仓 / 挂单' : 'User accounts positions / orders'}
          </h3>
          <p className="mt-0.5 text-[11px] text-[#848E9C]">
            {zh ? '显示所有使用本策略源的交易员当前持仓、限价单、止盈止损。' : 'All users using this strategy source.'}
          </p>
        </div>
        {isValidating || isLoading ? <Loader2 className="h-4 w-4 animate-spin text-[#d4ff33]/70" /> : null}
      </div>

      {!hasRows && !isLoading ? (
        <div className="rounded-lg border border-dashed border-[#46484d]/55 bg-nofx-bg-tertiary/20 py-8 text-center text-sm text-[#848E9C]">
          {zh ? '暂无用户账户持仓或挂单。' : 'No user account positions or orders.'}
        </div>
      ) : null}

      {followers.map((f) => {
        const positions = f.positions ?? []
        const orders = f.orders ?? []
        if (positions.length === 0 && orders.length === 0) return null
        return (
          <div key={f.trader_id} className="rounded-lg border border-[#2b3139] bg-black/20 p-3">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <div>
                <div className="font-semibold text-[#EAECEF]">{f.user_label || f.user_email_masked || '未知用户'}</div>
                <div className="mt-0.5 text-[11px] text-[#b7bdc6]">{f.trader_name || '—'}</div>
                <div className="font-mono text-[10px] text-[#5e6673]">{f.trader_id.slice(0, 10)}…</div>
              </div>
              <span className={f.is_running ? 'text-xs text-[#d4ff33]' : 'text-xs text-[#848E9C]'}>
                {f.is_running ? (zh ? '运行中' : 'Running') : zh ? '已停止' : 'Stopped'}
              </span>
            </div>

            {positions.length > 0 ? (
              <div className="mb-3">
                <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-[#848E9C]">
                  {zh ? '持仓' : 'Positions'}
                </div>
                <div className="space-y-1">
                  {positions.map((p) => (
                    <div key={p.id} className="rounded bg-nofx-bg-tertiary/50 px-2 py-1 text-[11px] text-[#EAECEF]">
                      <span className="font-mono">{p.symbol}</span>
                      <span className="ml-2">{p.side}</span>
                      <span className="ml-2 tabular-nums">qty {fmtNum(p.quantity)}</span>
                      <span className="ml-2 tabular-nums">@ {fmtNum(p.entry_price)}</span>
                      <span className="ml-2 tabular-nums">{p.leverage}x</span>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}

            {orders.length > 0 ? (
              <div>
                <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-[#848E9C]">
                  {zh ? '挂单 / 止盈止损' : 'Orders / TP SL'}
                </div>
                <div className="space-y-1">
                  {orders.map((o) => (
                    <div key={o.id} className="rounded bg-nofx-bg-tertiary/50 px-2 py-1 text-[11px] text-[#EAECEF]">
                      <span className="font-mono">{o.symbol}</span>
                      <span className="ml-2">{o.side}</span>
                      <span className="ml-2">{o.position_side || '—'}</span>
                      <span className="ml-2">{o.type}</span>
                      <span className="ml-2">{o.status}</span>
                      <span className="ml-2 tabular-nums">qty {fmtNum(o.quantity)}</span>
                      <span className="ml-2 tabular-nums">@ {fmtNum(o.price || o.stop_price || o.avg_fill_price)}</span>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}
