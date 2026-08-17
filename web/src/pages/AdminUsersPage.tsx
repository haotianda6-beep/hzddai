import { Users } from 'lucide-react'
import { useState } from 'react'
import useSWR from 'swr'
import { toast } from 'sonner'
import {
  AdminModuleHeader,
  AdminModuleShell,
} from '../components/admin/AdminModuleHeader'
import { api } from '../lib/api'

export function AdminUsersPage() {
  const [adjustUserId, setAdjustUserId] = useState<string | null>(null)
  const [delta, setDelta] = useState('')
  const [note, setNote] = useState('')
  const [confirmedDeposit, setConfirmedDeposit] = useState(false)
  const [originalLedgerId, setOriginalLedgerId] = useState('')
  const [busy, setBusy] = useState(false)
  const { data, error, mutate } = useSWR(
    'admin-users-overview',
    () => api.getAdminUsersOverview(),
    { refreshInterval: 15000 }
  )
  const rows = [...(data?.users || [])].sort(
    (a, b) =>
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  )

  function openAdjust(userId: string) {
    setAdjustUserId(userId)
    setDelta('')
    setNote('')
    setConfirmedDeposit(false)
    setOriginalLedgerId('')
  }

  async function submit() {
    if (
      !adjustUserId ||
      !Number.isFinite(Number(delta)) ||
      Number(delta) === 0
    ) {
      toast.error('请输入非零调账金额')
      return
    }
    setBusy(true)
    try {
      await api.postAdminWalletAdjust(
        adjustUserId,
        Number(delta),
        note.trim(),
        confirmedDeposit,
        Number(originalLedgerId) || undefined
      )
      await mutate()
      setAdjustUserId(null)
      toast.success('用户余额已更新')
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '调账失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AdminModuleShell>
      <AdminModuleHeader
        icon={Users}
        title="用户与余额"
        description="查看全部用户、交易所绑定和未平仓摘要，并执行站内余额调账"
        onRefresh={() => void mutate()}
      />
      {error && (
        <p className="mt-5 text-sm text-red-300">
          {error instanceof Error ? error.message : '加载失败'}
        </p>
      )}
      <section className="mt-5 overflow-x-auto border-y border-white/10">
        <table className="w-full min-w-[940px] text-left text-xs">
          <thead>
            <tr className="border-b border-white/10 text-[#848E9C]">
              <th className="px-3 py-3">用户</th>
              <th>站内余额</th>
              <th>交易员</th>
              <th>交易所</th>
              <th>币安未平仓</th>
              <th>注册时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/5">
            {rows.map((user) => (
              <tr key={user.id}>
                <td className="px-3 py-3">
                  <strong>{user.display_name || '未命名'}</strong>
                  <div>{user.email}</div>
                  <div className="font-mono text-[10px] text-[#5e6673]">
                    {user.id}
                  </div>
                </td>
                <td className="font-bold text-[#d4ff33]">
                  {Number(user.balance_usdt || 0).toFixed(4)} USDT
                </td>
                <td>{user.trader_count}</td>
                <td>
                  {user.exchanges?.length
                    ? user.exchanges
                        .map(
                          (exchange) =>
                            `${exchange.exchange_type} · ${exchange.account_name || '默认'}`
                        )
                        .join('；')
                    : '—'}
                </td>
                <td>
                  {user.binance_open_positions?.length
                    ? user.binance_open_positions
                        .map(
                          (position) =>
                            `${position.trader_name}: ${position.symbol} ${position.side} ${position.size}`
                        )
                        .join('；')
                    : '—'}
                </td>
                <td>{new Date(user.created_at).toLocaleString()}</td>
                <td>
                  <button
                    type="button"
                    onClick={() => openAdjust(user.id)}
                    className="rounded bg-[#d4ff33]/15 px-2.5 py-1.5 font-bold text-[#d4ff33]"
                  >
                    调账
                  </button>
                </td>
              </tr>
            ))}
            {!rows.length && (
              <tr>
                <td colSpan={7} className="py-12 text-center text-[#5e6673]">
                  暂无用户
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>
      {adjustUserId && (
        <div
          className="fixed inset-0 z-[80] flex items-center justify-center bg-black/70 p-4"
          role="dialog"
          aria-modal="true"
        >
          <div className="w-full max-w-md rounded-md border border-white/10 bg-nofx-bg-secondary p-6">
            <h2 className="text-lg font-bold">调整用户余额</h2>
            <p className="mt-1 font-mono text-[10px] text-[#848E9C]">
              {adjustUserId}
            </p>
            <label className="mt-4 block text-xs text-[#848E9C]">
              金额变动（正数增加，负数扣减）
              <input
                type="number"
                step="0.01"
                value={delta}
                onChange={(event) => setDelta(event.target.value)}
                className="mt-1 w-full rounded border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
              />
            </label>
            <label className="mt-3 block text-xs text-[#848E9C]">
              备注
              <input
                value={note}
                onChange={(event) => setNote(event.target.value)}
                className="mt-1 w-full rounded border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
              />
            </label>
            <label className="mt-3 flex items-center gap-2 text-xs text-[#b7bdc6]">
              <input
                type="checkbox"
                checked={confirmedDeposit}
                onChange={(event) => setConfirmedDeposit(event.target.checked)}
                disabled={Number(delta) <= 0}
                className="h-4 w-4 accent-[#d4ff33]"
              />
              标记为确认充值并参与新返佣
            </label>
            {Number(delta) < 0 && (
              <label className="mt-3 block text-xs text-[#848E9C]">
                原确认充值账本 ID
                <input
                  type="number"
                  min="1"
                  value={originalLedgerId}
                  onChange={(event) => setOriginalLedgerId(event.target.value)}
                  className="mt-1 w-full rounded border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
                />
              </label>
            )}
            <div className="mt-6 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setAdjustUserId(null)}
                className="rounded border border-white/10 px-4 py-2 text-sm"
              >
                取消
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => void submit()}
                className="rounded bg-[#d4ff33] px-4 py-2 text-sm font-bold text-black disabled:opacity-40"
              >
                {busy ? '提交中...' : '确认'}
              </button>
            </div>
          </div>
        </div>
      )}
    </AdminModuleShell>
  )
}
