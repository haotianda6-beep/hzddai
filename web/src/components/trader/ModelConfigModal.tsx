import React, { useState, useEffect, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { QRCodeSVG } from 'qrcode.react'
import { Trash2, Brain, ExternalLink, Check, Eye, EyeOff } from 'lucide-react'
import type { AIModel } from '../../types'
import type { Language } from '../../i18n/translations'
import { t } from '../../i18n/translations'
import { api } from '../../lib/api'
import { getModelIcon } from '../common/ModelIcons'
import {
  CLAW402_MODELS,
  AI_PROVIDER_CONFIG,
  getShortName,
} from './model-constants'
import { getBeginnerWalletAddress, getUserMode } from '../../lib/onboarding'
import { ROUTES } from '../../router/paths'

interface ModelConfigModalProps {
  allModels: AIModel[]
  configuredModels: AIModel[]
  editingModelId: string | null
  onSave: (
    modelId: string,
    apiKey: string,
    baseUrl?: string,
    modelName?: string
  ) => void
  onDelete: (modelId: string) => void
  onClose: () => void
  language: Language
}

export function ModelConfigModal({
  allModels,
  configuredModels,
  editingModelId,
  onSave,
  onDelete,
  onClose,
  language: _languageFromParent,
}: ModelConfigModalProps) {
  const navigate = useNavigate()
  /** 设置里「AI 模型」统一中文，避免界面语言为英文时仍全英 */
  const language = 'zh' satisfies Language
  void _languageFromParent

  const [selectedModelId, setSelectedModelId] = useState(editingModelId || '')
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [modelName, setModelName] = useState('')
  const configuredModel = useMemo(
    () =>
      configuredModels?.find((model) => model.id === selectedModelId) ||
      configuredModels?.find((model) => model.provider === selectedModelId) ||
      null,
    [configuredModels, selectedModelId]
  )

  // Always prefer allModels (supportedModels) for provider/id lookup;
  // fall back to configuredModels for edit mode details (apiKey etc.)
  const selectedModel = useMemo(
    () =>
      allModels?.find((m) => m.id === selectedModelId) ||
      allModels?.find((m) => m.provider === selectedModelId) ||
      configuredModel,
    [allModels, configuredModel, selectedModelId]
  )

  useEffect(() => {
    setApiKey(configuredModel?.apiKey || '')
    setBaseUrl(configuredModel?.customApiUrl || '')
    setModelName(configuredModel?.customModelName || '')
  }, [configuredModel, selectedModelId])

  const availableModels = allModels || []
  const sortedSidebarModels = useMemo(() => {
    const tier = (m: AIModel) => {
      if (m.provider === 'comkun_ai' || m.id === 'comkun_ai') return 0
      if (m.provider === 'comkun_proxy' || m.id === 'comkun_proxy') return 1
      if (m.provider === 'claw402' || m.id === 'claw402') return 2
      return 2
    }
    const list = [...availableModels]
    list.sort((a, b) => {
      const ac = tier(a)
      const bc = tier(b)
      if (ac !== bc) return ac - bc
      return (a.name || '').localeCompare(b.name || '')
    })
    return list
  }, [availableModels])

  useEffect(() => {
    if (!editingModelId) return
    const editingModel = configuredModels?.find((model) => model.id === editingModelId)
    const supportedModel = sortedSidebarModels.find(
      (model) => model.id === editingModelId || model.provider === editingModel?.provider
    )
    setSelectedModelId(supportedModel?.id || editingModelId)
  }, [configuredModels, editingModelId, sortedSidebarModels])

  useEffect(() => {
    if (editingModelId) return
    if (!selectedModelId && sortedSidebarModels.length > 0) {
      setSelectedModelId(sortedSidebarModels[0].id)
    }
  }, [editingModelId, sortedSidebarModels, selectedModelId])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedModelId) return
    const isComkun =
      selectedModel?.provider === 'comkun_ai' ||
      selectedModel?.id === 'comkun_ai' ||
      selectedModel?.provider === 'comkun_proxy' ||
      selectedModel?.id === 'comkun_proxy'
    let key = apiKey.trim()
    // Allow empty key only when the selected provider already has a saved key.
    if (!key && !configuredModel) {
      if (isComkun) {
        key = 'comkun-ai-placeholder'
      } else {
        return
      }
    }
    onSave(selectedModelId, key, baseUrl.trim() || undefined, modelName.trim() || undefined)
  }

  const configuredKeys = new Set(
    configuredModels?.flatMap((m) => [m.id, m.provider].filter(Boolean)) || []
  )
  const isClaw402Selected = selectedModel?.provider === 'claw402' || selectedModel?.id === 'claw402'
  const isBeginnerDefaultModel = isClaw402Selected && getUserMode() === 'beginner'
  const MODEL_FORM_ID = 'nofx-model-config-sheet-form'

  const providerSubtitle = (m: AIModel) => {
    const cfg = AI_PROVIDER_CONFIG[m.provider]
    if (m.provider === 'comkun_proxy' || m.id === 'comkun_proxy') return '平台余额扣费 · COMKUN 多模型代理'
    if (m.provider === 'claw402' || m.id === 'claw402') return 'USDC 按次 · 多模型聚合'
    if (m.provider === 'comkun_ai' || m.id === 'comkun_ai' || m.provider === 'ai') {
      return '策略市场跟单 · comkun-ai-follow'
    }
    return cfg?.apiName || m.provider
  }

  return (
    <div className="fixed inset-0 z-[55] flex items-start justify-center overflow-y-auto bg-black/70 p-2 backdrop-blur-sm sm:items-center sm:p-4">
      <div
        className="flex w-full max-w-5xl flex-col overflow-hidden rounded-2xl border border-zinc-800/90 bg-nofx-bg shadow-2xl"
        style={{ maxHeight: 'min(90vh, 900px)' }}
      >
        {/* 顶栏 */}
        <div className="flex shrink-0 items-center justify-between border-b border-zinc-800/90 px-4 py-3 sm:px-5 sm:py-4">
          <h2 className="text-lg font-bold text-white">
            {editingModelId ? t('editAIModel', language) : t('addAIModel', language)}
          </h2>
          <div className="flex items-center gap-1">
            {editingModelId && !isBeginnerDefaultModel && (
              <button
                type="button"
                onClick={() => onDelete(editingModelId)}
                className="rounded-lg p-2 text-red-400 transition-colors hover:bg-red-500/15"
                title="删除"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            )}
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg p-2 text-zinc-400 transition-colors hover:bg-white/10 hover:text-white"
              aria-label="关闭"
            >
              ✕
            </button>
          </div>
        </div>

        <div className="flex min-h-0 flex-1 flex-col md:flex-row">
          {/* 左侧：模型列表 */}
          <aside className="flex max-h-[38vh] shrink-0 flex-col border-b border-zinc-800/90 md:max-h-none md:w-[280px] md:border-b-0 md:border-r md:border-zinc-800/90 bg-nofx-bg-tertiary">
            <p className="px-4 pb-2 pt-4 text-xs font-medium uppercase tracking-wide text-zinc-500">
              AI 模型
            </p>
            <div className="min-h-0 flex-1 space-y-1 overflow-y-auto px-2 pb-3">
              {sortedSidebarModels.map((m) => {
                const sel = selectedModelId === m.id
                const isClaw = m.provider === 'claw402' || m.id === 'claw402'
                const isComkun = m.provider === 'comkun_ai' || m.id === 'comkun_ai'
                const isProxy = m.provider === 'comkun_proxy' || m.id === 'comkun_proxy'
                return (
                  <button
                    key={m.id}
                    type="button"
                    onClick={() => setSelectedModelId(m.id)}
                    className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors ${
                      sel ? 'bg-white/[0.07]' : 'hover:bg-white/[0.04]'
                    }`}
                  >
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-zinc-700/80 bg-black/40">
                      {isProxy || isComkun ? (
                        <img src="/icons/comkun_ai.svg" alt="" width={32} height={32} className="object-contain" />
                      ) : isClaw ? (
                        <img src="/icons/claw402.svg" alt="" width={32} height={32} />
                      ) : isComkun ? (
                        <img src="/icons/comkun-ai.png" alt="" width={32} height={32} className="object-contain" />
                      ) : (
                        getModelIcon(m.provider || m.id, { width: 28, height: 28 }) || (
                          <span className="text-sm font-bold text-zinc-400">{m.name[0]}</span>
                        )
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-semibold text-white">
                        {getShortName(m.name)}
                      </div>
                      <div className="truncate text-xs text-zinc-500">{providerSubtitle(m)}</div>
                      {(configuredKeys.has(m.id) || configuredKeys.has(m.provider)) && (
                        <span className="mt-0.5 inline-block rounded-full border border-emerald-500/25 bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-400">
                          已配置
                        </span>
                      )}
                    </div>
                    {sel ? (
                      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[#6D5AC8]">
                        <Check className="h-3.5 w-3.5 text-white" strokeWidth={3} />
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
            {selectedModel &&
            (selectedModel.provider === 'comkun_proxy' || selectedModel.id === 'comkun_proxy') ? (
              <ComkunProxyConfigForm
                formId={MODEL_FORM_ID}
                modelName={modelName}
                editingModelId={editingModelId}
                onModelNameChange={setModelName}
                onCancel={onClose}
                onSubmit={handleSubmit}
                language={language}
              />
            ) : selectedModel &&
            (selectedModel.provider === 'claw402' || selectedModel.id === 'claw402') ? (
              <Claw402ConfigForm
                formId={MODEL_FORM_ID}
                apiKey={apiKey}
                modelName={modelName}
                configuredModel={configuredModel}
                editingModelId={editingModelId}
                onApiKeyChange={setApiKey}
                onModelNameChange={setModelName}
                onCancel={onClose}
                onSubmit={handleSubmit}
                language={language}
              />
            ) : selectedModel ? (
              <>
                <div className="shrink-0 border-b border-zinc-800/80 px-4 py-3 sm:px-5 sm:py-4">
                  <div className="flex flex-wrap items-start gap-4">
                    <div className="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-zinc-700/80 bg-black/50">
                      {selectedModel.provider === 'comkun_ai' || selectedModel.id === 'comkun_ai' ? (
                        <img src="/icons/comkun-ai.png" alt="" width={40} height={40} className="object-contain" />
                      ) : getModelIcon(selectedModel.provider || selectedModel.id, {
                        width: 32,
                        height: 32,
                      }) || (
                        <span className="text-lg font-bold text-zinc-400">
                          {selectedModel.name[0]}
                        </span>
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <h3 className="text-lg font-bold text-white">
                        {getShortName(selectedModel.name)}
                      </h3>
                      <p className="text-xs text-zinc-500">
                        {AI_PROVIDER_CONFIG[selectedModel.provider]?.apiName ||
                          selectedModel.provider}{' '}
                        ·{' '}
                        {AI_PROVIDER_CONFIG[selectedModel.provider]?.defaultModel ||
                          selectedModel.id}
                      </p>
                    </div>
                    {selectedModel.provider === 'comkun_ai' || selectedModel.id === 'comkun_ai' ? (
                      <button
                        type="button"
                        onClick={() => {
                          onClose()
                          navigate(ROUTES.recharge)
                        }}
                        className="w-full shrink-0 rounded-lg border border-[#558b2f]/45 bg-[#1b2218]/80 px-4 py-2 text-sm font-semibold text-[#d4ff33] transition-colors hover:border-[#d4ff33]/55 hover:bg-[#243018] sm:w-auto"
                      >
                        充值平台余额
                      </button>
                    ) : !!AI_PROVIDER_CONFIG[selectedModel.provider]?.apiUrl ? (
                      <a
                        href={AI_PROVIDER_CONFIG[selectedModel.provider].apiUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="shrink-0 text-sm font-medium text-[#a78bfa] hover:text-[#c4b5fd] hover:underline"
                      >
                        官方网站 →
                      </a>
                    ) : null}
                  </div>
                </div>
                <StandardProviderConfigForm
                  formId={MODEL_FORM_ID}
                  embedChrome
                  isComkunProvider={
                    selectedModel.provider === 'comkun_ai' || selectedModel.id === 'comkun_ai'
                  }
                  selectedModel={selectedModel}
                  apiKey={apiKey}
                  baseUrl={baseUrl}
                  modelName={modelName}
                  editingModelId={editingModelId}
                  onApiKeyChange={setApiKey}
                  onBaseUrlChange={setBaseUrl}
                  onModelNameChange={setModelName}
                  onCancel={onClose}
                  onSubmit={handleSubmit}
                  language={language}
                />
              </>
            ) : (
              <div className="flex flex-1 items-center justify-center p-8 text-sm text-zinc-500">
                请从左侧选择一个模型
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// --- Sub-components for ModelConfigModal ---

function comkunProxyModelLogo(provider: string, id: string): string {
  const p = provider.toLowerCase()
  const m = id.toLowerCase()
  if (p.includes('openai') || m.includes('gpt')) return '/icons/openai.svg'
  if (p.includes('anthropic') || m.includes('claude')) return '/icons/claude.svg'
  if (p.includes('deepseek')) return '/icons/deepseek.svg'
  if (p.includes('alibaba') || m.includes('qwen')) return '/icons/qwen.svg'
  if (p.includes('moonshot') || m.includes('kimi')) return '/icons/kimi.svg'
  if (p.includes('google') || m.includes('gemini')) return '/icons/gemini.svg'
  if (p.includes('xai') || m.includes('grok')) return '/icons/grok.svg'
  return '/icons/comkun_ai.svg'
}

function ComkunProxyConfigForm({
  formId,
  modelName,
  editingModelId,
  onModelNameChange,
  onCancel,
  onSubmit,
  language,
}: {
  formId: string
  modelName: string
  editingModelId: string | null
  onModelNameChange: (value: string) => void
  onCancel: () => void
  onSubmit: (e: React.FormEvent) => void
  language: Language
}) {
  const selected = modelName || 'glm-5'
  return (
    <form id={formId} onSubmit={onSubmit} className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-5">
        <div className="rounded-xl border border-[#d4ff33]/20 bg-[#d4ff33]/5 p-4">
          <div className="flex items-center gap-3">
            <img src="/icons/comkun_ai.svg" alt="" width={40} height={40} />
            <div>
              <h3 className="text-lg font-bold text-white">COMKUN-AI 代理模型</h3>
              <p className="text-xs text-zinc-400">由平台统一管理 COMKUN 代理支付，费用从平台余额扣除，无需用户填写钱包私钥。</p>
            </div>
          </div>
        </div>

        <div>
          <div className="mb-2 text-sm font-semibold text-zinc-200">选择底层模型</div>
          <div className="grid gap-2 sm:grid-cols-2">
            {CLAW402_MODELS.map((m) => (
              <button
                key={m.id}
                type="button"
                onClick={() => onModelNameChange(m.id)}
                className={`rounded-xl border p-3 text-left transition-colors ${
                  selected === m.id
                    ? 'border-[#d4ff33]/70 bg-[#d4ff33]/15'
                    : 'border-zinc-800 bg-black/20 hover:border-[#d4ff33]/35'
                }`}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="flex items-center gap-2 text-sm font-bold text-white">
                    <img src={comkunProxyModelLogo(m.provider, m.id)} alt="" className="h-5 w-5 rounded-sm" />
                    {m.name}
                  </span>
                  <span className="text-xs text-[#d4ff33]">{m.desc}</span>
                </div>
                <div className="mt-1 text-xs text-zinc-500">{m.provider}</div>
              </button>
            ))}
          </div>
        </div>

        <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-4 text-xs leading-relaxed text-emerald-100/80">
          每次调用会先读取 COMKUN 代理实际报价，再按平台规则从平台余额扣费；余额不足时会提示充值，不会继续付款调用。
        </div>
      </div>

      <div className="flex shrink-0 flex-col-reverse gap-3 border-t border-zinc-800/90 bg-nofx-bg px-4 py-3 sm:flex-row sm:justify-end sm:px-5 sm:py-4">
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl bg-zinc-800 px-5 py-2.5 text-sm font-semibold text-zinc-200 transition-colors hover:bg-zinc-700"
        >
          取消
        </button>
        <button
          type="submit"
          className="rounded-xl bg-[#6D5AC8] px-6 py-2.5 text-sm font-bold text-white shadow-lg shadow-violet-900/20 transition-colors hover:bg-[#5d4cb8]"
        >
          {editingModelId ? t('saveConfig', language) : t('modelConfig.startTrading', language)}
        </button>
      </div>
    </form>
  )
}

function Claw402ConfigForm({
  formId,
  apiKey,
  modelName,
  configuredModel,
  editingModelId,
  onApiKeyChange,
  onModelNameChange,
  onCancel,
  onSubmit,
  language,
}: {
  formId: string
  apiKey: string
  modelName: string
  configuredModel: AIModel | null
  editingModelId: string | null
  onApiKeyChange: (value: string) => void
  onModelNameChange: (value: string) => void
  onCancel: () => void
  onSubmit: (e: React.FormEvent) => void
  language: Language
}) {
  const [walletAddress, setWalletAddress] = useState('')
  const [copiedAddr, setCopiedAddr] = useState(false)
  const [showDeposit, setShowDeposit] = useState(false)
  const [usdcBalance, setUsdcBalance] = useState<string | null>(null)
  const [keyError, setKeyError] = useState('')
  const [validating, setValidating] = useState(false)
  const [claw402Status, setClaw402Status] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<{ status: string; message: string } | null>(null)
  const [testing, setTesting] = useState(false)
  const [serverWalletAddress, setServerWalletAddress] = useState('')
  const [serverWalletBalance, setServerWalletBalance] = useState<string | null>(null)
  const localWalletAddress = getBeginnerWalletAddress()?.trim() || ''
  const configuredWalletAddress =
    configuredModel?.walletAddress?.trim() || localWalletAddress || serverWalletAddress
  const resolvedWalletAddress = walletAddress || configuredWalletAddress
  const resolvedUsdcBalance =
    usdcBalance ?? configuredModel?.balanceUsdc ?? serverWalletBalance ?? null
  const hasExistingWallet = Boolean(configuredWalletAddress)

  // Client-side validation helper
  const getClientError = (key: string): string => {
    if (!key) return ''
    if (!key.startsWith('0x')) return t('modelConfig.invalidKeyPrefix', language)
    if (key.length !== 66) return `${t('modelConfig.invalidKeyLength', language)} ${key.length}`
    if (!/^0x[0-9a-fA-F]{64}$/.test(key)) return t('modelConfig.invalidKeyChars', language)
    return ''
  }

  const isKeyValid = apiKey.length === 66 && apiKey.startsWith('0x') && /^0x[0-9a-fA-F]{64}$/.test(apiKey)

  useEffect(() => {
    if (hasExistingWallet) {
      setShowDeposit(true)
    }
  }, [hasExistingWallet])

  useEffect(() => {
    if (configuredModel?.walletAddress || localWalletAddress || serverWalletAddress) {
      return
    }

    let cancelled = false
    void api
      .getCurrentBeginnerWallet()
      .then((result) => {
        setClaw402Status(result.claw402_status || 'unknown')
        if (cancelled || !result.found || !result.address) {
          return
        }
        setServerWalletAddress(result.address)
        setServerWalletBalance(result.balance_usdc || null)
      })
      .catch(() => {
        // Ignore silently: this is a best-effort fallback for showing the current wallet.
      })

    return () => {
      cancelled = true
    }
  }, [configuredModel?.walletAddress, localWalletAddress, serverWalletAddress])

  // Debounced validation when apiKey changes
  useEffect(() => {
    setWalletAddress('')
    setUsdcBalance(null)
    setClaw402Status(null)
    setTestResult(null)

    const clientErr = getClientError(apiKey)
    setKeyError(clientErr)

    if (clientErr || !apiKey) {
      setValidating(false)
      return
    }

    setValidating(true)
    const timer = setTimeout(async () => {
      try {
        const res = await fetch('/api/wallet/validate', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ private_key: apiKey }),
        })
        const data = await res.json()
        if (data.valid) {
          setWalletAddress(data.address || '')
          setUsdcBalance(data.balance_usdc || '0.00')
          setClaw402Status(data.claw402_status || 'unknown')
          setKeyError('')
        } else {
          setKeyError(data.error || t('modelConfig.invalidKeyGeneric', language))
        }
      } catch {
        setKeyError(t('modelConfig.validationRequestFailed', language))
      } finally {
        setValidating(false)
      }
    }, 500)

    return () => clearTimeout(timer)
  }, [apiKey])

  const handleTestConnection = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      if (!apiKey && hasExistingWallet) {
        const result = await api.getCurrentBeginnerWallet()
        setClaw402Status(result.claw402_status || 'unknown')
        if (result.found && result.address) {
          setWalletAddress(result.address)
          setUsdcBalance(result.balance_usdc || '0.00')
          setShowDeposit(true)
        }
        setTestResult({
          status: result.claw402_status === 'ok' ? 'ok' : 'error',
          message: result.claw402_status === 'ok'
            ? t('modelConfig.claw402Connected', language)
            : t('modelConfig.claw402Unreachable', language),
        })
        return
      }

      const res = await fetch('/api/wallet/validate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ private_key: apiKey }),
      })
      const data = await res.json()
      if (data.valid) {
        setWalletAddress(data.address || '')
        setUsdcBalance(data.balance_usdc || '0.00')
        setClaw402Status(data.claw402_status || 'unknown')
        if (parseFloat(data.balance_usdc || '0') === 0) setShowDeposit(true)
        setTestResult({
          status: data.claw402_status === 'ok' ? 'ok' : 'error',
          message: data.claw402_status === 'ok'
            ? t('modelConfig.claw402Connected', language)
            : t('modelConfig.claw402Unreachable', language),
        })
      } else {
        setTestResult({
          status: 'error',
          message: data.error || t('modelConfig.invalidKeyGeneric', language),
        })
      }
    } catch {
      setTestResult({ status: 'error', message: t('modelConfig.claw402Unreachable', language) })
    } finally {
      setTesting(false)
    }
  }

  const balanceNum = resolvedUsdcBalance ? parseFloat(resolvedUsdcBalance) : 0

  return (
    <form
      id={formId}
      onSubmit={onSubmit}
      className="flex min-h-0 min-w-0 flex-1 flex-col"
    >
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-5">
      {/* COMKUN 代理模型旧配置兼容页 */}
      <div className="p-5 rounded-xl text-center" style={{ background: 'linear-gradient(135deg, rgba(212, 255, 51, 0.12) 0%, rgba(212, 255, 51, 0.12) 100%)', border: '1px solid rgba(212, 255, 51, 0.3)' }}>
        <div className="w-14 h-14 mx-auto rounded-2xl flex items-center justify-center mb-3 overflow-hidden">
          <img src="/icons/comkun_ai.svg" alt="COMKUN" width={56} height={56} />
        </div>
        <a href="https://claw402.ai" target="_blank" rel="noopener noreferrer" className="text-lg font-bold inline-flex items-center gap-1.5 hover:underline" style={{ color: '#EAECEF' }}>
          COMKUN 代理 <span className="text-xs font-normal" style={{ color: '#dce76a' }}>↗</span>
        </a>
        <div className="text-sm mt-1" style={{ color: '#A0AEC0' }}>
          {t('modelConfig.allModelsClaw', language)}
        </div>
        <div className="flex items-center justify-center gap-3 mt-3 flex-wrap">
          {['GPT', 'Claude', 'DeepSeek', 'Gemini', 'Grok', 'Qwen', 'Kimi'].map(name => (
            <span key={name} className="text-[11px] px-2 py-0.5 rounded-full" style={{ background: 'rgba(255,255,255,0.06)', color: '#A0AEC0' }}>
              {name}
            </span>
          ))}
        </div>
        <div className="mt-4 flex items-center justify-center gap-3 flex-wrap">
          <button
            type="button"
            onClick={handleTestConnection}
            disabled={testing || (!hasExistingWallet && !isKeyValid)}
            className="inline-flex items-center gap-2 rounded-xl px-4 py-2 text-xs font-semibold transition-all hover:scale-[1.02] disabled:cursor-not-allowed disabled:opacity-50"
            style={{ background: 'rgba(212, 255, 51, 0.15)', border: '1px solid rgba(212, 255, 51, 0.3)', color: '#dce76a' }}
          >
            <span>🔗</span>
            {testing ? t('modelConfig.testingConnection', language) : t('modelConfig.testConnection', language)}
          </button>
          {claw402Status ? (
            <div className="text-xs" style={{ color: claw402Status === 'ok' ? '#00E096' : '#F59E0B' }}>
              {claw402Status === 'ok'
                ? t('modelConfig.claw402Connected', language)
                : t('modelConfig.claw402Unreachable', language)}
            </div>
          ) : null}
        </div>
      </div>

      {/* Step 1: Select AI Model */}
      <div className="space-y-3">
        <label className="flex items-center gap-2 text-sm font-semibold" style={{ color: '#EAECEF' }}>
          <Brain className="w-4 h-4" style={{ color: '#c4cf45' }} />
          {t('modelConfig.selectAiModel', language)}
        </label>
        <div className="text-xs mb-2" style={{ color: '#848E9C' }}>
          {t('modelConfig.allModelsUnified', language)}
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          {CLAW402_MODELS.map((m) => {
            const isSelected = (modelName || 'glm-5') === m.id
            return (
              <button
                key={m.id}
                type="button"
                onClick={() => onModelNameChange(m.id)}
                className="flex items-start gap-2 px-3 py-2.5 rounded-xl text-left transition-all hover:scale-[1.02]"
                style={{
                  background: isSelected ? 'rgba(212, 255, 51, 0.2)' : '#0b0b0b',
                  border: isSelected ? '1.5px solid #c4cf45' : '1px solid #2B3139',
                }}
              >
                <span className="text-base mt-0.5">{m.icon}</span>
                <div className="flex-1 min-w-0">
                  <div className="text-xs font-semibold truncate" style={{ color: isSelected ? '#dce76a' : '#EAECEF' }}>
                    {m.name}
                  </div>
                  <div className="text-[10px] truncate" style={{ color: '#848E9C' }}>
                    {m.provider} · 参考价 {m.desc}
                  </div>
                  <div className="text-[10px]" style={{ color: '#00E096' }}>
                    约 ${m.price} / 次调用
                  </div>
                </div>
                {isSelected && (
                  <span className="text-[10px] mt-1" style={{ color: '#dce76a' }}>✓</span>
                )}
              </button>
            )
          })}
        </div>
      </div>

      {/* Step 2: Wallet Setup */}
      <div className="space-y-3">
        <label className="flex items-center gap-2 text-sm font-semibold" style={{ color: '#EAECEF' }}>
          <svg className="w-4 h-4" style={{ color: '#c4cf45' }} fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z" />
          </svg>
          {t('modelConfig.setupWallet', language)}
        </label>

        <div className="p-3 rounded-xl" style={{ background: 'rgba(212, 255, 51, 0.06)', border: '1px solid rgba(212, 255, 51, 0.15)' }}>
          <div className="text-xs mb-2" style={{ color: '#A0AEC0' }}>
            {t('modelConfig.walletInfo', language)}
          </div>
          <div className="text-xs space-y-1" style={{ color: '#848E9C' }}>
            <div className="flex items-center gap-1.5">
              <span style={{ color: '#00E096' }}>•</span>
              {t('modelConfig.exportKey', language)}
            </div>
            <div className="flex items-center gap-1.5">
              <span style={{ color: '#00E096' }}>•</span>
              {t('modelConfig.dedicatedWallet', language)}
            </div>
          </div>
        </div>

        {hasExistingWallet && (
          <div className="p-3 rounded-xl" style={{ background: 'rgba(0, 224, 150, 0.05)', border: '1px solid rgba(0, 224, 150, 0.18)' }}>
            <div className="text-xs font-semibold mb-1.5" style={{ color: '#00E096' }}>
              {language === 'zh' ? '已自动提取当前钱包' : 'Current wallet loaded automatically'}
            </div>
            <div className="text-[11px] leading-5" style={{ color: '#A0AEC0' }}>
              {language === 'zh'
                ? '你现在可以直接查看当前钱包地址、余额和充值二维码。只有在想更换钱包时，才需要重新输入新的私钥。'
                : 'You can view the current wallet address, balance, and deposit QR code right away. Only enter a new private key if you want to replace this wallet.'}
            </div>
            {!configuredModel?.walletAddress && localWalletAddress ? (
              <div className="mt-2 text-[10px]" style={{ color: '#848E9C' }}>
                {language === 'zh'
                  ? '当前地址来自本地已保存的新手钱包。'
                  : 'This address comes from the locally saved beginner wallet.'}
              </div>
            ) : null}
            {!configuredModel?.walletAddress && !localWalletAddress && serverWalletAddress ? (
              <div className="mt-2 text-[10px]" style={{ color: '#848E9C' }}>
                {language === 'zh'
                  ? '当前地址来自后端保存的钱包配置。'
                  : 'This address comes from the wallet saved on the server.'}
              </div>
            ) : null}
          </div>
        )}

        <div className="space-y-1.5">
          <div className="text-xs font-medium" style={{ color: '#A0AEC0' }}>
            {t('modelConfig.walletPrivateKey', language)}
          </div>
          <div className="flex gap-2">
            <input
              type="password"
              value={apiKey}
              onChange={(e) => onApiKeyChange(e.target.value)}
              placeholder={
                hasExistingWallet
                  ? language === 'zh'
                    ? '如需切换钱包，请手动输入新的私钥'
                    : 'Enter a new private key only if you want to switch wallets'
                  : '0x...'
              }
              className="flex-1 px-4 py-3 rounded-xl font-mono text-sm"
              style={{
                background: '#0b0b0b',
                border: keyError ? '1px solid #EF4444' : walletAddress ? '1px solid #00E096' : '1px solid #2B3139',
                color: '#EAECEF',
              }}
              required={!hasExistingWallet}
            />
          </div>

          {hasExistingWallet && !apiKey ? (
            <div className="text-[11px] leading-5" style={{ color: '#848E9C' }}>
              {language === 'zh'
                ? '后续这里只使用你第一次创建并保存的钱包；如果你要换钱包，请手动填写新的私钥。'
                : 'This screen keeps using the wallet created and saved the first time. Enter a new private key manually only if you want to switch wallets.'}
            </div>
          ) : null}

          <div className="flex items-start gap-1.5 text-[11px]" style={{ color: '#848E9C' }}>
            <span className="mt-px">🔒</span>
            <span>
              {t('modelConfig.privateKeyNote', language)}
            </span>
          </div>
        </div>

        {/* Wallet Validation Results */}
        {(apiKey || hasExistingWallet) && (
          <div className="space-y-2 pl-1">
            {/* Validating spinner */}
            {validating && (
              <div className="flex items-center gap-2 text-xs" style={{ color: '#dce76a' }}>
                <span className="animate-spin">⏳</span>
                {t('modelConfig.validating', language)}
              </div>
            )}

            {/* Error message */}
            {keyError && !validating && (
              <div className="flex items-center gap-2 text-xs" style={{ color: '#EF4444' }}>
                <span>❌</span>
                {keyError}
              </div>
            )}

            {/* Success: address + balance + status */}
            {resolvedWalletAddress && !validating && !keyError && (
              <>
                <div className="p-2.5 rounded-lg" style={{ background: 'rgba(212,255,51,0.06)', border: '1px solid rgba(212,255,51,0.15)' }}>
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-[11px]" style={{ color: '#A0AEC0' }}>
                      {t('modelConfig.walletAddress', language)}:
                    </span>
                    <button
                      type="button"
                      title={copiedAddr ? t('modelConfig.copied', language) : t('modelConfig.copyAddress', language)}
                      onClick={() => {
                        navigator.clipboard.writeText(resolvedWalletAddress)
                        setCopiedAddr(true)
                        setTimeout(() => setCopiedAddr(false), 2000)
                      }}
                      className="text-[10px] px-1.5 py-0.5 rounded"
                      style={{ background: 'rgba(212,255,51,0.1)', color: '#dce76a', border: 'none', cursor: 'pointer' }}
                    >
                      {copiedAddr ? '✅' : '📋'}
                    </button>
                  </div>
                  <code className="text-[11px] font-mono block select-all" style={{ color: '#dce76a' }}>{resolvedWalletAddress}</code>
                  <div className="text-[10px] mt-1.5" style={{ color: '#F59E0B' }}>
                    ⚠️ {language === 'zh' ? '请确认这是你的钱包地址（可在 MetaMask 中核对）' : 'Please confirm this is your wallet address (verify in MetaMask)'}
                  </div>
                </div>
                {resolvedUsdcBalance !== null && (
                  <div className="flex items-center gap-2 text-xs">
                    <span>💰</span>
                    <span style={{ color: balanceNum > 0 ? '#00E096' : '#F59E0B' }}>
                      {t('modelConfig.usdcBalance', language)}: ${resolvedUsdcBalance}
                    </span>
                    <button
                      type="button"
                      onClick={() => setShowDeposit(!showDeposit)}
                      className="text-[10px] px-2 py-0.5 rounded transition-all"
                      style={{ background: 'rgba(0,224,150,0.1)', color: '#00E096', border: 'none', cursor: 'pointer' }}
                    >
                      {showDeposit
                        ? (language === 'zh' ? '收起' : 'Hide')
                        : (language === 'zh' ? '💳 充值' : '💳 Deposit')}
                    </button>
                  </div>
                )}
                {showDeposit && (
                  <div className="p-3 rounded-xl mt-1" style={{ background: 'rgba(0, 224, 150, 0.04)', border: '1px solid rgba(0, 224, 150, 0.15)' }}>
                    <div className="text-xs font-semibold mb-2" style={{ color: '#00E096' }}>
                      💳 {language === 'zh' ? '充值 USDC (Base 链)' : 'Deposit USDC (Base Chain)'}
                    </div>
                    <div className="flex gap-3 items-start mb-3">
                      <div className="shrink-0 p-1.5 rounded-lg" style={{ background: '#fff' }}>
                        <QRCodeSVG value={resolvedWalletAddress} size={80} level="M" />
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="text-[11px] mb-1" style={{ color: '#A0AEC0' }}>
                          {language === 'zh' ? '扫码或复制地址转账' : 'Scan QR or copy address to transfer'}
                        </div>
                        <code className="text-[10px] font-mono break-all select-all block mb-1.5" style={{ color: '#dce76a' }}>{resolvedWalletAddress}</code>
                        <button
                          type="button"
                          onClick={() => {
                            navigator.clipboard.writeText(resolvedWalletAddress)
                            setCopiedAddr(true)
                            setTimeout(() => setCopiedAddr(false), 2000)
                          }}
                          className="text-[10px] px-2 py-0.5 rounded"
                          style={{ background: 'rgba(212,255,51,0.1)', color: '#dce76a', border: 'none', cursor: 'pointer' }}
                        >
                          {copiedAddr ? `✅ ${t('modelConfig.copied', language)}` : `📋 ${t('modelConfig.copyAddress', language)}`}
                        </button>
                      </div>
                    </div>
                    <div className="text-[10px] space-y-1" style={{ color: '#848E9C' }}>
                      <div>📱 {language === 'zh' ? '用交易所 App 扫描二维码直接转账' : 'Scan QR with exchange app to transfer'}</div>
                      <div>• {language === 'zh' ? '提币时网络选择 Base' : 'Choose Base network when withdrawing'}</div>
                      <div>• {language === 'zh' ? '或跨链桥: ' : 'Or bridge: '}<a href="https://bridge.base.org" target="_blank" rel="noopener" className="underline" style={{ color: '#dce76a' }}>bridge.base.org</a></div>
                      <div>• {language === 'zh' ? '最低充值 $1 USDC 即可开始' : 'Min $1 USDC to start'}</div>
                    </div>
                  </div>
                )}
                {!apiKey && hasExistingWallet && (
                  <div className="text-[11px]" style={{ color: '#848E9C' }}>
                    {language === 'zh'
                      ? '当前正在使用这个钱包充值。若要切换钱包，再输入新的私钥并保存即可。'
                      : 'This wallet is currently used for funding. Enter a new private key only if you want to switch wallets.'}
                  </div>
                )}
                {claw402Status && (
                  <div className="flex items-center gap-2 text-xs" style={{ color: claw402Status === 'ok' ? '#00E096' : '#EF4444' }}>
                    <span>{claw402Status === 'ok' ? '🟢' : '🔴'}</span>
                    {claw402Status === 'ok'
                      ? t('modelConfig.claw402Connected', language)
                      : t('modelConfig.claw402Unreachable', language)}
                  </div>
                )}
              </>
            )}

            {/* Test Connection button */}
            {(isKeyValid || hasExistingWallet) && !validating && (
              <button
                type="button"
                onClick={handleTestConnection}
                disabled={testing || (!hasExistingWallet && !isKeyValid)}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all hover:scale-[1.02] disabled:opacity-50"
                style={{ background: 'rgba(212, 255, 51, 0.15)', border: '1px solid rgba(212, 255, 51, 0.3)', color: '#dce76a' }}
              >
                <span>🔗</span>
                {testing ? t('modelConfig.testingConnection', language) : t('modelConfig.testConnection', language)}
              </button>
            )}

            {/* Test result */}
            {testResult && !testing && (
              <div className="flex items-center gap-2 text-xs" style={{ color: testResult.status === 'ok' ? '#00E096' : '#EF4444' }}>
                <span>{testResult.status === 'ok' ? '✅' : '❌'}</span>
                {testResult.message}
              </div>
            )}
          </div>
        )}
      </div>

      {/* USDC Recharge Guide */}
      <div className="p-4 rounded-xl" style={{ background: 'rgba(0, 224, 150, 0.05)', border: '1px solid rgba(0, 224, 150, 0.15)' }}>
        <div className="text-sm font-semibold mb-2 flex items-center gap-2" style={{ color: '#00E096' }}>
          {'💰 ' + t('modelConfig.howToFundUsdc', language)}
        </div>
        <div className="text-xs space-y-1.5" style={{ color: '#848E9C' }}>
          <div className="flex items-start gap-2">
            <span className="font-bold" style={{ color: '#A0AEC0' }}>1.</span>
            <span>{t('modelConfig.fundStep1', language)}</span>
          </div>
          <div className="flex items-start gap-2">
            <span className="font-bold" style={{ color: '#A0AEC0' }}>2.</span>
            <span>{t('modelConfig.fundStep2', language)}</span>
          </div>
          <div className="flex items-start gap-2">
            <span className="font-bold" style={{ color: '#A0AEC0' }}>3.</span>
            <span>{t('modelConfig.fundStep3', language)}</span>
          </div>
        </div>
      </div>

      </div>
      <div className="flex shrink-0 flex-col-reverse gap-3 border-t border-zinc-800/90 bg-nofx-bg px-4 py-3 sm:flex-row sm:justify-end sm:px-5 sm:py-4">
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl bg-zinc-800 px-5 py-2.5 text-sm font-semibold text-zinc-200 transition-colors hover:bg-zinc-700"
        >
          取消
        </button>
        <button
          type="submit"
          disabled={!isKeyValid && !hasExistingWallet}
          className="rounded-xl bg-[#6D5AC8] px-6 py-2.5 text-sm font-bold text-white shadow-lg shadow-violet-900/20 transition-colors hover:bg-[#5d4cb8] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {editingModelId
            ? t('saveConfig', language)
            : t('modelConfig.startTrading', language)}
        </button>
      </div>
    </form>
  )
}

function StandardProviderConfigForm({
  formId,
  embedChrome,
  isComkunProvider,
  selectedModel,
  apiKey,
  baseUrl,
  modelName,
  editingModelId,
  onApiKeyChange,
  onBaseUrlChange,
  onModelNameChange,
  onCancel,
  onSubmit,
  language,
}: {
  formId: string
  embedChrome?: boolean
  isComkunProvider?: boolean
  selectedModel: AIModel
  apiKey: string
  baseUrl: string
  modelName: string
  editingModelId: string | null
  onApiKeyChange: (value: string) => void
  onBaseUrlChange: (value: string) => void
  onModelNameChange: (value: string) => void
  onCancel: () => void
  onSubmit: (e: React.FormEvent) => void
  language: Language
}) {
  const [showKey, setShowKey] = useState(false)
  const keyOk = !!apiKey.trim() || !!editingModelId || !!isComkunProvider
  const hideCredentialFields = !!isComkunProvider

  return (
    <form
      id={formId}
      onSubmit={onSubmit}
      className="flex min-h-0 min-w-0 flex-1 flex-col"
    >
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-5">
        {!embedChrome && (
          <div className="flex items-center gap-4 rounded-xl border border-zinc-800/80 bg-nofx-bg-tertiary p-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-white/10 bg-black">
              {selectedModel.provider === 'comkun_ai' || selectedModel.id === 'comkun_ai' ? (
                <img src="/icons/comkun-ai.png" alt="" width={36} height={36} className="object-contain" />
              ) : (
                getModelIcon(selectedModel.provider || selectedModel.id, {
                  width: 32,
                  height: 32,
                }) || (
                  <span className="text-lg font-bold text-zinc-400">{selectedModel.name[0]}</span>
                )
              )}
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-lg font-semibold text-zinc-100">
                {getShortName(selectedModel.name)}
              </div>
              <div className="text-xs text-zinc-500">
                {selectedModel.provider} ·{' '}
                {AI_PROVIDER_CONFIG[selectedModel.provider]?.defaultModel || selectedModel.id}
              </div>
            </div>
            {AI_PROVIDER_CONFIG[selectedModel.provider] && (
              <a
                href={AI_PROVIDER_CONFIG[selectedModel.provider].apiUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="flex shrink-0 items-center gap-2 rounded-lg border border-violet-500/30 bg-violet-500/10 px-4 py-2 text-sm font-medium text-[#c4b5fd] transition-colors hover:bg-violet-500/20"
              >
                <ExternalLink className="h-4 w-4" />
                {t('modelConfig.getApiKey', language)}
              </a>
            )}
          </div>
        )}

        {selectedModel.provider === 'kimi' && (
          <div
            className="rounded-xl border border-red-500/30 bg-red-500/10 p-4"
          >
            <div className="flex items-start gap-2 text-sm text-red-300">
              <span>⚠️</span>
              <span>{t('kimiApiNote', language)}</span>
            </div>
          </div>
        )}

        {hideCredentialFields ? (
          <div className="rounded-xl border border-emerald-500/25 bg-emerald-500/5 p-4 text-sm leading-relaxed text-emerald-100/80">
            COMKUN-AI 费用直接从平台余额扣除，不需要填写 API Key、钱包私钥或自定义地址。
          </div>
        ) : (
          <>
            <div className="space-y-2">
              <label className="text-sm font-medium text-zinc-300">
                {t('modelConfig.apiKeyLabel', language)}
              </label>
              <div className="relative">
                <input
                  type={showKey ? 'text' : 'password'}
                  value={apiKey}
                  onChange={(e) => onApiKeyChange(e.target.value)}
                  placeholder={t('enterAPIKey', language)}
                  className="w-full rounded-xl border border-zinc-700/80 bg-nofx-bg-tertiary py-3 pl-4 pr-12 text-sm text-white outline-none ring-violet-500/30 placeholder:text-zinc-600 focus:border-violet-500/50 focus:ring-1"
                  required={!editingModelId && !isComkunProvider}
                />
                <button
                  type="button"
                  onClick={() => setShowKey((v) => !v)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 rounded-lg p-2 text-zinc-500 hover:bg-white/5 hover:text-zinc-300"
                  aria-label={showKey ? '隐藏' : '显示'}
                >
                  {showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              <p className="text-xs text-zinc-500">*API Key 将被加密存储，请确保密钥有效</p>
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium text-zinc-300">{t('customBaseURL', language)}</label>
              <input
                type="url"
                value={baseUrl}
                onChange={(e) => onBaseUrlChange(e.target.value)}
                placeholder={t('customBaseURLPlaceholder', language)}
                className="w-full rounded-xl border border-zinc-700/80 bg-nofx-bg-tertiary px-4 py-3 text-sm text-white outline-none placeholder:text-zinc-600 focus:border-violet-500/50 focus:ring-1 focus:ring-violet-500/30"
              />
              <p className="text-xs text-zinc-500">Base URL 用于自定义 API 服务器地址；留空则使用默认。</p>
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium text-zinc-300">模型版本</label>
              <input
                type="text"
                value={modelName}
                onChange={(e) => onModelNameChange(e.target.value)}
                placeholder={
                  AI_PROVIDER_CONFIG[selectedModel.provider]?.defaultModel ||
                  t('customModelNamePlaceholder', language)
                }
                className="w-full rounded-xl border border-zinc-700/80 bg-nofx-bg-tertiary px-4 py-3 text-sm text-white outline-none placeholder:text-zinc-600 focus:border-violet-500/50 focus:ring-1 focus:ring-violet-500/30"
              />
              <p className="text-xs text-zinc-500">{t('leaveBlankForDefaultModel', language)}</p>
            </div>
          </>
        )}

        <div
          className={`rounded-xl border p-4 ${embedChrome ? 'border-violet-500/25 bg-violet-500/5' : 'border-[rgba(212,255,51,0.2)] bg-[rgba(212,255,51,0.08)]'}`}
        >
          <div
            className={`mb-2 flex items-center gap-2 text-sm font-semibold ${embedChrome ? 'text-violet-200' : ''}`}
            style={embedChrome ? undefined : { color: '#dce76a' }}
          >
            <Brain className="h-4 w-4" />
            {t('information', language)}
          </div>
          <div className="space-y-1 text-xs text-zinc-500">
            <div>• {t('modelConfigInfo1', language)}</div>
            <div>• {t('modelConfigInfo2', language)}</div>
            <div>• {t('modelConfigInfo3', language)}</div>
          </div>
        </div>
      </div>

      <div className="flex shrink-0 flex-col-reverse gap-3 border-t border-zinc-800/90 bg-nofx-bg px-4 py-3 sm:flex-row sm:justify-end sm:px-5 sm:py-4">
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl bg-zinc-800 px-5 py-2.5 text-sm font-semibold text-zinc-200 transition-colors hover:bg-zinc-700"
        >
          取消
        </button>
        <button
          type="submit"
          disabled={!selectedModel || !keyOk}
          className="rounded-xl bg-[#6D5AC8] px-6 py-2.5 text-sm font-bold text-white shadow-lg shadow-violet-900/20 transition-colors hover:bg-[#5d4cb8] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {editingModelId ? t('saveConfig', language) : '确认选择'}
        </button>
      </div>
    </form>
  )
}
