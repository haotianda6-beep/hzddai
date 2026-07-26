import { useState, useEffect } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import useSWR from 'swr'
import {
  User,
  Building2,
  Cpu,
  Eye,
  EyeOff,
  Plus,
  Pencil,
  Trash2,
  Wallet,
  Gift,
  ArrowRightLeft,
  Headphones,
  Key,
  Copy,
  Shield,
  ReceiptText,
} from 'lucide-react'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import { ROUTES } from '../router/paths'
import { openLiveChatPanel } from '../lib/liveChatOpen'
import { ExchangeConfigModal } from '../components/trader/ExchangeConfigModal'
import { InviteRewardsSection } from '../components/profile/InviteRewardsSection'
import type {
  CreateExchangeRequest,
  UpdateExchangeConfigRequest,
} from '../types'
import type { AdminAIPlatformUsageRow } from '../lib/api/walletAdmin'

type Tab = 'profile' | 'exchanges' | 'models'

const formatUsageModelName = (row: AdminAIPlatformUsageRow) => {
  const provider = String(row.provider || '')
    .trim()
    .toLowerCase()
  const model = String(row.model || '').trim()
  if (
    provider === 'comkun_ai' ||
    provider === 'comkun-ai' ||
    /跟单/.test(model)
  )
    return 'comkun-ai'
  return model || '—'
}

const formatUsageProviderName = (row: AdminAIPlatformUsageRow) => {
  const provider = String(row.provider || '').trim()
  const model = formatUsageModelName(row)
  if (
    !provider ||
    provider === 'comkun_ai' ||
    provider === 'comkun-ai' ||
    provider === model
  )
    return ''
  return provider
}

const usageStatusClass = (status: string) =>
  status === 'success'
    ? 'bg-emerald-500/15 text-emerald-300'
    : status === 'refunded'
      ? 'bg-amber-500/15 text-amber-300'
      : status === 'failed'
        ? 'bg-red-500/15 text-red-300'
        : 'bg-zinc-500/15 text-zinc-300'

/** 麻将「红中」牌样式装饰（运营红中账号名片角标） */
function HongZhongMahjongTile({ className = '' }: { className?: string }) {
  return (
    <div
      className={`pointer-events-none flex h-[3.25rem] w-10 flex-col items-center justify-center rounded-md border-2 border-red-800 bg-gradient-to-b from-white via-red-50 to-red-100 shadow-[0_6px_16px_rgba(130,0,0,0.45),inset_0_1px_0_rgba(255,255,255,0.9)] ring-1 ring-red-300/40 ${className}`}
      aria-hidden
    >
      <span className="font-serif text-xl font-black leading-none text-red-600 drop-shadow-sm">
        中
      </span>
      <span className="mt-0.5 text-[7px] font-bold uppercase tracking-wider text-red-700">
        红中
      </span>
    </div>
  )
}

export function ProfilePage() {
  const { user, logout, applyUserProfile } = useAuth()
  const { language } = useLanguage()
  const [searchParams, setSearchParams] = useSearchParams()

  const [tab, setTab] = useState<Tab>('profile')
  const [displayName, setDisplayName] = useState(user?.display_name || '')
  const [saving, setSaving] = useState(false)

  const [newPassword, setNewPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [changingPassword, setChangingPassword] = useState(false)

  const [showExchangeModal, setShowExchangeModal] = useState(false)
  const [editingExchange, setEditingExchange] = useState<string | null>(null)
  const [deletingExchange, setDeletingExchange] = useState<string | null>(null)

  // ED25519 key pair generation
  const [keyPair, setKeyPair] = useState<{
    private_key_hex: string
    private_key_b64: string
    private_key_seed_hex: string
    private_key_seed_b64: string
    public_key_hex: string
    public_key_b64: string
    private_key_pem: string
    public_key_pem: string
    ssh_public_key: string
  } | null>(null)
  const [generatingKeyPair, setGeneratingKeyPair] = useState(false)
  const [showPrivateKey, setShowPrivateKey] = useState(false)

  useEffect(() => {
    setDisplayName(user?.display_name || '')
  }, [user?.display_name])

  useEffect(() => {
    const q = searchParams.get('tab')
    if (q === 'exchanges' || q === 'models' || q === 'profile') {
      setTab(q)
    }
  }, [searchParams])

  const selectTab = (id: Tab) => {
    setTab(id)
    if (id === 'profile') {
      setSearchParams({}, { replace: true })
    } else {
      setSearchParams({ tab: id }, { replace: true })
    }
  }

  const { data: models, mutate: mutateModels } = useSWR(
    tab === 'models' ? 'profile-ai-models' : null,
    () => api.getModelConfigs(),
    { refreshInterval: 5000, revalidateOnFocus: true }
  )

  const { data: wallet, mutate: mutateWallet } = useSWR(
    tab === 'models' ? 'profile-platform-wallet' : null,
    () => api.getWallet(),
    { refreshInterval: 5000, revalidateOnFocus: true }
  )

  const { data: aiUsage, mutate: mutateAIUsage } = useSWR(
    tab === 'models' ? 'profile-ai-platform-usage' : null,
    () => api.getAIPlatformUsage(50),
    { refreshInterval: 10000, revalidateOnFocus: true }
  )

  const { data: exchanges, mutate: mutateExchanges } = useSWR(
    tab === 'exchanges' ? 'profile-exchanges' : null,
    () => api.getExchangeConfigs(),
    { revalidateOnFocus: true }
  )

  const { data: inviteData } = useSWR(
    tab === 'profile' ? 'profile-invite-me' : null,
    () => api.getInviteMe(),
    { refreshInterval: 30000 }
  )

  const { data: walletProfile } = useSWR(
    tab === 'profile' ? 'profile-wallet-brief' : null,
    () => api.getWallet(),
    { refreshInterval: 15000 }
  )

  const { data: rebateBal, mutate: mutateRebateBal } = useSWR(
    tab === 'profile' ? 'profile-agent-rebate' : null,
    () => api.getAgentRebateBalance(),
    { refreshInterval: 30000 }
  )

  const { data: rebateDividends } = useSWR(
    tab === 'profile' ? 'profile-agent-rebate-dividends' : null,
    () => api.getAgentRebateDividends(),
    { refreshInterval: 60000 }
  )

  const [rebateTransferAmount, setRebateTransferAmount] = useState('')
  const [rebateTransferLoading, setRebateTransferLoading] = useState(false)

  const claw = models?.find(
    (m) => m.provider === 'comkun_proxy' || m.provider === 'claw402'
  )
  const platformBalance = wallet?.balance_usdt ?? user?.balance_usdt ?? 0
  const profilePlatformUsdt =
    walletProfile?.balance_usdt ?? user?.balance_usdt ?? 0

  const fmtRebateU = (s: string | undefined) => {
    const n = parseFloat(s || '0')
    if (Number.isNaN(n)) return '0'
    return n.toLocaleString(undefined, {
      minimumFractionDigits: 0,
      maximumFractionDigits: 8,
    })
  }

  const rebateExempt =
    rebateBal?.configured === true &&
    rebateBal.synced === true &&
    rebateBal.rebate_exempt === true

  const handleSaveProfile = async (e: React.FormEvent) => {
    e.preventDefault()
    const name = displayName.trim()
    if (name.length < 1 || name.length > 32) {
      toast.error('用户名为 1～32 个字符')
      return
    }
    setSaving(true)
    try {
      const updated = await api.updateDisplayName(name)
      applyUserProfile({
        display_name: updated.display_name,
        avatar_url: updated.avatar_url,
      })
      toast.success('已保存')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault()
    if (newPassword.length < 8) {
      toast.error('新密码至少 8 个字符')
      return
    }
    setChangingPassword(true)
    try {
      const res = await fetch('/api/user/password', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({ new_password: newPassword }),
      })
      if (!res.ok) {
        const data = (await res.json().catch(() => ({}))) as { error?: string }
        throw new Error(data.error || '更新密码失败')
      }
      toast.success('密码已更新')
      setNewPassword('')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '更新密码失败')
    } finally {
      setChangingPassword(false)
    }
  }

  const handleSaveExchange = async (
    exchangeId: string | null,
    exchangeType: string,
    accountName: string,
    apiKey: string,
    secretKey?: string,
    passphrase?: string,
    testnet?: boolean,
    hyperliquidWalletAddr?: string,
    asterUser?: string,
    asterSigner?: string,
    asterPrivateKey?: string,
    lighterWalletAddr?: string,
    lighterPrivateKey?: string,
    lighterApiKeyPrivateKey?: string,
    lighterApiKeyIndex?: number,
    outboundProxyUrl?: string,
    outboundProxyClear?: boolean,
    outboundProxyAutoAssign?: boolean,
    apiUrl?: string
  ) => {
    try {
      if (exchangeId) {
        const row: UpdateExchangeConfigRequest['exchanges'][string] = {
          enabled: true,
          api_key: apiKey || '',
          secret_key: secretKey || '',
          passphrase: passphrase || '',
          testnet: testnet || false,
          api_url: apiUrl || '',
          hyperliquid_wallet_addr: hyperliquidWalletAddr || '',
          aster_user: asterUser || '',
          aster_signer: asterSigner || '',
          aster_private_key: asterPrivateKey || '',
          lighter_wallet_addr: lighterWalletAddr || '',
          lighter_private_key: lighterPrivateKey || '',
          lighter_api_key_private_key: lighterApiKeyPrivateKey || '',
          lighter_api_key_index: lighterApiKeyIndex || 0,
        }
        if (exchangeType === 'binance') {
          if (outboundProxyClear) row.outbound_proxy_clear = true
          else if (outboundProxyUrl && outboundProxyUrl.trim())
            row.outbound_proxy_url = outboundProxyUrl.trim()
          if (outboundProxyAutoAssign) row.outbound_proxy_auto_assign = true
        }
        const request: UpdateExchangeConfigRequest = {
          exchanges: {
            [exchangeId]: row,
          },
        }
        await api.updateExchangeConfigsEncrypted(request)
        toast.success('交易所配置已更新')
      } else {
        const createRequest: CreateExchangeRequest = {
          exchange_type: exchangeType,
          account_name: accountName,
          enabled: true,
          api_key: apiKey || '',
          secret_key: secretKey || '',
          passphrase: passphrase || '',
          testnet: testnet || false,
          api_url: apiUrl || '',
          hyperliquid_wallet_addr: hyperliquidWalletAddr || '',
          aster_user: asterUser || '',
          aster_signer: asterSigner || '',
          aster_private_key: asterPrivateKey || '',
          lighter_wallet_addr: lighterWalletAddr || '',
          lighter_private_key: lighterPrivateKey || '',
          lighter_api_key_private_key: lighterApiKeyPrivateKey || '',
          lighter_api_key_index: lighterApiKeyIndex || 0,
        }
        if (exchangeType === 'binance') {
          if (outboundProxyUrl?.trim()) {
            createRequest.outbound_proxy_url = outboundProxyUrl.trim()
          }
          if (outboundProxyAutoAssign !== undefined) {
            createRequest.auto_assign_outbound_proxy = outboundProxyAutoAssign
          }
        }
        await api.createExchangeEncrypted(createRequest)
        toast.success('已添加交易所账户')
      }
      await mutateExchanges()
      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存交易所配置失败')
    }
  }

  const handleDeleteExchange = async (exchangeId: string) => {
    const exchange = exchanges?.find((item) => item.id === exchangeId)
    const accountName = exchange?.account_name || exchange?.name || exchangeId
    if (
      !window.confirm(
        `确定删除交易所账户「${accountName}」吗？此操作无法撤销。`
      )
    )
      return

    setDeletingExchange(exchangeId)
    try {
      await api.deleteExchange(exchangeId)
      toast.success('已删除该交易所账户')
      await mutateExchanges()
      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '删除失败')
    } finally {
      setDeletingExchange(null)
    }
  }

  const tabBtn = (id: Tab, icon: typeof User, label: string) => {
    const active = tab === id
    const Icon = icon
    return (
      <button
        type="button"
        key={id}
        onClick={() => selectTab(id)}
        className={`flex shrink-0 items-center gap-2 border-b-2 px-1 py-3 text-sm font-medium transition-colors ${
          active
            ? 'border-nofx-gold text-nofx-gold'
            : 'border-transparent text-zinc-500 hover:text-zinc-300'
        }`}
      >
        <Icon className={`h-4 w-4 ${active ? 'text-nofx-gold' : ''}`} />
        {label}
      </button>
    )
  }

  return (
    <div className="mx-auto max-w-3xl px-3 py-6 text-zinc-100 sm:px-4 sm:py-8">
      <h1 className="mb-2 text-xl font-bold text-white">个人中心</h1>
      <p className="mb-6 text-sm text-zinc-500">
        账号资料、交易所 API 与模型余额摘要；编辑 AI 模型请到「设置」。
      </p>

      <div className="mb-6 flex gap-4 overflow-x-auto border-b border-zinc-800 sm:mb-8 sm:flex-wrap sm:gap-6">
        {tabBtn('profile', User, '个人资料')}
        {tabBtn('exchanges', Building2, '交易所账户')}
        {tabBtn('models', Cpu, '人工智能模型')}
      </div>

      {tab === 'profile' && user && (
        <div className="space-y-10">
          <div className="rounded-xl border border-[#d4ff33]/25 bg-gradient-to-br from-[#d4ff33]/[0.07] to-transparent p-5">
            <div className="mb-4 flex items-center gap-2 text-white">
              <Wallet className="h-5 w-5 text-[#d4ff33]" aria-hidden />
              <h3 className="text-sm font-bold">余额一览</h3>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="rounded-lg border border-zinc-800 bg-black/30 px-4 py-3">
                <div className="text-[11px] font-medium uppercase tracking-wide text-zinc-500">
                  平台站内余额（AI 计费）
                </div>
                <div className="mt-1 font-['Space_Grotesk',monospace] text-2xl font-bold tabular-nums text-amber-400">
                  {profilePlatformUsdt.toFixed(4)}{' '}
                  <span className="text-sm font-semibold text-zinc-500">
                    USDT
                  </span>
                </div>
                <p className="mt-2 text-xs text-zinc-500">
                  用于 COMKUN-AI / 代理模型扣费，与下方邀请返佣独立记账。
                </p>
              </div>
              <div className="rounded-lg border border-zinc-800 bg-black/30 px-4 py-3">
                <div className="flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-zinc-500">
                  <Gift className="h-3.5 w-3.5 text-[#d4ff33]" aria-hidden />
                  邀请返佣余额
                </div>
                {!rebateBal || !rebateBal.configured ? (
                  <p className="mt-2 text-xs text-zinc-500">
                    未配置返利服务，请联系管理员配置主站与返利系统的对接。
                  </p>
                ) : !rebateBal.synced ? (
                  <p className="mt-2 text-xs text-amber-200/90">
                    {rebateBal.message ||
                      '返利账户未同步：请稍后再试或由管理员从返利后台合并用户。'}
                  </p>
                ) : rebateExempt ? (
                  <p className="mt-2 text-xs leading-relaxed text-rose-200/95">
                    本账号为<strong className="text-white">运营展示账号</strong>
                    ，不参与邀请返佣入账、团队业绩累计与大区/VIP5
                    分红；下方仅作等级展示参考。
                    {typeof rebateBal.rebate_vip_level === 'number' ? (
                      <span className="ml-1 font-mono text-white">
                        VIP{rebateBal.rebate_vip_level}
                      </span>
                    ) : null}
                  </p>
                ) : (
                  <>
                    <div className="mt-1 font-['Space_Grotesk',monospace] text-2xl font-bold tabular-nums text-[#d4ff33]">
                      {fmtRebateU(rebateBal.rebate_balance_usdt)}{' '}
                      <span className="text-sm font-semibold text-zinc-500">
                        USDT
                      </span>
                    </div>
                    <p className="mt-1 text-[11px] text-zinc-500">
                      返利侧充值记账（不可提现）：
                      <span className="font-mono text-zinc-300">
                        {fmtRebateU(rebateBal.recharge_balance_usdt)} USDT
                      </span>
                    </p>
                  </>
                )}
              </div>
            </div>

            {rebateBal &&
              rebateBal.configured &&
              rebateBal.synced &&
              !rebateExempt && (
                <div className="mt-4 flex flex-col gap-3 border-t border-zinc-800/80 pt-4 sm:flex-row sm:flex-wrap sm:items-end">
                  <form
                    className="flex w-full flex-col items-stretch gap-2 sm:w-auto sm:flex-1 sm:flex-row sm:flex-wrap sm:items-end"
                    onSubmit={async (e) => {
                      e.preventDefault()
                      const raw = rebateTransferAmount.trim()
                      if (!raw || Number(raw) <= 0) {
                        toast.error('请输入大于 0 的金额')
                        return
                      }
                      setRebateTransferLoading(true)
                      try {
                        await api.postAgentRebateTransferToRecharge(raw)
                        toast.success('已从返佣划转到返利「充值记账」')
                        setRebateTransferAmount('')
                        await mutateRebateBal()
                      } catch (err) {
                        toast.error(
                          err instanceof Error ? err.message : '划转失败'
                        )
                      } finally {
                        setRebateTransferLoading(false)
                      }
                    }}
                  >
                    <label className="flex min-w-0 flex-1 flex-col gap-1 text-xs text-zinc-400">
                      <span className="flex items-center gap-1">
                        <ArrowRightLeft
                          className="h-3.5 w-3.5 text-[#d4ff33]"
                          aria-hidden
                        />
                        返佣划转到充值记账
                      </span>
                      <input
                        type="text"
                        inputMode="decimal"
                        placeholder="金额 USDT"
                        value={rebateTransferAmount}
                        onChange={(e) =>
                          setRebateTransferAmount(e.target.value)
                        }
                        className="w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 font-mono text-sm text-white outline-none focus:border-[#d4ff33]/50 sm:w-40"
                      />
                    </label>
                    <button
                      type="submit"
                      disabled={rebateTransferLoading}
                      className="rounded-lg bg-[#d4ff33]/90 px-4 py-2 text-sm font-bold text-black hover:bg-[#d4ff33] disabled:opacity-50"
                    >
                      {rebateTransferLoading ? '提交中…' : '确认划转'}
                    </button>
                  </form>
                  <button
                    type="button"
                    onClick={() => {
                      openLiveChatPanel()
                      toast.message(
                        '请在客服对话中说明：返利提现、您的注册邮箱与金额',
                        { duration: 5000 }
                      )
                    }}
                    className="inline-flex w-full items-center justify-center gap-2 rounded-lg border border-zinc-600 bg-zinc-900/80 px-4 py-2 text-sm font-semibold text-zinc-100 hover:border-[#d4ff33]/45 hover:text-[#d4ff33] sm:w-auto"
                  >
                    <Headphones className="h-4 w-4" aria-hidden />
                    返佣提现（联系客服）
                  </button>
                </div>
              )}

            {rebateBal &&
              rebateBal.configured &&
              rebateBal.synced &&
              !rebateExempt &&
              rebateDividends &&
              rebateDividends.configured &&
              Array.isArray(rebateDividends.rows) &&
              rebateDividends.rows.length > 0 && (
                <div className="mt-4 rounded-lg border border-zinc-800 bg-black/25 px-4 py-3">
                  <div className="text-[11px] font-semibold uppercase tracking-wide text-zinc-500">
                    VIP5 周分红记录（已并入上方返佣余额）
                  </div>
                  <ul className="mt-2 max-h-40 space-y-2 overflow-y-auto text-xs text-zinc-300">
                    {rebateDividends.rows.slice(0, 20).map((row, i) => (
                      <li
                        key={i}
                        className="flex flex-wrap justify-between gap-2 border-b border-zinc-800/80 pb-2 last:border-0"
                      >
                        <span>
                          结算周 {row.week_start} ~ {row.week_end}
                        </span>
                        <span className="font-mono text-[#d4ff33]">
                          +{fmtRebateU(row.amount_usdt)} USDT
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

            <p className="mt-3 text-[11px] leading-relaxed text-zinc-500">
              说明：邀请返佣数据来自返利子系统；划转是把
              <strong className="text-zinc-400">返佣余额</strong>划到同系统中的
              <strong className="text-zinc-400">充值记账余额</strong>
              （仍不可直接当站内 AI 余额使用）。提现请通过客服人工处理。
            </p>
          </div>

          <form
            onSubmit={handleSaveProfile}
            className={
              rebateExempt
                ? 'relative overflow-hidden space-y-6 rounded-2xl border border-red-500/40 bg-gradient-to-br from-red-950/95 via-rose-950/90 to-red-900/95 p-6 shadow-[inset_0_1px_0_rgba(255,220,220,0.14),0_12px_40px_rgba(90,0,0,0.45)] ring-1 ring-red-400/25'
                : 'space-y-6'
            }
          >
            {rebateExempt && (
              <>
                <div
                  className="pointer-events-none absolute -left-[20%] top-0 h-full w-[55%] rotate-[18deg] bg-gradient-to-r from-transparent via-white/[0.07] to-transparent"
                  aria-hidden
                />
                <div
                  className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_80%_50%_at_70%_-10%,rgba(255,160,160,0.22),transparent_55%)]"
                  aria-hidden
                />
                <HongZhongMahjongTile className="absolute right-4 top-4 z-10 sm:right-6 sm:top-6" />
              </>
            )}
            <div
              className={`flex flex-col items-center gap-3 ${rebateExempt ? 'relative z-[1]' : ''}`}
            >
              <img
                src={user.avatar_url || '/icons/comkun-logo.png'}
                alt=""
                className={
                  rebateExempt
                    ? 'h-24 w-24 rounded-full border-2 border-red-400/70 bg-zinc-900 object-cover shadow-[0_0_24px_rgba(220,50,50,0.35)] ring-2 ring-red-500/30'
                    : 'h-24 w-24 rounded-full border-2 border-zinc-700 bg-zinc-900 object-cover'
                }
              />
              <p
                className={`text-xs ${rebateExempt ? 'text-red-100/75' : 'text-zinc-500'}`}
              >
                头像由系统根据账号生成，暂不支持自定义上传
              </p>
            </div>

            <div className={rebateExempt ? 'relative z-[1]' : ''}>
              <label
                className={`mb-1 block text-sm ${rebateExempt ? 'text-red-100/90' : 'text-zinc-400'}`}
              >
                用户名
              </label>
              <input
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                maxLength={32}
                className={
                  rebateExempt
                    ? 'w-full rounded-lg border border-red-500/45 bg-red-950/50 px-3 py-2.5 text-white outline-none ring-1 ring-red-400/20 placeholder:text-red-200/50 focus:border-red-400/70'
                    : 'w-full rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2.5 text-zinc-100 outline-none focus:border-nofx-gold/60'
                }
              />
            </div>

            <div className={rebateExempt ? 'relative z-[1]' : ''}>
              <label
                className={`mb-1 block text-sm ${rebateExempt ? 'text-red-100/90' : 'text-zinc-400'}`}
              >
                邮箱
              </label>
              <input
                readOnly
                value={user.email}
                className={
                  rebateExempt
                    ? 'w-full cursor-not-allowed rounded-lg border border-red-900/60 bg-red-950/40 px-3 py-2.5 text-red-100/85'
                    : 'w-full cursor-not-allowed rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2.5 text-zinc-400'
                }
              />
            </div>

            <div
              className={`flex flex-col gap-3 sm:flex-row ${rebateExempt ? 'relative z-[1]' : ''}`}
            >
              <button
                type="submit"
                disabled={saving}
                className={
                  rebateExempt
                    ? 'w-full rounded-lg bg-gradient-to-r from-red-600 to-rose-700 px-5 py-2 text-sm font-semibold text-white shadow-md hover:from-red-500 hover:to-rose-600 disabled:opacity-50 sm:w-auto'
                    : 'w-full rounded-lg bg-nofx-gold px-5 py-2 text-sm font-semibold text-black hover:bg-nofx-gold-highlight disabled:opacity-50 sm:w-auto'
                }
              >
                {saving ? '保存中…' : '保存资料'}
              </button>
              <button
                type="button"
                onClick={() => logout()}
                className="w-full rounded-lg bg-red-900/50 px-5 py-2 text-sm font-semibold text-red-200 hover:bg-red-900/70 sm:w-auto"
              >
                退出登录
              </button>
            </div>
          </form>

          {!rebateExempt && (
            <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
              <p className="text-xs text-zinc-500">
                邀请奖励可直达专页，复制链接更方便。
              </p>
              <Link
                to={ROUTES.invite}
                className="text-xs font-semibold text-[#d4ff33] underline-offset-2 hover:underline"
              >
                打开邀请奖励专页 →
              </Link>
            </div>
          )}

          {rebateExempt ? (
            <div className="rounded-xl border border-red-500/30 bg-red-950/40 p-5 text-sm text-red-100/90">
              <h3 className="font-bold text-white">邀请奖励</h3>
              <p className="mt-2 text-xs leading-relaxed text-red-100/80">
                运营展示账号不参与邀请关系与返佣统计；伞下用户已全部脱离本账号邀请链（如有疑问请联系技术）。
              </p>
            </div>
          ) : (
            <InviteRewardsSection
              inviteData={inviteData}
              fallbackInviteCode={user.invite_code}
            />
          )}

          <div className="border-t border-zinc-800 pt-8">
            <h3 className="mb-4 text-sm font-semibold text-white">修改密码</h3>
            <form onSubmit={handleChangePassword} className="space-y-4">
              <div>
                <label className="mb-2 block text-xs font-medium text-zinc-400">
                  新密码
                </label>
                <div className="relative">
                  <input
                    type={showPassword ? 'text' : 'password'}
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    className="w-full rounded-xl border border-zinc-700/80 bg-zinc-950/80 px-4 py-3 pr-11 text-sm text-white placeholder-zinc-600 outline-none focus:border-nofx-gold/60 focus:ring-1 focus:ring-nofx-gold/30"
                    placeholder="至少 8 个字符"
                    autoComplete="new-password"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3.5 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300"
                    aria-label="切换密码可见"
                  >
                    {showPassword ? (
                      <EyeOff className="h-4 w-4" />
                    ) : (
                      <Eye className="h-4 w-4" />
                    )}
                  </button>
                </div>
              </div>
              <button
                type="submit"
                disabled={changingPassword || newPassword.length < 8}
                className="w-full rounded-xl bg-nofx-gold py-3 text-sm font-semibold text-black transition-all hover:bg-nofx-gold-highlight disabled:cursor-not-allowed disabled:opacity-50"
              >
                {changingPassword ? '更新中…' : '更新密码'}
              </button>
            </form>
          </div>
        </div>
      )}

      {tab === 'exchanges' && (
        <div className="space-y-4">
          <p className="text-sm text-zinc-500">
            在此添加或编辑交易所 API。设置页仅保留 AI 模型配置。
          </p>
          <div className="flex sm:justify-end">
            <button
              type="button"
              onClick={() => {
                setEditingExchange(null)
                setShowExchangeModal(true)
              }}
              className="flex w-full items-center justify-center gap-1.5 rounded-lg bg-nofx-gold/10 px-3 py-2 text-xs font-medium text-nofx-gold transition-colors hover:bg-nofx-gold/20 sm:w-auto"
            >
              <Plus className="h-4 w-4" />
              添加交易所
            </button>
          </div>
          {!exchanges ? (
            <div className="py-12 text-center text-sm text-zinc-500">
              加载中…
            </div>
          ) : (
            <ul className="space-y-2">
              {exchanges.map((ex) => (
                <li
                  key={ex.id}
                  className="flex flex-col items-stretch gap-3 rounded-lg border border-zinc-800 bg-zinc-900/60 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="flex min-w-0 flex-1 flex-col gap-3 sm:flex-row sm:items-center">
                    <div className="min-w-0 sm:w-40 sm:shrink-0">
                      <span className="block truncate font-medium sm:inline">
                        {ex.account_name || ex.name || ex.id}
                      </span>
                      <span className="text-xs text-zinc-500 sm:ml-2">
                        {ex.exchange_type}
                      </span>
                    </div>
                    {ex.type === 'cex' && (
                      <div className="flex min-w-0 flex-1 items-center gap-2 rounded-md border border-zinc-800 bg-black/25 px-3 py-2 sm:border-0 sm:bg-transparent sm:px-0 sm:py-0">
                        <span className="shrink-0 text-[11px] text-zinc-500">
                          API 白名单 IP
                        </span>
                        {ex.outbound_proxy_whitelist_host ? (
                          <>
                            <code className="min-w-0 truncate font-mono text-xs text-zinc-200">
                              {ex.outbound_proxy_whitelist_host}
                            </code>
                            <button
                              type="button"
                              onClick={() => {
                                void navigator.clipboard.writeText(
                                  ex.outbound_proxy_whitelist_host || ''
                                )
                                toast.success('白名单 IP 已复制')
                              }}
                              className="shrink-0 rounded-md p-1.5 text-zinc-500 transition-colors hover:bg-nofx-gold/10 hover:text-nofx-gold"
                              aria-label={`复制 ${ex.account_name || ex.name || '交易所'} 的白名单 IP`}
                              title="复制白名单 IP"
                            >
                              <Copy className="h-3.5 w-3.5" />
                            </button>
                          </>
                        ) : (
                          <span className="text-xs text-amber-300/80">
                            未分配，请联系管理员
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                  <div className="flex shrink-0 gap-2">
                    <button
                      type="button"
                      onClick={() => {
                        setEditingExchange(ex.id)
                        setShowExchangeModal(true)
                      }}
                      className="flex flex-1 items-center justify-center gap-1 rounded-lg border border-zinc-700 px-2 py-1 text-xs text-zinc-300 hover:border-nofx-gold/50 hover:text-nofx-gold sm:flex-none"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                      编辑
                    </button>
                    <button
                      type="button"
                      disabled={deletingExchange === ex.id}
                      onClick={() => void handleDeleteExchange(ex.id)}
                      className="flex flex-1 items-center justify-center gap-1 rounded-lg border border-red-500/30 px-2 py-1 text-xs text-red-400 transition-colors hover:border-red-400/60 hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-50 sm:flex-none"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {deletingExchange === ex.id ? '删除中…' : '删除'}
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
          {exchanges && exchanges.length === 0 && (
            <p className="text-sm text-zinc-500">
              暂无交易所账户，请点击「添加交易所」。
            </p>
          )}

          {/* ED25519 Key Generator */}
          <div className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5">
            <div className="mb-3 flex flex-wrap items-center gap-2">
              <Key className="h-5 w-5 text-nofx-gold" />
              <span className="font-bold text-white">ED25519 密钥生成器</span>
              <span className="rounded bg-zinc-800 px-2 py-0.5 text-[10px] text-zinc-500 sm:ml-auto">
                Binance / OKX 兼容
              </span>
            </div>
            <p className="mb-4 text-xs text-zinc-500">
              为交易所 API 生成 ED25519 密钥对。私钥仅在生成时可见，请立即保存。
            </p>

            {!keyPair ? (
              <button
                type="button"
                disabled={generatingKeyPair}
                onClick={async () => {
                  setGeneratingKeyPair(true)
                  try {
                    const kp = await api.generateED25519KeyPair()
                    setKeyPair(kp)
                    toast.success('密钥对已生成')
                  } catch {
                    toast.error('密钥生成失败，请重试')
                  } finally {
                    setGeneratingKeyPair(false)
                  }
                }}
                className="flex w-full items-center justify-center gap-2 rounded-lg bg-nofx-gold/10 px-4 py-2.5 text-sm font-medium text-nofx-gold transition-colors hover:bg-nofx-gold/20 disabled:opacity-50 sm:w-auto"
              >
                <Shield className="h-4 w-4" />
                {generatingKeyPair ? '生成中…' : '生成密钥对'}
              </button>
            ) : (
              <div className="space-y-4">
                {/* PEM 公钥 — 交易所主要使用此格式 */}
                <div>
                  <div className="mb-1 flex flex-wrap items-center gap-2 text-xs text-zinc-400">
                    <span className="rounded bg-emerald-900/30 px-1.5 py-0.5 text-[10px] text-emerald-400">
                      交易所用
                    </span>
                    <span>公钥 PEM</span>
                    <button
                      type="button"
                      onClick={() => {
                        void navigator.clipboard.writeText(
                          keyPair.public_key_pem
                        )
                        toast.success('公钥 PEM 已复制')
                      }}
                      className="flex items-center gap-1 text-nofx-gold hover:underline"
                    >
                      <Copy className="h-3 w-3" /> 复制
                    </button>
                  </div>
                  <pre className="overflow-x-auto rounded bg-zinc-950 px-3 py-2 font-mono text-xs text-emerald-400 whitespace-pre-wrap break-all">
                    {keyPair.public_key_pem}
                  </pre>
                </div>

                {/* PEM 私钥 */}
                <div>
                  <div className="mb-1 flex flex-wrap items-center gap-2 text-xs text-zinc-400">
                    <span className="rounded bg-amber-900/30 px-1.5 py-0.5 text-[10px] text-amber-400">
                      交易所用
                    </span>
                    <span>私钥 PEM</span>
                    <button
                      type="button"
                      onClick={() => setShowPrivateKey(!showPrivateKey)}
                      className="flex items-center gap-1 text-zinc-400 hover:text-white"
                    >
                      {showPrivateKey ? (
                        <EyeOff className="h-3 w-3" />
                      ) : (
                        <Eye className="h-3 w-3" />
                      )}
                      {showPrivateKey ? '隐藏' : '显示'}
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        void navigator.clipboard.writeText(
                          keyPair.private_key_pem
                        )
                        toast.success('私钥 PEM 已复制')
                      }}
                      className="flex items-center gap-1 text-nofx-gold hover:underline"
                    >
                      <Copy className="h-3 w-3" /> 复制
                    </button>
                  </div>
                  <pre className="overflow-x-auto rounded bg-zinc-950 px-3 py-2 font-mono text-xs whitespace-pre-wrap break-all">
                    {showPrivateKey ? (
                      keyPair.private_key_pem
                    ) : (
                      <span className="text-red-400">
                        •••••••••• 点击「显示」查看私钥
                      </span>
                    )}
                  </pre>
                </div>

                {/* 公钥十六进制 */}
                <details className="cursor-pointer">
                  <summary className="text-xs text-zinc-500 hover:text-zinc-400">
                    高级：公钥 Hex / Base64（32 字节原始格式）
                  </summary>
                  <div className="mt-2 space-y-2">
                    <div>
                      <div className="mb-1 flex flex-wrap items-center gap-2 text-xs text-zinc-500">
                        <span>Hex</span>
                        <button
                          type="button"
                          onClick={() => {
                            void navigator.clipboard.writeText(
                              keyPair.public_key_hex
                            )
                            toast.success('公钥 Hex 已复制')
                          }}
                          className="flex items-center gap-1 text-nofx-gold hover:underline"
                        >
                          <Copy className="h-3 w-3" /> 复制
                        </button>
                      </div>
                      <div className="rounded bg-zinc-950 px-3 py-2 font-mono text-[10px] text-zinc-400 break-all">
                        {keyPair.public_key_hex}
                      </div>
                    </div>
                    <div>
                      <div className="mb-1 flex flex-wrap items-center gap-2 text-xs text-zinc-500">
                        <span>Base64</span>
                        <button
                          type="button"
                          onClick={() => {
                            void navigator.clipboard.writeText(
                              keyPair.public_key_b64
                            )
                            toast.success('公钥 Base64 已复制')
                          }}
                          className="flex items-center gap-1 text-nofx-gold hover:underline"
                        >
                          <Copy className="h-3 w-3" /> 复制
                        </button>
                      </div>
                      <div className="rounded bg-zinc-950 px-3 py-2 font-mono text-[10px] text-zinc-400 break-all">
                        {keyPair.public_key_b64}
                      </div>
                    </div>
                    {/* SSH 公钥 */}
                    <div>
                      <div className="mb-1 flex flex-wrap items-center gap-2 text-xs text-zinc-500">
                        <span>SSH 格式</span>
                        <button
                          type="button"
                          onClick={() => {
                            void navigator.clipboard.writeText(
                              keyPair.ssh_public_key
                            )
                            toast.success('SSH 公钥已复制')
                          }}
                          className="flex items-center gap-1 text-nofx-gold hover:underline"
                        >
                          <Copy className="h-3 w-3" /> 复制
                        </button>
                      </div>
                      <div className="rounded bg-zinc-950 px-3 py-2 font-mono text-[10px] text-zinc-400 break-all">
                        {keyPair.ssh_public_key}
                      </div>
                    </div>
                  </div>
                </details>

                {/* Reset */}
                <button
                  type="button"
                  onClick={() => {
                    setKeyPair(null)
                    setShowPrivateKey(false)
                  }}
                  className="text-xs text-zinc-500 hover:text-zinc-300"
                >
                  清除并重新生成
                </button>

                <p className="text-[11px] text-amber-500">
                  ⚠️
                  私钥仅显示一次。请立即复制并安全保存。切勿将私钥分享给任何人。
                </p>
              </div>
            )}
          </div>
        </div>
      )}

      {tab === 'models' && (
        <div className="space-y-6">
          <p className="text-sm text-zinc-500">
            下方显示你的平台余额。COMKUN-AI
            与代理模型调用费用都会从平台余额扣除，每约 5 秒自动刷新。
          </p>

          <div className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5">
            <div className="mb-4 flex items-center gap-2 text-nofx-gold">
              <Cpu className="h-5 w-5" />
              <span className="font-bold">
                {claw?.name || 'COMKUN-AI 代理模型'}
              </span>
            </div>
            <div className="mb-2 text-xs uppercase tracking-wide text-zinc-500">
              平台余额（USDT）
            </div>
            <div className="font-['Space_Grotesk',monospace] text-3xl font-bold tabular-nums text-amber-400">
              {platformBalance.toFixed(4)} USDT
            </div>
            <p className="mt-3 text-xs text-zinc-500">
              这里显示的是站内平台余额，不再显示链上 USDC
              钱包余额。邀请返佣与划转请打开「个人资料」页。
            </p>
          </div>

          <div className="rounded-lg border border-zinc-800 bg-zinc-950/50 p-4">
            <div className="mb-2 text-xs font-bold text-zinc-500">
              全部模型状态
            </div>
            <ul className="space-y-2 text-sm">
              {(models ?? []).map((m) => (
                <li key={m.id} className="flex min-w-0 justify-between gap-2">
                  <span className="truncate">{m.name}</span>
                  <span
                    className={m.enabled ? 'text-emerald-400' : 'text-zinc-600'}
                  >
                    {m.enabled ? '已启用' : '未启用'}
                  </span>
                </li>
              ))}
            </ul>
          </div>

          <div className="rounded-lg border border-zinc-800 bg-zinc-950/50 p-4">
            <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-center gap-2">
                <ReceiptText className="h-4 w-4 text-nofx-gold" aria-hidden />
                <div>
                  <div className="text-xs font-bold text-zinc-300">
                    AI 调用账单
                  </div>
                  <div className="text-[11px] text-zinc-500">
                    最近 50 条 COMKUN-AI / 代理模型扣费记录
                  </div>
                </div>
              </div>
              <button
                type="button"
                onClick={() => void mutateAIUsage()}
                className="shrink-0 text-xs text-nofx-gold hover:underline"
              >
                刷新账单
              </button>
            </div>

            {!aiUsage ? (
              <div className="py-6 text-center text-sm text-zinc-500">
                账单加载中…
              </div>
            ) : (aiUsage.items ?? []).length === 0 ? (
              <div className="rounded-lg border border-dashed border-zinc-800 px-4 py-6 text-center text-sm text-zinc-500">
                暂无 AI 调用账单
              </div>
            ) : (
              <>
                <div className="space-y-2 md:hidden">
                  {(aiUsage.items ?? []).map((row: AdminAIPlatformUsageRow) => (
                    <div
                      key={row.id}
                      className="rounded-lg border border-zinc-800 bg-black/20 p-3 text-xs"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="font-semibold text-white">
                            {formatUsageModelName(row)}
                          </div>
                          {formatUsageProviderName(row) && (
                            <div className="truncate text-[11px] text-zinc-500">
                              {formatUsageProviderName(row)}
                            </div>
                          )}
                        </div>
                        <span
                          className={`shrink-0 rounded-full px-2 py-1 text-[11px] font-bold ${usageStatusClass(row.status)}`}
                        >
                          {row.status}
                        </span>
                      </div>
                      <div className="mt-3 grid gap-2 text-zinc-500">
                        <div className="flex justify-between gap-3">
                          <span>时间</span>
                          <span className="text-right text-zinc-300">
                            {new Date(row.created_at).toLocaleString()}
                          </span>
                        </div>
                        <div className="flex justify-between gap-3">
                          <span>扣费</span>
                          <span className="font-mono font-bold tabular-nums text-nofx-gold">
                            {Number(row.charged_usdt ?? 0).toFixed(6)} USDT
                          </span>
                        </div>
                        <div className="flex justify-between gap-3">
                          <span>余额变化</span>
                          <span className="font-mono tabular-nums text-zinc-300">
                            {Number(row.wallet_balance_before ?? 0).toFixed(4)}{' '}
                            → {Number(row.wallet_balance_after ?? 0).toFixed(4)}
                          </span>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
                <div className="hidden max-h-80 overflow-auto rounded-lg border border-zinc-800 md:block">
                  <table className="w-full min-w-[680px] border-collapse text-left text-xs">
                    <thead className="sticky top-0 bg-zinc-950">
                      <tr className="border-b border-zinc-800 text-zinc-500">
                        <th className="px-3 py-2">时间</th>
                        <th className="px-3 py-2">模型</th>
                        <th className="px-3 py-2">扣费</th>
                        <th className="px-3 py-2">余额变化</th>
                        <th className="px-3 py-2">状态</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-zinc-900">
                      {(aiUsage.items ?? []).map(
                        (row: AdminAIPlatformUsageRow) => (
                          <tr key={row.id} className="hover:bg-white/[0.02]">
                            <td className="px-3 py-2 text-zinc-400">
                              {new Date(row.created_at).toLocaleString()}
                            </td>
                            <td className="px-3 py-2">
                              <div className="font-semibold text-white">
                                {formatUsageModelName(row)}
                              </div>
                              {formatUsageProviderName(row) && (
                                <div className="text-[11px] text-zinc-500">
                                  {formatUsageProviderName(row)}
                                </div>
                              )}
                            </td>
                            <td className="px-3 py-2 font-mono font-bold tabular-nums text-nofx-gold">
                              {Number(row.charged_usdt ?? 0).toFixed(6)} USDT
                            </td>
                            <td className="px-3 py-2 font-mono tabular-nums text-zinc-300">
                              {Number(row.wallet_balance_before ?? 0).toFixed(
                                4
                              )}{' '}
                              →{' '}
                              {Number(row.wallet_balance_after ?? 0).toFixed(4)}
                            </td>
                            <td className="px-3 py-2">
                              <span
                                className={`rounded-full px-2 py-1 text-[11px] font-bold ${usageStatusClass(row.status)}`}
                              >
                                {row.status}
                              </span>
                            </td>
                          </tr>
                        )
                      )}
                    </tbody>
                  </table>
                </div>
              </>
            )}
          </div>

          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <button
              type="button"
              onClick={() => {
                void mutateWallet()
                void mutateModels()
                void mutateAIUsage()
              }}
              className="text-left text-sm text-nofx-gold hover:underline"
            >
              立即刷新余额
            </button>
            <Link
              to={ROUTES.settings}
              className="inline-block text-sm text-zinc-400 hover:text-nofx-gold hover:underline"
            >
              前往「设置」编辑模型 →
            </Link>
          </div>
        </div>
      )}

      {showExchangeModal && (
        <ExchangeConfigModal
          allExchanges={exchanges ?? []}
          editingExchangeId={editingExchange}
          onSave={handleSaveExchange}
          onDelete={handleDeleteExchange}
          onClose={() => {
            setShowExchangeModal(false)
            setEditingExchange(null)
          }}
          language={language}
        />
      )}
    </div>
  )
}
