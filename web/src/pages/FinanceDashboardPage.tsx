import { useMemo, useState } from 'react'
import useSWR from 'swr'
import { toast } from 'sonner'
import { Link } from 'react-router-dom'
import { RefreshCw, Wallet } from 'lucide-react'
import { api } from '../lib/api'
import { ROUTES } from '../router/paths'

export function FinanceDashboardPage() {
  const { data, error, isLoading, mutate } = useSWR('finance-users-lite', () => api.getFinanceUsersLite(), {
    refreshInterval: 20000,
  })
  const [q, setQ] = useState('')
  const [adjustUserId, setAdjustUserId] = useState<string | null>(null)
  const [amountInput, setAmountInput] = useState('')
  const [noteInput, setNoteInput] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const users = data?.users ?? []
  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    if (!s) return users
    return users.filter((u) => {
      const mail = (u.email || '').toLowerCase()
      const name = (u.display_name || '').toLowerCase()
      const id = (u.id || '').toLowerCase()
      return mail.includes(s) || name.includes(s) || id.includes(s)
    })
  }, [users, q])

  const submitAdjust = async () => {
    if (!adjustUserId) return
    const amt = Number(amountInput)
    if (!Number.isFinite(amt) || amt <= 0) {
      toast.error('请输入大于 0 的入账金额（USDT）')
      return
    }
    setSubmitting(true)
    try {
      await api.postFinanceWalletAdjust(adjustUserId, amt, noteInput.trim())
      toast.success('已为客户入账')
      setAdjustUserId(null)
      setAmountInput('')
      setNoteInput('')
      await mutate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '入账失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="mx-auto max-w-6xl px-4 pb-16 pt-8 text-[#EAECEF] sm:px-6">
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <Link to={ROUTES.profile} className="text-sm text-[#848E9C] hover:text-[#d4ff33]">
            ← 返回个人中心
          </Link>
          <h1 className="mt-2 font-['Space_Grotesk',sans-serif] text-2xl font-bold tracking-tight sm:text-3xl">
            财务台 · 客户入账
          </h1>
          <p className="mt-2 max-w-2xl text-sm text-[#848E9C]">
            与管理员「正数调账」相同：增加客户站内余额、记入流水（finance_adjust）、并同步返利侧充值与分佣。仅支持正数入账；不可给自己入账。
          </p>
        </div>
        <button
          type="button"
          onClick={() => void mutate()}
          className="inline-flex items-center justify-center gap-2 self-start rounded-lg border border-[#2b3139] bg-[#1c1c1c] px-4 py-2 text-sm font-semibold text-[#d4ff33] hover:border-[#d4ff33]/40"
        >
          <RefreshCw className="h-4 w-4" />
          刷新列表
        </button>
      </div>

      {error && (
        <div className="mb-6 rounded-xl border border-red-500/35 bg-red-950/40 px-4 py-3 text-sm text-red-100">
          加载失败：{error instanceof Error ? error.message : String(error)}
        </div>
      )}

      <div className="mb-4">
        <input
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="按邮箱、昵称、用户 ID 筛选…"
          className="w-full max-w-md rounded-xl border border-[#2b3139] bg-[#121212] px-4 py-2.5 text-sm outline-none focus:border-[#d4ff33]/45"
        />
      </div>

      {isLoading && !data && (
        <div className="py-16 text-center text-sm text-[#848E9C]">加载中…</div>
      )}

      {!isLoading && data && (
        <div className="overflow-x-auto rounded-xl border border-[#2b3139] bg-[#121212]">
          <table className="w-full min-w-[720px] border-collapse text-left text-sm">
            <thead>
              <tr className="border-b border-[#2b3139] text-xs uppercase tracking-wide text-[#848E9C]">
                <th className="px-4 py-3 font-semibold">用户 ID</th>
                <th className="px-4 py-3 font-semibold">邮箱</th>
                <th className="px-4 py-3 font-semibold">昵称</th>
                <th className="px-4 py-3 font-semibold">站内余额</th>
                <th className="px-4 py-3 font-semibold">操作</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((u) => (
                <tr key={u.id} className="border-b border-[#2b3139]/60 hover:bg-white/[0.02]">
                  <td className="px-4 py-2.5 font-mono text-xs text-[#b7bdc6]">{u.id}</td>
                  <td className="px-4 py-2.5">{u.email}</td>
                  <td className="px-4 py-2.5">{u.display_name || '—'}</td>
                  <td className="px-4 py-2.5 font-mono tabular-nums text-[#d4ff33]">
                    {(u.balance_usdt ?? 0).toFixed(4)}
                  </td>
                  <td className="px-4 py-2.5">
                    <button
                      type="button"
                      onClick={() => {
                        setAdjustUserId(u.id)
                        setAmountInput('')
                        setNoteInput('')
                      }}
                      className="rounded-lg border border-[#d4ff33]/35 bg-[#d4ff33]/10 px-3 py-1.5 text-xs font-bold text-[#d4ff33] hover:bg-[#d4ff33]/15"
                    >
                      入账
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {filtered.length === 0 && (
            <div className="px-4 py-10 text-center text-sm text-[#848E9C]">无匹配用户</div>
          )}
        </div>
      )}

      {adjustUserId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4">
          <div className="w-full max-w-md rounded-2xl border border-[#2b3139] bg-[#121212] p-6 shadow-2xl">
            <div className="flex items-center gap-2 text-[#d4ff33]">
              <Wallet className="h-5 w-5" />
              <h2 className="text-lg font-bold">为客户入账</h2>
            </div>
            <p className="mt-2 text-xs text-[#848E9C]">目标用户 ID（不可改）：</p>
            <p className="mt-1 font-mono text-sm text-[#EAECEF]">{adjustUserId}</p>
            <label className="mt-4 block text-sm font-medium text-[#b7bdc6]">入账金额（USDT，正数）</label>
            <input
              type="number"
              min={0.0001}
              step="any"
              value={amountInput}
              onChange={(e) => setAmountInput(e.target.value)}
              className="mt-1 w-full rounded-lg border border-[#2b3139] bg-black/30 px-3 py-2 font-mono text-sm outline-none focus:border-[#d4ff33]/45"
              placeholder="例如 100"
            />
            <label className="mt-3 block text-sm font-medium text-[#b7bdc6]">备注（可选，写入流水）</label>
            <input
              type="text"
              value={noteInput}
              onChange={(e) => setNoteInput(e.target.value)}
              className="mt-1 w-full rounded-lg border border-[#2b3139] bg-black/30 px-3 py-2 text-sm outline-none focus:border-[#d4ff33]/45"
              placeholder="例如 客服确认到账"
            />
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
                onClick={() => void submitAdjust()}
                className="rounded-lg bg-[#d4ff33] px-4 py-2 text-sm font-bold text-black disabled:opacity-50"
              >
                {submitting ? '提交中…' : '确认入账'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
