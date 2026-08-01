import { useMemo, useState, type ReactNode } from 'react'
import useSWR from 'swr'
import { toast } from 'sonner'
import { Link } from 'react-router-dom'
import { AlertTriangle, ChevronDown, RefreshCw, Shield } from 'lucide-react'
import { api } from '../lib/api'
import type {
  AdminAIPlatformUsageRow,
  AdminBinanceBrokerRebateRow,
  AdminOutboundProxyFaultRow,
  AdminOutboundProxyPoolRow,
  AdminRunningTraderRow,
  AdminUserRow,
} from '../lib/api/walletAdmin'
import { ROUTES } from '../router/paths'

/** 管理端持仓摘要：只展示多空（中文） */
function formatAdminPositionSide(side: string): string {
  const u = String(side || '')
    .trim()
    .toUpperCase()
  if (u === 'LONG' || u === 'BUY') return '多'
  if (u === 'SHORT' || u === 'SELL') return '空'
  return side || '—'
}

function formatUsageModelName(row: AdminAIPlatformUsageRow): string {
  const provider = String(row.provider || '')
    .trim()
    .toLowerCase()
  const model = String(row.model || '').trim()
  if (
    provider === 'comkun_ai' ||
    provider === 'comkun-ai' ||
    /跟单/.test(model)
  )
    return 'comkun-ai'
  return model || '—'
}

function formatUsageProviderName(row: AdminAIPlatformUsageRow): string {
  const provider = String(row.provider || '').trim()
  const model = formatUsageModelName(row)
  if (
    !provider ||
    provider === 'comkun_ai' ||
    provider === 'comkun-ai' ||
    provider === model
  )
    return ''
  return provider
}

/** 管理后台长区块：点击标题栏展开/收起，减轻首屏滚动（如代理 IP 长表） */
function AdminCollapsibleSection({
  title,
  subtitle,
  defaultOpen = true,
  className = 'mb-8 rounded-xl border border-white/10 bg-nofx-bg-secondary',
  titleClassName = 'text-lg font-bold text-white',
  headerRight,
  children,
}: {
  title: string
  subtitle?: ReactNode
  defaultOpen?: boolean
  className?: string
  titleClassName?: string
  headerRight?: ReactNode
  children: ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <section className={`${className} overflow-hidden`}>
      <div className="flex flex-wrap items-start gap-2 border-b border-white/10 bg-black/15 px-4 py-3 sm:px-5 sm:py-4">
        <button
          type="button"
          className="flex min-w-0 flex-1 items-start gap-2 text-left sm:gap-3"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
        >
          <ChevronDown
            className={`mt-0.5 h-5 w-5 shrink-0 text-[#d4ff33] transition-transform duration-200 ${open ? 'rotate-0' : '-rotate-90'}`}
            aria-hidden
          />
          <span className="min-w-0">
            <h2 className={titleClassName}>{title}</h2>
            {subtitle ? (
              <div className="mt-1 text-xs text-[#848E9C]">{subtitle}</div>
            ) : null}
          </span>
        </button>
        {headerRight ? (
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            {headerRight}
          </div>
        ) : null}
      </div>
      {open ? <div className="p-4 sm:p-5">{children}</div> : null}
    </section>
  )
}

export function AdminDashboardPage() {
  const { data, error, isLoading, mutate } = useSWR(
    'admin-users-overview',
    () => api.getAdminUsersOverview(),
    {
      refreshInterval: 15000,
    }
  )
  const [adjustUserId, setAdjustUserId] = useState<string | null>(null)
  const [deltaInput, setDeltaInput] = useState('')
  const [noteInput, setNoteInput] = useState('')
  const [confirmedDeposit, setConfirmedDeposit] = useState(false)
  const [originalDepositLedgerId, setOriginalDepositLedgerId] = useState('')
  const [usageUserId, setUsageUserId] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [flattenSymbolInput, setFlattenSymbolInput] = useState('')
  const [flattenModalOpen, setFlattenModalOpen] = useState(false)
  const [flattenSubmitting, setFlattenSubmitting] = useState(false)
  const [traderControlBusyId, setTraderControlBusyId] = useState<string | null>(
    null
  )
  const [proxyImportText, setProxyImportText] = useState('')
  const [proxyImportBusy, setProxyImportBusy] = useState(false)
  const [proxyAssignPoolId, setProxyAssignPoolId] = useState<string | null>(
    null
  )
  const [proxyAssignHost, setProxyAssignHost] = useState('')
  const [proxyAssignPick, setProxyAssignPick] = useState('')
  const [proxyAssignUserId, setProxyAssignUserId] = useState('')
  const [proxyAssignExchangeId, setProxyAssignExchangeId] = useState('')
  const [proxyAssignBusy, setProxyAssignBusy] = useState(false)
  const { data: usageData, mutate: mutateUsage } = useSWR(
    ['admin-ai-platform-usage', usageUserId],
    () => api.getAdminAIPlatformUsage(usageUserId || undefined),
    { refreshInterval: 30000 }
  )
  const { data: rebateData, mutate: mutateRebates } = useSWR(
    'admin-binance-broker-rebates',
    () => api.getAdminBinanceBrokerRebates(),
    { refreshInterval: 60000 }
  )
  const { data: proxyPoolData, mutate: mutateProxyPool } = useSWR(
    'admin-outbound-proxy-pool',
    () => api.getAdminOutboundProxyPool(),
    { refreshInterval: 15000 }
  )
  const { data: proxyFaultData, mutate: mutateProxyFaults } = useSWR(
    'admin-outbound-proxy-faults',
    () => api.getAdminOutboundProxyFaults(50),
    { refreshInterval: 15000 }
  )

  const users = data?.users ?? []
  const runningTraders = data?.running_traders ?? []
  const runningMeta = data?.running_traders_meta
  const proxyFaultEvents = proxyFaultData?.events ?? []

  const proxyFaultTypeZh = (t: string) => {
    switch (String(t || '').toLowerCase()) {
      case 'timeout':
        return '超时'
      case 'refused':
        return '拒绝连接'
      default:
        return '其他'
    }
  }

  const sorted = useMemo(() => {
    return [...users].sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    )
  }, [users])

  /** 代理绑定下拉：仅币安，展示「用户昵称 → 交易员名 · 账户备注」 */
  const proxyBinancePickRows = useMemo(() => {
    const rows: { user_id: string; exchange_id: string; label: string }[] = []
    for (const u of sorted) {
      for (const t of u.traders ?? []) {
        if (t.missing_exchange) continue
        if (String(t.exchange_type || '').toLowerCase() !== 'binance') continue
        const who = (u.display_name || u.email || u.id).trim()
        const acc = (t.account_name || '默认').trim()
        rows.push({
          user_id: u.id,
          exchange_id: t.exchange_id,
          label: `${who} → ${t.name}（币安 · ${acc}）`,
        })
      }
    }
    return rows
  }, [sorted])

  /** 所有「在跑」交易员的合计（按当前接口快照相加） */
  const runningTradersTotals = useMemo(() => {
    let totalPnl = 0
    let marginUsed = 0
    let available = 0
    let unrealized = 0
    let runningCount = 0
    for (const row of runningTraders as AdminRunningTraderRow[]) {
      if (row.is_running === false) continue
      runningCount += 1
      totalPnl += Number(row.total_pnl ?? 0)
      marginUsed += Number(row.margin_used ?? 0)
      available += Number(row.available_balance ?? 0)
      unrealized += Number(row.total_unrealized_profit ?? 0)
    }
    return { totalPnl, marginUsed, available, unrealized, count: runningCount }
  }, [runningTraders])

  const handleAdjust = async () => {
    if (!adjustUserId) return
    const delta = Number(deltaInput)
    if (!Number.isFinite(delta) || delta === 0) {
      toast.error('请输入非零数字（正数为增加，负数为扣减）')
      return
    }
    setSubmitting(true)
    try {
      await api.postAdminWalletAdjust(
        adjustUserId,
        delta,
        noteInput.trim(),
        confirmedDeposit,
        Number(originalDepositLedgerId) || undefined
      )
      toast.success('已更新该用户余额')
      setAdjustUserId(null)
      setDeltaInput('')
      setNoteInput('')
      setConfirmedDeposit(false)
      setOriginalDepositLedgerId('')
      await mutate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '调账失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleAdminTraderControl = async (
    traderId: string,
    action: 'start' | 'stop' | 'sync_positions'
  ) => {
    setTraderControlBusyId(traderId)
    try {
      if (action === 'start') {
        await api.postAdminTraderStart(traderId)
        toast.success('已提交启动')
      } else if (action === 'stop') {
        const r = await api.postAdminTraderStop(traderId)
        toast.success(r.warning ? '已停止（见提示）' : '已提交停止')
        if (r.warning && r.message) toast.info(String(r.message))
      } else {
        const r = await api.postAdminTraderSyncPositionsFromExchange(traderId)
        toast.success(r.message || '已同步持仓')
      }
      await mutate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '操作失败')
    } finally {
      setTraderControlBusyId(null)
    }
  }

  const handleFlattenFollowTraders = async () => {
    setFlattenSubmitting(true)
    try {
      const sym = flattenSymbolInput.trim()
      const res = await api.postAdminFlattenAllComkunFollowTraders(
        sym || undefined
      )
      toast.success(
        `处理完成：共 ${res.trader_count} 个跟单交易员，成功 ${res.ok_count}，失败 ${res.fail_count}` +
          (res.symbol_filter ? `（仅 ${res.symbol_filter}）` : '（全部合约）')
      )
      setFlattenModalOpen(false)
      setFlattenSymbolInput('')
      await mutate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '平仓请求失败')
    } finally {
      setFlattenSubmitting(false)
    }
  }

  return (
    <div className="min-h-screen bg-nofx-bg-tertiary px-4 pb-16 pt-20 text-[#EAECEF] sm:px-8">
      <div className="mx-auto max-w-[1400px]">
        <div className="mb-8 flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <Shield className="h-8 w-8 text-[#d4ff33]" aria-hidden />
            <div>
              <h1 className="text-2xl font-bold tracking-tight">管理后台</h1>
              <p className="mt-1 text-sm text-[#848E9C]">
                全部注册用户、站内余额；运行中列表取自本机数据库快照与持仓表（不请求交易所，免限频）
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => {
                void mutate()
                void mutateUsage()
                void mutateRebates()
                void mutateProxyFaults()
              }}
              className="inline-flex items-center gap-2 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm hover:bg-white/10"
            >
              <RefreshCw className="h-4 w-4" />
              刷新
            </button>
            <Link
              to={ROUTES.strategyMarket}
              className="rounded-lg border border-white/10 px-3 py-2 text-sm text-[#d4ff33] hover:bg-white/5"
            >
              回策略市场
            </Link>
          </div>
        </div>

        {error && (
          <div className="mb-6 rounded-xl border border-red-500/40 bg-red-950/40 px-4 py-3 text-sm text-red-100">
            {error instanceof Error ? error.message : '加载失败'}
          </div>
        )}

        {isLoading && !data && (
          <p className="text-sm text-[#848E9C]">加载中…</p>
        )}

        <AdminCollapsibleSection
          defaultOpen={proxyFaultEvents.length > 0}
          className={`mb-8 rounded-xl border ${
            proxyFaultEvents.length > 0
              ? 'border-red-500/45 bg-red-950/30'
              : 'border-[#d4ff33]/25 bg-[#d4ff33]/5'
          }`}
          title="出口代理异常"
          titleClassName={`text-lg font-bold ${proxyFaultEvents.length > 0 ? 'text-red-200' : 'text-[#d4ff33]'}`}
          subtitle={
            <>
              CEX REST 经 SOCKS5 出口失败时记录（同交易员+代理 5
              分钟内合并）。用户可能看不到持仓/余额；请检查代理池或对该交易所「强制回收」后换健康出口。
              {proxyFaultEvents.length > 0 ? (
                <span className="mt-1 block font-medium text-red-200/90">
                  当前有 {proxyFaultEvents.length} 条近期告警，请优先处理。
                </span>
              ) : null}
            </>
          }
          headerRight={
            <button
              type="button"
              onClick={() => void mutateProxyFaults()}
              className="rounded-lg border border-white/10 px-3 py-2 text-xs text-[#d4ff33] hover:bg-white/5"
            >
              刷新告警
            </button>
          }
        >
          {proxyFaultEvents.length === 0 ? (
            <p className="text-sm text-[#848E9C]">暂无出口代理故障记录。</p>
          ) : (
            <div className="overflow-x-auto rounded-xl border border-white/10">
              <table className="w-full min-w-[920px] border-collapse text-left text-xs">
                <thead>
                  <tr className="border-b border-white/10 uppercase tracking-wider text-[#848E9C]">
                    <th className="px-3 py-3">最近</th>
                    <th className="px-3 py-3">用户</th>
                    <th className="px-3 py-3">交易员</th>
                    <th className="px-3 py-3">出口</th>
                    <th className="px-3 py-3">类型</th>
                    <th className="px-3 py-3">次数</th>
                    <th className="px-3 py-3">说明</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/5">
                  {proxyFaultEvents.map((row: AdminOutboundProxyFaultRow) => (
                    <tr key={row.id} className="hover:bg-white/[0.02]">
                      <td className="px-3 py-3 whitespace-nowrap text-[#b7bdc6]">
                        {row.last_at
                          ? new Date(row.last_at).toLocaleString()
                          : '—'}
                      </td>
                      <td className="px-3 py-3">
                        <div className="font-medium text-[#eaecef]">
                          {row.user_display_name ||
                            row.user_email ||
                            row.user_id ||
                            '—'}
                        </div>
                        {row.user_email ? (
                          <div className="mt-0.5 text-[10px] text-[#5e6673]">
                            {row.user_email}
                          </div>
                        ) : null}
                      </td>
                      <td className="px-3 py-3 text-[#b7bdc6]">
                        {row.trader_name || row.trader_id || '—'}
                      </td>
                      <td className="px-3 py-3 font-mono text-[#d4ff33]">
                        {row.display_host || row.proxy_redacted || '—'}
                      </td>
                      <td className="px-3 py-3">
                        <span className="rounded bg-red-500/20 px-2 py-0.5 text-[11px] font-medium text-red-200">
                          {proxyFaultTypeZh(row.error_type)}
                        </span>
                      </td>
                      <td className="px-3 py-3 tabular-nums text-[#eaecef]">
                        {row.hit_count ?? 1}
                      </td>
                      <td className="max-w-[360px] px-3 py-3 text-[11px] text-[#848E9C]">
                        {row.last_error || '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </AdminCollapsibleSection>

        <AdminCollapsibleSection
          defaultOpen={false}
          className="mb-8 rounded-xl border border-red-500/30 bg-red-950/25"
          title="合规跟单 · 一键平仓"
          subtitle="对库里所有绑定「市场合规跟单」策略的交易员，按顺序向交易所提交市价平仓（会先撤该合约未成交单）。用于跟单端长时间未平仓时的应急。下方可填合约名（如 SOL 或 SOLUSDT），留空则平掉全部非零持仓。"
        >
          <div className="flex flex-wrap items-start gap-3">
            <AlertTriangle
              className="mt-0.5 h-6 w-6 shrink-0 text-red-300"
              aria-hidden
            />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-end gap-3">
                <div>
                  <label
                    htmlFor="admin-flatten-symbol"
                    className="mb-1 block text-[11px] text-[#848E9C]"
                  >
                    仅平该合约（可选）
                  </label>
                  <input
                    id="admin-flatten-symbol"
                    type="text"
                    value={flattenSymbolInput}
                    onChange={(e) => setFlattenSymbolInput(e.target.value)}
                    placeholder="留空 = 全部合约"
                    className="w-48 rounded-lg border border-white/15 bg-black/30 px-3 py-2 font-mono text-sm text-white placeholder:text-[#5e6673]"
                    autoComplete="off"
                  />
                </div>
                <button
                  type="button"
                  onClick={() => setFlattenModalOpen(true)}
                  className="rounded-lg border border-red-400/50 bg-red-600/80 px-4 py-2 text-sm font-medium text-white hover:bg-red-600"
                >
                  一键全平跟单客户持仓
                </button>
              </div>
            </div>
          </div>
        </AdminCollapsibleSection>

        {flattenModalOpen ? (
          <div
            className="fixed inset-0 z-[100] flex items-center justify-center bg-black/70 px-4"
            role="dialog"
            aria-modal="true"
            aria-labelledby="flatten-modal-title"
          >
            <div className="max-w-lg rounded-xl border border-red-500/40 bg-[#1a1d24] p-6 shadow-xl">
              <h3
                id="flatten-modal-title"
                className="text-lg font-bold text-white"
              >
                确认向交易所提交平仓？
              </h3>
              <p className="mt-3 text-sm text-[#b7bdc6]">
                将对<strong className="text-white">所有合规跟单交易员</strong>
                逐个下单市价平仓。
                {flattenSymbolInput.trim()
                  ? ` 仅处理合约：${flattenSymbolInput.trim().toUpperCase()}（会自动补 USDT 后缀若未写）。`
                  : ' 当前留空：将平掉每个账户里全部非零持仓。'}
              </p>
              <p className="mt-2 text-xs text-amber-200/90">
                此操作不可自动撤销，请确认已无更好的处理方式。
              </p>
              <div className="mt-6 flex flex-wrap justify-end gap-2">
                <button
                  type="button"
                  disabled={flattenSubmitting}
                  onClick={() => setFlattenModalOpen(false)}
                  className="rounded-lg border border-white/15 px-4 py-2 text-sm text-[#EAECEF] hover:bg-white/10"
                >
                  取消
                </button>
                <button
                  type="button"
                  disabled={flattenSubmitting}
                  onClick={() => void handleFlattenFollowTraders()}
                  className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-500 disabled:opacity-50"
                >
                  {flattenSubmitting ? '执行中…' : '确认平仓'}
                </button>
              </div>
            </div>
          </div>
        ) : null}

        <AdminCollapsibleSection
          defaultOpen
          className="mb-8 rounded-lg border border-[#d4ff33]/25 bg-[#d4ff33]/5"
          title="合作伙伴总账"
          subtitle="确认充值、差额返佣、身份变更、IB升级、工作室申请和TRC20提现集中管理。"
        >
          <a
            href="/hongzhong/dashboard"
            className="inline-flex rounded bg-[#d4ff33] px-4 py-2 text-sm font-bold text-black hover:brightness-105"
          >
            打开完整总账
          </a>
        </AdminCollapsibleSection>

        <AdminCollapsibleSection
          defaultOpen={false}
          className="mb-10 rounded-xl border border-white/10 bg-nofx-bg-secondary"
          title="交易员列表（本机快照 + 启停）"
          subtitle="包含全部交易员：运行中行显示权益与持仓快照；未运行行无快照。持仓来自本机库非实时查交易所——若实盘已平仍显示有仓，点「同步持仓」按交易所重写。右侧可代客户启动或停止跟单。"
        >
          {runningMeta?.hint ? (
            <div className="mb-4 rounded-lg border border-[#d4ff33]/20 bg-[#d4ff33]/5 px-4 py-3 text-sm text-[#b7bdc6]">
              {runningMeta.hint}
            </div>
          ) : null}
          {runningTradersTotals.count > 0 && (
            <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
              <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3">
                <div className="text-[11px] uppercase tracking-wider text-[#848E9C]">
                  在跑人数
                </div>
                <div className="mt-1 font-mono text-xl font-bold text-white">
                  {runningTradersTotals.count}
                </div>
              </div>
              <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3">
                <div className="text-[11px] uppercase tracking-wider text-[#848E9C]">
                  合计总盈亏
                </div>
                <div
                  className={`mt-1 font-mono text-xl font-bold ${runningTradersTotals.totalPnl >= 0 ? 'text-emerald-300' : 'text-red-300'}`}
                >
                  {runningTradersTotals.totalPnl >= 0 ? '+' : ''}
                  {runningTradersTotals.totalPnl.toFixed(4)}{' '}
                  <span className="text-sm font-normal text-[#848E9C]">
                    USDT
                  </span>
                </div>
              </div>
              <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3">
                <div className="text-[11px] uppercase tracking-wider text-[#848E9C]">
                  合计保证金占用
                </div>
                <div className="mt-1 font-mono text-xl font-bold text-[#b7bdc6]">
                  {runningTradersTotals.marginUsed.toFixed(4)}{' '}
                  <span className="text-sm font-normal text-[#848E9C]">
                    USDT
                  </span>
                </div>
              </div>
              <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3">
                <div className="text-[11px] uppercase tracking-wider text-[#848E9C]">
                  合计可用余额
                </div>
                <div className="mt-1 font-mono text-xl font-bold text-[#d4ff33]">
                  {runningTradersTotals.available.toFixed(4)}{' '}
                  <span className="text-sm font-normal text-[#848E9C]">
                    USDT
                  </span>
                </div>
              </div>
              <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3 sm:col-span-2 lg:col-span-1">
                <div className="text-[11px] uppercase tracking-wider text-[#848E9C]">
                  合计持仓浮盈亏
                </div>
                <div
                  className={`mt-1 font-mono text-xl font-bold ${runningTradersTotals.unrealized >= 0 ? 'text-emerald-300' : 'text-red-300'}`}
                >
                  {runningTradersTotals.unrealized >= 0 ? '+' : ''}
                  {runningTradersTotals.unrealized.toFixed(4)}{' '}
                  <span className="text-sm font-normal text-[#848E9C]">
                    USDT
                  </span>
                </div>
              </div>
            </div>
          )}
          <div className="mt-4 overflow-x-auto rounded-xl border border-white/10">
            <table className="w-full min-w-[1240px] border-collapse text-left text-xs">
              <thead>
                <tr className="border-b border-white/10 uppercase tracking-wider text-[#848E9C]">
                  <th className="px-3 py-3">交易员</th>
                  <th className="px-3 py-3">用户</th>
                  <th className="px-3 py-3">运行</th>
                  <th className="px-3 py-3">总权益</th>
                  <th className="px-3 py-3">可用余额</th>
                  <th className="px-3 py-3">总盈亏</th>
                  <th className="px-3 py-3">保证金占用 (USDT / %)</th>
                  <th className="px-3 py-3">持仓浮盈亏</th>
                  <th className="px-3 py-3">当前持仓</th>
                  <th className="px-3 py-3">来源 / 时间</th>
                  <th className="px-3 py-3">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {runningTraders.length === 0 ? (
                  <tr>
                    <td
                      colSpan={11}
                      className="px-3 py-8 text-center text-[#848E9C]"
                    >
                      暂无交易员
                    </td>
                  </tr>
                ) : (
                  runningTraders.map((row: AdminRunningTraderRow) => {
                    const pnl = Number(row.total_pnl ?? 0)
                    const upnl = Number(row.total_unrealized_profit ?? 0)
                    const on = row.is_running !== false
                    const busy = traderControlBusyId === row.trader_id
                    return (
                      <tr key={row.trader_id} className="hover:bg-white/[0.02]">
                        <td className="px-3 py-3">
                          <div className="font-bold text-white">
                            {row.trader_name}
                          </div>
                          <div className="font-mono text-[10px] text-[#5e6673]">
                            {row.trader_id}
                          </div>
                        </td>
                        <td className="px-3 py-3">
                          <div className="text-white">
                            {row.user_display_name || '—'}
                          </div>
                          <div className="text-[#848E9C]">{row.user_email}</div>
                        </td>
                        <td className="px-3 py-3">
                          <span
                            className={`rounded px-2 py-0.5 text-[11px] font-medium ${on ? 'bg-emerald-500/20 text-emerald-300' : 'bg-white/10 text-[#848E9C]'}`}
                          >
                            {on ? '运行中' : '已停止'}
                          </span>
                        </td>
                        <td className="px-3 py-3 font-mono text-[#EAECEF]">
                          {Number(row.total_equity ?? 0).toFixed(4)}
                        </td>
                        <td className="px-3 py-3 font-mono text-[#d4ff33]">
                          {Number(row.available_balance ?? 0).toFixed(4)}
                        </td>
                        <td
                          className={`px-3 py-3 font-mono font-bold ${pnl >= 0 ? 'text-emerald-300' : 'text-red-300'}`}
                        >
                          {pnl >= 0 ? '+' : ''}
                          {pnl.toFixed(4)}
                          <div className="text-[10px] opacity-80">
                            {Number(row.total_pnl_pct ?? 0).toFixed(2)}%
                          </div>
                        </td>
                        <td className="px-3 py-3 font-mono text-[#b7bdc6]">
                          {Number(row.margin_used ?? 0).toFixed(4)} /{' '}
                          {Number(row.margin_used_pct ?? 0).toFixed(2)}%
                        </td>
                        <td
                          className={`px-3 py-3 font-mono font-bold ${upnl >= 0 ? 'text-emerald-300' : 'text-red-300'}`}
                        >
                          {upnl >= 0 ? '+' : ''}
                          {upnl.toFixed(4)}
                        </td>
                        <td className="px-3 py-3 text-[#b7bdc6]">
                          {(row.positions ?? []).length === 0 ? (
                            '—'
                          ) : (
                            <ul className="max-w-[280px] space-y-1">
                              {(row.positions ?? []).map((p, i) => {
                                const entry = Number(p.entry_price ?? 0)
                                return (
                                  <li
                                    key={`${row.trader_id}-${p.symbol}-${i}`}
                                    className="font-mono text-[11px]"
                                  >
                                    {p.symbol} {formatAdminPositionSide(p.side)}{' '}
                                    入场 {entry > 0 ? entry.toFixed(4) : '—'}
                                  </li>
                                )
                              })}
                            </ul>
                          )}
                        </td>
                        <td className="px-3 py-3 text-[#848E9C]">
                          <div>
                            {row.error ||
                              (row.metrics_source === 'exchange_live'
                                ? '交易所实时'
                                : row.metrics_source === 'equity_snapshot'
                                  ? '本机快照'
                                  : '正常')}
                          </div>
                          {row.metrics_source === 'exchange_live' &&
                          row.checked_at ? (
                            <div className="mt-1 font-mono text-[10px] text-emerald-400/90">
                              实时 {new Date(row.checked_at).toLocaleString()}
                            </div>
                          ) : null}
                          {row.metrics_source === 'equity_snapshot' &&
                          row.snapshot_at ? (
                            <div className="mt-1 font-mono text-[10px] text-[#5e6673]">
                              快照 {new Date(row.snapshot_at).toLocaleString()}
                            </div>
                          ) : null}
                        </td>
                        <td className="px-3 py-3 whitespace-nowrap">
                          <div className="flex flex-wrap gap-1.5">
                            <button
                              type="button"
                              disabled={busy || on}
                              onClick={() =>
                                void handleAdminTraderControl(
                                  row.trader_id,
                                  'start'
                                )
                              }
                              className="rounded border border-emerald-500/40 bg-emerald-600/25 px-2 py-1 text-[11px] text-emerald-200 hover:bg-emerald-600/40 disabled:cursor-not-allowed disabled:opacity-40"
                            >
                              启动
                            </button>
                            <button
                              type="button"
                              disabled={busy || !on}
                              onClick={() =>
                                void handleAdminTraderControl(
                                  row.trader_id,
                                  'stop'
                                )
                              }
                              className="rounded border border-red-500/40 bg-red-600/25 px-2 py-1 text-[11px] text-red-200 hover:bg-red-600/40 disabled:cursor-not-allowed disabled:opacity-40"
                            >
                              停止
                            </button>
                            <button
                              type="button"
                              disabled={busy}
                              title="按交易所当前持仓重写本机 OPEN 记录，消除幽灵持仓"
                              onClick={() =>
                                void handleAdminTraderControl(
                                  row.trader_id,
                                  'sync_positions'
                                )
                              }
                              className="rounded border border-[#d4ff33]/35 bg-[#d4ff33]/10 px-2 py-1 text-[11px] text-[#d4ff33] hover:bg-[#d4ff33]/20 disabled:cursor-not-allowed disabled:opacity-40"
                            >
                              同步持仓
                            </button>
                          </div>
                        </td>
                      </tr>
                    )
                  })
                )}
              </tbody>
            </table>
          </div>
        </AdminCollapsibleSection>

        <AdminCollapsibleSection
          defaultOpen={false}
          className="mt-10 rounded-xl border border-white/10 bg-nofx-bg-secondary"
          title="SOCKS5 出口代理池（CEX REST）"
          titleClassName="text-lg font-bold text-[#d4ff33]"
          subtitle={
            <>
              每行一条：完整 socks5:// URL，或{' '}
              <code className="rounded bg-black/40 px-1">
                host|port|user|pass|到期(可选)
              </code>
              。客户新建币安账户并勾选「代理池」时自动分配，一人一出口不重复。
              <span className="mt-1 block text-[#d4ff33]/90">
                未分配的行可点「绑定交易所」，填入下方用户列表里的 user_id
                与对应币安账户的 exchange_id（UUID）。
                要把别人的代理换给另一人：先对该行「强制回收」，再在回收后的未分配行上绑定目标账户。
              </span>
            </>
          }
          headerRight={
            <button
              type="button"
              onClick={() => void mutateProxyPool()}
              className="rounded-lg border border-white/10 px-3 py-2 text-xs text-[#d4ff33] hover:bg-white/5"
            >
              刷新列表
            </button>
          }
        >
          <div className="flex flex-col gap-2 md:flex-row md:items-end">
            <textarea
              value={proxyImportText}
              onChange={(e) => setProxyImportText(e.target.value)}
              placeholder={
                '示例：\n47.1.2.3|11819|user|pass|2026-12-31 23:59:59'
              }
              rows={4}
              className="min-h-[100px] flex-1 rounded-xl border border-white/10 bg-black/30 px-3 py-2 font-mono text-xs text-[#eaecef] outline-none focus:border-[#d4ff33]/40"
            />
            <button
              type="button"
              disabled={proxyImportBusy || !proxyImportText.trim()}
              onClick={async () => {
                setProxyImportBusy(true)
                try {
                  const r =
                    await api.postAdminOutboundProxyPoolImport(proxyImportText)
                  toast.success(`导入完成：新增 ${r.added}，跳过 ${r.skipped}`)
                  if ((r.errors ?? []).length > 0)
                    toast.info(r.errors.slice(0, 5).join('；'))
                  setProxyImportText('')
                  await mutateProxyPool()
                } catch (e) {
                  toast.error(e instanceof Error ? e.message : '导入失败')
                } finally {
                  setProxyImportBusy(false)
                }
              }}
              className="rounded-xl bg-[#d4ff33] px-5 py-3 text-sm font-bold text-black hover:bg-[#e5ff66] disabled:opacity-40"
            >
              {proxyImportBusy ? '导入中…' : '批量导入'}
            </button>
          </div>
          <div className="mt-6 overflow-x-auto rounded-xl border border-white/10">
            <table className="w-full min-w-[960px] border-collapse text-left text-xs">
              <thead>
                <tr className="border-b border-white/10 uppercase tracking-wider text-[#848E9C]">
                  <th className="px-3 py-3">出口主机/IP</th>
                  <th className="px-3 py-3">到期</th>
                  <th className="px-3 py-3">剩余(秒)</th>
                  <th className="px-3 py-3">分配用户</th>
                  <th className="px-3 py-3">交易所备注名</th>
                  <th className="px-3 py-3 text-right">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {(proxyPoolData?.entries ?? []).length === 0 ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-3 py-8 text-center text-[#848E9C]"
                    >
                      暂无记录，请在上方粘贴导入。
                    </td>
                  </tr>
                ) : (
                  (proxyPoolData?.entries ?? []).map(
                    (row: AdminOutboundProxyPoolRow) => (
                      <tr key={row.id} className="hover:bg-white/[0.02]">
                        <td className="px-3 py-3 font-mono text-[#d4ff33]">
                          {row.display_host || '—'}
                        </td>
                        <td className="px-3 py-3 text-[#b7bdc6]">
                          {row.expires_at
                            ? new Date(row.expires_at).toLocaleString()
                            : '—'}
                        </td>
                        <td className="px-3 py-3 tabular-nums text-[#b7bdc6]">
                          {row.seconds_until_expiry != null
                            ? row.seconds_until_expiry
                            : '—'}
                        </td>
                        <td className="px-3 py-3">
                          <div className="font-medium text-[#eaecef]">
                            {row.assigned_user_display_name ||
                              row.assigned_user_email ||
                              '—'}
                          </div>
                          <div className="mt-0.5 text-[10px] text-[#5e6673]">
                            {row.assigned_user_id || ''}
                          </div>
                        </td>
                        <td className="px-3 py-3 text-[#b7bdc6]">
                          {row.assigned_exchange_account_name || '—'}
                        </td>
                        <td className="px-3 py-3 text-right">
                          {!row.assigned_exchange_id ? (
                            <div className="flex flex-wrap justify-end gap-1">
                              <button
                                type="button"
                                className="rounded border border-[#d4ff33]/45 px-2 py-1 text-[11px] text-[#d4ff33] hover:bg-[#d4ff33]/10"
                                onClick={() => {
                                  setProxyAssignPoolId(row.id)
                                  setProxyAssignHost(row.display_host || '')
                                  setProxyAssignPick('')
                                  setProxyAssignUserId('')
                                  setProxyAssignExchangeId('')
                                }}
                              >
                                绑定交易所
                              </button>
                              <button
                                type="button"
                                className="rounded border border-red-500/40 px-2 py-1 text-[11px] text-red-200 hover:bg-red-500/15"
                                onClick={async () => {
                                  if (!confirm('删除这条未分配的代理？')) return
                                  try {
                                    await api.deleteAdminOutboundProxyPoolEntry(
                                      row.id
                                    )
                                    toast.success('已删除')
                                    await mutateProxyPool()
                                  } catch (e) {
                                    toast.error(
                                      e instanceof Error
                                        ? e.message
                                        : '删除失败'
                                    )
                                  }
                                }}
                              >
                                删除
                              </button>
                            </div>
                          ) : (
                            <button
                              type="button"
                              className="rounded border border-amber-500/40 px-2 py-1 text-[11px] text-amber-200 hover:bg-amber-500/15"
                              onClick={async () => {
                                if (
                                  !confirm(
                                    '强制回收该代理并清空用户交易所里的出口配置？'
                                  )
                                )
                                  return
                                try {
                                  await api.postAdminOutboundProxyPoolRelease(
                                    row.id
                                  )
                                  toast.success('已回收')
                                  await mutateProxyPool()
                                  await mutate()
                                } catch (e) {
                                  toast.error(
                                    e instanceof Error ? e.message : '回收失败'
                                  )
                                }
                              }}
                            >
                              强制回收
                            </button>
                          )}
                        </td>
                      </tr>
                    )
                  )
                )}
              </tbody>
            </table>
          </div>

          {proxyAssignPoolId ? (
            <div
              className="fixed inset-0 z-[80] flex items-center justify-center bg-black/70 p-4"
              role="dialog"
              aria-modal="true"
            >
              <div className="w-full max-w-md rounded-xl border border-white/15 bg-nofx-bg-secondary p-5 shadow-xl">
                <h3 className="text-base font-bold text-[#eaecef]">
                  绑定代理到交易所
                </h3>
                <p className="mt-1 font-mono text-xs text-[#d4ff33]">
                  {proxyAssignHost || proxyAssignPoolId}
                </p>
                <p className="mt-2 text-xs text-[#848E9C]">
                  下面列出所有用户的
                  <strong className="text-[#eaecef]">币安交易员</strong>
                  （绑定到哪张 API 密钥），选中即可，无需再抄 UUID。
                </p>
                <label className="mt-4 block text-xs text-[#848E9C]">
                  选择要绑定出口的用户 → 交易员 → 币安账户
                  <select
                    className="mt-1 w-full rounded-lg border border-white/10 bg-black/40 px-3 py-2 text-xs text-[#eaecef] outline-none focus:border-[#d4ff33]/40"
                    value={proxyAssignPick}
                    onChange={(e) => {
                      const v = e.target.value
                      setProxyAssignPick(v)
                      if (!v) {
                        setProxyAssignUserId('')
                        setProxyAssignExchangeId('')
                        return
                      }
                      try {
                        const o = JSON.parse(v) as {
                          user_id: string
                          exchange_id: string
                        }
                        setProxyAssignUserId(o.user_id)
                        setProxyAssignExchangeId(o.exchange_id)
                      } catch {
                        setProxyAssignUserId('')
                        setProxyAssignExchangeId('')
                      }
                    }}
                  >
                    <option value="">请选择…</option>
                    {proxyBinancePickRows.map((r) => (
                      <option
                        key={`${r.user_id}-${r.exchange_id}`}
                        value={JSON.stringify({
                          user_id: r.user_id,
                          exchange_id: r.exchange_id,
                        })}
                      >
                        {r.label}
                      </option>
                    ))}
                  </select>
                </label>
                {proxyBinancePickRows.length === 0 ? (
                  <p className="mt-2 text-xs text-amber-200/90">
                    暂无币安交易员条目：请
                    <strong className="text-[#eaecef]">
                      刷新页面
                    </strong>并部署含{' '}
                    <code className="text-[#d4ff33]">traders</code>{' '}
                    字段的最新后端；或展开下方手动填写。
                  </p>
                ) : null}
                <details className="mt-4 rounded-lg border border-white/10 bg-black/25 p-3">
                  <summary className="cursor-pointer text-xs text-[#848E9C]">
                    手动输入 user_id / exchange_id
                  </summary>
                  <label className="mt-3 block text-xs text-[#848E9C]">
                    user_id
                    <input
                      value={proxyAssignUserId}
                      onChange={(e) => {
                        setProxyAssignUserId(e.target.value)
                        setProxyAssignPick('')
                      }}
                      className="mt-1 w-full rounded-lg border border-white/10 bg-black/40 px-3 py-2 font-mono text-xs text-[#eaecef] outline-none focus:border-[#d4ff33]/40"
                      placeholder="9c8d3cb3-..."
                    />
                  </label>
                  <label className="mt-3 block text-xs text-[#848E9C]">
                    exchange_id（币安账户）
                    <input
                      value={proxyAssignExchangeId}
                      onChange={(e) => {
                        setProxyAssignExchangeId(e.target.value)
                        setProxyAssignPick('')
                      }}
                      className="mt-1 w-full rounded-lg border border-white/10 bg-black/40 px-3 py-2 font-mono text-xs text-[#eaecef] outline-none focus:border-[#d4ff33]/40"
                      placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                    />
                  </label>
                </details>
                <div className="mt-5 flex justify-end gap-2">
                  <button
                    type="button"
                    className="rounded-lg border border-white/15 px-4 py-2 text-xs text-[#b7bdc6] hover:bg-white/5"
                    onClick={() => setProxyAssignPoolId(null)}
                  >
                    取消
                  </button>
                  <button
                    type="button"
                    disabled={
                      proxyAssignBusy ||
                      !proxyAssignUserId.trim() ||
                      !proxyAssignExchangeId.trim()
                    }
                    className="rounded-lg bg-[#d4ff33] px-4 py-2 text-xs font-bold text-black hover:bg-[#e5ff66] disabled:opacity-40"
                    onClick={async () => {
                      setProxyAssignBusy(true)
                      try {
                        await api.postAdminOutboundProxyPoolAssign(
                          proxyAssignPoolId,
                          {
                            user_id: proxyAssignUserId.trim(),
                            exchange_id: proxyAssignExchangeId.trim(),
                          }
                        )
                        toast.success('已绑定')
                        setProxyAssignPoolId(null)
                        await mutateProxyPool()
                        await mutate()
                      } catch (e) {
                        toast.error(e instanceof Error ? e.message : '绑定失败')
                      } finally {
                        setProxyAssignBusy(false)
                      }
                    }}
                  >
                    {proxyAssignBusy ? '提交中…' : '确认绑定'}
                  </button>
                </div>
              </div>
            </div>
          ) : null}
        </AdminCollapsibleSection>

        <AdminCollapsibleSection
          defaultOpen
          className="rounded-xl border border-white/10 bg-nofx-bg-secondary"
          title="全部用户 · 余额与调账"
          subtitle="注册用户列表、站内余额、交易所绑定与币安未平仓摘要；右侧「调账」同财务入账逻辑（可正负）。"
        >
          <div className="overflow-x-auto rounded-xl border border-white/10 bg-black/20">
            <table className="w-full min-w-[900px] border-collapse text-left text-sm">
              <thead>
                <tr className="border-b border-white/10 text-xs uppercase tracking-wider text-[#848E9C]">
                  <th className="px-3 py-3">用户</th>
                  <th className="px-3 py-3">余额(站内)</th>
                  <th className="px-3 py-3">交易员</th>
                  <th className="px-3 py-3">交易所</th>
                  <th className="px-3 py-3">币安未平仓</th>
                  <th className="px-3 py-3 text-right">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {sorted.map((u: AdminUserRow) => (
                  <tr key={u.id} className="hover:bg-white/[0.02]">
                    <td className="px-3 py-3 align-top">
                      <div className="font-medium">{u.display_name || '—'}</div>
                      <div className="mt-0.5 text-xs text-[#848E9C]">
                        {u.email}
                      </div>
                      <div className="mt-1 font-mono text-[10px] text-[#5e6673]">
                        {u.id}
                      </div>
                    </td>
                    <td className="px-3 py-3 align-top tabular-nums font-semibold text-[#d4ff33]">
                      {Number(u.balance_usdt ?? 0).toFixed(2)} USDT
                    </td>
                    <td className="px-3 py-3 align-top tabular-nums">
                      {u.trader_count}
                    </td>
                    <td className="px-3 py-3 align-top text-xs text-[#b7bdc6]">
                      {(u.exchanges || []).length === 0 ? (
                        '—'
                      ) : (
                        <ul className="max-w-[220px] space-y-1">
                          {u.exchanges.map((ex) => (
                            <li key={ex.id}>
                              {ex.exchange_type} · {ex.account_name || '默认'}{' '}
                              {ex.enabled ? '' : '(关)'}
                              <span
                                className="mt-0.5 block font-mono text-[10px] text-[#5e6673]"
                                title="exchange_id"
                              >
                                {ex.id}
                              </span>
                            </li>
                          ))}
                        </ul>
                      )}
                    </td>
                    <td className="px-3 py-3 align-top text-xs text-[#b7bdc6]">
                      {(u.binance_open_positions || []).length === 0 ? (
                        '—'
                      ) : (
                        <ul className="max-w-[280px] space-y-1">
                          {u.binance_open_positions.map((p, i) => (
                            <li key={`${p.trader_id}-${p.symbol}-${i}`}>
                              {p.trader_name}: {p.symbol} {p.side} 数量 {p.size}
                            </li>
                          ))}
                        </ul>
                      )}
                    </td>
                    <td className="px-3 py-3 align-top text-right">
                      <button
                        type="button"
                        onClick={() => {
                          setAdjustUserId(u.id)
                          setDeltaInput('')
                          setNoteInput('')
                          setConfirmedDeposit(false)
                          setOriginalDepositLedgerId('')
                        }}
                        className="rounded-lg bg-[#d4ff33]/15 px-2 py-1 text-xs font-bold text-[#d4ff33] hover:bg-[#d4ff33]/25"
                      >
                        调账
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <p className="mt-4 text-center text-xs text-[#5e6673]">
            数据时间：{data?.generated_at || '—'} ·
            币安仓位来自本系统记录的未平仓表
          </p>
        </AdminCollapsibleSection>

        <AdminCollapsibleSection
          defaultOpen={false}
          className="mt-10 rounded-xl border border-white/10 bg-nofx-bg-secondary"
          title="币安返佣统计"
          subtitle="通过 Binance Broker API 查询现货/合约返佣明细；需配置 BINANCE_BROKER_API_KEY / BINANCE_BROKER_API_SECRET。"
          headerRight={
            <button
              type="button"
              onClick={() => void mutateRebates()}
              className="rounded-lg border border-white/10 px-3 py-2 text-xs text-[#d4ff33] hover:bg-white/5"
            >
              刷新返佣
            </button>
          }
        >
          {!rebateData?.configured ? (
            <div className="mt-4 rounded-xl border border-amber-500/25 bg-amber-500/10 px-4 py-4 text-sm text-amber-100">
              {rebateData?.message || '未配置币安 Broker API Key'}
            </div>
          ) : (
            <>
              <div className="mt-4 flex flex-wrap gap-2">
                {Object.entries(rebateData.totals || {}).map(
                  ([asset, amount]) => (
                    <div
                      key={asset}
                      className="rounded-xl border border-[#d4ff33]/20 bg-[#d4ff33]/10 px-4 py-3"
                    >
                      <div className="text-[11px] text-[#848E9C]">{asset}</div>
                      <div className="font-mono text-lg font-bold text-[#d4ff33]">
                        {Number(amount).toFixed(8)}
                      </div>
                    </div>
                  )
                )}
                {Object.keys(rebateData.totals || {}).length === 0 ? (
                  <div className="text-sm text-[#848E9C]">暂无返佣记录</div>
                ) : null}
              </div>
              {(rebateData.errors ?? []).length > 0 ? (
                <div className="mt-3 rounded-lg border border-red-500/25 bg-red-500/10 px-3 py-2 text-xs text-red-200">
                  {rebateData.errors?.join('；')}
                </div>
              ) : null}
              <div className="mt-4 overflow-x-auto rounded-xl border border-white/10">
                <table className="w-full min-w-[900px] border-collapse text-left text-xs">
                  <thead>
                    <tr className="border-b border-white/10 uppercase tracking-wider text-[#848E9C]">
                      <th className="px-3 py-3">时间</th>
                      <th className="px-3 py-3">市场</th>
                      <th className="px-3 py-3">币种</th>
                      <th className="px-3 py-3">返佣</th>
                      <th className="px-3 py-3">客户/子账户</th>
                      <th className="px-3 py-3">交易对</th>
                      <th className="px-3 py-3">类型</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-white/5">
                    {(rebateData.items ?? [])
                      .slice(0, 200)
                      .map((row: AdminBinanceBrokerRebateRow, idx) => (
                        <tr
                          key={`${row.market}-${row.time}-${idx}`}
                          className="hover:bg-white/[0.02]"
                        >
                          <td className="px-3 py-3 text-[#b7bdc6]">
                            {row.time
                              ? new Date(row.time).toLocaleString()
                              : '—'}
                          </td>
                          <td className="px-3 py-3">{row.market}</td>
                          <td className="px-3 py-3">{row.asset || '—'}</td>
                          <td className="px-3 py-3 font-mono font-bold text-[#d4ff33]">
                            {Number(row.amount ?? 0).toFixed(8)}
                          </td>
                          <td className="px-3 py-3 font-mono text-[10px] text-[#848E9C]">
                            {row.customer_id || row.sub_account_id || '—'}
                          </td>
                          <td className="px-3 py-3">{row.symbol || '—'}</td>
                          <td className="px-3 py-3">
                            {row.income_type || '—'}
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </AdminCollapsibleSection>

        <AdminCollapsibleSection
          defaultOpen={false}
          className="mt-10 rounded-xl border border-white/10 bg-nofx-bg-secondary"
          title="AI 调用账单流水"
          subtitle="查看每次 COMKUN 代理调用的真实成本、加服务费后扣费和用户余额变化。"
          headerRight={
            <select
              value={usageUserId}
              onChange={(e) => setUsageUserId(e.target.value)}
              className="rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
            >
              <option value="">全部客户</option>
              {sorted.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.email}
                </option>
              ))}
            </select>
          }
        >
          <div className="mt-4 overflow-x-auto rounded-xl border border-white/10">
            <table className="w-full min-w-[1100px] border-collapse text-left text-xs">
              <thead>
                <tr className="border-b border-white/10 uppercase tracking-wider text-[#848E9C]">
                  <th className="px-3 py-3">时间</th>
                  <th className="px-3 py-3">客户</th>
                  <th className="px-3 py-3">模型</th>
                  <th className="px-3 py-3">真实花费</th>
                  <th className="px-3 py-3">服务费后扣费</th>
                  <th className="px-3 py-3">余额变化</th>
                  <th className="px-3 py-3">状态</th>
                  <th className="px-3 py-3">交易员</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {(usageData?.items ?? []).length === 0 ? (
                  <tr>
                    <td
                      colSpan={8}
                      className="px-3 py-8 text-center text-[#848E9C]"
                    >
                      暂无 AI 调用流水
                    </td>
                  </tr>
                ) : (
                  (usageData?.items ?? []).map(
                    (row: AdminAIPlatformUsageRow) => (
                      <tr key={row.id} className="hover:bg-white/[0.02]">
                        <td className="px-3 py-3 text-[#b7bdc6]">
                          {new Date(row.created_at).toLocaleString()}
                        </td>
                        <td className="px-3 py-3">
                          <div className="font-medium text-white">
                            {row.user_display_name || '—'}
                          </div>
                          <div className="text-[#848E9C]">
                            {row.user_email || row.user_id}
                          </div>
                        </td>
                        <td className="px-3 py-3">
                          <div className="font-semibold text-white">
                            {formatUsageModelName(row)}
                          </div>
                          {formatUsageProviderName(row) ? (
                            <div className="text-[#848E9C]">
                              {formatUsageProviderName(row)}
                            </div>
                          ) : null}
                        </td>
                        <td className="px-3 py-3 tabular-nums text-[#b2dfdb]">
                          {Number(row.actual_cost_usdc ?? 0).toFixed(6)} USDC
                        </td>
                        <td className="px-3 py-3 tabular-nums font-bold text-[#d4ff33]">
                          {Number(row.charged_usdt ?? 0).toFixed(6)} USDT
                        </td>
                        <td className="px-3 py-3 tabular-nums text-[#c5e1a5]">
                          {Number(row.wallet_balance_before ?? 0).toFixed(2)} →{' '}
                          {Number(row.wallet_balance_after ?? 0).toFixed(2)}
                        </td>
                        <td className="px-3 py-3">
                          <span
                            className={`rounded-full px-2 py-1 text-[11px] font-bold ${
                              row.status === 'success'
                                ? 'bg-emerald-500/15 text-emerald-300'
                                : row.status === 'refunded'
                                  ? 'bg-amber-500/15 text-amber-300'
                                  : row.status === 'failed'
                                    ? 'bg-red-500/15 text-red-300'
                                    : 'bg-zinc-500/15 text-zinc-300'
                            }`}
                          >
                            {row.status}
                          </span>
                        </td>
                        <td className="px-3 py-3 font-mono text-[10px] text-[#848E9C]">
                          {row.trader_id || '—'}
                        </td>
                      </tr>
                    )
                  )
                )}
              </tbody>
            </table>
          </div>
        </AdminCollapsibleSection>
      </div>

      {adjustUserId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
          <div className="w-full max-w-md rounded-2xl border border-white/10 bg-nofx-bg-secondary p-6 shadow-2xl">
            <h2 className="text-lg font-bold">调整用户余额</h2>
            <p className="mt-2 text-xs text-[#848E9C]">
              用户 ID：{adjustUserId}
            </p>
            <label className="mt-4 block text-xs text-[#848E9C]">
              金额变动（USDT，正数增加、负数扣减）
              <input
                type="number"
                step="0.01"
                value={deltaInput}
                onChange={(e) => setDeltaInput(e.target.value)}
                className="mt-1 w-full rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
              />
            </label>
            <label className="mt-3 block text-xs text-[#848E9C]">
              备注（可选）
              <input
                type="text"
                value={noteInput}
                onChange={(e) => setNoteInput(e.target.value)}
                className="mt-1 w-full rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
              />
            </label>
            <label className="mt-3 flex items-center gap-2 text-xs text-[#b7bdc6]">
              <input
                type="checkbox"
                checked={confirmedDeposit}
                onChange={(e) => setConfirmedDeposit(e.target.checked)}
                disabled={Number(deltaInput) <= 0}
                className="h-4 w-4 accent-[#d4ff33]"
              />
              标记为确认充值并参与新返佣
            </label>
            {Number(deltaInput) < 0 ? (
              <label className="mt-3 block text-xs text-[#848E9C]">
                原确认充值账本ID（填写后同步扣回返佣）
                <input
                  type="number"
                  min="1"
                  value={originalDepositLedgerId}
                  onChange={(e) => setOriginalDepositLedgerId(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
                />
              </label>
            ) : null}
            <div className="mt-6 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setAdjustUserId(null)}
                className="rounded-lg px-4 py-2 text-sm text-[#848E9C] hover:bg-white/5"
              >
                取消
              </button>
              <button
                type="button"
                disabled={submitting}
                onClick={() => void handleAdjust()}
                className="rounded-lg bg-[#d4ff33] px-4 py-2 text-sm font-bold text-black disabled:opacity-50"
              >
                {submitting ? '提交中…' : '确认'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
