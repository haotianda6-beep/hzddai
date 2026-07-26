import { toast } from 'sonner'
import type { InviteMePayload, InviteNetworkPayload } from '../../lib/api/walletAdmin'

type Props = {
  inviteData?: InviteMePayload | null
  /** 优先：GET /api/invite/network（含伞下树接口返回的邀请码与链接） */
  network?: InviteNetworkPayload | null
  /** 接口尚未返回时的兜底（本地用户的 invite_code） */
  fallbackInviteCode?: string | null
}

/** 主站邀请奖励：邀请码、链接；若有 network 则统计来自伞下接口 */
export function InviteRewardsSection({ inviteData, network, fallbackInviteCode }: Props) {
  const code = network?.invite_code || inviteData?.invite_code || fallbackInviteCode || '—'
  const link = network?.invite_link || inviteData?.invite_link
  const invitedCount =
    network != null ? network.root.direct_invite_count : (inviteData?.invited_count ?? 0)

  return (
    <div className="rounded-xl border border-[#d4ff33]/20 bg-[#d4ff33]/5 p-5">
      <h3 className="text-sm font-bold text-white">我的邀请奖励</h3>
      <p className="mt-1 text-xs text-zinc-400">
        好友通过你的专属链接注册后会出现在下方列表；奖励结算按运营规则执行。
      </p>
      <div className="mt-4 grid gap-3 sm:grid-cols-[1fr_auto]">
        <div className="rounded-lg border border-zinc-800 bg-black/30 px-3 py-2">
          <div className="text-[11px] text-zinc-500">邀请码</div>
          <div className="break-all font-mono text-lg font-bold text-[#d4ff33]">{code}</div>
        </div>
        <button
          type="button"
          onClick={async () => {
            if (!link) return
            await navigator.clipboard.writeText(link)
            toast.success('邀请链接已复制')
          }}
          disabled={!link}
          className="rounded-lg bg-[#d4ff33] px-4 py-2 text-sm font-bold text-black disabled:cursor-not-allowed disabled:opacity-50"
        >
          复制邀请链接
        </button>
      </div>
      <div className="mt-4 text-sm text-zinc-300">
        直推已注册 <span className="font-bold text-[#d4ff33]">{invitedCount}</span> 人
        {network && (
          <span className="block text-zinc-500 sm:ml-2 sm:inline">
            （伞下共 {network.stats.total_descendants} 人，详见下方关系树）
          </span>
        )}
      </div>
      {network == null && (
        <ul className="mt-3 max-h-64 space-y-2 overflow-y-auto">
          {(inviteData?.invited_users ?? []).length === 0 ? (
            <li className="text-xs text-zinc-500">暂无邀请用户</li>
          ) : (
            inviteData!.invited_users.map((u) => (
              <li key={u.id} className="rounded-lg border border-zinc-800 bg-black/20 px-3 py-2 text-xs">
                <div className="font-medium text-white">{u.display_name || '未命名用户'}</div>
                <div className="break-all text-zinc-500">
                  {u.email} · {new Date(u.created_at).toLocaleString()}
                </div>
              </li>
            ))
          )}
        </ul>
      )}
      {network != null && (
        <p className="mt-3 text-xs text-zinc-500">
          每位下级的充值金额、VIP 等级、再下级人数已在下方「伞下关系树」中分级展开查看。
        </p>
      )}
    </div>
  )
}
