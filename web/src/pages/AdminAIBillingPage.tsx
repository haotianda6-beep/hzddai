import { ReceiptText } from 'lucide-react'
import { useState } from 'react'
import useSWR from 'swr'
import {
  AdminModuleHeader,
  AdminModuleShell,
} from '../components/admin/AdminModuleHeader'
import { api } from '../lib/api'
import type { AdminAIPlatformUsageRow } from '../lib/api/walletAdmin'

function modelName(row: AdminAIPlatformUsageRow) {
  return row.provider === 'comkun_ai' || /跟单/.test(row.model)
    ? 'comkun-ai'
    : row.model || '—'
}

export function AdminAIBillingPage() {
  const [userId, setUserId] = useState('')
  const { data: users } = useSWR('admin-users-overview', () =>
    api.getAdminUsersOverview()
  )
  const { data, error, mutate } = useSWR(
    ['admin-ai-platform-usage', userId],
    () => api.getAdminAIPlatformUsage(userId || undefined),
    { refreshInterval: 30000 }
  )
  return (
    <AdminModuleShell>
      <AdminModuleHeader
        icon={ReceiptText}
        title="AI 调用账单"
        description="核对每次代理调用的真实成本、服务费和用户余额变化"
        onRefresh={() => void mutate()}
      />
      <div className="mt-5 flex justify-end">
        <label className="text-xs text-[#848E9C]">
          筛选客户
          <select
            value={userId}
            onChange={(event) => setUserId(event.target.value)}
            className="ml-2 rounded border border-white/10 bg-black/30 px-3 py-2 text-sm text-white"
          >
            <option value="">全部客户</option>
            {(users?.users || []).map((user) => (
              <option key={user.id} value={user.id}>
                {user.email}
              </option>
            ))}
          </select>
        </label>
      </div>
      {error && (
        <p className="mt-4 text-sm text-red-300">
          {error instanceof Error ? error.message : '加载失败'}
        </p>
      )}
      <section className="mt-4 overflow-x-auto border-y border-white/10">
        <table className="w-full min-w-[1050px] text-left text-xs">
          <thead>
            <tr className="border-b border-white/10 text-[#848E9C]">
              <th className="px-3 py-3">时间</th>
              <th>客户</th>
              <th>模型</th>
              <th>真实花费</th>
              <th>服务费后扣费</th>
              <th>余额变化</th>
              <th>状态</th>
              <th>交易员</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/5">
            {(data?.items || []).map((row: AdminAIPlatformUsageRow) => (
              <tr key={row.id}>
                <td className="px-3 py-3">
                  {new Date(row.created_at).toLocaleString()}
                </td>
                <td>
                  {row.user_display_name || row.user_email || row.user_id}
                </td>
                <td>{modelName(row)}</td>
                <td>{Number(row.actual_cost_usdc || 0).toFixed(6)} USDC</td>
                <td className="font-bold text-[#d4ff33]">
                  {Number(row.charged_usdt || 0).toFixed(6)} USDT
                </td>
                <td>
                  {Number(row.wallet_balance_before || 0).toFixed(2)} →{' '}
                  {Number(row.wallet_balance_after || 0).toFixed(2)}
                </td>
                <td>{row.status}</td>
                <td className="font-mono text-[10px]">
                  {row.trader_id || '—'}
                </td>
              </tr>
            ))}
            {!data?.items.length && (
              <tr>
                <td colSpan={8} className="py-12 text-center text-[#5e6673]">
                  暂无 AI 调用流水
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>
    </AdminModuleShell>
  )
}
