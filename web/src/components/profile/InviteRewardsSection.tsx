import { Clipboard } from 'lucide-react'
import { toast } from 'sonner'
import type { InviteMePayload } from '../../lib/api/walletAdmin'

type Props = {
  inviteData?: InviteMePayload | null
  fallbackInviteCode?: string | null
}

export function InviteRewardsSection({
  inviteData,
  fallbackInviteCode,
}: Props) {
  const code = inviteData?.invite_code || fallbackInviteCode || '—'
  return (
    <section className="border-y border-zinc-800 py-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <div className="text-xs text-zinc-500">全新邀请码</div>
          <div className="mt-1 font-mono text-xl font-bold text-[#d4ff33]">
            {code}
          </div>
          <div className="mt-1 text-xs text-zinc-500">
            直邀用户 {inviteData?.invited_count ?? 0} 人
          </div>
        </div>
        <button
          type="button"
          disabled={!inviteData?.invite_link}
          onClick={async () => {
            if (!inviteData?.invite_link) return
            await navigator.clipboard.writeText(inviteData.invite_link)
            toast.success('邀请链接已复制')
          }}
          className="inline-flex items-center gap-2 rounded bg-[#d4ff33] px-4 py-2 text-sm font-bold text-black disabled:opacity-50"
        >
          <Clipboard className="h-4 w-4" /> 复制邀请链接
        </button>
      </div>
    </section>
  )
}
