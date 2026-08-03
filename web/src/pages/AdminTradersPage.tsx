import { Bot } from 'lucide-react'
import { useMemo, useState } from 'react'
import useSWR from 'swr'
import { toast } from 'sonner'
import {
  AdminModuleHeader,
  AdminModuleShell,
} from '../components/admin/AdminModuleHeader'
import { api } from '../lib/api'
import type { AdminRunningTraderRow } from '../lib/api/walletAdmin'

function sideName(side: string) {
  const value = side.toUpperCase()
  return value === 'LONG' || value === 'BUY'
    ? '多'
    : value === 'SHORT' || value === 'SELL'
      ? '空'
      : side || '—'
}

export function AdminTradersPage() {
  const [busy, setBusy] = useState<string | null>(null)
  const { data, error, mutate } = useSWR(
    'admin-users-overview',
    () => api.getAdminUsersOverview(),
    { refreshInterval: 15000 }
  )
  const rows = data?.running_traders ?? []
  const totals = useMemo(
    () =>
      rows.reduce(
        (sum, row) => {
          if (row.is_running === false) return sum
          sum.count += 1
          sum.pnl += Number(row.total_pnl || 0)
          sum.margin += Number(row.margin_used || 0)
          sum.available += Number(row.available_balance || 0)
          return sum
        },
        { count: 0, pnl: 0, margin: 0, available: 0 }
      ),
    [rows]
  )

  async function control(traderId: string, action: 'start' | 'stop' | 'sync') {
    setBusy(traderId)
    try {
      if (action === 'start') await api.postAdminTraderStart(traderId)
      if (action === 'stop') await api.postAdminTraderStop(traderId)
      if (action === 'sync')
        await api.postAdminTraderSyncPositionsFromExchange(traderId)
      await mutate()
      toast.success(action === 'sync' ? '持仓已同步' : '交易员状态已更新')
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '操作失败')
    } finally {
      setBusy(null)
    }
  }

  return (
    <AdminModuleShell>
      <AdminModuleHeader
        icon={Bot}
        title="交易员管理"
        description="查看本机快照，并代客户启动、停止或同步交易所持仓"
        onRefresh={() => void mutate()}
      />
      {error && (
        <p className="mt-5 text-sm text-red-300">
          {error instanceof Error ? error.message : '加载失败'}
        </p>
      )}
      <section className="mt-5 grid overflow-hidden rounded-md border border-white/10 bg-nofx-bg-secondary sm:grid-cols-2 lg:grid-cols-4">
        {[
          ['运行中', totals.count],
          ['合计盈亏', totals.pnl.toFixed(4)],
          ['保证金占用', totals.margin.toFixed(4)],
          ['可用余额', totals.available.toFixed(4)],
        ].map(([label, value]) => (
          <div
            key={label}
            className="border-b border-r border-white/10 px-4 py-4 lg:border-b-0"
          >
            <div className="text-[11px] text-[#848E9C]">{label}</div>
            <div className="mt-1 text-lg font-bold tabular-nums text-white">
              {value}
            </div>
          </div>
        ))}
      </section>
      {data?.running_traders_meta?.hint && (
        <p className="mt-4 text-xs text-[#848E9C]">
          {data.running_traders_meta.hint}
        </p>
      )}
      <section className="mt-5 overflow-x-auto border-y border-white/10">
        <table className="w-full min-w-[1180px] text-left text-xs">
          <thead>
            <tr className="border-b border-white/10 text-[#848E9C]">
              <th className="px-3 py-3">交易员</th>
              <th>用户</th>
              <th>状态</th>
              <th>权益 / 可用</th>
              <th>盈亏</th>
              <th>保证金</th>
              <th>持仓</th>
              <th>数据来源</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/5">
            {rows.map((row: AdminRunningTraderRow) => {
              const on = row.is_running !== false
              return (
                <tr key={row.trader_id}>
                  <td className="px-3 py-3">
                    <strong>{row.trader_name}</strong>
                    <div className="font-mono text-[10px] text-[#5e6673]">
                      {row.trader_id}
                    </div>
                  </td>
                  <td>{row.user_display_name || row.user_email}</td>
                  <td className={on ? 'text-emerald-300' : 'text-[#848E9C]'}>
                    {on ? '运行中' : '已停止'}
                  </td>
                  <td>
                    {Number(row.total_equity || 0).toFixed(4)} /{' '}
                    {Number(row.available_balance || 0).toFixed(4)}
                  </td>
                  <td
                    className={
                      Number(row.total_pnl || 0) >= 0
                        ? 'text-emerald-300'
                        : 'text-red-300'
                    }
                  >
                    {Number(row.total_pnl || 0).toFixed(4)}
                  </td>
                  <td>{Number(row.margin_used || 0).toFixed(4)}</td>
                  <td>
                    {(row.positions || []).length ? (
                      <div className="space-y-1">
                        {row.positions?.map((position, index) => (
                          <div key={`${position.symbol}-${index}`}>
                            {position.symbol} {sideName(position.side)} ·{' '}
                            {Number(position.quantity ?? position.size ?? 0)}
                          </div>
                        ))}
                      </div>
                    ) : (
                      '—'
                    )}
                  </td>
                  <td>
                    {row.error ||
                      (row.metrics_source === 'exchange_live'
                        ? '交易所实时'
                        : '本机快照')}
                  </td>
                  <td>
                    <div className="flex gap-1">
                      <button
                        disabled={busy === row.trader_id || on}
                        onClick={() => void control(row.trader_id, 'start')}
                        className="rounded border border-emerald-500/40 px-2 py-1 text-emerald-300 disabled:opacity-30"
                      >
                        启动
                      </button>
                      <button
                        disabled={busy === row.trader_id || !on}
                        onClick={() => void control(row.trader_id, 'stop')}
                        className="rounded border border-red-500/40 px-2 py-1 text-red-300 disabled:opacity-30"
                      >
                        停止
                      </button>
                      <button
                        disabled={busy === row.trader_id}
                        onClick={() => void control(row.trader_id, 'sync')}
                        className="rounded border border-[#d4ff33]/40 px-2 py-1 text-[#d4ff33] disabled:opacity-30"
                      >
                        同步持仓
                      </button>
                    </div>
                  </td>
                </tr>
              )
            })}
            {!rows.length && (
              <tr>
                <td colSpan={9} className="py-12 text-center text-[#5e6673]">
                  暂无交易员
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>
    </AdminModuleShell>
  )
}
