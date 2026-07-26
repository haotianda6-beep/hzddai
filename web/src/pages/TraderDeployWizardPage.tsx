/**
 * AI 交易员部署向导 — 四步流程（参考视频：策略 → AI 模型 → 交易所账户 → 审核）
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import useSWR from 'swr'
import {
  ArrowLeft,
  ArrowRight,
  Check,
  Trash2,
  Plus,
  Pencil,
  RadioTower,
} from 'lucide-react'
import { toast } from 'sonner'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import type {
  CreateExchangeRequest,
  UpdateExchangeConfigRequest,
} from '../types'
import { ROUTES } from '../router/paths'
import { t } from '../i18n/translations'
import { ExchangeConfigModal } from '../components/trader/ExchangeConfigModal'
import type { CreateTraderRequest, Exchange, Strategy } from '../types'
import { ApiError } from '../lib/httpClient'
import {
  filterModelsForStrategyConfig,
  isComkunAIModel,
  strategyConfigRequiresComkunAI,
} from '../lib/comkunStrategyModelBinding'

const DRAFT_KEY = 'nofx_trader_deploy_draft_v1'

type Step = 1 | 2 | 3 | 4
type StrategyTab = 'local' | 'subscribed'

function hashId(id: string): number {
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0
  return Math.abs(h)
}

function demoReturn(id: string): string {
  const h = hashId(id) % 2800
  const sign = h % 17 === 0 ? -1 : 1
  const v = ((h % 220) + 10) / 10
  return `${sign > 0 ? '+' : '-'}${v.toFixed(2)}%`
}

function demoAum(id: string): string {
  const h = hashId(id) % 5000
  const v = (h / 10 + 100).toFixed(2)
  return `${v} 美元`
}

function isListedStrategy(s: Strategy): boolean {
  const a = s.market_access
  if (
    a === 'subscription' ||
    a === 'public' ||
    a === 'open_source' ||
    a === 'private'
  )
    return true
  return !!(s.is_public && (a === undefined || a !== 'off'))
}

function filterEnabledExchanges(list: Exchange[]): Exchange[] {
  return list.filter((e) => {
    if (!e.enabled) return false
    if (e.id === 'aster') {
      return !!e.asterUser?.trim() && !!e.asterSigner?.trim()
    }
    if (e.id === 'hyperliquid') {
      return !!e.hyperliquidWalletAddr?.trim()
    }
    return true
  })
}

interface DraftShape {
  step: Step
  strategyTab: StrategyTab
  strategyId: string
  modelId: string
  exchangeId: string
  traderName: string
  scanInterval: number
  isCrossMargin: boolean
  showInCompetition: boolean
}

const defaultDraft = (): DraftShape => ({
  step: 1,
  strategyTab: 'local',
  strategyId: '',
  modelId: '',
  exchangeId: '',
  traderName: '',
  scanInterval: 3,
  isCrossMargin: true,
  showInCompetition: true,
})

function loadDraftFromStorage(): DraftShape {
  const base = defaultDraft()
  try {
    const raw = sessionStorage.getItem(DRAFT_KEY)
    if (raw) {
      const p = JSON.parse(raw) as Partial<DraftShape>
      Object.assign(base, p)
    }
  } catch {
    /* ignore */
  }
  base.step = clampStep(base.step)
  return base
}

export function TraderDeployWizardPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const { token, user } = useAuth()
  const { language } = useLanguage()

  const [draft, setDraft] = useState<DraftShape>(() => loadDraftFromStorage())

  const [submitting, setSubmitting] = useState(false)
  const [showExchangeModal, setShowExchangeModal] = useState(false)
  const [editingExchange, setEditingExchange] = useState<string | null>(null)

  const step = draft.step

  const setStep = useCallback((s: Step) => {
    setDraft((d) => ({ ...d, step: s }))
  }, [])

  /** 主导航「配置」等入口带 ?fresh=1：从第 1 步开始并清空草稿 */
  useEffect(() => {
    if (searchParams.get('fresh') !== '1') return
    sessionStorage.removeItem(DRAFT_KEY)
    setDraft(defaultDraft())
    setSearchParams({}, { replace: true })
  }, [searchParams, setSearchParams])

  useEffect(() => {
    sessionStorage.setItem(DRAFT_KEY, JSON.stringify(draft))
  }, [draft])

  const { data: strategies = [], isLoading: loadingStrategies } = useSWR(
    token ? 'wizard-strategies' : null,
    () => api.getStrategies(),
    { revalidateOnFocus: false }
  )

  const { data: allModels = [], isLoading: loadingModels } = useSWR(
    token ? 'wizard-models' : null,
    () => api.getModelConfigs(),
    { revalidateOnFocus: false }
  )

  const {
    data: allExchanges = [],
    isLoading: loadingExchanges,
    mutate: mutateExchanges,
  } = useSWR(
    token ? 'wizard-exchanges' : null,
    () => api.getExchangeConfigs(),
    {
      revalidateOnFocus: false,
    }
  )

  const enabledModels = useMemo(
    () => allModels.filter((m) => m.enabled),
    [allModels]
  )

  const enabledExchanges = useMemo(
    () => filterEnabledExchanges(allExchanges),
    [allExchanges]
  )

  const localStrategies = strategies
  const subscribedStrategies = useMemo(
    () => strategies.filter(isListedStrategy),
    [strategies]
  )

  const strategyList =
    draft.strategyTab === 'local' ? localStrategies : subscribedStrategies

  const selectedStrategy = strategies.find((s) => s.id === draft.strategyId)
  const selectedModel = allModels.find((m) => m.id === draft.modelId)
  const selectedExchange = allExchanges.find((e) => e.id === draft.exchangeId)
  const strategyPrefersComkunAI = strategyConfigRequiresComkunAI(
    selectedStrategy?.config,
    selectedStrategy?.id
  )

  const modelsForWizard = useMemo(
    () =>
      filterModelsForStrategyConfig(
        enabledModels,
        selectedStrategy?.config ?? undefined,
        selectedStrategy?.id
      ),
    [enabledModels, selectedStrategy?.config, selectedStrategy?.id]
  )

  useEffect(() => {
    const allowed = filterModelsForStrategyConfig(
      enabledModels,
      selectedStrategy?.config ?? undefined,
      selectedStrategy?.id
    )
    if (allowed.length === 0) {
      setDraft((d) => (d.modelId ? { ...d, modelId: '' } : d))
      return
    }
    if (!allowed.some((m) => m.id === draft.modelId)) {
      setDraft((d) => ({ ...d, modelId: allowed[0].id }))
    }
  }, [
    draft.modelId,
    draft.strategyId,
    enabledModels,
    selectedStrategy?.config,
    selectedStrategy?.id,
  ])

  const stepTitle = (n: Step) => {
    if (n === 1) return '步骤 1：选择交易策略'
    if (n === 2) return '步骤 2：选择 AI 模型'
    if (n === 3) return '步骤 3：选择交易所账户'
    return '步骤 4：核对并创建'
  }

  const stepDesc = (n: Step) => {
    if (n === 1) return '为本交易员选定策略；可先在策略实验室或策略市场准备。'
    if (n === 2) {
      return strategyPrefersComkunAI
        ? '该策略推荐优先选择 COMKUN-AI；其他已启用模型也会显示为备选。'
        : '选择已启用的大模型；可在设置里添加并启用新模型（普通策略不可选 COMKUN-AI）。'
    }
    if (n === 3)
      return '选择已启用的交易所账户；没有账户时点下方「连接新交易所」在本页填写 API。'
    return '确认策略、模型、账户及扫描间隔等信息，然后创建交易员。'
  }

  const canNextStep1 = !!draft.strategyId
  const canNextStep2 =
    !!draft.modelId && modelsForWizard.some((m) => m.id === draft.modelId)
  const canNextStep3 =
    !!draft.exchangeId &&
    enabledExchanges.some((e) => e.id === draft.exchangeId)
  const canSubmit =
    !!draft.strategyId &&
    !!draft.modelId &&
    !!draft.exchangeId &&
    draft.scanInterval >= 3

  const goNext = () => {
    if (step === 1 && !canNextStep1) {
      toast.error('请先选择一个策略')
      return
    }
    if (step === 2 && !canNextStep2) {
      if (modelsForWizard.length === 0) {
        toast.error(
          strategyPrefersComkunAI
            ? '没有可选模型，请先到设置中启用 COMKUN-AI 或其他 AI 模型'
            : '没有可选模型，请检查设置中的模型配置'
        )
      } else {
        toast.error('请选择一个已启用的 AI 模型')
      }
      return
    }
    if (step === 3 && !canNextStep3) {
      toast.error('请选择一个已启用的交易所账户')
      return
    }
    if (step < 4) setStep((step + 1) as Step)
  }

  const goBack = () => {
    if (step > 1) setStep((step - 1) as Step)
    else navigate(ROUTES.dashboard)
  }

  const clearDraft = () => {
    sessionStorage.removeItem(DRAFT_KEY)
    setDraft(defaultDraft())
    toast.success('已清除草稿')
  }

  const openExchangeModal = (editId: string | null) => {
    setEditingExchange(editId)
    setShowExchangeModal(true)
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
        const { id } = await api.createExchangeEncrypted(createRequest)
        toast.success('已添加交易所账户')
        await mutateExchanges()
        setShowExchangeModal(false)
        setEditingExchange(null)
        setDraft((d) => ({ ...d, exchangeId: id }))
        return
      }
      await mutateExchanges()
      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存交易所配置失败')
    }
  }

  const handleDeleteExchange = async (exchangeId: string) => {
    try {
      await api.deleteExchange(exchangeId)
      toast.success('已删除该交易所账户')
      await mutateExchanges()
      setShowExchangeModal(false)
      setEditingExchange(null)
      setDraft((d) =>
        d.exchangeId === exchangeId ? { ...d, exchangeId: '' } : d
      )
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  const handleSubmit = async () => {
    const name =
      draft.traderName.trim() || `AI交易员_${Date.now().toString(36).slice(-4)}`
    const body: CreateTraderRequest = {
      name,
      ai_model_id: draft.modelId,
      exchange_id: draft.exchangeId,
      strategy_id: draft.strategyId,
      scan_interval_minutes: draft.scanInterval,
      is_cross_margin: draft.isCrossMargin,
      show_in_competition: draft.showInCompetition,
    }
    setSubmitting(true)
    try {
      const created = await api.createTrader(body)
      sessionStorage.removeItem(DRAFT_KEY)
      if (created.startup_warning) {
        toast.warning('交易员已保存', {
          description: created.startup_warning,
          duration: 12000,
        })
      } else {
        toast.success('AI 交易员已创建')
      }
      navigate(ROUTES.dashboard)
    } catch (e) {
      const msg =
        e instanceof ApiError
          ? e.message
          : e instanceof Error
            ? e.message
            : 'Error'
      toast.error('创建失败', { description: msg })
    } finally {
      setSubmitting(false)
    }
  }

  const displayName =
    (user?.display_name && user.display_name.trim()) ||
    user?.email?.split('@')[0] ||
    ''

  return (
    <div className="min-h-screen bg-nofx-bg pb-40 text-zinc-100 sm:pb-28">
      <div className="border-b border-zinc-800/80 bg-nofx-bg-tertiary">
        <div className="mx-auto flex max-w-6xl items-start justify-between gap-3 px-3 py-3 sm:items-center sm:gap-4 sm:px-4 sm:py-4">
          <div className="flex min-w-0 items-center gap-3">
            <Link
              to={ROUTES.dashboard}
              className="flex shrink-0 items-center gap-2 rounded-lg border border-zinc-700/80 px-2.5 py-2 text-xs font-medium text-zinc-300 transition-colors hover:border-nofx-gold/50 hover:text-white sm:px-3"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              <span className="hidden sm:inline">返回看板</span>
            </Link>
            <div className="min-w-0">
              <h1 className="truncate font-['Space_Grotesk',system-ui] text-lg font-bold tracking-tight text-white sm:text-xl">
                部署 AI 交易员
              </h1>
              <p className="truncate text-[11px] text-zinc-500">
                四步完成：策略 → 模型 → 交易所账户 → 核对创建
              </p>
            </div>
          </div>
          {user && (
            <div className="hidden items-center gap-2 sm:flex">
              {user.avatar_url ? (
                <img
                  src={user.avatar_url}
                  alt=""
                  className="h-8 w-8 rounded-full border border-zinc-700 object-cover"
                />
              ) : (
                <div className="flex h-8 w-8 items-center justify-center rounded-full bg-nofx-gold text-xs font-bold text-black">
                  {(displayName || user.email)[0]?.toUpperCase()}
                </div>
              )}
              <span className="max-w-[8rem] truncate text-xs text-zinc-400">
                {displayName || user.email}
              </span>
            </div>
          )}
        </div>
      </div>

      <div className="mx-auto flex max-w-6xl flex-col gap-5 px-3 py-5 sm:px-4 sm:py-8 lg:flex-row lg:gap-8">
        {/* 左侧步骤条 */}
        <aside className="w-full shrink-0 border-b border-zinc-800 pb-6 lg:w-52 lg:border-b-0 lg:border-r lg:pb-0 lg:pr-6">
          <ol className="relative flex flex-row justify-between gap-2 lg:flex-col lg:justify-start lg:gap-0 lg:pl-2">
            <div className="absolute left-[18px] top-8 hidden h-[calc(100%-3rem)] border-l border-dotted border-zinc-700 lg:block" />
            {([1, 2, 3, 4] as const).map((n, i) => {
              const done = step > n
              const active = step === n
              const labels = ['策略', 'AI 模型', '交易所', '核对']
              return (
                <li
                  key={n}
                  className="relative z-[1] flex flex-1 lg:flex-initial"
                >
                  <div
                    className={`flex w-full items-center justify-center gap-2 rounded-xl px-1 py-2 sm:justify-start sm:px-2 lg:px-3 lg:py-3 ${
                      active ? 'bg-nofx-gold/10 ring-1 ring-nofx-gold/40' : ''
                    }`}
                  >
                    <div
                      className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold sm:h-9 sm:w-9 ${
                        done
                          ? 'bg-emerald-500/20 text-emerald-400'
                          : active
                            ? 'bg-nofx-gold text-black'
                            : 'bg-zinc-800 text-zinc-500'
                      }`}
                    >
                      {done ? <Check className="h-4 w-4" /> : n}
                    </div>
                    <div className="hidden min-w-0 flex-1 sm:block">
                      <div
                        className={`text-xs font-semibold sm:text-sm ${
                          active
                            ? 'text-nofx-gold'
                            : done
                              ? 'text-zinc-200'
                              : 'text-zinc-500'
                        }`}
                      >
                        {labels[i]}
                      </div>
                    </div>
                  </div>
                </li>
              )
            })}
          </ol>
        </aside>

        {/* 主内容 */}
        <main className="min-w-0 flex-1">
          <h2 className="font-['Space_Grotesk',system-ui] text-xl font-bold text-nofx-gold sm:text-2xl">
            {stepTitle(step)}
          </h2>
          <p className="mt-2 text-sm text-zinc-400">{stepDesc(step)}</p>

          {step === 1 && (
            <div className="mt-8">
              <div className="mb-4 flex gap-2 rounded-xl border border-zinc-800 bg-zinc-900/40 p-1">
                <button
                  type="button"
                  onClick={() =>
                    setDraft((d) => ({ ...d, strategyTab: 'local' }))
                  }
                  className={`flex-1 rounded-lg px-2 py-2 text-xs font-semibold transition-colors sm:px-4 sm:text-sm ${
                    draft.strategyTab === 'local'
                      ? 'bg-nofx-gold text-black'
                      : 'text-zinc-400 hover:text-zinc-200'
                  }`}
                >
                  本地策略
                </button>
                <button
                  type="button"
                  onClick={() =>
                    setDraft((d) => ({ ...d, strategyTab: 'subscribed' }))
                  }
                  className={`flex-1 rounded-lg px-2 py-2 text-xs font-semibold transition-colors sm:px-4 sm:text-sm ${
                    draft.strategyTab === 'subscribed'
                      ? 'bg-nofx-gold text-black'
                      : 'text-zinc-400 hover:text-zinc-200'
                  }`}
                >
                  已订阅 / 上架
                </button>
              </div>

              <div className="overflow-hidden rounded-xl border border-zinc-800 bg-nofx-bg-secondary">
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 border-b border-zinc-800 px-4 py-3 text-[11px] font-bold uppercase tracking-wider text-zinc-500">
                  <span>策略</span>
                  <span className="hidden text-right sm:block">
                    资产 / 演示涨跌
                  </span>
                  <span className="text-right">选择</span>
                </div>
                {loadingStrategies ? (
                  <div className="px-4 py-12 text-center text-sm text-zinc-500">
                    加载中…
                  </div>
                ) : strategyList.length === 0 ? (
                  <div className="px-4 py-12 text-center text-sm text-zinc-500">
                    暂无策略。请先到策略实验室创建，或从策略市场探索。
                  </div>
                ) : (
                  <ul className="divide-y divide-zinc-800/80">
                    {strategyList.map((s) => {
                      const sel = draft.strategyId === s.id
                      const ret = demoReturn(s.id)
                      const up = !ret.startsWith('-')
                      return (
                        <li
                          key={s.id}
                          className={`grid cursor-pointer grid-cols-1 items-center gap-3 px-4 py-4 transition-colors sm:grid-cols-[1fr_auto_auto] ${
                            sel ? 'bg-nofx-gold/10' : 'hover:bg-zinc-900/60'
                          }`}
                          onClick={() =>
                            setDraft((d) => ({ ...d, strategyId: s.id }))
                          }
                        >
                          <div>
                            <div className="font-semibold text-white">
                              {s.name}
                            </div>
                            <div className="mt-0.5 line-clamp-1 text-xs text-zinc-500">
                              {s.description || '—'}
                            </div>
                          </div>
                          <div className="flex flex-col items-start gap-1 sm:items-end">
                            <span className="text-xs text-zinc-400">
                              {demoAum(s.id)}
                            </span>
                            <span
                              className="text-sm font-bold tabular-nums"
                              style={{ color: up ? '#0ECB81' : '#F6465D' }}
                            >
                              {ret}
                            </span>
                          </div>
                          <div className="flex justify-end">
                            <button
                              type="button"
                              aria-label="select"
                              onClick={(e) => {
                                e.stopPropagation()
                                setDraft((d) => ({ ...d, strategyId: s.id }))
                              }}
                              className={`flex h-8 w-8 items-center justify-center rounded-full border-2 transition-colors ${
                                sel
                                  ? 'border-nofx-gold bg-nofx-gold'
                                  : 'border-zinc-600 hover:border-zinc-400'
                              }`}
                            >
                              {sel ? (
                                <Check className="h-4 w-4 text-black" />
                              ) : null}
                            </button>
                          </div>
                        </li>
                      )
                    })}
                  </ul>
                )}
                <div className="border-t border-zinc-800 p-4">
                  <Link
                    to={ROUTES.strategyMarket}
                    className="flex w-full items-center justify-center gap-2 rounded-xl border border-dashed border-nofx-gold/45 py-3 text-sm font-semibold text-nofx-gold transition-colors hover:bg-nofx-gold/10"
                  >
                    <Plus className="h-4 w-4" />
                    探索市场策略
                  </Link>
                </div>
              </div>
            </div>
          )}

          {step === 2 && (
            <div className="mt-8">
              <div className="overflow-hidden rounded-xl border border-zinc-800 bg-nofx-bg-secondary">
                {loadingModels ? (
                  <div className="px-4 py-12 text-center text-sm text-zinc-500">
                    加载中…
                  </div>
                ) : enabledModels.length === 0 ? (
                  <div className="px-4 py-12 text-center text-sm text-zinc-500">
                    没有已启用的模型。请先到设置中连接并启用 AI 模型。
                    <div className="mt-4">
                      <Link
                        to={ROUTES.settings}
                        className="text-nofx-gold underline hover:text-yellow-300"
                      >
                        打开设置
                      </Link>
                    </div>
                  </div>
                ) : modelsForWizard.length === 0 ? (
                  <div className="px-4 py-12 text-center text-sm text-zinc-500">
                    {strategyPrefersComkunAI
                      ? '当前策略推荐 COMKUN-AI；请先到「设置」启用 COMKUN-AI 或其他 AI 模型。'
                      : '没有与当前策略匹配的模型（普通策略不能选 COMKUN-AI）。'}
                    <div className="mt-4">
                      <Link
                        to={ROUTES.settings}
                        className="text-nofx-gold underline hover:text-yellow-300"
                      >
                        打开设置
                      </Link>
                    </div>
                  </div>
                ) : (
                  <ul className="divide-y divide-zinc-800/80">
                    {modelsForWizard.map((m) => {
                      const sel = draft.modelId === m.id
                      const recommended =
                        strategyPrefersComkunAI && isComkunAIModel(m)
                      const alternate = strategyPrefersComkunAI && !recommended
                      return (
                        <li
                          key={m.id}
                          className={`flex cursor-pointer items-center justify-between gap-4 px-4 py-4 transition-colors ${
                            recommended
                              ? sel
                                ? 'bg-nofx-gold/15 ring-1 ring-inset ring-nofx-gold/45'
                                : 'bg-nofx-gold/10 hover:bg-nofx-gold/15'
                              : alternate
                                ? sel
                                  ? 'bg-zinc-800/60 ring-1 ring-inset ring-zinc-600/80'
                                  : 'bg-zinc-950/25 opacity-65 hover:bg-zinc-900/60 hover:opacity-95'
                                : sel
                                  ? 'bg-nofx-gold/10'
                                  : 'hover:bg-zinc-900/60'
                          }`}
                          onClick={() =>
                            setDraft((d) => ({ ...d, modelId: m.id }))
                          }
                        >
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <span
                                className={`font-semibold ${alternate ? 'text-zinc-300' : 'text-white'}`}
                              >
                                {m.name}
                              </span>
                              {strategyPrefersComkunAI ? (
                                <span
                                  className={`rounded-full px-2 py-0.5 text-[10px] font-bold ${
                                    recommended
                                      ? 'border border-nofx-gold/45 bg-nofx-gold/15 text-nofx-gold'
                                      : 'border border-zinc-600/70 bg-zinc-800/50 text-zinc-400'
                                  }`}
                                >
                                  {recommended ? '推荐' : '备选'}
                                </span>
                              ) : null}
                              <span
                                className={`rounded-full border px-2 py-0.5 text-[10px] font-bold ${
                                  alternate
                                    ? 'border-zinc-600/70 bg-zinc-800/40 text-zinc-500'
                                    : 'border-sky-500/30 bg-sky-500/10 text-sky-300'
                                }`}
                              >
                                {m.provider}
                              </span>
                              <span className="rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-bold text-emerald-300">
                                正常
                              </span>
                              <span className="rounded-full border border-teal-500/30 bg-teal-500/10 px-2 py-0.5 text-[10px] font-bold text-teal-300">
                                使用中
                              </span>
                            </div>
                            <div className="mt-1 text-xs text-zinc-500">
                              {m.customModelName || m.name}
                              {m.balanceUsdc ? ` · USDC ${m.balanceUsdc}` : ''}
                            </div>
                          </div>
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation()
                              setDraft((d) => ({ ...d, modelId: m.id }))
                            }}
                            className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 transition-colors ${
                              sel
                                ? 'border-nofx-gold bg-nofx-gold'
                                : 'border-zinc-600 hover:border-zinc-400'
                            }`}
                          >
                            {sel ? (
                              <Check className="h-4 w-4 text-black" />
                            ) : null}
                          </button>
                        </li>
                      )
                    })}
                  </ul>
                )}
                <div className="border-t border-zinc-800 p-4">
                  <Link
                    to={ROUTES.settings}
                    className="flex w-full items-center justify-center gap-2 rounded-xl border border-dashed border-nofx-gold/45 py-3 text-sm font-semibold text-nofx-gold transition-colors hover:bg-nofx-gold/10"
                  >
                    <Plus className="h-4 w-4" />
                    连接新 AI 模型
                  </Link>
                </div>
              </div>
            </div>
          )}

          {step === 3 && (
            <div className="mt-8">
              <div className="overflow-hidden rounded-xl border border-zinc-800 bg-nofx-bg-secondary">
                {loadingExchanges ? (
                  <div className="px-4 py-12 text-center text-sm text-zinc-500">
                    加载中…
                  </div>
                ) : enabledExchanges.length === 0 ? (
                  <div className="px-4 py-12 text-center text-sm text-zinc-500">
                    还没有可用的交易所账户。点击下方按钮在本页填写 API 并保存。
                    <div className="mt-4">
                      <button
                        type="button"
                        onClick={() => openExchangeModal(null)}
                        className="rounded-lg bg-nofx-gold px-4 py-2 text-sm font-semibold text-black hover:bg-nofx-gold-highlight"
                      >
                        添加交易所账户
                      </button>
                    </div>
                  </div>
                ) : (
                  <ul className="divide-y divide-zinc-800/80">
                    {enabledExchanges.map((ex) => {
                      const sel = draft.exchangeId === ex.id
                      return (
                        <li
                          key={ex.id}
                          className={`flex cursor-pointer flex-col items-stretch justify-between gap-3 px-4 py-4 transition-colors sm:flex-row sm:items-center sm:gap-4 ${
                            sel ? 'bg-nofx-gold/10' : 'hover:bg-zinc-900/60'
                          }`}
                          onClick={() =>
                            setDraft((d) => ({ ...d, exchangeId: ex.id }))
                          }
                        >
                          <div className="min-w-0 flex-1">
                            <div className="font-semibold text-white">
                              {ex.account_name || ex.name}
                            </div>
                            <div className="mt-1 text-xs text-zinc-500">
                              {ex.exchange_type}
                            </div>
                            <span className="mt-2 inline-block rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-bold text-emerald-300">
                              已启用
                            </span>
                            {[
                              'binance',
                              'bybit',
                              'okx',
                              'bitget',
                              'gate',
                            ].includes(ex.exchange_type) ? (
                              <div className="mt-2 space-y-1 rounded-lg border border-zinc-700/80 bg-zinc-950/80 px-2 py-2 text-[11px] leading-snug text-zinc-400">
                                <div className="break-all">
                                  <span className="text-zinc-500">
                                    {t('whitelistProxyLabel', language)}:{' '}
                                  </span>
                                  <span className="font-mono text-nofx-gold">
                                    {ex.outbound_proxy_whitelist_host?.trim() ||
                                      '—'}
                                  </span>
                                </div>
                                <p className="text-[10px] text-zinc-600">
                                  {t('wizardBinanceWhitelistHint', language)}
                                </p>
                              </div>
                            ) : null}
                          </div>
                          <div className="flex shrink-0 items-center justify-between gap-2 sm:justify-end">
                            <button
                              type="button"
                              onClick={(e) => {
                                e.stopPropagation()
                                openExchangeModal(ex.id)
                              }}
                              className="flex h-9 flex-1 items-center justify-center gap-1 rounded-lg border border-zinc-600 px-3 text-xs text-zinc-300 hover:border-nofx-gold/50 hover:text-nofx-gold sm:h-8 sm:flex-none sm:px-2"
                            >
                              <Pencil className="h-3.5 w-3.5" />
                              编辑
                            </button>
                            <button
                              type="button"
                              onClick={(e) => {
                                e.stopPropagation()
                                setDraft((d) => ({ ...d, exchangeId: ex.id }))
                              }}
                              className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 transition-colors ${
                                sel
                                  ? 'border-nofx-gold bg-nofx-gold'
                                  : 'border-zinc-600 hover:border-zinc-400'
                              }`}
                            >
                              {sel ? (
                                <Check className="h-4 w-4 text-black" />
                              ) : null}
                            </button>
                          </div>
                        </li>
                      )
                    })}
                  </ul>
                )}
                <div className="border-t border-zinc-800 p-4">
                  <button
                    type="button"
                    onClick={() => openExchangeModal(null)}
                    className="flex w-full items-center justify-center gap-2 rounded-xl border border-dashed border-nofx-gold/45 py-3 text-sm font-semibold text-nofx-gold transition-colors hover:bg-nofx-gold/10"
                  >
                    <Plus className="h-4 w-4" />
                    连接新交易所
                  </button>
                </div>
              </div>
            </div>
          )}

          {step === 4 && (
            <div className="mt-8 space-y-6 rounded-xl border border-zinc-800 bg-nofx-bg-secondary p-4 sm:p-6">
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <div className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">
                    策略
                  </div>
                  <div className="mt-1 text-sm font-semibold text-white">
                    {selectedStrategy?.name || '—'}
                  </div>
                </div>
                <div>
                  <div className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">
                    AI 模型
                  </div>
                  <div className="mt-1 text-sm font-semibold text-white">
                    {selectedModel?.name || '—'}
                  </div>
                </div>
                <div>
                  <div className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">
                    交易所
                  </div>
                  <div className="mt-1 text-sm font-semibold text-white">
                    {(selectedExchange?.account_name ||
                      selectedExchange?.name) ??
                      '—'}{' '}
                    <span className="text-zinc-500">
                      ({selectedExchange?.exchange_type})
                    </span>
                  </div>
                </div>
                <div>
                  <div className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">
                    扫描间隔（分钟）
                  </div>
                  <input
                    type="number"
                    min={3}
                    value={draft.scanInterval}
                    onChange={(e) =>
                      setDraft((d) => ({
                        ...d,
                        scanInterval: Math.max(
                          3,
                          parseInt(e.target.value, 10) || 3
                        ),
                      }))
                    }
                    className="mt-1 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-base text-white outline-none focus:border-nofx-gold sm:text-sm"
                  />
                </div>
              </div>
              {selectedExchange &&
                ['binance', 'bybit', 'okx', 'bitget', 'gate'].includes(
                  selectedExchange.exchange_type
                ) && (
                  <div className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-4">
                    <div className="text-[11px] font-bold uppercase tracking-wider text-amber-200/90">
                      {t('whitelistIP', language)}
                    </div>
                    <p className="mt-2 text-xs leading-relaxed text-zinc-400">
                      {t('whitelistDualIntro', language)}
                    </p>
                    <div className="mt-3 grid gap-2 text-sm">
                      <div className="rounded-lg border border-zinc-700/80 bg-zinc-950/60 p-3">
                        <div className="text-[10px] font-semibold uppercase text-zinc-500">
                          {t('whitelistProxyLabel', language)}
                        </div>
                        <div className="mt-1 break-all font-mono text-nofx-gold">
                          {selectedExchange.outbound_proxy_whitelist_host?.trim() ||
                            '—'}
                        </div>
                      </div>
                    </div>
                  </div>
                )}
              <div>
                <label className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">
                  交易员名称
                </label>
                <input
                  value={draft.traderName}
                  onChange={(e) =>
                    setDraft((d) => ({ ...d, traderName: e.target.value }))
                  }
                  placeholder="留空则自动生成名称"
                  className="mt-1 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2.5 text-base text-white outline-none focus:border-nofx-gold sm:text-sm"
                />
              </div>
              <label className="flex cursor-pointer items-center gap-2 text-sm text-zinc-300">
                <input
                  type="checkbox"
                  checked={draft.isCrossMargin}
                  onChange={(e) =>
                    setDraft((d) => ({ ...d, isCrossMargin: e.target.checked }))
                  }
                  className="rounded border-zinc-600"
                />
                全仓模式
              </label>
              <label className="flex cursor-pointer items-center gap-2 text-sm text-zinc-300">
                <input
                  type="checkbox"
                  checked={draft.showInCompetition}
                  onChange={(e) =>
                    setDraft((d) => ({
                      ...d,
                      showInCompetition: e.target.checked,
                    }))
                  }
                  className="rounded border-zinc-600"
                />
                在竞技场展示
              </label>
            </div>
          )}
        </main>
      </div>

      {/* 底部操作栏 */}
      <div className="fixed bottom-0 left-0 right-0 z-40 border-t border-zinc-800 bg-nofx-bg-tertiary/95 px-3 py-3 backdrop-blur-md sm:px-4 sm:py-4">
        <div className="mx-auto flex max-w-6xl flex-col items-stretch justify-between gap-3 sm:flex-row sm:items-center">
          <div className="flex w-full items-center gap-2 sm:w-auto">
            {step > 1 && (
              <button
                type="button"
                onClick={goBack}
                className="flex-1 rounded-full border border-zinc-600 px-5 py-2.5 text-sm font-semibold text-zinc-200 transition-colors hover:border-zinc-500 hover:bg-zinc-800 sm:flex-none"
              >
                返回
              </button>
            )}
            {step < 4 ? (
              <button
                type="button"
                onClick={goNext}
                className="inline-flex flex-1 items-center justify-center gap-2 rounded-full bg-nofx-gold px-6 py-2.5 text-sm font-bold text-black shadow-[0_0_24px_rgba(212,255,51,0.28)] transition-colors hover:bg-nofx-gold-highlight sm:flex-none"
              >
                下一步
                <ArrowRight className="h-4 w-4" />
              </button>
            ) : (
              <button
                type="button"
                disabled={!canSubmit || submitting}
                onClick={() => void handleSubmit()}
                className="inline-flex flex-1 items-center justify-center gap-2 rounded-full bg-nofx-gold px-6 py-2.5 text-sm font-bold text-black shadow-[0_0_24px_rgba(212,255,51,0.28)] transition-colors hover:bg-nofx-gold-highlight disabled:opacity-50 sm:flex-none"
              >
                <RadioTower className="h-4 w-4" />
                {submitting ? '创建中…' : '完成创建'}
              </button>
            )}
          </div>
          <button
            type="button"
            onClick={clearDraft}
            className="inline-flex w-full items-center justify-center gap-2 text-xs font-medium text-zinc-500 transition-colors hover:text-red-400 sm:w-auto"
          >
            <Trash2 className="h-3.5 w-3.5" />
            清除草稿
          </button>
        </div>
      </div>

      {showExchangeModal && (
        <ExchangeConfigModal
          allExchanges={allExchanges}
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

function clampStep(s: unknown): Step {
  const raw =
    typeof s === 'number' && !Number.isNaN(s)
      ? s
      : parseInt(String(s ?? '1'), 10)
  const n = Number.isNaN(raw) ? 1 : raw
  if (n >= 4) return 4
  if (n <= 1) return 1
  return n as Step
}
