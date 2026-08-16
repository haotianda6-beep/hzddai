import { useState } from 'react'
import useSWR from 'swr'
import { toast } from 'sonner'
import { Link } from 'react-router-dom'
import { Copy, Mail, Wallet, Headphones, MessageCircle } from 'lucide-react'
import { api } from '../lib/api'
import { openLiveChatPanel } from '../lib/liveChatOpen'
import { ROUTES } from '../router/paths'

/** 客服联系邮箱：构建时可设 VITE_COMKUN_SUPPORT_EMAIL */
const SUPPORT_EMAIL =
  (import.meta.env.VITE_COMKUN_SUPPORT_EMAIL as string | undefined)?.trim() || 'haotianda6@gmail.com'

export function RechargePage() {
  const { data: wallet } = useSWR('platform-wallet-recharge', () => api.getWallet(), {
    refreshInterval: 20000,
  })
  const [copied, setCopied] = useState(false)
  const balance = wallet?.balance_usdt ?? 0
  const debt = balance < 0 ? -balance : 0

  const copyEmail = async () => {
    try {
      await navigator.clipboard.writeText(SUPPORT_EMAIL)
      setCopied(true)
      toast.success('已复制客服邮箱')
      setTimeout(() => setCopied(false), 2000)
    } catch {
      toast.error('复制失败，请手动复制')
    }
  }

  return (
    <div className="mx-auto max-w-2xl px-3 pb-20 pt-6 sm:px-6 sm:pt-12">
      <div className="mb-8">
        <Link
          to={ROUTES.strategyMarket}
          className="text-sm text-[#848E9C] transition-colors hover:text-[#d4ff33]"
        >
          ← 返回策略市场
        </Link>
        <h1 className="mt-4 font-['Space_Grotesk',sans-serif] text-2xl font-bold tracking-tight text-[#EAECEF] sm:text-3xl">
          账户充值
        </h1>
        <p className="mt-2 text-sm text-[#848E9C]">站内余额用于购买策略市场中的订阅类策略，由人工客服确认到账。</p>
      </div>

      <div className="mb-8 flex flex-col items-start gap-4 rounded-2xl border border-[#2b3139] bg-nofx-bg-secondary p-5 sm:flex-row sm:items-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-[#d4ff33]/15">
          <Wallet className="h-6 w-6 text-[#d4ff33]" aria-hidden />
        </div>
        <div>
          <p className="text-xs font-medium uppercase tracking-wider text-[#848E9C]">当前站内余额</p>
          <p
            className={`mt-1 font-['Space_Grotesk',sans-serif] text-2xl font-bold tabular-nums ${
              balance < 0 ? 'text-red-400' : 'text-[#d4ff33]'
            }`}
          >
            {balance.toFixed(4)} <span className="text-base font-semibold text-[#b7bdc6]">USDT</span>
          </p>
          {debt > 0 && (
            <p className="mt-1 text-xs font-semibold text-red-300">
              当前欠费 {debt.toFixed(4)} USDT，请充值补足到余额不低于 0。
            </p>
          )}
        </div>
      </div>

      <div className="space-y-6 rounded-2xl border border-[#2b3139] bg-nofx-bg-secondary p-6">
        <div className="flex gap-3">
          <Headphones className="mt-0.5 h-5 w-5 shrink-0 text-[#d4ff33]" aria-hidden />
          <div>
            <h2 className="text-lg font-bold text-[#EAECEF]">人工充值</h2>
            <p className="mt-2 text-sm leading-relaxed text-[#b7bdc6]">
              请先<strong className="text-[#d4ff33]">联系在线客服</strong>（下方按钮会直接打开对话窗口），说明您的
              <strong className="text-[#EAECEF]">注册邮箱</strong>与<strong className="text-[#EAECEF]">充值金额</strong>
              。客服核对收款后会为您在系统内入账，您将收到
              <strong className="text-[#d4ff33]">「充值成功」</strong>类站内通知（铃铛图标内「系统通知」区域）。
            </p>
          </div>
        </div>

        <button
          type="button"
          onClick={() => openLiveChatPanel()}
          className="flex w-full items-center justify-center gap-2 rounded-xl bg-[#d4ff33] px-4 py-3.5 text-sm font-bold text-black shadow-[0_0_24px_rgba(212,255,51,0.22)] transition-opacity hover:opacity-95"
        >
          <MessageCircle className="h-5 w-5 shrink-0" aria-hidden />
          联系在线客服（打开 Tawk 对话）
        </button>

        <div className="flex flex-col gap-3 sm:flex-row">
          <button
            type="button"
            onClick={() => void copyEmail()}
            className="inline-flex flex-1 items-center justify-center gap-2 rounded-xl border border-[#2b3139] bg-nofx-bg-secondary px-4 py-3 text-sm font-semibold text-[#EAECEF] transition-colors hover:border-[#d4ff33]/40 hover:bg-nofx-bg-tertiary"
          >
            <Copy className="h-4 w-4 text-[#d4ff33]" />
            {copied ? '已复制' : '复制客服邮箱'}
          </button>
          <a
            href={`mailto:${SUPPORT_EMAIL}?subject=${encodeURIComponent('COMKUN-AI 站内余额人工充值')}&body=${encodeURIComponent('我的注册邮箱：\n计划充值金额（USDT）：\n付款方式/流水号：\n')}`}
            className="inline-flex flex-1 items-center justify-center gap-2 rounded-xl border border-[#d4ff33]/35 bg-transparent px-4 py-3 text-sm font-semibold text-[#d4ff33] transition-colors hover:bg-[#d4ff33]/10"
          >
            <Mail className="h-4 w-4 shrink-0" />
            发邮件联系客服
          </a>
        </div>

        <p className="break-all rounded-xl border border-dashed border-[#46484d]/50 bg-black/20 px-4 py-3 text-center font-mono text-sm text-[#d4ff33]">
          {SUPPORT_EMAIL}
        </p>
      </div>
    </div>
  )
}
