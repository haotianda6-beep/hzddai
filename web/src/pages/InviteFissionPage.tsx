import { Link } from 'react-router-dom'
import useSWR from 'swr'
import { ChevronDown, ChevronLeft, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useAuth } from '../contexts/AuthContext'
import { api } from '../lib/api'
import type { InviteNetworkNode } from '../lib/api/walletAdmin'
import { ROUTES } from '../router/paths'
import { InviteRewardsSection } from '../components/profile/InviteRewardsSection'
import {
  REBATE_BASE_DIRECT_PERCENT,
  REBATE_STUDIO_EXTRA_PERCENT,
  REBATE_VIP_EXTRA_PERCENT,
  rebateVipExtraPercent,
  rebateVipTierTotalPercent,
  rebateVipTierTotalPercentWithStudio,
} from '../lib/rebateVipRates'

function fmtU(n: number) {
  if (typeof n !== 'number' || Number.isNaN(n)) return '0'
  return n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function fmtStrU(s: string | undefined) {
  const n = Number(s)
  return fmtU(Number.isFinite(n) ? n : 0)
}

function TreeRow({ node, depth }: { node: InviteNetworkNode; depth: number }) {
  const [open, setOpen] = useState(depth < 2)
  const hasKids = node.children?.length > 0
  const vipLabel =
    node.rebate_synced && typeof node.rebate_vip_level === 'number'
      ? `VIP${node.rebate_vip_level}`
      : node.rebate_synced
        ? 'VIP?'
        : '返利未同步'

  const studioTag =
    node.rebate_synced && node.rebate_is_studio === true ? (
      <span className="rounded border border-amber-400/40 bg-amber-950/40 px-1.5 py-0.5 text-[10px] font-bold text-amber-100">
        工作室
      </span>
    ) : null

  return (
    <div className="border-b border-zinc-800/80">
      <button
        type="button"
        onClick={() => hasKids && setOpen(!open)}
        className={`flex w-full items-start gap-2 py-2 text-left text-xs ${
          hasKids ? 'cursor-pointer hover:bg-white/5' : 'cursor-default'
        }`}
        style={{ paddingLeft: 8 + depth * 14 }}
      >
        <span className="mt-0.5 shrink-0 text-zinc-500">
          {hasKids ? (
            open ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )
          ) : (
            <span className="inline-block w-4" />
          )}
        </span>
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
            <span className="font-medium text-white">{node.display_name || '未命名'}</span>
            <span className="rounded bg-zinc-800 px-1.5 py-0.5 font-mono text-[10px] text-[#d4ff33]">{vipLabel}</span>
            {studioTag}
          </div>
          <div className="break-all text-[11px] text-zinc-500">{node.email}</div>
          <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-[11px] text-zinc-400 sm:grid-cols-4">
            <span>
              本人消费 <b className="text-zinc-200">{fmtU(node.personal_consumption_usdt)}</b> U
            </span>
            <span>
              团队消费 <b className="text-zinc-200">{fmtU(node.team_consumption_usdt)}</b> U
            </span>
            <span>
              直推人数 <b className="text-zinc-200">{node.direct_invite_count}</b>
            </span>
            {node.rebate_synced && node.rebate_team_total != null ? (
              <span title="返利侧统计的团队累计可返佣消费业绩">
                返利团队业绩 <b className="text-zinc-200">{node.rebate_team_total}</b> U
              </span>
            ) : (
              <span className="text-zinc-600">返利侧未同步则无等级数据</span>
            )}
          </div>
          {node.rebate_synced ? (
            <div className="text-[10px] text-zinc-500">
              返利可提现 <b className="text-zinc-300">{fmtStrU(node.rebate_balance_usdt)}</b> U
              <span className="text-zinc-600"> · </span>
              返利充值记账 <b className="text-zinc-300">{fmtStrU(node.rebate_recharge_balance_usdt)}</b> U
            </div>
          ) : null}
        </div>
      </button>
      {hasKids && open ? (
        <div>
          {node.children.map((ch) => (
            <TreeRow key={ch.id} node={ch} depth={depth + 1} />
          ))}
        </div>
      ) : null}
    </div>
  )
}

/** 主站邀请奖励专页：复制链接 + 伞下整树（可返佣消费、VIP 等级、直推人数） */
export function InviteFissionPage() {
  const { user } = useAuth()
  const { data: network, error: netErr, isLoading } = useSWR('invite-network', () => api.getInviteNetwork(), {
    refreshInterval: 30000,
    revalidateOnFocus: true,
  })

  const { data: inviteFallback } = useSWR(
    netErr ? 'profile-invite-me-fallback' : null,
    () => api.getInviteMe(),
    { revalidateOnFocus: true }
  )

  const inviteData = inviteFallback

  return (
    <div className="mx-auto max-w-3xl px-3 py-6 text-zinc-100 sm:px-4 sm:py-8">
      <Link
        to={ROUTES.profile}
        className="mb-6 inline-flex items-center gap-1 text-sm text-zinc-400 transition-colors hover:text-[#d4ff33]"
      >
        <ChevronLeft className="h-4 w-4" />
        个人中心
      </Link>

      <h1 className="mb-1 text-xl font-bold text-white">邀请奖励</h1>
      <p className="mb-6 text-sm text-zinc-500">
        复制专属链接发展下级；下方展示伞下全部层级关系、每人可返佣消费与（已同步返利时的）VIP 等级。
      </p>

      <InviteRewardsSection
        inviteData={inviteData}
        network={network}
        fallbackInviteCode={user?.invite_code}
      />

      <div className="mt-8 rounded-xl border border-[#d4ff33]/20 bg-[#d4ff33]/5 p-5">
        <h3 className="text-sm font-bold text-white">伞下关系树</h3>
        <p className="mt-1 text-xs text-zinc-500">
          「本人消费」= max(主站钱包可返佣消费流水合计, 返利侧累计消费业绩)。40U 周卡体验不计入消费返佣与邀请树消费统计。「团队消费」为伞下树内「本人消费」之和。「返利团队业绩 / VIP」依赖返利子系统。
        </p>

        {isLoading && <p className="mt-4 text-xs text-zinc-500">加载中…</p>}
        {netErr && (
          <p className="mt-4 text-xs text-amber-400/90">
            关系树接口暂不可用（{String(netErr)}），仅显示上方邀请码区域。
          </p>
        )}
        {network && (
          <>
            <div className="mt-4 rounded-lg border border-zinc-800 bg-black/30 p-3 text-xs text-zinc-300">
              <div className="font-medium text-white">我的概览</div>
              <div className="mt-2 grid gap-2 sm:grid-cols-2">
                <div>
                  直推人数：<b className="text-[#d4ff33]">{network.root.direct_invite_count}</b>
                </div>
                <div>
                  伞下总人数：<b className="text-[#d4ff33]">{network.stats.total_descendants}</b>（最大深度{' '}
                  {network.stats.max_depth}）
                </div>
                <div>
                  本人累计消费：<b>{fmtU(network.root.personal_consumption_usdt)}</b> U
                </div>
                <div>
                  团队累计消费（树内合计）：<b>{fmtU(network.root.team_consumption_usdt)}</b> U
                </div>
                {network.root.rebate_synced ? (
                  <>
                    <div>
                      返利可提现余额：<b>{fmtStrU(network.root.rebate_balance_usdt)}</b> U
                    </div>
                    <div>
                      返利侧充值记账：<b>{fmtStrU(network.root.rebate_recharge_balance_usdt)}</b> U
                    </div>
                  </>
                ) : null}
                <div className="sm:col-span-2 space-y-1.5">
                  <div>
                    VIP等级：
                    {network.root.rebate_synced && typeof network.root.rebate_vip_level === 'number' ? (
                      <>
                        <b className="text-[#d4ff33]">VIP{network.root.rebate_vip_level}</b>
                        {network.root.rebate_vip_level >= 1 && network.root.rebate_vip_level <= 5 ? (
                          <span className="text-zinc-400">
                            {' '}
                            · 等级额外奖励{' '}
                            <b className="text-[#d4ff33]">{rebateVipExtraPercent(network.root.rebate_vip_level)}%</b>
                            <span className="text-zinc-500">（在基础 {REBATE_BASE_DIRECT_PERCENT}% 之上）</span>
                            {' · '}
                            返利直推合计约{' '}
                            <b className="text-[#d4ff33]">{rebateVipTierTotalPercent(network.root.rebate_vip_level)}%</b>
                            {network.root.rebate_is_studio === true ? (
                              <span className="text-zinc-400">
                                {' '}
                                · 工作室再 +{REBATE_STUDIO_EXTRA_PERCENT}% →{' '}
                                <b className="text-amber-200">
                                  {rebateVipTierTotalPercentWithStudio(network.root.rebate_vip_level)}%
                                </b>
                              </span>
                            ) : null}
                          </span>
                        ) : network.root.rebate_vip_level === 0 ? (
                          <span className="text-zinc-500">
                            {' '}
                            · 仅基础直推 {REBATE_BASE_DIRECT_PERCENT}%（未达 VIP1 团队门槛则无等级额外）
                            {network.root.rebate_is_studio === true ? (
                              <span className="text-zinc-400">
                                {' '}
                                · 工作室再 +{REBATE_STUDIO_EXTRA_PERCENT}% →{' '}
                                <b className="text-amber-200">
                                  {REBATE_BASE_DIRECT_PERCENT + REBATE_STUDIO_EXTRA_PERCENT}%
                                </b>
                              </span>
                            ) : null}
                          </span>
                        ) : (
                          <span className="text-zinc-500"> · 等级额外以返利系统为准</span>
                        )}
                      </>
                    ) : (
                      <span className="text-zinc-500">未同步或不可用</span>
                    )}
                  </div>
                  <div className="rounded-md border border-[#d4ff33]/15 bg-black/20 px-2 py-2 text-[11px] leading-relaxed text-zinc-400">
                    <div className="mb-1 font-medium text-zinc-300">等级额外奖励比例（与返利后台一致）</div>
                    <div className="overflow-x-auto">
                      <div className="grid min-w-[420px] grid-cols-[auto_1fr_1fr] gap-x-3 gap-y-1 font-mono text-[10px] sm:text-[11px]">
                        <span className="text-zinc-500">等级</span>
                        <span className="text-zinc-500">额外（叠在基础 {REBATE_BASE_DIRECT_PERCENT}% 上）</span>
                        <span className="text-zinc-500">直推合计</span>
                        {([1, 2, 3, 4, 5] as const).map((lv) => {
                          const ex = REBATE_VIP_EXTRA_PERCENT[lv]
                          return (
                            <span key={lv} className="contents">
                              <span className="text-zinc-300">VIP{lv}</span>
                              <span className="text-[#d4ff33]">{ex}%</span>
                              <span>{REBATE_BASE_DIRECT_PERCENT + ex}%</span>
                            </span>
                          )
                        })}
                      </div>
                    </div>
                    <p className="mt-2 text-zinc-500">
                      直推基础为 {REBATE_BASE_DIRECT_PERCENT}%（VIP0）；VIP1～5 执行 <b className="text-zinc-400">15% + 上表额外</b>
                      ；工作室身份直推名义再 +{REBATE_STUDIO_EXTRA_PERCENT}% ；VIP5 同级周分红池为小区周合计的 5%
                      （与返利子系统一致）。
                    </p>
                  </div>
                </div>
              </div>
            </div>

            <div className="mt-4 max-h-[70vh] overflow-auto rounded-lg border border-zinc-800 bg-black/20">
              {network.tree.length === 0 ? (
                <p className="p-4 text-xs text-zinc-500">暂无伞下用户，邀请好友注册后会出现在这里。</p>
              ) : (
                network.tree.map((n) => <TreeRow key={n.id} node={n} depth={0} />)
              )}
            </div>
          </>
        )}
      </div>

      <p className="mt-6 text-center text-[11px] text-zinc-600">
        邀请返佣余额、划转等仍在「个人中心 → 个人资料」页查看。
      </p>
    </div>
  )
}
