import { Activity } from 'lucide-react'
import useSWR from 'swr'
import {
  AdminModuleHeader,
  AdminModuleShell,
} from '../components/admin/AdminModuleHeader'
import { api } from '../lib/api'

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="border-b border-r border-white/10 px-4 py-4 last:border-r-0 lg:border-b-0">
      <div className="text-[11px] text-[#848E9C]">{label}</div>
      <div className="mt-1 text-lg font-bold tabular-nums text-white">
        {value}
      </div>
    </div>
  )
}

export function AdminFollowingStatsPage() {
  const { data, error, isLoading, mutate } = useSWR(
    'admin-comkun-following-stats',
    () => api.getAdminComkunFollowingStats(),
    { refreshInterval: 30000 }
  )
  const platform = data?.platform
  return (
    <AdminModuleShell>
      <AdminModuleHeader
        icon={Activity}
        title="跟单统计"
        description="运行中的有效跟单与有效订阅分开统计，运行人数不含仅订阅未启动的用户"
        onRefresh={() => void mutate()}
      />
      {error && <p className="mt-5 text-sm text-red-300">统计加载失败</p>}
      {isLoading && !data && (
        <p className="py-16 text-center text-sm text-[#848E9C]">加载统计...</p>
      )}
      {data && (
        <>
          <section className="mt-5 grid overflow-hidden rounded-md border border-white/10 bg-nofx-bg-secondary sm:grid-cols-3">
            <Metric
              label="平台运行中的有效跟单用户"
              value={platform?.running_follower_count ?? 0}
            />
            <Metric
              label="平台运行中的有效交易员"
              value={platform?.running_trader_count ?? 0}
            />
            <Metric
              label="平台有效订阅用户"
              value={platform?.subscribed_follower_count ?? 0}
            />
          </section>
          <section className="mt-6 overflow-x-auto border-y border-white/10">
            <table className="w-full min-w-[760px] text-left text-xs">
              <thead className="text-[#848E9C]">
                <tr className="border-b border-white/10">
                  <th className="px-3 py-3">策略</th>
                  <th>运行中的有效跟单用户</th>
                  <th>运行中的有效交易员</th>
                  <th>有效订阅用户</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {data.strategies.map((row) => (
                  <tr key={`${row.sourceStrategyId}-${row.strategyId}`}>
                    <td className="px-3 py-3">
                      <strong className="text-white">
                        {row.strategyName || '未命名策略'}
                      </strong>
                      <div className="font-mono text-[10px] text-[#5e6673]">
                        {row.strategyId}
                      </div>
                    </td>
                    <td>{row.runningFollowerCount}</td>
                    <td>{row.runningTraderCount}</td>
                    <td>{row.subscribedFollowerCount}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        </>
      )}
    </AdminModuleShell>
  )
}
