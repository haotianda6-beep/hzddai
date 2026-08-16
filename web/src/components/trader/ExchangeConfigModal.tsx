import React, { useState, useEffect } from 'react'
import type { Exchange } from '../../types'
import { t, type Language } from '../../i18n/translations'
import { getExchangeIcon } from '../common/ExchangeIcons'
import {
  TwoStageKeyModal,
  type TwoStageKeyModalResult,
} from '../modals/TwoStageKeyModal'
import {
  WebCryptoEnvironmentCheck,
  type WebCryptoCheckStatus,
} from '../common/WebCryptoEnvironmentCheck'
import {
  BookOpen,
  Trash2,
  HelpCircle,
  ExternalLink,
  UserPlus,
  Key,
  Shield,
  Check,
  Copy,
  ArrowRight,
} from 'lucide-react'
import { toast } from 'sonner'
import { Tooltip } from './Tooltip'
import { getShortName } from './utils'

// Supported exchange templates
export const SUPPORTED_EXCHANGE_TEMPLATES = [
  { exchange_type: 'binance', name: 'Binance Futures', type: 'cex' as const },
  { exchange_type: 'bybit', name: 'Bybit Futures', type: 'cex' as const },
  { exchange_type: 'okx', name: 'OKX Futures', type: 'cex' as const },
  { exchange_type: 'bitget', name: 'Bitget Futures', type: 'cex' as const },
  { exchange_type: 'gate', name: 'Gate.io Futures', type: 'cex' as const },
  { exchange_type: 'kucoin', name: 'KuCoin Futures', type: 'cex' as const },
  { exchange_type: 'hyperliquid', name: 'Hyperliquid', type: 'dex' as const },
  { exchange_type: 'aster', name: 'Aster DEX', type: 'dex' as const },
  { exchange_type: 'lighter', name: 'Lighter', type: 'dex' as const },
  { exchange_type: 'indodax', name: 'Indodax', type: 'cex' as const },
  { exchange_type: 'hz', name: 'BALIB 交易账户', type: 'cex' as const },
]

export function getExchangeCredentialFields(exchangeType: string) {
  return {
    apiUrl: exchangeType === 'hz',
    apiKey: true,
    secretKey: true,
    passphrase:
      exchangeType === 'okx' ||
      exchangeType === 'bitget' ||
      exchangeType === 'kucoin',
  }
}

interface ExchangeConfigModalProps {
  allExchanges: Exchange[]
  editingExchangeId: string | null
  onSave: (
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
    /** CEX：从管理员代理池自动/重新分配 */
    outboundProxyAutoAssign?: boolean,
    apiUrl?: string
  ) => Promise<void>
  onDelete: (exchangeId: string) => void
  onClose: () => void
  language: Language
}

const EXCHANGE_API_DOCS: Record<string, string> = {
  binance: 'https://www.binance.com/en/my/settings/api-management',
  okx: 'https://www.okx.com/account/my-api',
  bybit: 'https://www.bybit.com/app/user/api-management',
  bitget: 'https://www.bitget.com/account/api-management',
  gate: 'https://www.gate.io/myaccount/api_key_manage',
  kucoin: 'https://www.kucoin.com/account/api',
  hyperliquid: 'https://app.hyperliquid.xyz/',
  aster: 'https://www.asterdex.com/',
  lighter: 'https://app.lighter.xyz/',
  indodax: 'https://indodax.com/settings/api',
}

export function ExchangeConfigModal({
  allExchanges,
  editingExchangeId,
  onSave,
  onDelete,
  onClose,
  language,
}: ExchangeConfigModalProps) {
  const [selectedExchangeType, setSelectedExchangeType] = useState('okx')
  const [apiKey, setApiKey] = useState('')
  const [secretKey, setSecretKey] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [apiUrl, setApiUrl] = useState('')
  const [testnet, setTestnet] = useState(false)
  const [showGuide, setShowGuide] = useState(false)
  const [copiedProxyHint, setCopiedProxyHint] = useState(false)
  const [webCryptoStatus, setWebCryptoStatus] =
    useState<WebCryptoCheckStatus>('idle')
  const [showBinanceGuide, setShowBinanceGuide] = useState(false)

  // Aster fields
  const [asterUser, setAsterUser] = useState('')
  const [asterSigner, setAsterSigner] = useState('')
  const [asterPrivateKey, setAsterPrivateKey] = useState('')

  // Hyperliquid fields
  const [hyperliquidWalletAddr, setHyperliquidWalletAddr] = useState('')

  // Lighter fields
  const [lighterWalletAddr, setLighterWalletAddr] = useState('')
  const [lighterApiKeyPrivateKey, setLighterApiKeyPrivateKey] = useState('')
  const [lighterApiKeyIndex, setLighterApiKeyIndex] = useState(0)

  // Other state
  const [secureInputTarget, setSecureInputTarget] = useState<
    null | 'hyperliquid' | 'aster' | 'lighter'
  >(null)
  const [isSaving, setIsSaving] = useState(false)
  const [accountName, setAccountName] = useState('')

  const selectedExchange = editingExchangeId
    ? allExchanges?.find((e) => e.id === editingExchangeId)
    : null

  useEffect(() => {
    if (editingExchangeId && selectedExchange?.exchange_type) {
      setSelectedExchangeType(selectedExchange.exchange_type)
    }
  }, [editingExchangeId, selectedExchange?.exchange_type])

  const selectedTemplate = editingExchangeId
    ? SUPPORTED_EXCHANGE_TEMPLATES.find(
        (t) => t.exchange_type === selectedExchange?.exchange_type
      )
    : SUPPORTED_EXCHANGE_TEMPLATES.find(
        (t) => t.exchange_type === selectedExchangeType
      )

  const currentExchangeType = editingExchangeId
    ? selectedExchange?.exchange_type
    : selectedExchangeType
  const credentialFields = getExchangeCredentialFields(
    currentExchangeType || ''
  )
  const currentExchangeUsesDedicatedProxy = [
    'binance',
    'bybit',
    'okx',
    'bitget',
    'gate',
  ].includes(currentExchangeType || '')

  const exchangeRegistrationLinks: Record<
    string,
    { url: string; hasReferral?: boolean }
  > = {
    binance: {
      url: 'https://www.binance.com/join?ref=COMKUNENG',
      hasReferral: true,
    },
    okx: { url: 'https://www.okx.com/join/1865360', hasReferral: true },
    bybit: { url: 'https://partner.bybit.com/b/83856', hasReferral: true },
    bitget: {
      url: 'https://www.bitget.com/referral/register?from=referral&clacCode=c8a43172',
      hasReferral: true,
    },
    gate: { url: 'https://www.gatenode.xyz/share/VQBGUAxY', hasReferral: true },
    kucoin: {
      url: 'https://www.kucoin.com/r/broker/CXEV7XKK',
      hasReferral: true,
    },
    hyperliquid: {
      url: 'https://app.hyperliquid.xyz/join/AITRADING',
      hasReferral: true,
    },
    aster: {
      url: 'https://www.asterdex.com/en/referral/fdfc0e',
      hasReferral: true,
    },
    lighter: {
      url: 'https://app.lighter.xyz/?referral=68151432',
      hasReferral: true,
    },
    indodax: { url: 'https://indodax.com/ref/Saep23/1', hasReferral: true },
  }

  // Initialize form when editing
  useEffect(() => {
    if (editingExchangeId && selectedExchange) {
      setAccountName(selectedExchange.account_name || '')
      setApiKey(selectedExchange.apiKey || '')
      setSecretKey(selectedExchange.secretKey || '')
      setApiUrl(selectedExchange.apiUrl || '')
      setPassphrase('')
      setTestnet(selectedExchange.testnet || false)
      setAsterUser(selectedExchange.asterUser || '')
      setAsterSigner(selectedExchange.asterSigner || '')
      setAsterPrivateKey('')
      setHyperliquidWalletAddr(selectedExchange.hyperliquidWalletAddr || '')
      setLighterWalletAddr(selectedExchange.lighterWalletAddr || '')
      setLighterApiKeyPrivateKey('')
      setLighterApiKeyIndex(selectedExchange.lighterApiKeyIndex || 0)
    }
  }, [editingExchangeId, selectedExchange])

  const handleCopyIP = async (ip: string) => {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(ip)
        setCopiedProxyHint(true)
        setTimeout(() => setCopiedProxyHint(false), 2000)
        toast.success(t('ipCopied', language))
      } else {
        const textArea = document.createElement('textarea')
        textArea.value = ip
        textArea.style.position = 'fixed'
        textArea.style.left = '-999999px'
        document.body.appendChild(textArea)
        textArea.select()
        document.execCommand('copy')
        document.body.removeChild(textArea)
        setCopiedProxyHint(true)
        setTimeout(() => setCopiedProxyHint(false), 2000)
        toast.success(t('ipCopied', language))
      }
    } catch {
      toast.error(t('copyIPFailed', language))
    }
  }

  const secureInputContextLabel =
    secureInputTarget === 'aster'
      ? t('asterExchangeName', language)
      : secureInputTarget === 'hyperliquid'
        ? t('hyperliquidExchangeName', language)
        : undefined

  const handleSecureInputComplete = ({ value }: TwoStageKeyModalResult) => {
    const trimmed = value.trim()
    if (secureInputTarget === 'hyperliquid') setApiKey(trimmed)
    if (secureInputTarget === 'aster') setAsterPrivateKey(trimmed)
    if (secureInputTarget === 'lighter') {
      setLighterApiKeyPrivateKey(trimmed)
      toast.success(t('lighterApiKeyImported', language))
    }
    setSecureInputTarget(null)
  }

  const maskSecret = (secret: string) => {
    if (!secret || secret.length === 0) return ''
    if (secret.length <= 8) return '*'.repeat(secret.length)
    return (
      secret.slice(0, 4) +
      '*'.repeat(Math.max(secret.length - 8, 4)) +
      secret.slice(-4)
    )
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (isSaving) return
    if (!currentExchangeType) return

    const trimmedAccountName = accountName.trim()
    if (!trimmedAccountName) {
      toast.error(t('exchangeConfig.pleaseEnterAccountName', language))
      return
    }

    const exchangeId = editingExchangeId || null
    const exchangeType = currentExchangeType || ''

    setIsSaving(true)
    try {
      if (currentExchangeType === 'hz') {
        if (
          !apiUrl.trim() ||
          (!editingExchangeId && (!apiKey.trim() || !secretKey.trim()))
        )
          return
        await onSave(
          exchangeId,
          exchangeType,
          trimmedAccountName,
          apiKey.trim(),
          secretKey.trim(),
          '',
          false,
          undefined,
          undefined,
          undefined,
          undefined,
          undefined,
          undefined,
          undefined,
          0,
          undefined,
          false,
          false,
          apiUrl.trim()
        )
      } else if (currentExchangeType === 'binance') {
        if (!apiKey.trim() || !secretKey.trim()) return
        // 新建：勿传 false，否则会把 auto_assign_outbound_proxy 序列化为 false，服务端不会从池分配
        // 编辑：若尚未绑定代理池且未配置自定义出口，则向服务端申请从池首次分配
        const needFirstTimePool =
          !!exchangeId &&
          !!selectedExchange &&
          !selectedExchange.outbound_proxy_from_pool &&
          !selectedExchange.outbound_proxy_configured
        const poolAutoAssign: boolean | undefined = exchangeId
          ? needFirstTimePool
            ? true
            : false
          : undefined
        await onSave(
          exchangeId,
          exchangeType,
          trimmedAccountName,
          apiKey.trim(),
          secretKey.trim(),
          '',
          testnet,
          undefined,
          undefined,
          undefined,
          undefined,
          undefined,
          undefined,
          undefined,
          0,
          undefined,
          false,
          poolAutoAssign
        )
      } else if (
        currentExchangeType === 'bybit' ||
        currentExchangeType === 'indodax'
      ) {
        if (!apiKey.trim() || !secretKey.trim()) return
        await onSave(
          exchangeId,
          exchangeType,
          trimmedAccountName,
          apiKey.trim(),
          secretKey.trim(),
          '',
          testnet
        )
      } else if (
        currentExchangeType === 'okx' ||
        currentExchangeType === 'bitget' ||
        currentExchangeType === 'kucoin'
      ) {
        if (!apiKey.trim() || !secretKey.trim() || !passphrase.trim()) return
        await onSave(
          exchangeId,
          exchangeType,
          trimmedAccountName,
          apiKey.trim(),
          secretKey.trim(),
          passphrase.trim(),
          testnet
        )
      } else if (currentExchangeType === 'hyperliquid') {
        if (!apiKey.trim() || !hyperliquidWalletAddr.trim()) return
        await onSave(
          exchangeId,
          exchangeType,
          trimmedAccountName,
          apiKey.trim(),
          '',
          '',
          testnet,
          hyperliquidWalletAddr.trim()
        )
      } else if (currentExchangeType === 'aster') {
        if (!asterUser.trim() || !asterSigner.trim() || !asterPrivateKey.trim())
          return
        await onSave(
          exchangeId,
          exchangeType,
          trimmedAccountName,
          '',
          '',
          '',
          testnet,
          undefined,
          asterUser.trim(),
          asterSigner.trim(),
          asterPrivateKey.trim()
        )
      } else if (currentExchangeType === 'lighter') {
        if (!lighterWalletAddr.trim() || !lighterApiKeyPrivateKey.trim()) return
        await onSave(
          exchangeId,
          exchangeType,
          trimmedAccountName,
          '',
          '',
          '',
          testnet,
          undefined,
          undefined,
          undefined,
          undefined,
          lighterWalletAddr.trim(),
          '',
          lighterApiKeyPrivateKey.trim(),
          lighterApiKeyIndex
        )
      } else {
        if (!apiKey.trim() || !secretKey.trim()) return
        await onSave(
          exchangeId,
          exchangeType,
          trimmedAccountName,
          apiKey.trim(),
          secretKey.trim(),
          '',
          testnet
        )
      }
    } finally {
      setIsSaving(false)
    }
  }

  const cryptoBlocked =
    webCryptoStatus !== 'secure' && webCryptoStatus !== 'disabled'

  return (
    <div className="fixed inset-0 z-[55] flex items-start justify-center overflow-y-auto bg-black/70 p-2 backdrop-blur-sm sm:items-center sm:p-4">
      <div
        className="flex w-full max-w-5xl flex-col overflow-hidden rounded-2xl border border-zinc-800/90 bg-nofx-bg shadow-2xl"
        style={{ maxHeight: 'min(90vh, 920px)' }}
      >
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-zinc-800/90 px-4 py-3 sm:px-5 sm:py-4">
          <h2 className="text-lg font-bold text-white">
            {editingExchangeId
              ? t('editExchange', language)
              : t('addExchange', language)}
          </h2>
          <div className="flex shrink-0 items-center gap-1">
            {currentExchangeType === 'binance' && (
              <button
                type="button"
                onClick={() => setShowGuide(true)}
                className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs font-semibold text-amber-200 transition-colors hover:bg-amber-500/20"
              >
                <BookOpen className="h-4 w-4" />
                {t('viewGuide', language)}
              </button>
            )}
            {editingExchangeId && (
              <button
                type="button"
                onClick={() => onDelete(editingExchangeId)}
                className="rounded-lg p-2 text-red-400 transition-colors hover:bg-red-500/15"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            )}
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg p-2 text-zinc-400 transition-colors hover:bg-white/10 hover:text-white"
            >
              ✕
            </button>
          </div>
        </div>

        <div className="flex min-h-0 flex-1 flex-col md:flex-row">
          {/* 左侧：交易所列表 */}
          <aside className="flex max-h-[40vh] shrink-0 flex-col border-b border-zinc-800/90 bg-nofx-bg-tertiary md:max-h-none md:w-[280px] md:border-b-0 md:border-r md:border-zinc-800/90">
            <p className="px-4 pb-2 pt-4 text-xs font-medium uppercase tracking-wide text-zinc-500">
              {t('exchangeConfig.selectExchange', language)}
            </p>
            <div className="min-h-0 flex-1 space-y-1 overflow-y-auto px-2 pb-3">
              {SUPPORTED_EXCHANGE_TEMPLATES.map((template) => {
                const sel = selectedExchangeType === template.exchange_type
                const disabled =
                  cryptoBlocked &&
                  !['hyperliquid', 'aster', 'lighter'].includes(
                    template.exchange_type
                  )
                return (
                  <button
                    key={template.exchange_type}
                    type="button"
                    disabled={disabled}
                    onClick={() =>
                      setSelectedExchangeType(template.exchange_type)
                    }
                    className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors ${
                      sel
                        ? 'bg-white/[0.07]'
                        : 'hover:bg-white/[0.04] disabled:cursor-not-allowed disabled:opacity-40'
                    }`}
                  >
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-zinc-700/80 bg-black/40">
                      {getExchangeIcon(template.exchange_type, {
                        width: 32,
                        height: 32,
                      })}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-semibold text-white">
                        {getShortName(template.name)}
                      </div>
                      <div className="truncate text-xs text-zinc-500">
                        {template.type === 'cex' ? 'CEX' : 'DEX'}
                      </div>
                    </div>
                    {sel ? (
                      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[#6D5AC8]">
                        <Check
                          className="h-3.5 w-3.5 text-white"
                          strokeWidth={3}
                        />
                      </div>
                    ) : (
                      <div className="h-6 w-6 shrink-0" />
                    )}
                  </button>
                )
              })}
            </div>
          </aside>

          {/* 右侧：表单 */}
          <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-nofx-bg">
            {selectedTemplate ? (
              <form
                onSubmit={handleSubmit}
                className="flex min-h-0 flex-1 flex-col"
              >
                <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-4 sm:p-5">
                  <div className="rounded-xl border border-zinc-800/80 bg-zinc-900/40 p-3">
                    <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">
                      <Shield className="h-4 w-4" />
                      {t('environmentSteps.checkTitle', language)}
                    </div>
                    <WebCryptoEnvironmentCheck
                      language={language}
                      variant="card"
                      onStatusChange={setWebCryptoStatus}
                    />
                  </div>

                  <div className="flex flex-wrap gap-2">
                    {exchangeRegistrationLinks[currentExchangeType || ''] && (
                      <a
                        href={
                          exchangeRegistrationLinks[currentExchangeType || '']
                            .url
                        }
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex flex-1 items-center justify-center gap-2 rounded-xl border border-zinc-700/80 bg-nofx-bg-secondary px-4 py-2.5 text-xs font-semibold text-zinc-200 transition-colors hover:border-violet-500/40 hover:bg-zinc-800/80"
                      >
                        <UserPlus className="h-3.5 w-3.5 text-violet-300" />
                        {t('exchangeConfig.register', language)}
                        {exchangeRegistrationLinks[currentExchangeType || '']
                          ?.hasReferral ? (
                          <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] text-emerald-400">
                            {t('exchangeConfig.bonus', language)}
                          </span>
                        ) : null}
                        <ExternalLink className="h-3.5 w-3.5 text-zinc-500" />
                      </a>
                    )}
                    {EXCHANGE_API_DOCS[currentExchangeType || ''] && (
                      <a
                        href={EXCHANGE_API_DOCS[currentExchangeType || '']}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex flex-1 items-center justify-center gap-2 rounded-xl border border-zinc-700/80 bg-nofx-bg-secondary px-4 py-2.5 text-xs font-semibold text-zinc-200 transition-colors hover:border-violet-500/40 hover:bg-zinc-800/80"
                      >
                        <Key className="h-3.5 w-3.5 text-violet-300" />
                        创建 API
                        <ExternalLink className="h-3.5 w-3.5 text-zinc-500" />
                      </a>
                    )}
                  </div>

                  {/* Account Name */}
                  <div className="space-y-2">
                    <label
                      className="flex items-center gap-2 text-sm font-semibold"
                      style={{ color: '#EAECEF' }}
                    >
                      <Key className="w-4 h-4" style={{ color: '#F0B90B' }} />
                      {t('exchangeConfig.accountName', language)} *
                    </label>
                    <input
                      type="text"
                      value={accountName}
                      onChange={(e) => setAccountName(e.target.value)}
                      placeholder={t(
                        'exchangeConfig.accountNamePlaceholder',
                        language
                      )}
                      className="w-full px-4 py-3 rounded-xl text-base"
                      style={{
                        background: '#0b0b0b',
                        border: '1px solid #2B3139',
                        color: '#EAECEF',
                      }}
                      required
                    />
                  </div>

                  {/* CEX Fields */}
                  {(currentExchangeType === 'binance' ||
                    currentExchangeType === 'bybit' ||
                    currentExchangeType === 'okx' ||
                    currentExchangeType === 'bitget' ||
                    currentExchangeType === 'gate' ||
                    currentExchangeType === 'kucoin' ||
                    currentExchangeType === 'indodax' ||
                    currentExchangeType === 'hz') && (
                    <>
                      {credentialFields.apiUrl && (
                        <div className="space-y-2">
                          <label
                            className="flex items-center gap-2 text-sm font-semibold"
                            style={{ color: '#EAECEF' }}
                          >
                            <ExternalLink
                              className="w-4 h-4"
                              style={{ color: '#F0B90B' }}
                            />
                            HZ API URL
                          </label>
                          <input
                            type="url"
                            value={apiUrl}
                            onChange={(e) => setApiUrl(e.target.value)}
                            placeholder="https://trade.kunai.fun/api/v1"
                            className="w-full px-4 py-3 rounded-xl"
                            style={{
                              background: '#0b0b0b',
                              border: '1px solid #2B3139',
                              color: '#EAECEF',
                            }}
                            required
                          />
                        </div>
                      )}
                      {currentExchangeType === 'binance' && (
                        <div
                          className="p-4 rounded-xl cursor-pointer transition-colors"
                          style={{
                            background: '#1a3a52',
                            border: '1px solid #2b5278',
                          }}
                          onClick={() => setShowBinanceGuide(!showBinanceGuide)}
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-2">
                              <span style={{ color: '#58a6ff' }}>ℹ️</span>
                              <span
                                className="text-sm font-medium"
                                style={{ color: '#EAECEF' }}
                              >
                                {t(
                                  'exchangeConfig.useBinanceFuturesApi',
                                  language
                                )}
                              </span>
                            </div>
                            <span style={{ color: '#8b949e' }}>
                              {showBinanceGuide ? '▲' : '▼'}
                            </span>
                          </div>
                          {showBinanceGuide && (
                            <div
                              className="mt-3 pt-3 text-sm"
                              style={{
                                borderTop: '1px solid #2b5278',
                                color: '#c9d1d9',
                              }}
                            >
                              <a
                                href="https://www.binance.com/zh-CN/support/faq/how-to-create-api-keys-on-binance-360002502072"
                                target="_blank"
                                rel="noopener noreferrer"
                                className="inline-flex items-center gap-1 hover:underline"
                                style={{ color: '#58a6ff' }}
                                onClick={(e) => e.stopPropagation()}
                              >
                                {t('exchangeConfig.viewTutorial', language)}{' '}
                                <ExternalLink className="w-3 h-3" />
                              </a>
                            </div>
                          )}
                        </div>
                      )}

                      <div className="space-y-2">
                        <label
                          className="flex items-center gap-2 text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          <Key
                            className="w-4 h-4"
                            style={{ color: '#F0B90B' }}
                          />
                          {t('apiKey', language)}
                        </label>
                        <input
                          type="password"
                          value={apiKey}
                          onChange={(e) => setApiKey(e.target.value)}
                          placeholder={t('enterAPIKey', language)}
                          className="w-full px-4 py-3 rounded-xl"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                          required={
                            currentExchangeType !== 'hz' || !editingExchangeId
                          }
                        />
                      </div>

                      <div className="space-y-2">
                        <label
                          className="flex items-center gap-2 text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          <Shield
                            className="w-4 h-4"
                            style={{ color: '#F0B90B' }}
                          />
                          {t('secretKey', language)}
                        </label>
                        <input
                          type="password"
                          value={secretKey}
                          onChange={(e) => setSecretKey(e.target.value)}
                          placeholder={t('enterSecretKey', language)}
                          className="w-full px-4 py-3 rounded-xl"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                          required={
                            currentExchangeType !== 'hz' || !editingExchangeId
                          }
                        />
                      </div>

                      {currentExchangeUsesDedicatedProxy && (
                        <div
                          className="p-4 rounded-xl"
                          style={{
                            background: 'rgba(240, 185, 11, 0.1)',
                            border: '1px solid rgba(240, 185, 11, 0.2)',
                          }}
                        >
                          <div
                            className="text-sm font-semibold mb-1"
                            style={{ color: '#F0B90B' }}
                          >
                            {t('whitelistIP', language)}
                          </div>
                          <p
                            className="text-xs mb-1 leading-relaxed"
                            style={{ color: '#848E9C' }}
                          >
                            {t('whitelistDualIntro', language)}
                          </p>
                          <p
                            className="text-xs mb-3 leading-relaxed"
                            style={{ color: 'rgba(132, 142, 156, 0.9)' }}
                          >
                            {t('binanceProxyAssignOnSaveNote', language)}
                          </p>

                          <div
                            className="mb-2 text-[11px] font-semibold uppercase tracking-wide"
                            style={{ color: '#B7BDC6' }}
                          >
                            {t('whitelistProxyLabel', language)}
                          </div>
                          {selectedExchange?.outbound_proxy_whitelist_host ? (
                            <div
                              className="mb-3 flex items-center gap-2 p-3 rounded-lg"
                              style={{ background: '#0b0b0b' }}
                            >
                              <code
                                className="flex-1 text-sm font-mono break-all"
                                style={{ color: '#F0B90B' }}
                              >
                                {selectedExchange.outbound_proxy_whitelist_host}
                              </code>
                              <button
                                type="button"
                                onClick={() =>
                                  handleCopyIP(
                                    selectedExchange.outbound_proxy_whitelist_host!
                                  )
                                }
                                className="flex shrink-0 items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-semibold transition-all hover:scale-105"
                                style={{
                                  background: 'rgba(240, 185, 11, 0.2)',
                                  color: '#F0B90B',
                                }}
                              >
                                <Copy className="w-3 h-3" />
                                {copiedProxyHint
                                  ? t('ipCopied', language)
                                  : t('copyIP', language)}
                              </button>
                            </div>
                          ) : (
                            <p
                              className="mb-3 text-xs leading-relaxed"
                              style={{ color: '#848E9C' }}
                            >
                              {t('binanceWhitelistPending', language)}
                            </p>
                          )}

                          {selectedExchange?.outbound_proxy_pool_expires_at ? (
                            <p
                              className="mt-1 text-[11px]"
                              style={{ color: 'rgba(252, 213, 53, 0.8)' }}
                            >
                              {t('binanceProxyExpiryNote', language)}
                              {selectedExchange.outbound_proxy_pool_expires_at}
                            </p>
                          ) : null}
                        </div>
                      )}

                      {(currentExchangeType === 'binance' ||
                        currentExchangeType === 'gate') && (
                        <label
                          className="flex cursor-pointer items-start gap-3 rounded-xl border p-4"
                          style={{
                            borderColor: 'rgba(240, 185, 11, 0.25)',
                            background: 'rgba(240, 185, 11, 0.06)',
                          }}
                        >
                          <input
                            type="checkbox"
                            checked={testnet}
                            onChange={(e) => setTestnet(e.target.checked)}
                            className="mt-1 h-4 w-4 accent-nofx-gold"
                          />
                          <div>
                            <div
                              className="text-sm font-semibold"
                              style={{ color: '#F0B90B' }}
                            >
                              {currentExchangeType === 'gate'
                                ? t('useTestnet', language)
                                : t('binanceDemoTrading', language)}
                            </div>
                            <p
                              className="mt-1 text-xs leading-relaxed"
                              style={{ color: '#848E9C' }}
                            >
                              {currentExchangeType === 'gate'
                                ? t('testnetDescription', language)
                                : t('binanceDemoTradingDesc', language)}
                            </p>
                            {currentExchangeType === 'gate' && (
                              <p
                                className="mt-1 text-[11px] leading-relaxed"
                                style={{ color: '#F0B90B' }}
                              >
                                Gate.io Futures Testnet API Key only; it cannot
                                be used with the live account.
                              </p>
                            )}
                          </div>
                        </label>
                      )}

                      {credentialFields.passphrase && (
                        <div className="space-y-2">
                          <label
                            className="flex items-center gap-2 text-sm font-semibold"
                            style={{ color: '#EAECEF' }}
                          >
                            <Key
                              className="w-4 h-4"
                              style={{ color: '#F0B90B' }}
                            />
                            {t('passphrase', language)}
                          </label>
                          <input
                            type="password"
                            value={passphrase}
                            onChange={(e) => setPassphrase(e.target.value)}
                            placeholder={t('enterPassphrase', language)}
                            className="w-full px-4 py-3 rounded-xl"
                            style={{
                              background: '#0b0b0b',
                              border: '1px solid #2B3139',
                              color: '#EAECEF',
                            }}
                            required
                          />
                        </div>
                      )}
                    </>
                  )}

                  {/* Aster Fields */}
                  {currentExchangeType === 'aster' && (
                    <>
                      <div
                        className="p-4 rounded-xl"
                        style={{
                          background: 'rgba(139, 92, 246, 0.1)',
                          border: '1px solid rgba(139, 92, 246, 0.3)',
                        }}
                      >
                        <div className="flex items-start gap-2">
                          <span style={{ fontSize: '16px' }}>🔐</span>
                          <div>
                            <div
                              className="text-sm font-semibold mb-1"
                              style={{ color: '#A78BFA' }}
                            >
                              {t('asterApiProTitle', language)}
                            </div>
                            <div
                              className="text-xs"
                              style={{ color: '#848E9C' }}
                            >
                              {t('asterApiProDesc', language)}
                            </div>
                          </div>
                        </div>
                      </div>
                      <div className="space-y-2">
                        <label
                          className="flex items-center gap-2 text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          {t('asterUserLabel', language)}
                          <Tooltip content={t('asterUserDesc', language)}>
                            <HelpCircle
                              className="w-4 h-4 cursor-help"
                              style={{ color: '#A78BFA' }}
                            />
                          </Tooltip>
                        </label>
                        <input
                          type="text"
                          value={asterUser}
                          onChange={(e) => setAsterUser(e.target.value)}
                          placeholder={t('enterAsterUser', language)}
                          className="w-full px-4 py-3 rounded-xl"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <label
                          className="flex items-center gap-2 text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          {t('asterSignerLabel', language)}
                          <Tooltip content={t('asterSignerDesc', language)}>
                            <HelpCircle
                              className="w-4 h-4 cursor-help"
                              style={{ color: '#A78BFA' }}
                            />
                          </Tooltip>
                        </label>
                        <input
                          type="text"
                          value={asterSigner}
                          onChange={(e) => setAsterSigner(e.target.value)}
                          placeholder={t('enterAsterSigner', language)}
                          className="w-full px-4 py-3 rounded-xl"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <label
                          className="flex items-center gap-2 text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          {t('asterPrivateKeyLabel', language)}
                          <Tooltip content={t('asterPrivateKeyDesc', language)}>
                            <HelpCircle
                              className="w-4 h-4 cursor-help"
                              style={{ color: '#A78BFA' }}
                            />
                          </Tooltip>
                        </label>
                        <input
                          type="password"
                          value={asterPrivateKey}
                          onChange={(e) => setAsterPrivateKey(e.target.value)}
                          placeholder={t('enterAsterPrivateKey', language)}
                          className="w-full px-4 py-3 rounded-xl"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                          required
                        />
                      </div>
                    </>
                  )}

                  {/* Hyperliquid Fields */}
                  {currentExchangeType === 'hyperliquid' && (
                    <>
                      <div
                        className="p-4 rounded-xl"
                        style={{
                          background: 'rgba(127, 231, 204, 0.1)',
                          border: '1px solid rgba(127, 231, 204, 0.3)',
                        }}
                      >
                        <div className="flex items-start gap-2">
                          <span style={{ fontSize: '16px' }}>🔐</span>
                          <div>
                            <div
                              className="text-sm font-semibold mb-1"
                              style={{ color: '#7FE7CC' }}
                            >
                              {t('hyperliquidAgentWalletTitle', language)}
                            </div>
                            <div
                              className="text-xs"
                              style={{ color: '#848E9C' }}
                            >
                              {t('hyperliquidAgentWalletDesc', language)}
                            </div>
                          </div>
                        </div>
                      </div>
                      <div className="space-y-2">
                        <label
                          className="text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          {t('hyperliquidAgentPrivateKey', language)}
                        </label>
                        <div className="flex flex-col gap-2 sm:flex-row">
                          <input
                            type="text"
                            value={maskSecret(apiKey)}
                            readOnly
                            placeholder={t(
                              'enterHyperliquidAgentPrivateKey',
                              language
                            )}
                            className="flex-1 px-4 py-3 rounded-xl"
                            style={{
                              background: '#0b0b0b',
                              border: '1px solid #2B3139',
                              color: '#EAECEF',
                            }}
                          />
                          <button
                            type="button"
                            onClick={() => setSecureInputTarget('hyperliquid')}
                            className="px-4 py-3 rounded-xl text-sm font-semibold transition-all hover:scale-105"
                            style={{ background: '#7FE7CC', color: '#000' }}
                          >
                            {apiKey
                              ? t('secureInputReenter', language)
                              : t('secureInputButton', language)}
                          </button>
                        </div>
                      </div>
                      <div className="space-y-2">
                        <label
                          className="text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          {t('hyperliquidMainWalletAddress', language)}
                        </label>
                        <input
                          type="text"
                          value={hyperliquidWalletAddr}
                          onChange={(e) =>
                            setHyperliquidWalletAddr(e.target.value)
                          }
                          placeholder={t(
                            'enterHyperliquidMainWalletAddress',
                            language
                          )}
                          className="w-full px-4 py-3 rounded-xl"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                          required
                        />
                      </div>
                    </>
                  )}

                  {/* Lighter Fields */}
                  {currentExchangeType === 'lighter' && (
                    <>
                      <div
                        className="p-4 rounded-xl"
                        style={{
                          background: 'rgba(59, 130, 246, 0.1)',
                          border: '1px solid rgba(59, 130, 246, 0.3)',
                        }}
                      >
                        <div className="flex items-start gap-2">
                          <span style={{ fontSize: '16px' }}>🔐</span>
                          <div>
                            <div
                              className="text-sm font-semibold mb-1"
                              style={{ color: '#3B82F6' }}
                            >
                              {t('exchangeConfig.lighterApiKeySetup', language)}
                            </div>
                            <div
                              className="text-xs"
                              style={{ color: '#848E9C' }}
                            >
                              {t('exchangeConfig.lighterApiKeyDesc', language)}
                            </div>
                          </div>
                        </div>
                      </div>
                      <div className="space-y-2">
                        <label
                          className="text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          {t('lighterWalletAddress', language)} *
                        </label>
                        <input
                          type="text"
                          value={lighterWalletAddr}
                          onChange={(e) => setLighterWalletAddr(e.target.value)}
                          placeholder={t('enterLighterWalletAddress', language)}
                          className="w-full px-4 py-3 rounded-xl"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <label
                          className="flex items-center gap-2 text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          {t('lighterApiKeyPrivateKey', language)} *
                          <button
                            type="button"
                            onClick={() => setSecureInputTarget('lighter')}
                            className="text-xs underline"
                            style={{ color: '#3B82F6' }}
                          >
                            {t('secureInputButton', language)}
                          </button>
                        </label>
                        <input
                          type="password"
                          value={lighterApiKeyPrivateKey}
                          onChange={(e) =>
                            setLighterApiKeyPrivateKey(e.target.value)
                          }
                          placeholder={t(
                            'enterLighterApiKeyPrivateKey',
                            language
                          )}
                          className="w-full px-4 py-3 rounded-xl font-mono"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <label
                          className="flex items-center gap-2 text-sm font-semibold"
                          style={{ color: '#EAECEF' }}
                        >
                          {t('exchangeConfig.apiKeyIndex', language)}
                          <Tooltip
                            content={t(
                              'exchangeConfig.apiKeyIndexTooltip',
                              language
                            )}
                          >
                            <HelpCircle
                              className="w-4 h-4 cursor-help"
                              style={{ color: '#3B82F6' }}
                            />
                          </Tooltip>
                        </label>
                        <input
                          type="number"
                          min={0}
                          max={255}
                          value={lighterApiKeyIndex}
                          onChange={(e) =>
                            setLighterApiKeyIndex(parseInt(e.target.value) || 0)
                          }
                          className="w-full px-4 py-3 rounded-xl"
                          style={{
                            background: '#0b0b0b',
                            border: '1px solid #2B3139',
                            color: '#EAECEF',
                          }}
                        />
                      </div>
                    </>
                  )}
                </div>

                <div className="flex shrink-0 flex-col-reverse gap-3 border-t border-zinc-800/90 bg-nofx-bg px-4 py-3 sm:flex-row sm:justify-end sm:px-5 sm:py-4">
                  <button
                    type="button"
                    onClick={onClose}
                    className="rounded-xl bg-zinc-800 px-5 py-2.5 text-sm font-semibold text-zinc-200 transition-colors hover:bg-zinc-700"
                  >
                    取消
                  </button>
                  <button
                    type="submit"
                    disabled={isSaving || !accountName.trim()}
                    className="inline-flex items-center justify-center gap-2 rounded-xl bg-[#6D5AC8] px-6 py-2.5 text-sm font-bold text-white shadow-lg shadow-violet-900/20 transition-colors hover:bg-[#5d4cb8] disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    {isSaving ? (
                      t('saving', language)
                    ) : (
                      <>
                        {t('saveConfig', language)}
                        <ArrowRight className="h-4 w-4" />
                      </>
                    )}
                  </button>
                </div>
              </form>
            ) : (
              <div className="flex flex-1 items-center justify-center p-8 text-sm text-zinc-500">
                请从左侧选择一个交易所
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Binance Guide Modal */}
      {showGuide && (
        <div
          className="fixed inset-0 bg-black/75 flex items-center justify-center z-50 p-4"
          onClick={() => setShowGuide(false)}
        >
          <div
            className="w-full max-w-4xl rounded-2xl p-4 sm:p-6"
            style={{ background: '#1c1c1c' }}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-4">
              <h3
                className="text-xl font-bold flex items-center gap-2"
                style={{ color: '#EAECEF' }}
              >
                <BookOpen className="w-6 h-6" style={{ color: '#F0B90B' }} />
                {t('binanceSetupGuide', language)}
              </h3>
              <button
                onClick={() => setShowGuide(false)}
                className="px-4 py-2 rounded-lg text-sm font-semibold"
                style={{ background: '#2B3139', color: '#848E9C' }}
              >
                {t('closeGuide', language)}
              </button>
            </div>
            <div className="overflow-y-auto max-h-[80vh]">
              <img
                src="/images/guide.png"
                alt={t('binanceSetupGuide', language)}
                className="w-full h-auto rounded-lg"
              />
            </div>
          </div>
        </div>
      )}

      {/* Secure Input Modal */}
      <TwoStageKeyModal
        isOpen={secureInputTarget !== null}
        language={language}
        contextLabel={secureInputContextLabel}
        expectedLength={64}
        onCancel={() => setSecureInputTarget(null)}
        onComplete={handleSecureInputComplete}
      />
    </div>
  )
}
