import { Check, X } from 'lucide-react'
import { useState } from 'react'
import type { AdminPartnerDashboard, PartnerRole } from '../lib/api/walletAdmin'
import { AdminPartnerHistoryTables } from './AdminPartnerHistoryTables'

export type PartnerAdminTab =
  | 'users'
  | 'deposits'
  | 'commissions'
  | 'withdrawals'
  | 'studios'
  | 'audits'

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

const money = (value: string) => `${Number(value || 0).toFixed(2)} U`
const time = (value: string) => new Date(value).toLocaleString()

function Empty({ colSpan }: { colSpan: number }) {
  return (
    <tr>
      <td colSpan={colSpan} className="px-3 py-10 text-center text-[#5e6673]">
        暂无记录
      </td>
    </tr>
  )
}

export function AdminPartnerLedgerTables({
  tab,
  data,
  busy,
  onRole,
  onReview,
}: {
  tab: PartnerAdminTab
  data: AdminPartnerDashboard
  busy: string | null
  onRole: (userId: string, role: PartnerRole) => void
  onReview: (
    kind: 'studio-requests' | 'withdrawals',
    id: number,
    action: 'approve' | 'reject' | 'paid'
  ) => void
}) {
  const [roleDraft, setRoleDraft] = useState<Record<string, PartnerRole>>({})
  const tableClass = 'w-full min-w-[820px] border-collapse text-left text-xs'
  const headClass = 'border-b border-white/10 text-[#848E9C]'
  const cellClass = 'px-3 py-3'

  if (tab === 'users') {
    return (
      <table className={tableClass}>
        <thead>
          <tr className={headClass}>
            <th className={cellClass}>用户</th>
            <th>上级</th>
            <th>身份</th>
            <th>确认充值</th>
            <th>可提现 / 冻结</th>
            <th>调整身份</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-white/5">
          {data.users.map((row) => {
            const draft = roleDraft[row.platform_user_id] || row.role
            return (
              <tr key={row.platform_user_id}>
                <td className={cellClass}>
                  <strong className="text-white">
                    {row.nickname || '未命名'}
                  </strong>
                  <div className="font-mono text-[10px] text-[#5e6673]">
                    {row.platform_user_id}
                  </div>
                </td>
                <td>{row.parent_platform_user_id || '—'}</td>
                <td>{roleName[row.role]}</td>
                <td>{money(row.deposit_usdt)}</td>
                <td>
                  {money(row.available_usdt)} / {money(row.frozen_usdt)}
                </td>
                <td>
                  <div className="flex items-center gap-2">
                    <select
                      value={draft}
                      onChange={(event) =>
                        setRoleDraft((old) => ({
                          ...old,
                          [row.platform_user_id]: event.target
                            .value as PartnerRole,
                        }))
                      }
                      className="rounded border border-white/15 bg-black/30 px-2 py-1.5 text-white"
                    >
                      {Object.entries(roleName).map(([value, label]) => (
                        <option key={value} value={value}>
                          {label}
                        </option>
                      ))}
                    </select>
                    <button
                      type="button"
                      disabled={
                        busy === `role-${row.platform_user_id}` ||
                        draft === row.role
                      }
                      onClick={() => onRole(row.platform_user_id, draft)}
                      className="rounded bg-[#d4ff33] px-2.5 py-1.5 font-bold text-black disabled:opacity-40"
                    >
                      保存
                    </button>
                  </div>
                </td>
              </tr>
            )
          })}
          {!data.users.length && <Empty colSpan={6} />}
        </tbody>
      </table>
    )
  }

  if (tab === 'deposits' || tab === 'commissions' || tab === 'audits') {
    return <AdminPartnerHistoryTables tab={tab} data={data} />
  }

  if (tab === 'withdrawals') {
    return (
      <table className={tableClass}>
        <thead>
          <tr className={headClass}>
            <th className={cellClass}>用户</th>
            <th>金额</th>
            <th>地址</th>
            <th>状态</th>
            <th>时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-white/5">
          {data.withdrawals.map((row) => (
            <tr key={row.id}>
              <td className={cellClass}>
                {row.nickname || row.platform_user_id}
              </td>
              <td>{money(row.amount_usdt)}</td>
              <td className="max-w-52 truncate font-mono" title={row.address}>
                {row.address}
              </td>
              <td>{statusName[row.status] || row.status}</td>
              <td>{time(row.requested_at)}</td>
              <td>
                <div className="flex gap-1">
                  {row.status === 'pending' && (
                    <ReviewButtons
                      busy={busy === `withdrawals-${row.id}`}
                      onApprove={() =>
                        onReview('withdrawals', row.id, 'approve')
                      }
                      onReject={() => onReview('withdrawals', row.id, 'reject')}
                    />
                  )}
                  {row.status === 'approved' && (
                    <>
                      <button
                        type="button"
                        disabled={Boolean(busy)}
                        onClick={() => onReview('withdrawals', row.id, 'paid')}
                        className="rounded border border-[#d4ff33]/45 px-2 py-1 text-[#d4ff33]"
                      >
                        标记已付款
                      </button>
                      <button
                        type="button"
                        disabled={Boolean(busy)}
                        onClick={() =>
                          onReview('withdrawals', row.id, 'reject')
                        }
                        className="rounded border border-red-500/40 px-2 py-1 text-red-300"
                      >
                        驳回
                      </button>
                    </>
                  )}
                </div>
              </td>
            </tr>
          ))}
          {!data.withdrawals.length && <Empty colSpan={6} />}
        </tbody>
      </table>
    )
  }

  if (tab === 'studios') {
    return (
      <table className={tableClass}>
        <thead>
          <tr className={headClass}>
            <th className={cellClass}>候选用户</th>
            <th>提报分公司</th>
            <th>申请说明</th>
            <th>状态</th>
            <th>时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-white/5">
          {data.studio_requests.map((row) => (
            <tr key={row.id}>
              <td className={cellClass}>
                {row.candidate_nickname || row.candidate_platform_user_id}
              </td>
              <td>{row.branch_platform_user_id}</td>
              <td>{row.request_note || '—'}</td>
              <td>{statusName[row.status] || row.status}</td>
              <td>{time(row.created_at)}</td>
              <td>
                {row.status === 'pending' && (
                  <ReviewButtons
                    busy={busy === `studio-requests-${row.id}`}
                    onApprove={() =>
                      onReview('studio-requests', row.id, 'approve')
                    }
                    onReject={() =>
                      onReview('studio-requests', row.id, 'reject')
                    }
                  />
                )}
              </td>
            </tr>
          ))}
          {!data.studio_requests.length && <Empty colSpan={6} />}
        </tbody>
      </table>
    )
  }

  return null
}

function ReviewButtons({
  busy,
  onApprove,
  onReject,
}: {
  busy: boolean
  onApprove: () => void
  onReject: () => void
}) {
  return (
    <div className="flex gap-1">
      <button
        type="button"
        disabled={busy}
        onClick={onApprove}
        className="inline-flex items-center gap-1 rounded border border-emerald-500/40 px-2 py-1 text-emerald-300"
      >
        <Check className="h-3 w-3" />
        通过
      </button>
      <button
        type="button"
        disabled={busy}
        onClick={onReject}
        className="inline-flex items-center gap-1 rounded border border-red-500/40 px-2 py-1 text-red-300"
      >
        <X className="h-3 w-3" />
        驳回
      </button>
    </div>
  )
}
