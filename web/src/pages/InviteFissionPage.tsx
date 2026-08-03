import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import useSWR from 'swr'
import {
  ArrowLeft,
  Check,
  Clipboard,
  Landmark,
  Users,
  WalletCards,
} from 'lucide-react'
import { toast } from 'sonner'
import { api } from '../lib/api'
import { getAccountBadgeName } from '../lib/accountBadge'
import type { PartnerDashboard, PartnerRole } from '../lib/api/walletAdmin'
import { ROUTES } from '../router/paths'
import { useAuth } from '../contexts/AuthContext'

const roleName: Record<PartnerRole, string> = {
  retail: '散户',
  ib: 'IB',
  studio: '工作室',
  branch: '分公司',
}
const statusName: Record<string, string> = {
  pending: '待审核',
  approved: '已批准',
  paid: '已付款',
  rejected: '已驳回',
}

function money(value?: string) {
  const n = Number(value || 0)
  return Number.isFinite(n)
    ? n.toLocaleString(undefined, {
        minimumFractionDigits: 2,
        maximumFractionDigits: 8,
      })
    : '0.00'
}

function Metric({
  label,
  value,
  suffix = 'U',
}: {
  label: string
  value: string
  suffix?: string
}) {
  return (
    <div className="border-r border-zinc-800 px-4 py-4 last:border-r-0">
      <div className="text-[11px] text-zinc-500">{label}</div>
      <div className="mt-1 text-xl font-bold tabular-nums text-white">
        {value}{' '}
        <span className="text-xs font-normal text-zinc-500">{suffix}</span>
      </div>
    </div>
  )
}

export function InviteFissionPage() {
  const { user } = useAuth()
  const [tab, setTab] = useState<
    'network' | 'deposits' | 'commissions' | 'withdrawals'
  >('network')
  const [amount, setAmount] = useState('')
  const [address, setAddress] = useState('')
  const [busy, setBusy] = useState(false)
  const { data, error, isLoading, mutate } = useSWR<PartnerDashboard>(
    'partner-dashboard-v2',
    () => api.getPartnerDashboard(),
    { refreshInterval: 10000, revalidateOnFocus: true }
  )
  const { data: invite } = useSWR('partner-invite-code', () =>
    api.getInviteMe()
  )

  const teamDeposit = useMemo(
    () =>
      data?.network.reduce(
        (sum, row) => sum + Number(row.deposit_usdt || 0),
        0
      ) || 0,
    [data]
  )

  async function withdraw() {
    if (Number(amount) < 100 || address.trim().length < 20) {
      toast.error('最低提现100U，请填写有效TRC20地址')
      return
    }
    setBusy(true)
    try {
      await api.postPartnerWithdrawal(amount, address)
      setAmount('')
      setAddress('')
      await mutate()
      toast.success('提现申请已提交')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '提交失败')
    } finally {
      setBusy(false)
    }
  }

  async function nominate(candidateUserId: string) {
    setBusy(true)
    try {
      await api.postPartnerStudioRequest(candidateUserId)
      toast.success('工作室申请已提交总管理员审核')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '提交失败')
    } finally {
      setBusy(false)
    }
  }

  if (isLoading)
    return (
      <div className="px-4 py-24 text-center text-sm text-zinc-500">
        加载账单...
      </div>
    )
  if (error || !data)
    return (
      <div className="px-4 py-24 text-center text-sm text-red-300">
        合作伙伴账单暂不可用
      </div>
    )

  const canWithdraw = data.user.role !== 'retail'
  const tabs = [
    ['network', '伞下用户'],
    ['deposits', '充值账单'],
    ['commissions', '返佣账单'],
    ['withdrawals', '提现记录'],
  ] as const

  return (
    <div className="mx-auto max-w-6xl px-3 py-6 text-zinc-100 sm:px-5 sm:py-8">
      <Link
        to={ROUTES.profile}
        className="mb-5 inline-flex items-center gap-1 text-sm text-zinc-400 hover:text-[#d4ff33]"
      >
        <ArrowLeft className="h-4 w-4" /> 个人中心
      </Link>

      <div className="flex flex-wrap items-end justify-between gap-4 border-b border-zinc-800 pb-5">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-xl font-bold text-white">合作伙伴中心</h1>
            <span className="rounded border border-[#d4ff33]/35 bg-[#d4ff33]/10 px-2 py-0.5 text-xs font-bold text-[#d4ff33]">
              {getAccountBadgeName(Boolean(user?.is_admin), data.user.role)}
              {!user?.is_admin ? ` · ${data.user.role_rate_percent}%` : ''}
            </span>
          </div>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-zinc-500">
            <span className="font-mono">
              {invite?.invite_code || '--------'}
            </span>
            <button
              type="button"
              disabled={!invite?.invite_link}
              onClick={async () => {
                if (!invite?.invite_link) return
                await navigator.clipboard.writeText(invite.invite_link)
                toast.success('邀请链接已复制')
              }}
              className="inline-flex h-8 items-center justify-center gap-1.5 rounded border border-zinc-700 px-2.5 text-zinc-300 hover:border-[#d4ff33] hover:text-[#d4ff33]"
              title="复制邀请链接"
            >
              <Clipboard className="h-3.5 w-3.5" />
              <span>复制邀请链接</span>
            </button>
          </div>
        </div>
        <div className="text-right text-xs text-zinc-500">每10秒更新</div>
      </div>

      <section className="mt-5 grid grid-cols-2 overflow-hidden rounded-md border border-zinc-800 bg-zinc-950/50 lg:grid-cols-4">
        <Metric
          label="可提现返佣"
          value={money(data.user.available_balance_usdt)}
        />
        <Metric
          label="审核中提现"
          value={money(data.user.frozen_balance_usdt)}
        />
        <Metric label="伞下确认充值" value={money(String(teamDeposit))} />
        <Metric
          label="伞下用户"
          value={String(data.network.length)}
          suffix="人"
        />
      </section>

      {(data.user.role === 'retail' || data.user.qualified_at) && (
        <section className="mt-5 border-y border-zinc-800 py-4">
          <div className="flex items-center justify-between text-sm">
            <strong>IB 升级进度</strong>
            <span className="font-mono text-[#d4ff33]">
              {data.ib_progress.qualified_count}/
              {data.ib_progress.required_count}
            </span>
          </div>
          <div className="mt-3 grid grid-cols-5 gap-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <div
                key={i}
                className={`h-2 rounded-sm ${i < data.ib_progress.qualified_count ? 'bg-[#d4ff33]' : 'bg-zinc-800'}`}
              />
            ))}
          </div>
          <p className="mt-2 text-xs text-zinc-500">
            5名直邀用户分别确认充值100U；门槛500U单独标记且不补发IB返佣。
          </p>
        </section>
      )}

      {canWithdraw && (
        <section className="mt-5 flex flex-col gap-3 border-b border-zinc-800 pb-5 sm:flex-row sm:items-end">
          <label className="min-w-0 flex-1 text-xs text-zinc-500">
            TRC20 地址
            <input
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              className="mt-1 w-full rounded border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-white outline-none focus:border-[#d4ff33]"
            />
          </label>
          <label className="text-xs text-zinc-500">
            提现金额
            <input
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              inputMode="decimal"
              placeholder="最低100U"
              className="mt-1 w-full rounded border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-white outline-none focus:border-[#d4ff33] sm:w-40"
            />
          </label>
          <button
            type="button"
            disabled={busy}
            onClick={() => void withdraw()}
            className="inline-flex h-10 items-center justify-center gap-2 rounded bg-[#d4ff33] px-4 text-sm font-bold text-black disabled:opacity-50"
          >
            <WalletCards className="h-4 w-4" /> 申请提现
          </button>
        </section>
      )}

      <div className="mt-6 flex gap-1 overflow-x-auto border-b border-zinc-800">
        {tabs.map(([key, label]) => (
          <button
            key={key}
            type="button"
            onClick={() => setTab(key)}
            className={`whitespace-nowrap border-b-2 px-4 py-2.5 text-sm ${tab === key ? 'border-[#d4ff33] text-white' : 'border-transparent text-zinc-500 hover:text-zinc-300'}`}
          >
            {label}
          </button>
        ))}
      </div>

      <section className="overflow-x-auto border-b border-zinc-800">
        {tab === 'network' && (
          <table className="w-full min-w-[760px] text-left text-xs">
            <thead className="text-zinc-500">
              <tr>
                <th className="py-3">用户</th>
                <th>身份</th>
                <th>层级</th>
                <th>累计确认充值</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {data.network.map((row) => (
                <tr key={row.platform_user_id}>
                  <td className="py-3">
                    <span
                      style={{ paddingLeft: Math.min(row.depth - 1, 6) * 14 }}
                    >
                      {row.nickname || '未命名'}
                    </span>
                    <div className="font-mono text-[10px] text-zinc-600">
                      {row.platform_user_id}
                    </div>
                  </td>
                  <td>{roleName[row.role]}</td>
                  <td>{row.depth}</td>
                  <td>{money(row.deposit_usdt)} U</td>
                  <td>
                    {data.user.role === 'branch' &&
                    (row.role === 'retail' || row.role === 'ib') ? (
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => void nominate(row.platform_user_id)}
                        className="inline-flex items-center gap-1 rounded border border-zinc-700 px-2 py-1 text-zinc-300 hover:border-[#d4ff33]"
                      >
                        <Landmark className="h-3.5 w-3.5" /> 提报工作室
                      </button>
                    ) : (
                      '—'
                    )}
                  </td>
                </tr>
              ))}
              {!data.network.length && (
                <tr>
                  <td colSpan={5} className="py-10 text-center text-zinc-600">
                    暂无伞下用户
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
        {tab === 'deposits' && (
          <LedgerTable
            rows={data.deposits.map((r) => [
              r.nickname || r.user_uid,
              `${money(r.amount_usdt)} U`,
              r.event_type === 'reversal' ? '充值冲正' : '确认充值',
              new Date(r.occurred_at).toLocaleString(),
            ])}
          />
        )}
        {tab === 'commissions' && (
          <LedgerTable
            rows={data.commissions.map((r) => [
              r.source_nickname || r.source_platform_user_id,
              `${money(r.amount_usdt)} U`,
              `${r.rate_percent}% · ${roleName[r.role]}`,
              new Date(r.created_at).toLocaleString(),
            ])}
          />
        )}
        {tab === 'withdrawals' && (
          <LedgerTable
            rows={data.withdrawals.map((r) => [
              `#${r.id}`,
              `${money(r.amount_usdt)} U`,
              statusName[r.status] || r.status,
              new Date(r.requested_at).toLocaleString(),
            ])}
          />
        )}
      </section>
    </div>
  )
}

function LedgerTable({ rows }: { rows: string[][] }) {
  return (
    <table className="w-full min-w-[680px] text-left text-xs">
      <thead className="text-zinc-500">
        <tr>
          <th className="py-3">对象</th>
          <th>金额</th>
          <th>类型 / 比例</th>
          <th>时间</th>
        </tr>
      </thead>
      <tbody className="divide-y divide-zinc-800">
        {rows.map((row, i) => (
          <tr key={i}>
            {row.map((cell, j) => (
              <td key={j} className="py-3 pr-4">
                {j === 2 && cell.includes('确认') ? (
                  <span className="inline-flex items-center gap-1 text-[#d4ff33]">
                    <Check className="h-3.5 w-3.5" />
                    {cell}
                  </span>
                ) : (
                  cell
                )}
              </td>
            ))}
          </tr>
        ))}
        {!rows.length && (
          <tr>
            <td colSpan={4} className="py-10 text-center text-zinc-600">
              <Users className="mx-auto mb-2 h-5 w-5" />
              暂无记录
            </td>
          </tr>
        )}
      </tbody>
    </table>
  )
}
