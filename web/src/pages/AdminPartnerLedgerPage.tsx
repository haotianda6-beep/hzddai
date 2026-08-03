import { ArrowLeft, RefreshCw, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import useSWR from 'swr'
import { toast } from 'sonner'
import { api } from '../lib/api'
import type { PartnerRole } from '../lib/api/walletAdmin'
import { ROUTES } from '../router/paths'
import {
  AdminPartnerLedgerTables,
  type PartnerAdminTab,
} from './AdminPartnerLedgerTables'

const tabs: Array<[PartnerAdminTab, string]> = [
  ['users', '用户与身份'],
  ['deposits', '充值账单'],
  ['commissions', '返佣账单'],
  ['withdrawals', '提现审核'],
  ['studios', '工作室审核'],
  ['audits', '身份变更'],
]

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-b border-r border-white/10 px-4 py-4 last:border-r-0 lg:border-b-0">
      <div className="text-[11px] text-[#848E9C]">{label}</div>
      <div className="mt-1 text-lg font-bold tabular-nums text-white">
        {value}
      </div>
    </div>
  )
}

export function AdminPartnerLedgerPage() {
  const [tab, setTab] = useState<PartnerAdminTab>('users')
  const [busy, setBusy] = useState<string | null>(null)
  const { data, error, isLoading, mutate } = useSWR(
    'admin-partner-dashboard',
    () => api.getAdminPartnerDashboard(),
    { refreshInterval: 10000, revalidateOnFocus: true }
  )

  async function setRole(userId: string, role: PartnerRole) {
    if (!confirm(`确认把该用户身份调整为 ${role}？`)) return
    setBusy(`role-${userId}`)
    try {
      await api.postAdminPartnerRole(userId, role, '主站超级管理员调整')
      await mutate()
      toast.success('身份已更新')
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '身份更新失败')
    } finally {
      setBusy(null)
    }
  }

  async function review(
    kind: 'studio-requests' | 'withdrawals',
    id: number,
    action: 'approve' | 'reject' | 'paid'
  ) {
    if (!confirm('确认执行此审核操作？')) return
    setBusy(`${kind}-${id}`)
    try {
      await api.postAdminPartnerReview(kind, id, action)
      await mutate()
      toast.success('审核状态已更新')
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '审核失败')
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="min-h-screen bg-nofx-bg-tertiary px-4 pb-16 pt-20 text-[#EAECEF] sm:px-8">
      <main className="mx-auto max-w-[1400px]">
        <Link
          to={ROUTES.admin}
          className="inline-flex items-center gap-1 text-sm text-[#848E9C] hover:text-[#d4ff33]"
        >
          <ArrowLeft className="h-4 w-4" /> 管理后台
        </Link>
        <header className="mt-5 flex flex-wrap items-center justify-between gap-4 border-b border-white/10 pb-5">
          <div className="flex items-center gap-3">
            <ShieldCheck className="h-8 w-8 text-[#d4ff33]" />
            <div>
              <h1 className="text-2xl font-bold">合作伙伴总账</h1>
              <p className="mt-1 text-sm text-[#848E9C]">
                充值、差额返佣、身份、工作室申请和提现统一管理
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={() => void mutate()}
            className="inline-flex items-center gap-2 rounded border border-white/10 px-3 py-2 text-sm hover:bg-white/5"
          >
            <RefreshCw className="h-4 w-4" />
            刷新
          </button>
        </header>

        {error && (
          <div className="mt-5 rounded border border-red-500/40 bg-red-950/30 px-4 py-3 text-sm text-red-200">
            {error instanceof Error ? error.message : '总账加载失败'}
          </div>
        )}
        {isLoading && !data && (
          <div className="py-16 text-center text-sm text-[#848E9C]">
            加载总账...
          </div>
        )}
        {data && (
          <>
            <section className="mt-5 grid overflow-hidden rounded-md border border-white/10 bg-nofx-bg-secondary sm:grid-cols-2 lg:grid-cols-6">
              <Metric label="全部用户" value={`${data.summary.users} 人`} />
              <Metric
                label="确认充值"
                value={`${Number(data.summary.confirmed_deposits_usdt).toFixed(2)} U`}
              />
              <Metric
                label="已分配返佣"
                value={`${Number(data.summary.allocated_commission_usdt).toFixed(2)} U`}
              />
              <Metric
                label="平台留存"
                value={`${Number(data.summary.platform_remainder_usdt).toFixed(2)} U`}
              />
              <Metric
                label="可提现负债"
                value={`${Number(data.summary.available_liability_usdt).toFixed(2)} U`}
              />
              <Metric
                label="冻结负债"
                value={`${Number(data.summary.frozen_liability_usdt).toFixed(2)} U`}
              />
            </section>
            <div className="mt-6 flex gap-1 overflow-x-auto border-b border-white/10">
              {tabs.map(([key, label]) => (
                <button
                  key={key}
                  type="button"
                  onClick={() => setTab(key)}
                  className={`whitespace-nowrap border-b-2 px-4 py-3 text-sm ${tab === key ? 'border-[#d4ff33] text-white' : 'border-transparent text-[#848E9C]'}`}
                >
                  {label}
                </button>
              ))}
            </div>
            <section className="overflow-x-auto border-b border-white/10 bg-black/10">
              <AdminPartnerLedgerTables
                tab={tab}
                data={data}
                busy={busy}
                onRole={(id, role) => void setRole(id, role)}
                onReview={(kind, id, action) => void review(kind, id, action)}
              />
            </section>
          </>
        )}
      </main>
    </div>
  )
}
