import type { AdminPartnerDashboard, PartnerRole } from '../lib/api/walletAdmin'
import type { PartnerAdminTab } from './AdminPartnerLedgerTables'

const roleName: Record<PartnerRole, string> = {
  retail: '散户',
  ib: 'IB',
  studio: '工作室',
  branch: '分公司',
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

export function AdminPartnerHistoryTables({
  tab,
  data,
}: {
  tab: PartnerAdminTab
  data: AdminPartnerDashboard
}) {
  const tableClass = 'w-full min-w-[820px] border-collapse text-left text-xs'
  const headClass = 'border-b border-white/10 text-[#848E9C]'
  if (tab === 'deposits')
    return (
      <table className={tableClass}>
        <thead>
          <tr className={headClass}>
            <th className="px-3 py-3">用户</th>
            <th>金额</th>
            <th>类型</th>
            <th>来源 / 外部编号</th>
            <th>时间</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-white/5">
          {data.deposits.map((row) => (
            <tr key={row.id}>
              <td className="px-3 py-3">
                {row.nickname || row.platform_user_id}
              </td>
              <td className="text-[#d4ff33]">{money(row.amount_usdt)}</td>
              <td>{row.event_type === 'reversal' ? '充值冲正' : '确认充值'}</td>
              <td>
                {row.source} · {row.external_ref || '—'}
              </td>
              <td>{time(row.occurred_at)}</td>
            </tr>
          ))}
          {!data.deposits.length && <Empty colSpan={5} />}
        </tbody>
      </table>
    )
  if (tab === 'commissions')
    return (
      <table className={tableClass}>
        <thead>
          <tr className={headClass}>
            <th className="px-3 py-3">充值用户</th>
            <th>收佣用户</th>
            <th>金额</th>
            <th>比例 / 身份</th>
            <th>时间</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-white/5">
          {data.commissions.map((row) => (
            <tr key={row.id}>
              <td className="px-3 py-3">{row.source_platform_user_id}</td>
              <td>{row.recipient_platform_user_id}</td>
              <td className="text-[#d4ff33]">{money(row.amount_usdt)}</td>
              <td>
                {row.rate_percent}% · {roleName[row.recipient_role]}
              </td>
              <td>{time(row.created_at)}</td>
            </tr>
          ))}
          {!data.commissions.length && <Empty colSpan={5} />}
        </tbody>
      </table>
    )
  return (
    <table className={tableClass}>
      <thead>
        <tr className={headClass}>
          <th className="px-3 py-3">用户</th>
          <th>原身份</th>
          <th>新身份</th>
          <th>操作人 / 来源</th>
          <th>备注</th>
          <th>时间</th>
        </tr>
      </thead>
      <tbody className="divide-y divide-white/5">
        {data.role_audits.map((row) => (
          <tr key={row.id}>
            <td className="px-3 py-3">{row.platform_user_id}</td>
            <td>{roleName[row.old_role]}</td>
            <td>{roleName[row.new_role]}</td>
            <td>
              {row.actor} · {row.source}
            </td>
            <td>{row.note || '—'}</td>
            <td>{time(row.created_at)}</td>
          </tr>
        ))}
        {!data.role_audits.length && <Empty colSpan={6} />}
      </tbody>
    </table>
  )
}
