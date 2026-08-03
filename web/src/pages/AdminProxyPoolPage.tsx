import { Network } from 'lucide-react'
import { useMemo, useState } from 'react'
import useSWR from 'swr'
import { toast } from 'sonner'
import {
  AdminModuleHeader,
  AdminModuleShell,
} from '../components/admin/AdminModuleHeader'
import { api } from '../lib/api'

export function AdminProxyPoolPage() {
  const [lines, setLines] = useState('')
  const [importing, setImporting] = useState(false)
  const [assignId, setAssignId] = useState<string | null>(null)
  const [pick, setPick] = useState('')
  const [assigning, setAssigning] = useState(false)
  const { data: users, mutate: mutateUsers } = useSWR(
    'admin-users-overview',
    () => api.getAdminUsersOverview()
  )
  const { data, error, mutate } = useSWR(
    'admin-outbound-proxy-pool',
    () => api.getAdminOutboundProxyPool(),
    { refreshInterval: 15000 }
  )
  const options = useMemo(
    () =>
      (users?.users || []).flatMap((user) =>
        (user.traders || [])
          .filter(
            (trader) =>
              !trader.missing_exchange &&
              trader.exchange_type.toLowerCase() === 'binance'
          )
          .map((trader) => ({
            value: JSON.stringify({
              user_id: user.id,
              exchange_id: trader.exchange_id,
            }),
            label: `${user.display_name || user.email} → ${trader.name}（${trader.account_name || '默认'}）`,
          }))
      ),
    [users]
  )

  async function importLines() {
    setImporting(true)
    try {
      const result = await api.postAdminOutboundProxyPoolImport(lines)
      setLines('')
      await mutate()
      toast.success(`导入完成：新增 ${result.added}，跳过 ${result.skipped}`)
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '导入失败')
    } finally {
      setImporting(false)
    }
  }

  async function remove(id: string) {
    if (!confirm('确认删除这条未分配代理？')) return
    try {
      await api.deleteAdminOutboundProxyPoolEntry(id)
      await mutate()
      toast.success('代理已删除')
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '删除失败')
    }
  }

  async function release(id: string) {
    if (!confirm('确认回收并清空该交易所的出口配置？')) return
    try {
      await api.postAdminOutboundProxyPoolRelease(id)
      await Promise.all([mutate(), mutateUsers()])
      toast.success('代理已回收')
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '回收失败')
    }
  }

  async function assign() {
    if (!assignId || !pick) return
    setAssigning(true)
    try {
      const target = JSON.parse(pick) as {
        user_id: string
        exchange_id: string
      }
      await api.postAdminOutboundProxyPoolAssign(assignId, target)
      setAssignId(null)
      setPick('')
      await Promise.all([mutate(), mutateUsers()])
      toast.success('代理已绑定')
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '绑定失败')
    } finally {
      setAssigning(false)
    }
  }

  return (
    <AdminModuleShell>
      <AdminModuleHeader
        icon={Network}
        title="SOCKS5 出口代理池"
        description="导入、绑定、回收并维护币安交易所专用出口"
        onRefresh={() => {
          void mutate()
          void mutateUsers()
        }}
      />
      {error && (
        <p className="mt-5 text-sm text-red-300">
          {error instanceof Error ? error.message : '加载失败'}
        </p>
      )}
      <section className="mt-5 flex flex-col gap-3 border-b border-white/10 pb-5 sm:flex-row sm:items-end">
        <label className="min-w-0 flex-1 text-xs text-[#848E9C]">
          每行一条完整 socks5 URL，或 host|port|user|pass|到期
          <textarea
            value={lines}
            onChange={(event) => setLines(event.target.value)}
            rows={4}
            className="mt-2 w-full rounded border border-white/10 bg-black/30 px-3 py-2 font-mono text-xs text-white"
          />
        </label>
        <button
          type="button"
          disabled={importing || !lines.trim()}
          onClick={() => void importLines()}
          className="rounded bg-[#d4ff33] px-5 py-3 text-sm font-bold text-black disabled:opacity-40"
        >
          {importing ? '导入中...' : '批量导入'}
        </button>
      </section>
      <section className="mt-5 overflow-x-auto border-y border-white/10">
        <table className="w-full min-w-[900px] text-left text-xs">
          <thead>
            <tr className="border-b border-white/10 text-[#848E9C]">
              <th className="px-3 py-3">出口主机 / IP</th>
              <th>到期</th>
              <th>剩余秒数</th>
              <th>分配用户</th>
              <th>交易所备注</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/5">
            {(data?.entries || []).map((row) => (
              <tr key={row.id}>
                <td className="px-3 py-3 font-mono text-[#d4ff33]">
                  {row.display_host}
                </td>
                <td>
                  {row.expires_at
                    ? new Date(row.expires_at).toLocaleString()
                    : '—'}
                </td>
                <td>{row.seconds_until_expiry ?? '—'}</td>
                <td>
                  {row.assigned_user_display_name ||
                    row.assigned_user_email ||
                    '—'}
                </td>
                <td>{row.assigned_exchange_account_name || '—'}</td>
                <td>
                  {row.assigned_exchange_id ? (
                    <button
                      type="button"
                      onClick={() => void release(row.id)}
                      className="rounded border border-amber-500/40 px-2 py-1 text-amber-300"
                    >
                      强制回收
                    </button>
                  ) : (
                    <div className="flex gap-1">
                      <button
                        type="button"
                        onClick={() => {
                          setAssignId(row.id)
                          setPick('')
                        }}
                        className="rounded border border-[#d4ff33]/40 px-2 py-1 text-[#d4ff33]"
                      >
                        绑定交易所
                      </button>
                      <button
                        type="button"
                        onClick={() => void remove(row.id)}
                        className="rounded border border-red-500/40 px-2 py-1 text-red-300"
                      >
                        删除
                      </button>
                    </div>
                  )}
                </td>
              </tr>
            ))}
            {!data?.entries.length && (
              <tr>
                <td colSpan={6} className="py-12 text-center text-[#5e6673]">
                  暂无代理
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>
      {assignId && (
        <div
          className="fixed inset-0 z-[80] flex items-center justify-center bg-black/70 p-4"
          role="dialog"
          aria-modal="true"
        >
          <div className="w-full max-w-md rounded-md border border-white/10 bg-nofx-bg-secondary p-6">
            <h2 className="text-lg font-bold">绑定代理到币安账户</h2>
            <label className="mt-4 block text-xs text-[#848E9C]">
              选择用户、交易员和账户
              <select
                value={pick}
                onChange={(event) => setPick(event.target.value)}
                className="mt-2 w-full rounded border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
              >
                <option value="">请选择</option>
                {options.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
            {!options.length && (
              <p className="mt-3 text-xs text-amber-300">
                暂无可绑定的币安账户。
              </p>
            )}
            <div className="mt-6 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setAssignId(null)}
                className="rounded border border-white/10 px-4 py-2 text-sm"
              >
                取消
              </button>
              <button
                type="button"
                disabled={assigning || !pick}
                onClick={() => void assign()}
                className="rounded bg-[#d4ff33] px-4 py-2 text-sm font-bold text-black disabled:opacity-40"
              >
                {assigning ? '提交中...' : '确认绑定'}
              </button>
            </div>
          </div>
        </div>
      )}
    </AdminModuleShell>
  )
}
