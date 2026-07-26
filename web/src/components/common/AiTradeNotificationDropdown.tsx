import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import useSWR from 'swr'
import { toast } from 'sonner'
import {
  Bell,
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Info,
  CheckCheck,
} from 'lucide-react'
import { useAuth } from '../../contexts/AuthContext'
import { api } from '../../lib/api'
import type { DecisionRecord, DecisionAction, TraderInfo } from '../../types'

const READ_IDS_KEY = 'comkun-notif-read-ids-v1'
const NOTIFY_TRADER_LS_KEY = 'nofx_notification_trader_id'
const MARKET_REVIEW_MARKER_RE =
  /(?:审核标记：)?market_review:([0-9a-fA-F-]{20,})/i

function pickNotifyTraderId(
  traders: TraderInfo[] | undefined
): string | undefined {
  const list = traders || []
  if (!list.length) return undefined
  try {
    const saved = localStorage.getItem(NOTIFY_TRADER_LS_KEY)
    if (saved && list.some((t) => t.trader_id === saved)) return saved
  } catch {
    /* ignore */
  }
  return list[0]?.trader_id
}
const MAX_READ_IDS = 400

function loadReadIds(): Set<string> {
  try {
    const raw = localStorage.getItem(READ_IDS_KEY)
    if (!raw) return new Set()
    const arr = JSON.parse(raw) as string[]
    return new Set(Array.isArray(arr) ? arr : [])
  } catch {
    return new Set()
  }
}

function saveReadIds(ids: Set<string>) {
  try {
    const arr = [...ids]
    const tail = arr.length > MAX_READ_IDS ? arr.slice(-MAX_READ_IDS) : arr
    localStorage.setItem(READ_IDS_KEY, JSON.stringify(tail))
  } catch {
    /* 隐私模式 / 配额满：勿抛错导致整页崩溃 */
  }
}

/** 将英文动作码转为中文（通知里统一中文） */
function actionLabelZh(raw: string): string {
  const x = (raw || '').toLowerCase().trim()
  const map: Record<string, string> = {
    open_long: '开多仓',
    open_short: '开空仓',
    close_long: '平多仓',
    close_short: '平空仓',
    take_profit: '止盈',
    take_profit_long: '多仓止盈',
    take_profit_short: '空仓止盈',
    stop_loss: '止损',
    stop_loss_long: '多仓止损',
    stop_loss_short: '空仓止损',
    adjust_leverage: '调整杠杆',
    modify_margin: '调整保证金',
    hold: '观望',
    wait: '等待',
  }
  if (map[x]) return map[x]
  if (x.includes('take_profit')) return '止盈'
  if (x.includes('stop_loss')) return '止损'
  if (x.includes('open')) return '开仓'
  if (x.includes('close')) return '平仓'
  return raw ? raw.replace(/_/g, ' ') : '操作'
}

function formatAction(a: DecisionAction): string {
  const label = actionLabelZh(a.action || '')
  const tp = a.take_profit != null ? ` 止盈价 ${a.take_profit}` : ''
  const sl = a.stop_loss != null ? ` 止损价 ${a.stop_loss}` : ''
  const ok = a.success === false ? ` 失败${a.error ? `：${a.error}` : ''}` : ''
  return `【${label}】${a.symbol || '—'} 数量 ${a.quantity ?? '—'} 杠杆 ${a.leverage ?? '—'}x${tp}${sl}${ok}`
}

function actionBrief(a: DecisionAction): string {
  const sym = a.symbol || '—'
  const act = actionLabelZh(a.action || '')
  return `${sym} · ${act}`
}

function isNotifyWorthyAction(a: DecisionAction): boolean {
  const x = (a.action || '').toLowerCase()
  if (
    x === 'open_long' ||
    x === 'open_short' ||
    x === 'close_long' ||
    x === 'close_short'
  )
    return true
  if (
    x.includes('take_profit') ||
    x.includes('stop_loss') ||
    x.includes('tp') ||
    x.includes('sl')
  )
    return true
  if (
    x.includes('leverage') ||
    x.includes('margin') ||
    x.includes('adjust') ||
    x.includes('modify')
  )
    return true
  return false
}

function recordsToFeed(
  records: DecisionRecord[]
): { id: string; time: string; text: string; brief: string }[] {
  const rows: { id: string; time: string; text: string; brief: string }[] = []
  for (const r of records) {
    const t = r.timestamp || ''
    const acts = r.decisions || []
    acts.forEach((a, i) => {
      if (!isNotifyWorthyAction(a)) return
      rows.push({
        id: `ai-${t}-a-${i}`,
        time: t,
        text: formatAction(a),
        brief: actionBrief(a),
      })
    })
  }
  return rows.slice(0, 80)
}

function notifTone(
  title: string,
  body: string
): 'ok' | 'warn' | 'err' | 'info' {
  const s = `${title} ${body}`.toLowerCase()
  if (/拒绝|失败|error|reject|denied|invalid/i.test(s)) return 'err'
  if (/止损|liquidat|warning|风险/i.test(s)) return 'warn'
  if (/成功|充值|入账|完成|opened|filled|✓/i.test(s)) return 'ok'
  return 'info'
}

function parseMarketReviewNotification(body: string): {
  text: string
  strategyId?: string
} {
  const raw = body || ''
  const match = raw.match(MARKET_REVIEW_MARKER_RE)
  const strategyId = match?.[1]
  const text = raw
    .split('\n')
    .filter((line) => !MARKET_REVIEW_MARKER_RE.test(line))
    .join('\n')
    .trim()
  return { text: text || raw, strategyId }
}

function ToneIcon({ tone }: { tone: 'ok' | 'warn' | 'err' | 'info' }) {
  const cls = 'h-5 w-5 shrink-0'
  if (tone === 'ok')
    return <CheckCircle2 className={`${cls} text-emerald-400`} aria-hidden />
  if (tone === 'warn')
    return <AlertTriangle className={`${cls} text-[#d4ff33]`} aria-hidden />
  if (tone === 'err')
    return <XCircle className={`${cls} text-red-400`} aria-hidden />
  return <Info className={`${cls} text-sky-400`} aria-hidden />
}

export function AiTradeNotificationDropdown() {
  const { token, user } = useAuth()
  const [open, setOpen] = useState(false)
  const [readIds, setReadIds] = useState<Set<string>>(() => loadReadIds())
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [reviewBusyId, setReviewBusyId] = useState<string | null>(null)
  const ref = useRef<HTMLDivElement>(null)

  const persistRead = useCallback((next: Set<string>) => {
    setReadIds(next)
    saveReadIds(next)
  }, [])

  const { data: tradersForBell } = useSWR(
    token ? 'my-traders-bell' : null,
    () => api.getTraders(true),
    {
      refreshInterval: 60000,
      revalidateOnFocus: false,
    }
  )
  const notifyTraderId = useMemo(
    () => pickNotifyTraderId(tradersForBell),
    [tradersForBell]
  )

  const { data: records, isLoading } = useSWR(
    token && notifyTraderId ? ['ai-trade-notifications', notifyTraderId] : null,
    () => api.getLatestDecisions(notifyTraderId, 30, true),
    {
      refreshInterval: token && notifyTraderId ? 20000 : 0,
      revalidateOnFocus: true,
    }
  )

  const {
    data: notifPack,
    isLoading: notifLoading,
    mutate: mutateNotifications,
  } = useSWR(
    token ? ['user-notifications-bell'] : null,
    () => api.getUserNotifications(),
    { refreshInterval: token ? 25000 : 0, revalidateOnFocus: true }
  )

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (!open) return
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])

  const feed = useMemo(() => recordsToFeed(records || []), [records])

  const systemRows = useMemo(
    () =>
      (notifPack?.notifications ?? []).map((n) => {
        const parsed = parseMarketReviewNotification(n.body)
        return {
          id: `sys-${n.id}`,
          time: n.created_at,
          title: n.broadcast ? `【公告】${n.title}` : n.title,
          text: parsed.text,
          tone: notifTone(n.title, parsed.text),
          preview:
            (parsed.text || '').split('\n')[0].slice(0, 72) +
            ((parsed.text || '').length > 72 ? '…' : ''),
          isBroadcast: Boolean(n.broadcast),
          marketReviewStrategyId: parsed.strategyId,
        }
      }),
    [notifPack]
  )

  const unreadCount = useMemo(() => {
    let n = 0
    for (const r of systemRows) {
      if (!readIds.has(r.id)) n++
    }
    for (const r of feed) {
      if (!readIds.has(r.id)) n++
    }
    return n
  }, [systemRows, feed, readIds])

  const markRead = useCallback(
    (id: string) => {
      if (readIds.has(id)) return
      const next = new Set(readIds)
      next.add(id)
      persistRead(next)
    },
    [readIds, persistRead]
  )

  const markAllRead = useCallback(() => {
    const next = new Set(readIds)
    systemRows.forEach((r) => next.add(r.id))
    feed.forEach((r) => next.add(r.id))
    persistRead(next)
  }, [readIds, systemRows, feed, persistRead])

  const toggleRow = useCallback(
    (id: string) => {
      markRead(id)
      setExpandedId((prev) => (prev === id ? null : id))
    },
    [markRead]
  )

  const handleMarketReview = useCallback(
    async (strategyId: string, action: 'approve' | 'reject') => {
      const busyKey = `${strategyId}:${action}`
      setReviewBusyId(busyKey)
      try {
        await api.postAdminStrategyMarketReview(strategyId, action)
        toast.success(action === 'approve' ? '已同意显示' : '已拒绝显示')
        await mutateNotifications()
      } catch (err) {
        toast.error(err instanceof Error ? err.message : '审核处理失败')
      } finally {
        setReviewBusyId((current) => (current === busyKey ? null : current))
      }
    },
    [mutateNotifications]
  )

  const onBellClick = useCallback(() => {
    setOpen((v) => !v)
  }, [])

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={onBellClick}
        className="relative rounded-lg p-2 text-nofx-text-muted transition-colors hover:bg-white/[0.06] hover:text-[#d4ff33]"
        title="系统通知与 AI 交易通知"
      >
        <Bell className="h-5 w-5" />
        {unreadCount > 0 && (
          <span className="absolute -right-0.5 -top-0.5 flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-bold leading-none text-white shadow-sm">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div
          className="fixed left-2 right-2 top-[70px] z-[60] max-h-[min(78vh,580px)] overflow-hidden rounded-2xl border border-white/10 shadow-[0_24px_60px_rgba(0,0,0,0.75)] sm:absolute sm:left-auto sm:right-0 sm:top-full sm:mt-2 sm:w-[min(100vw-1.25rem,420px)]"
          style={{ backgroundColor: '#1c1c1c' }}
        >
          <div
            className="flex items-start justify-between gap-2 border-b border-white/10 px-3 py-3"
            style={{ backgroundColor: '#131313' }}
          >
            <div>
              <p className="text-base font-bold text-[#eaecef]">通知中心</p>
              <p className="mt-1 text-[13px] leading-snug text-[#aaabb0]">
                点击一条可展开详情；已读不再计入铃铛数字
              </p>
            </div>
            {(systemRows.length > 0 || feed.length > 0) && (
              <button
                type="button"
                onClick={markAllRead}
                className="inline-flex shrink-0 items-center gap-1 rounded-lg border border-[#d4ff33]/35 bg-[#d4ff33]/10 px-2.5 py-1.5 text-[13px] font-semibold text-[#d4ff8a] transition-colors hover:bg-[#d4ff33]/18"
              >
                <CheckCheck className="h-4 w-4" aria-hidden />
                全部已读
              </button>
            )}
          </div>

          <div
            className="max-h-[min(62vh,500px)] overflow-y-auto px-2.5 py-2.5"
            style={{ backgroundColor: '#1c1c1c' }}
          >
            {!notifLoading &&
              !isLoading &&
              systemRows.length === 0 &&
              feed.length === 0 && (
                <p className="px-2 py-8 text-center text-[14px] leading-relaxed text-[#aaabb0]">
                  暂无通知。充值入账、管理员调账后在此显示系统通知；在仪表盘选择交易员后，可在此看到该交易员的
                  AI 交易动态。
                </p>
              )}

            {systemRows.length > 0 && (
              <>
                <p className="mb-2 px-1 text-[13px] font-bold text-[#848E9C]">
                  系统通知
                </p>
                <ul className="space-y-2">
                  {systemRows.map((row) => {
                    const isOpen = expandedId === row.id
                    const unread = !readIds.has(row.id)
                    return (
                      <li key={row.id}>
                        <div
                          role="button"
                          tabIndex={0}
                          onClick={() => toggleRow(row.id)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' || e.key === ' ') {
                              e.preventDefault()
                              toggleRow(row.id)
                            }
                          }}
                          className={`w-full rounded-xl border px-3 py-2.5 text-left transition-all ${
                            unread
                              ? 'border-[#d4ff33]/35 bg-[#131313]'
                              : 'border-white/10 bg-[#131313] hover:border-white/15'
                          } cursor-pointer`}
                        >
                          <div className="flex items-start gap-2.5">
                            <ToneIcon tone={row.tone} />
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1">
                                {isOpen ? (
                                  <ChevronDown
                                    className="h-4 w-4 shrink-0 text-[#848E9C]"
                                    aria-hidden
                                  />
                                ) : (
                                  <ChevronRight
                                    className="h-4 w-4 shrink-0 text-[#848E9C]"
                                    aria-hidden
                                  />
                                )}
                                <span className="truncate text-[15px] font-semibold text-[#eaecef]">
                                  {row.title}
                                  {row.isBroadcast ? (
                                    <span className="ml-2 rounded border border-amber-400/40 bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-bold text-amber-200">
                                      全员
                                    </span>
                                  ) : null}
                                </span>
                              </div>
                              {!isOpen && (
                                <p className="mt-1 line-clamp-2 pl-[22px] text-[14px] leading-snug text-[#aaabb0]">
                                  {row.preview || '点击查看详情'}
                                </p>
                              )}
                              <p className="mt-1 pl-[22px] text-[12px] text-[#5e6673]">
                                {row.time.length >= 19
                                  ? row.time.slice(0, 19).replace('T', ' ')
                                  : row.time}
                              </p>
                              {isOpen && (
                                <>
                                  <div className="mt-2 whitespace-pre-wrap border-t border-white/10 pt-2 pl-[22px] text-[14px] leading-relaxed text-[#e6e6e6]">
                                    {row.text}
                                  </div>
                                  {user?.is_admin &&
                                  row.marketReviewStrategyId ? (
                                    <div className="mt-3 flex flex-wrap gap-2 border-t border-white/10 pt-3 pl-[22px]">
                                      <button
                                        type="button"
                                        disabled={
                                          reviewBusyId ===
                                          `${row.marketReviewStrategyId}:approve`
                                        }
                                        onClick={(e) => {
                                          e.stopPropagation()
                                          void handleMarketReview(
                                            row.marketReviewStrategyId!,
                                            'approve'
                                          )
                                        }}
                                        className="rounded-lg border border-emerald-400/45 bg-emerald-500/15 px-3 py-1.5 text-[13px] font-semibold text-emerald-200 transition-colors hover:bg-emerald-500/25 disabled:opacity-50"
                                      >
                                        同意显示
                                      </button>
                                      <button
                                        type="button"
                                        disabled={
                                          reviewBusyId ===
                                          `${row.marketReviewStrategyId}:reject`
                                        }
                                        onClick={(e) => {
                                          e.stopPropagation()
                                          void handleMarketReview(
                                            row.marketReviewStrategyId!,
                                            'reject'
                                          )
                                        }}
                                        className="rounded-lg border border-red-400/45 bg-red-500/15 px-3 py-1.5 text-[13px] font-semibold text-red-200 transition-colors hover:bg-red-500/25 disabled:opacity-50"
                                      >
                                        不同意
                                      </button>
                                    </div>
                                  ) : null}
                                </>
                              )}
                            </div>
                          </div>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              </>
            )}

            {feed.length > 0 && (
              <>
                <p className="mb-2 mt-3 px-1 text-[13px] font-bold text-[#848E9C]">
                  智能交易动态
                </p>
                <ul className="space-y-2">
                  {feed.map((row) => {
                    const isOpen = expandedId === row.id
                    const unread = !readIds.has(row.id)
                    return (
                      <li key={row.id}>
                        <button
                          type="button"
                          onClick={() => toggleRow(row.id)}
                          className={`w-full rounded-xl border px-3 py-2.5 text-left transition-all ${
                            unread
                              ? 'border-sky-500/35 bg-[#131313]'
                              : 'border-white/10 bg-[#131313] hover:border-white/15'
                          }`}
                        >
                          <div className="flex items-start gap-2.5">
                            <Info
                              className="mt-0.5 h-4 w-4 shrink-0 text-sky-400"
                              aria-hidden
                            />
                            <div className="min-w-0 flex-1 font-sans">
                              <div className="flex items-center gap-1 text-[13px]">
                                {isOpen ? (
                                  <ChevronDown
                                    className="h-4 w-4 shrink-0 text-[#848E9C]"
                                    aria-hidden
                                  />
                                ) : (
                                  <ChevronRight
                                    className="h-4 w-4 shrink-0 text-[#848E9C]"
                                    aria-hidden
                                  />
                                )}
                                <span className="text-[#aaabb0]">
                                  {row.time.slice(5, 19).replace('T', ' ')}
                                </span>
                              </div>
                              {!isOpen && (
                                <p className="mt-1 pl-[22px] text-[14px] text-[#eaecef]">
                                  {row.brief}
                                </p>
                              )}
                              {isOpen && (
                                <p className="mt-2 border-t border-white/10 pt-2 pl-[22px] text-[14px] leading-relaxed text-[#e6e6e6]">
                                  {row.text}
                                </p>
                              )}
                            </div>
                          </div>
                        </button>
                      </li>
                    )
                  })}
                </ul>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
