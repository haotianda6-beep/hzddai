import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import useSWR from 'swr'
import { toast } from 'sonner'
import {
  ArrowRightLeft,
  Banknote,
  Headphones,
  Landmark,
  MessageCircle,
  Wallet,
} from 'lucide-react'
import { useAuth } from '../contexts/AuthContext'
import { api } from '../lib/api'
import { openLiveChatPanel } from '../lib/liveChatOpen'
import { ROUTES } from '../router/paths'

const SUPPORT_EMAIL =
  (import.meta.env.VITE_COMKUN_SUPPORT_EMAIL as string | undefined)?.trim() || 'haotianda6@gmail.com'

function fmt(value: number) {
  return value.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  })
}

function readFinanceTotal(email?: string, walletBalance = 0) {
  if (!email) return 0
  try {
    const raw = localStorage.getItem(`auto_arbitrage_${email}`)
    if (!raw) return 0
    const saved = JSON.parse(raw) as { financeTotal?: number }
    const value = Number(saved.financeTotal ?? 0)
    if (!Number.isFinite(value)) return 0
    return Math.min(Math.max(value, 0), Math.max(walletBalance, 0))
  } catch {
    return 0
  }
}

export function WalletPage() {
  const { user } = useAuth()
  const { data: wallet } = useSWR(user ? 'platform-wallet-page' : null, () => api.getWallet(), {
    refreshInterval: 20_000,
    revalidateOnFocus: true,
  })
  const [financeTotal, setFinanceTotal] = useState(0)
  const [withdrawAmount, setWithdrawAmount] = useState('')
  const [withdrawAddress, setWithdrawAddress] = useState('')
  const total = wallet?.balance_usdt ?? user?.balance_usdt ?? 0

  useEffect(() => {
    setFinanceTotal(readFinanceTotal(user?.email, total))
  }, [total, user?.email])

  const mainAccount = Math.max(total - financeTotal, 0)
  const debt = total < 0 ? -total : 0
  const financePct = total > 0 ? Math.min((financeTotal / total) * 100, 100) : 0
  const rows = useMemo(
    () => [
      {
        title: '主账户',
        amount: mainAccount,
        icon: Wallet,
        hint: '充值入账、平台消费、提现申请',
        tone: 'text-[#d4ff33]',
      },
      {
        title: '理财账户',
        amount: financeTotal,
        icon: Landmark,
        hint: '套利项目资金',
        tone: 'text-emerald-300',
      },
    ],
    [financeTotal, mainAccount]
  )

  const openWithdrawChat = () => {
    const amount = Number(withdrawAmount)
    if (!Number.isFinite(amount) || amount <= 0) {
      toast.error('请输入提现金额')
      return
    }
    if (amount > mainAccount) {
      toast.error('提现金额不能超过主账户余额')
      return
    }
    openLiveChatPanel()
    toast.message('请在客服对话中发送提现金额、收款方式和注册邮箱')
  }

  return (
    <div className="min-h-[calc(100vh-64px)] bg-[#07080a] px-3 py-6 text-[#e8e8ec] sm:px-5 sm:py-8">
      <div className="pointer-events-none fixed inset-0 bg-[radial-gradient(circle_at_20%_10%,rgba(212,255,51,0.08),transparent_28%),linear-gradient(rgba(212,255,51,0.035)_1px,transparent_1px),linear-gradient(90deg,rgba(212,255,51,0.03)_1px,transparent_1px)] bg-[size:auto,56px_56px,56px_56px]" />
      <div className="relative mx-auto max-w-6xl space-y-5">
        <section className="overflow-hidden rounded-[28px] border border-[#d4ff33]/18 bg-[#101114]/95 p-5 shadow-[0_28px_100px_rgba(0,0,0,0.45)] sm:p-7">
          <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <Link
                to={ROUTES.strategyMarket}
                className="text-xs font-semibold text-[#848E9C] transition-colors hover:text-[#d4ff33]"
              >
                返回策略市场
              </Link>
              <h1 className="mt-4 text-3xl font-black tracking-tight text-white sm:text-5xl">钱包</h1>
              <p className="mt-3 text-sm text-[#8c93a3]">总览与账户分布</p>
            </div>
            <div className="rounded-3xl border border-white/10 bg-black/25 p-5">
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[#7b8190]">总览</p>
              <p className={`mt-2 break-words text-3xl font-black tabular-nums sm:text-4xl ${total < 0 ? 'text-red-400' : 'text-[#d4ff33]'}`}>
                {fmt(total)} <span className="text-base text-[#b7bdc6]">USDT</span>
              </p>
              {debt > 0 ? (
                <p className="mt-2 text-xs font-semibold text-red-300">当前欠费 {fmt(debt)} USDT</p>
              ) : null}
            </div>
          </div>
          <div className="mt-6 h-2 overflow-hidden rounded-full bg-white/8">
            <div className="h-full rounded-full bg-emerald-300" style={{ width: `${financePct}%` }} />
          </div>
        </section>

        <section className="grid gap-4 lg:grid-cols-2">
          {rows.map((row) => {
            const Icon = row.icon
            return (
              <article key={row.title} className="rounded-[24px] border border-white/10 bg-[#111114]/95 p-5">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <p className="text-sm font-bold text-white">{row.title}</p>
                    <p className="mt-1 text-xs text-[#7b8190]">{row.hint}</p>
                  </div>
                  <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl border border-[#d4ff33]/22 bg-[#d4ff33]/10">
                    <Icon className="h-5 w-5 text-[#d4ff33]" aria-hidden />
                  </div>
                </div>
                <p className={`mt-5 break-words text-2xl font-black tabular-nums sm:text-3xl ${row.tone}`}>
                  {fmt(row.amount)} <span className="text-base text-[#b7bdc6]">USDT</span>
                </p>
                {row.title === '理财账户' ? (
                  <Link
                    to={ROUTES.autoArbitrage}
                    className="mt-5 inline-flex h-10 items-center justify-center gap-2 rounded-xl border border-[#d4ff33]/30 bg-[#d4ff33]/10 px-4 text-xs font-black text-[#d4ff33] hover:bg-[#d4ff33]/15"
                  >
                    <ArrowRightLeft className="h-4 w-4" />
                    资金划转
                  </Link>
                ) : null}
              </article>
            )
          })}
        </section>

        <section className="grid gap-4 lg:grid-cols-[1.1fr_0.9fr]">
          <div className="rounded-[24px] border border-white/10 bg-[#111114]/95 p-5">
            <div className="mb-5 flex items-center gap-3">
              <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-[#d4ff33]/12">
                <Banknote className="h-5 w-5 text-[#d4ff33]" aria-hidden />
              </div>
              <div>
                <h2 className="text-lg font-black text-white">提现申请</h2>
                <p className="mt-1 text-xs text-[#7b8190]">当前可提主账户余额 {fmt(mainAccount)} USDT</p>
              </div>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="text-xs font-semibold text-[#9d9daa]">
                提现金额
                <input
                  type="number"
                  min="0"
                  max={mainAccount}
                  step="0.01"
                  value={withdrawAmount}
                  onChange={(event) => setWithdrawAmount(event.target.value)}
                  className="mt-2 h-12 w-full rounded-xl border border-white/10 bg-black/30 px-3 text-sm font-bold text-white outline-none focus:border-[#d4ff33]/60"
                  placeholder="输入 USDT 金额"
                />
              </label>
              <label className="text-xs font-semibold text-[#9d9daa]">
                收款方式 / 地址
                <input
                  value={withdrawAddress}
                  onChange={(event) => setWithdrawAddress(event.target.value)}
                  className="mt-2 h-12 w-full rounded-xl border border-white/10 bg-black/30 px-3 text-sm font-bold text-white outline-none focus:border-[#d4ff33]/60"
                  placeholder="客服核对时使用"
                />
              </label>
            </div>
            <button
              type="button"
              onClick={openWithdrawChat}
              className="mt-4 inline-flex h-12 w-full items-center justify-center gap-2 rounded-xl bg-[#d4ff33] px-4 text-sm font-black text-black shadow-[0_0_24px_rgba(212,255,51,0.18)] hover:bg-[#e4ff64]"
            >
              <MessageCircle className="h-5 w-5" aria-hidden />
              联系客服提交提现
            </button>
          </div>

          <div className="rounded-[24px] border border-white/10 bg-[#111114]/95 p-5">
            <div className="mb-5 flex items-center gap-3">
              <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-[#d4ff33]/12">
                <Headphones className="h-5 w-5 text-[#d4ff33]" aria-hidden />
              </div>
              <div>
                <h2 className="text-lg font-black text-white">充值与客服</h2>
                <p className="mt-1 text-xs text-[#7b8190]">充值入账继续走人工确认</p>
              </div>
            </div>
            <div className="space-y-3">
              <Link
                to={ROUTES.recharge}
                className="flex h-11 items-center justify-center rounded-xl border border-[#d4ff33]/30 bg-[#d4ff33]/10 text-sm font-bold text-[#d4ff33] hover:bg-[#d4ff33]/15"
              >
                打开充值页面
              </Link>
              <button
                type="button"
                onClick={() => openLiveChatPanel()}
                className="flex h-11 w-full items-center justify-center rounded-xl border border-white/10 text-sm font-bold text-white hover:border-[#d4ff33]/35"
              >
                联系在线客服
              </button>
              <p className="break-all rounded-xl border border-dashed border-white/10 bg-black/20 px-3 py-3 text-center font-mono text-xs text-[#d4ff33]">
                {SUPPORT_EMAIL}
              </p>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
