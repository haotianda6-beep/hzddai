import useSWR from 'swr'
import { useEffect, useState } from 'react'
import { Users, ListOrdered } from 'lucide-react'
import { api } from '../../lib/api'
import type {
  ComkunMasterBoardOrder,
  ComkunMasterBoardPayload,
  ComkunMasterBoardPosition,
} from '../../lib/api/strategies'
import type { Language } from '../../i18n/translations'

function fmtBoardPrice(n: number | undefined | null): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  const abs = Math.abs(n)
  const maxFrac = abs >= 1000 ? 2 : abs >= 1 ? 4 : 8
  return n.toLocaleString(undefined, { maximumFractionDigits: maxFrac })
}

function fmtBoardPnL(n: number | undefined | null, lang: Language): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  const abs = Math.abs(n)
  const maxFrac = abs >= 1 ? 2 : 4
  const s = n.toLocaleString(undefined, { maximumFractionDigits: maxFrac, signDisplay: 'always' })
  return lang === 'zh' ? `${s} USDT` : `${s} USDT`
}

/** 持仓方向：long/short 或 BUY/SELL → 简体中文 */
function positionDirZh(p: ComkunMasterBoardPosition): string {
  const s = (p.side || '').toLowerCase()
  if (s === 'long' || s === 'buy') return '开多'
  if (s === 'short' || s === 'sell') return '开空'
  return p.side ? String(p.side) : '—'
}

/** 挂单方向简述：开仓意图或条件平仓 */
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

export function ComkunMasterFollowersSidebar({
  strategyId,
  language,
  embedded = false,
}: {
  strategyId: string
  language: Language
  /** 嵌在主控看板右侧 AI 分析下方时，去掉 grid 定位并压缩高度 */
  embedded?: boolean
}) {
  const [lastFetchAt, setLastFetchAt] = useState<number | null>(null)
  // 与 ComkunFollowListingDataBoard 共用同一 SWR key，避免同页对 comkun-master-board 重复请求（减轻币安限频）
  const { data: payload, error, isLoading } = useSWR<ComkunMasterBoardPayload>(
    strategyId ? ['comkun-master-board', strategyId] : null,
    () => api.getComkunMasterBoard(strategyId),
    {
      refreshInterval: 20000,
      revalidateOnFocus: true,
      keepPreviousData: true,
      errorRetryCount: 2,
      shouldRetryOnError: true,
    }
  )
  const showInitialLoading = isLoading && !payload && !error

  useEffect(() => {
    if (payload && !error) setLastFetchAt(Date.now())
  }, [payload, error])

  const loadErr = error
    ? language === 'zh'
      ? `加载失败：${error instanceof Error ? error.message : String(error)}`
      : `Failed: ${error instanceof Error ? error.message : String(error)}`
    : null

  const title = language === 'zh' ? 'COMKUN-AI · 跟单客户' : 'COMKUN-AI · Followers'
  const subtitle =
    language === 'zh' ? '绑定本主控策略的跟单交易员（约 5 秒刷新）' : 'Traders following this master (≈5s refresh)'
  const emptyMsg =
    language === 'zh'
      ? '暂无跟单客户。客户复制本策略并启动被控交易员后会显示在此。'
      : 'No followers yet.'
  const running = language === 'zh' ? '运行中' : 'Running'
  const stopped = language === 'zh' ? '已停止' : 'Stopped'
  const hintLabel = language === 'zh' ? '提示' : 'Hint'

  const followers = payload?.followers ?? []
  const zh = language === 'zh'

  return (
    <aside
      className={
        embedded
          ? 'flex min-w-0 w-full flex-none flex-col overflow-visible rounded-xl border-0 bg-nofx-bg-secondary/95 p-3 shadow-[0_0_24px_rgba(0,0,0,0.35)] md:p-4'
          : 'order-2 flex min-h-0 min-w-0 w-full max-h-[min(72vh,36rem)] flex-col self-start overflow-hidden rounded-xl border-0 bg-nofx-bg-secondary/95 p-3 shadow-[0_0_24px_rgba(0,0,0,0.35)] md:p-4 lg:order-none lg:col-start-2 lg:row-start-1 lg:w-auto lg:max-h-[calc(100vh-5.5rem)] lg:self-start'
      }
    >
      <div className="mb-2 flex shrink-0 items-center gap-2 border-b border-[#2b3139]/80 pb-2">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-[#c4cf45]/40 bg-[#c4cf45]/12 text-[#c4cf45] shadow-[0_0_14px_rgba(212,255,51,0.22)]">
          <Users className="h-4 w-4" aria-hidden />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="font-['Space_Grotesk',sans-serif] text-lg font-bold text-[#EAECEF]">{title}</h2>
          <p className="text-xs text-[#848E9C]">{subtitle}</p>
          {followers.length > 0 && (
            <p className="mt-0.5 text-[11px] text-[#5e6673]">
              {zh ? `共 ${followers.length} 个账户` : `${followers.length} accounts`}
            </p>
          )}
        </div>
      </div>

      <div
        className={
          embedded
            ? 'mt-0 space-y-2 pr-1 pt-0'
            : 'custom-scrollbar mt-0 min-h-0 flex-1 space-y-2 overflow-y-auto overscroll-contain pr-1 pt-0'
        }
      >
        {loadErr && (
          <div className="rounded-lg border border-red-500/25 bg-red-500/10 px-3 py-2 text-xs text-red-200/90">{loadErr}</div>
        )}
        {showInitialLoading && (
          <div className="py-12 text-center text-sm text-[#848E9C]">{zh ? '加载中…' : 'Loading…'}</div>
        )}
        {!showInitialLoading && !loadErr && followers.length === 0 && (
          <div className="py-12 text-center text-[#848E9C] opacity-80">
            <div className="mb-2 text-3xl opacity-40 grayscale">👥</div>
            <p className="text-sm">{emptyMsg}</p>
          </div>
        )}
        {followers.map((f) => (
          <div
            key={f.trader_id}
            className="rounded-lg border border-[#2b3139]/80 bg-[#1c1c1c]/90 p-3 text-left shadow-[0_1px_0_rgba(255,255,255,0.03)_inset]"
          >
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="truncate font-semibold text-[#EAECEF]">{f.trader_name || '—'}</div>
                {(f.user_label || f.user_email_masked) && (
                  <div className="truncate text-[11px] text-[#848E9C]">{f.user_label || f.user_email_masked}</div>
                )}
                <div className="mt-0.5 font-mono text-[11px] text-[#5e6673]">{f.trader_id.slice(0, 10)}…</div>
              </div>
              <span
                className={`shrink-0 rounded px-2 py-0.5 text-xs font-bold ${
                  f.is_running ? 'bg-[#c4cf45]/15 text-[#c4cf45]' : 'bg-white/5 text-[#848E9C]'
                }`}
              >
                {f.is_running ? running : stopped}
              </span>
            </div>
            {f.exchange_type && (
              <div className="mt-1.5 text-[11px] uppercase text-[#5e6673]">{f.exchange_type}</div>
            )}

            <div className="mt-2 space-y-1.5">
              <div className="text-xs font-medium text-[#848E9C]">{zh ? '持仓' : 'Positions'}</div>
              {!f.positions?.length ? (
                <div className="text-xs text-[#5e6673]">{zh ? '无' : 'None'}</div>
              ) : (
                <div className="space-y-1">
                  {f.positions.map((p) => {
                    const upnl = p.unrealized_pnl
                    const upnlColor =
                      upnl === undefined || Number.isNaN(upnl)
                        ? 'text-[#848E9C]'
                        : upnl > 0
                          ? 'text-emerald-400/95'
                          : upnl < 0
                            ? 'text-rose-400/95'
                            : 'text-[#B7BDC6]'
                    return (
                      <div
                        key={String(p.id)}
                        className="rounded border border-[#2b3139]/60 bg-black/25 px-2 py-1.5 font-mono text-xs leading-relaxed text-[#B7BDC6]"
                      >
                        <span className="text-[#EAECEF]">{p.symbol}</span>
                        {zh ? (
                          <>
                            <span className="ml-1.5 text-[#c4cf45]/90">{positionDirZh(p)}</span>
                            <span className="ml-1.5 tabular-nums">
                              {zh ? '入场' : 'Entry'} {fmtBoardPrice(p.entry_price)}
                            </span>
                            <span className="ml-1.5 tabular-nums">{p.leverage}x</span>
                            <span className={`ml-1.5 tabular-nums ${upnlColor}`}>
                              {zh ? '浮盈亏' : 'uPnL'} {fmtBoardPnL(upnl, language)}
                            </span>
                          </>
                        ) : (
                          <>
                            <span className="ml-1.5 uppercase text-[#c4cf45]/90">{p.side}</span>
                            <span className="ml-1.5 tabular-nums">entry {fmtBoardPrice(p.entry_price)}</span>
                            <span className="ml-1.5 tabular-nums">{p.leverage}x</span>
                            <span className={`ml-1.5 tabular-nums ${upnlColor}`}>uPnL {fmtBoardPnL(upnl, language)}</span>
                          </>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}

              <div className="pt-0.5 text-xs font-medium text-[#848E9C]">{zh ? '挂单' : 'Orders'}</div>
              {!f.orders?.length ? (
                <div className="text-xs text-[#5e6673]">{zh ? '无' : 'None'}</div>
              ) : (
                <div className="space-y-1">
                  {f.orders.map((o) => {
                    const trig = orderTriggerPriceZh(o)
                    const margin = o.estimated_margin
                    const tp = o.take_profit_price
                    const sl = o.stop_loss_price
                    return (
                      <div
                        key={String(o.id)}
                        className="rounded border border-[#2b3139]/60 bg-black/25 px-2 py-1.5 font-mono text-xs leading-relaxed text-[#B7BDC6]"
                      >
                        <span className="text-[#EAECEF]">{o.symbol}</span>
                        {zh ? (
                          <>
                            <span className="ml-1.5 text-[#c4cf45]/85">
                              {orderTypeBriefZh(o.type)} · {orderIntentZh(o)}
                            </span>
                            <span className="ml-1.5 tabular-nums">
                              @{fmtBoardPrice(trig)}{' '}
                              <span className="text-[#5e6673]">
                                （{margin !== undefined && margin > 0 ? `保证金≈${fmtBoardPrice(margin)}` : '保证金 —'}）
                              </span>
                            </span>
                            <span className="ml-1.5 inline-flex flex-wrap items-baseline gap-x-1 tabular-nums text-emerald-300/85">
                              <span className="text-[#848E9C]">止盈</span>
                              <span className="font-semibold">{fmtBoardPrice(tp)}</span>
                            </span>
                            <span className="ml-1.5 inline-flex flex-wrap items-baseline gap-x-1 tabular-nums text-rose-300/85">
                              <span className="text-[#848E9C]">止损</span>
                              <span className="font-semibold">{fmtBoardPrice(sl)}</span>
                            </span>
                          </>
                        ) : (
                          <>
                            <span className="ml-1.5 text-[#c4cf45]/85">
                              {orderTypeBriefZh(o.type)} · {o.side}/{o.position_side || '—'}
                            </span>
                            <span className="ml-1.5 tabular-nums">
                              @{fmtBoardPrice(trig)} margin≈{fmtBoardPrice(margin)}
                            </span>
                            <span className="ml-1.5 tabular-nums">TP {fmtBoardPrice(tp)}</span>
                            <span className="ml-1.5 tabular-nums">SL {fmtBoardPrice(sl)}</span>
                          </>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
            </div>

            {f.status_hint ? (
              <div className="mt-2 rounded border border-amber-500/20 bg-amber-500/5 px-3 py-2 text-sm leading-snug text-amber-100/95">
                <span className="font-semibold text-amber-200/90">{hintLabel}: </span>
                <span className="break-words">{f.status_hint}</span>
              </div>
            ) : null}
          </div>
        ))}
      </div>

      <div className="mt-2 flex shrink-0 flex-wrap items-center gap-x-2 gap-y-1 border-t border-[#2b3139]/60 pt-2 text-[11px] text-[#5e6673]">
        <ListOrdered className="h-3.5 w-3.5 shrink-0 opacity-70" aria-hidden />
        <span>{zh ? '数据来自策略主控看板接口' : 'From comkun-master-board API'}</span>
        {lastFetchAt ? (
          <span className="tabular-nums">
            {zh ? '上次更新' : 'Updated'}:{' '}
            {new Date(lastFetchAt).toLocaleString(zh ? 'zh-CN' : 'en-US', { hour12: false })}
          </span>
        ) : null}
      </div>
    </aside>
  )
}
