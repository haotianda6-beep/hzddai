import { Network, Search } from 'lucide-react'
import { useState } from 'react'
import useSWR from 'swr'
import {
  AdminModuleHeader,
  AdminModuleShell,
} from '../components/admin/AdminModuleHeader'
import { api } from '../lib/api'

export function AdminTeamPage() {
  const [branchId, setBranchId] = useState('')
  const [query, setQuery] = useState('')
  const [role, setRole] = useState('')
  const [page, setPage] = useState(1)
  const key = ['admin-team-details', branchId, query, role, page]
  const { data, error, isLoading, mutate } = useSWR(
    key,
    () =>
      api.getAdminTeamDetails({ branchId, query, role, page, pageSize: 20 }),
    { keepPreviousData: true }
  )
  const totalPages = Math.max(1, Math.ceil((data?.members.total ?? 0) / 20))
  return (
    <AdminModuleShell>
      <AdminModuleHeader
        icon={Network}
        title="团队详情"
        description="分公司与伞下成员的管理视图；仅显示必要标识，不展示邮箱或交易凭据"
        onRefresh={() => void mutate()}
      />
      {error && (
        <p className="mt-5 text-sm text-red-300">
          团队详情加载失败，请检查返佣服务连接
        </p>
      )}
      <section className="mt-5 grid gap-3 sm:grid-cols-3">
        {(data?.branches ?? []).map((branch) => (
          <button
            key={branch.id}
            type="button"
            onClick={() => {
              setBranchId(branch.id)
              setPage(1)
            }}
            className={`rounded-md border p-4 text-left ${branchId === branch.id ? 'border-[#d4ff33]/70 bg-white/5' : 'border-white/10 bg-nofx-bg-secondary'}`}
          >
            <div className="font-semibold text-white">
              {branch.name || '未命名分公司'}
            </div>
            <div className="mt-1 font-mono text-[10px] text-[#5e6673]">
              {branch.id}
            </div>
            <div className="mt-2 text-xs text-[#848E9C]">
              状态 {branch.status} · 归属 {branch.parentId || '顶层'}
            </div>
            <div className="mt-3 text-xs text-[#848E9C]">
              直属 {branch.directUserCount} 人 · 伞下 {branch.umbrellaUserCount}{' '}
              人
            </div>
          </button>
        ))}
      </section>
      <section className="mt-5 flex flex-wrap gap-2">
        <label className="flex min-w-[220px] flex-1 items-center gap-2 rounded border border-white/10 bg-nofx-bg-secondary px-3 py-2">
          <Search className="h-4 w-4 text-[#848E9C]" />
          <input
            value={query}
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
            placeholder="搜索稳定 ID 或管理名称"
            className="w-full bg-transparent text-sm outline-none"
          />
        </label>
        <select
          value={role}
          onChange={(event) => {
            setRole(event.target.value)
            setPage(1)
          }}
          className="rounded border border-white/10 bg-nofx-bg-secondary px-3 py-2 text-sm"
        >
          <option value="">全部层级</option>
          <option value="studio">工作室</option>
          <option value="retail">用户</option>
          <option value="branch">分公司</option>
        </select>
        <button
          type="button"
          onClick={() => {
            setBranchId('')
            setRole('')
            setQuery('')
            setPage(1)
          }}
          className="rounded border border-white/10 px-3 py-2 text-sm text-[#848E9C]"
        >
          清除筛选
        </button>
      </section>
      {isLoading && !data && (
        <p className="py-16 text-center text-sm text-[#848E9C]">加载团队...</p>
      )}
      {data && (
        <section className="mt-5 overflow-x-auto border-y border-white/10">
          <table className="w-full min-w-[900px] text-left text-xs">
            <thead className="text-[#848E9C]">
              <tr className="border-b border-white/10">
                <th className="px-3 py-3">成员</th>
                <th>层级</th>
                <th>归属分公司</th>
                <th>上级 ID</th>
                <th>状态</th>
                <th>注册/激活</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {data.members.items.map((member) => (
                <tr key={member.id}>
                  <td className="px-3 py-3">
                    <strong className="text-white">
                      {member.name || '未命名'}
                    </strong>
                    <div className="font-mono text-[10px] text-[#5e6673]">
                      {member.id}
                    </div>
                  </td>
                  <td>
                    {member.role} · L{member.level}
                  </td>
                  <td className="font-mono text-[10px]">
                    {member.branchId || '—'}
                  </td>
                  <td className="font-mono text-[10px]">
                    {member.parentId || '—'}
                  </td>
                  <td>{member.status}</td>
                  <td>{member.activated ? '已激活' : '未激活'}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="flex items-center justify-between px-3 py-3 text-xs text-[#848E9C]">
            <span>共 {data.members.total} 人</span>
            <div className="flex gap-2">
              <button
                type="button"
                disabled={page <= 1}
                onClick={() => setPage((value) => value - 1)}
                className="rounded border border-white/10 px-3 py-1 disabled:opacity-40"
              >
                上一页
              </button>
              <span className="px-2 py-1">
                {page}/{totalPages}
              </span>
              <button
                type="button"
                disabled={page >= totalPages}
                onClick={() => setPage((value) => value + 1)}
                className="rounded border border-white/10 px-3 py-1 disabled:opacity-40"
              >
                下一页
              </button>
            </div>
          </div>
        </section>
      )}
    </AdminModuleShell>
  )
}
