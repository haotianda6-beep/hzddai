import { useState, useEffect, useCallback, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import {
  Plus,
  Trash2,
  Check,
  Zap,
  Activity,
  Save,
  Globe,
  Tag,
  X,
  RefreshCw,
} from 'lucide-react'
import type {
  Strategy,
  StrategyConfig,
  GridStrategyConfig,
  StrategyMarketAccess,
} from '../types'
import {
  defaultMartingaleProgramConfig,
  isProgramMartingaleStrategyStudioStrategy,
} from '../types/strategy'
import { confirmToast, notify } from '../lib/notify'
import { defaultGridConfig } from '../components/strategy/GridConfigEditor'
import { StrategyStudioLuminescentPanels } from '../components/strategy/studio/StrategyStudioLuminescentPanels'
import { t } from '../i18n/translations'
import '../pages/landing/luminescent.css'

const API_BASE = import.meta.env.VITE_API_BASE || ''

/** 与后端 DB 一致（不含「改过名→市场按仅展示」的虚拟档位） */
function rawMarketAccess(s: Strategy): StrategyMarketAccess {
  const a = s.market_access
  if (
    a === 'off' ||
    a === 'private' ||
    a === 'subscription' ||
    a === 'public' ||
    a === 'open_source'
  ) {
    return a
  }
  if (s.is_public && s.config_visible) return 'public'
  if (s.is_public && !s.config_visible) return 'subscription'
  return 'off'
}

/** 界面与后端一致：只有管理员审核后写入的 market_access 才算已上架。 */
function effectiveMarketAccess(s: Strategy): StrategyMarketAccess {
  return rawMarketAccess(s)
}

function isLockedMarketCopy(s: Strategy): boolean {
  if (s.content_locked) return true
  if (s.source_strategy_id && s.source_market_access !== 'open_source')
    return true
  const n = (s.name || '').toLowerCase()
  return (
    (n.includes('已购') || n.includes('purchased')) &&
    !n.includes('开源') &&
    !n.includes('open source')
  )
}

export function StrategyStudioPage() {
  const { token } = useAuth()
  const { language } = useLanguage()
  const [searchParams] = useSearchParams()
  const strategyIdFromUrl = (
    searchParams.get('id') ||
    searchParams.get('strategy') ||
    ''
  ).trim()

  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [selectedStrategy, setSelectedStrategy] = useState<Strategy | null>(
    null
  )
  const [editingConfig, setEditingConfig] = useState<StrategyConfig | null>(
    null
  )
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [isPublishingMarket, setIsPublishingMarket] = useState(false)
  const [estimatedTokens, setEstimatedTokens] = useState(0)
  /** 用于 Token 进度条上限（取估算接口里各模型上下文窗口的最小值，粗代表「预算」） */
  const [tokenBudget, setTokenBudget] = useState(131072)
  const [error, setError] = useState<string | null>(null)
  const [hasChanges, setHasChanges] = useState(false)
  const [publishMarketOpen, setPublishMarketOpen] = useState(false)
  const [salePriceDraft, setSalePriceDraft] = useState('')
  /** 弹窗中选中的市场上架权限 */
  const [listingDraft, setListingDraft] = useState<StrategyMarketAccess>('off')

  const gridConfigCacheRef = useRef<Record<string, GridStrategyConfig>>({})

  // Fetch strategies
  const fetchStrategies = useCallback(async () => {
    if (!token) return
    try {
      const response = await fetch(`${API_BASE}/api/strategies`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!response.ok) throw new Error('Failed to fetch strategies')
      const data = await response.json()
      const list: Strategy[] = data.strategies || []
      setStrategies(list)

      let selected: Strategy | null = null
      if (strategyIdFromUrl) {
        selected = list.find((x) => x.id === strategyIdFromUrl) ?? null
        if (!selected) {
          notify.warning(
            language === 'zh'
              ? '该策略不在当前账号下，已打开你的默认策略。从市场进入他人策略时，可复制公开配置到自己的策略中。'
              : language === 'id'
                ? 'Strategi ini tidak ada di akun Anda; strategi default dibuka.'
                : 'That strategy is not in your account; opened your default strategy. For others’ listings, copy the public config into your own strategy.'
          )
        }
      }
      if (!selected) {
        selected =
          list.find((s: Strategy) => s.is_active) ||
          (list.length > 0 ? list[0] : null)
      }
      if (selected) {
        setSelectedStrategy(selected)
        setEditingConfig(selected.config)
      } else {
        setSelectedStrategy(null)
        setEditingConfig(null)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setIsLoading(false)
    }
  }, [token, strategyIdFromUrl, language])

  useEffect(() => {
    fetchStrategies()
  }, [fetchStrategies])

  useEffect(() => {
    if (!selectedStrategy?.id || !editingConfig?.grid_config) return

    gridConfigCacheRef.current[selectedStrategy.id] = {
      ...editingConfig.grid_config,
    }
  }, [selectedStrategy?.id, editingConfig?.grid_config])

  /** Token 估算（原 TokenEstimateBar 逻辑，新 UI 不再内嵌该组件） */
  useEffect(() => {
    if (!editingConfig) return
    const t = window.setTimeout(async () => {
      try {
        const response = await fetch(
          `${API_BASE}/api/strategies/estimate-tokens`,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ config: editingConfig }),
          }
        )
        if (response.ok) {
          const data = await response.json()
          setEstimatedTokens(data.total ?? 0)
          const limits = (
            data.model_limits as { context_limit?: number }[] | undefined
          )
            ?.map((m) => m.context_limit)
            .filter((n): n is number => typeof n === 'number' && n > 0)
          if (limits && limits.length > 0) {
            setTokenBudget(Math.min(...limits))
          } else {
            setTokenBudget(131072)
          }
        }
      } catch {
        /* 非关键 */
      }
    }, 600)
    return () => clearTimeout(t)
  }, [editingConfig])

  // Track previous language to detect actual changes
  const prevLanguageRef = useRef(language)

  // When language changes, update prompt sections to match the new language
  useEffect(() => {
    const updatePromptSectionsForLanguage = async () => {
      // Only update if language actually changed (not on initial mount)
      if (prevLanguageRef.current === language) return
      prevLanguageRef.current = language

      if (!token) return

      try {
        // Fetch default config for the new language
        const response = await fetch(
          `${API_BASE}/api/strategies/default-config?lang=${language}`,
          { headers: { Authorization: `Bearer ${token}` } }
        )
        if (!response.ok) return
        const defaultConfig = await response.json()

        // 随界面语言切换默认「一整段策略」模板
        setEditingConfig((prev) => {
          if (!prev) return prev
          return {
            ...prev,
            language: language as 'zh' | 'en',
            strategy_prompt: defaultConfig.strategy_prompt,
          }
        })
        setHasChanges(true)
      } catch (err) {
        console.error('Failed to update prompt sections for language:', err)
      }
    }

    updatePromptSectionsForLanguage()
  }, [language, token]) // Only trigger when language changes

  // Create new strategy
  const handleCreateStrategy = async () => {
    if (!token) return
    try {
      const configResponse = await fetch(
        `${API_BASE}/api/strategies/default-config?lang=${language}`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
      if (!configResponse.ok) throw new Error('Failed to fetch default config')
      const defaultConfig = await configResponse.json()

      const response = await fetch(`${API_BASE}/api/strategies`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          name: tr('newStrategyName'),
          description: '',
          config: defaultConfig,
        }),
      })
      if (!response.ok) throw new Error('Failed to create strategy')
      const result = await response.json()
      await fetchStrategies()
      // Auto-select the newly created strategy
      if (result.id) {
        const now = new Date().toISOString()
        const newStrategy = {
          id: result.id,
          name: tr('newStrategyName'),
          description: '',
          is_active: false,
          is_default: false,
          market_access: 'off' as StrategyMarketAccess,
          show_after_rename: false,
          is_public: false,
          config_visible: true,
          content_locked: false,
          config: defaultConfig,
          created_at: now,
          updated_at: now,
        }
        setSelectedStrategy(newStrategy)
        setEditingConfig(defaultConfig)
        setHasChanges(false)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  // Delete strategy
  const handleDeleteStrategy = async (id: string) => {
    if (!token) return
    const strategy = strategies.find((item) => item.id === id)

    if (strategy?.is_active) {
      notify.error(tr('cannotDeleteActiveStrategy'))
      return
    }

    // Check if strategy is in use by any trader before showing dialog
    try {
      const tradersResp = await fetch(`${API_BASE}/api/my-traders`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (tradersResp.ok) {
        const traderList = await tradersResp.json()
        const using = traderList.filter((t: any) => t.strategy_id === id)
        if (using.length > 0) {
          const names = using.map((t: any) => t.trader_name).join(', ')
          notify.error(`Strategy is in use by: ${names}`)
          return
        }
      }
    } catch {
      // fetch failed — proceed, backend will guard
    }

    const confirmed = await confirmToast(tr('confirmDeleteStrategy'), {
      title: tr('confirmDelete'),
      okText: tr('delete'),
      cancelText: tr('cancel'),
    })
    if (!confirmed) return

    try {
      const response = await fetch(`${API_BASE}/api/strategies/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!response.ok) {
        const data = await response.json().catch(() => ({}))
        notify.error(data.error || 'Failed to delete strategy')
        return
      }
      notify.success(tr('strategyDeleted'))
      if (selectedStrategy?.id === id) {
        setSelectedStrategy(null)
        setEditingConfig(null)
        setHasChanges(false)
      }
      await fetchStrategies()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  // Activate strategy
  const handleActivateStrategy = async (id: string) => {
    if (!token) return
    try {
      const response = await fetch(
        `${API_BASE}/api/strategies/${id}/activate`,
        {
          method: 'POST',
          headers: { Authorization: `Bearer ${token}` },
        }
      )
      if (!response.ok) throw new Error('Failed to activate strategy')
      await fetchStrategies()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  /** 已上架策略：把当前内容推送到策略市场并递增 market_revision，便于订阅方检测新版本 */
  const handlePublishMarketUpdate = async () => {
    if (!token || !selectedStrategy || !editingConfig) return
    if (effectiveMarketAccess(selectedStrategy) === 'off') {
      notify.error(tr('marketUpdateNeedsPublic'))
      return
    }
    setIsPublishingMarket(true)
    try {
      const configWithLanguage = {
        ...editingConfig,
        language: language as 'zh' | 'en',
      }
      const response = await fetch(
        `${API_BASE}/api/strategies/${selectedStrategy.id}/market-update`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({
            name: selectedStrategy.name,
            description: selectedStrategy.description,
            config: configWithLanguage,
            market_access: effectiveMarketAccess(selectedStrategy),
          }),
        }
      )
      const data = await response.json().catch(() => ({}))
      if (!response.ok) {
        notify.error(
          typeof data.error === 'string' ? data.error : tr('marketUpdateFailed')
        )
        return
      }
      setHasChanges(false)
      const rev =
        typeof data.market_revision === 'number'
          ? data.market_revision
          : undefined
      const updatedAt =
        typeof data.updated_at === 'string' ? data.updated_at : undefined
      setSelectedStrategy((s) => {
        if (!s || s.id !== selectedStrategy.id) return s
        return {
          ...s,
          ...(rev != null ? { market_revision: rev } : {}),
          ...(updatedAt ? { updated_at: updatedAt } : {}),
        }
      })
      setStrategies((list) =>
        list.map((s) =>
          s.id === selectedStrategy.id
            ? {
                ...s,
                ...(rev != null ? { market_revision: rev } : {}),
                ...(updatedAt ? { updated_at: updatedAt } : {}),
              }
            : s
        )
      )
      notify.success(
        rev != null
          ? tr('marketUpdateSuccess').replace('{{rev}}', String(rev))
          : tr('marketUpdateSuccess').replace('{{rev}}', '?')
      )
    } catch (err) {
      notify.error(
        err instanceof Error ? err.message : tr('marketUpdateFailed')
      )
    } finally {
      setIsPublishingMarket(false)
    }
  }

  /** 保存策略到服务器（构建器保存按钮与「发布到市场」弹窗共用） */
  const putStrategyToServer = async (
    strategyId: string,
    payload: {
      name: string
      description: string
      config: StrategyConfig
      market_access: StrategyMarketAccess
    }
  ) => {
    if (!token) throw new Error('Unauthorized')
    const configWithLanguage = {
      ...payload.config,
      language: language as 'zh' | 'en',
    }
    const response = await fetch(`${API_BASE}/api/strategies/${strategyId}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        name: payload.name,
        description: payload.description,
        config: configWithLanguage,
        market_access: payload.market_access,
      }),
    })
    if (!response.ok) {
      let msg = 'Failed to save strategy'
      try {
        const j = (await response.json()) as { error?: string }
        if (j?.error) msg = j.error
      } catch {
        /* ignore */
      }
      throw new Error(msg)
    }
  }

  // Save strategy
  const handleSaveStrategy = async () => {
    if (!token || !selectedStrategy || !editingConfig) return
    if (estimatedTokens >= 128000 && currentStrategyType === 'ai_trading') {
      notify.warning(tr('tokenExceedWarning'))
      // continue with save
    }
    setIsSaving(true)
    try {
      await putStrategyToServer(selectedStrategy.id, {
        name: selectedStrategy.name,
        description: selectedStrategy.description,
        config: editingConfig,
        market_access: rawMarketAccess(selectedStrategy),
      })
      setHasChanges(false)
      notify.success(tr('strategySaved'))
      await fetchStrategies()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setIsSaving(false)
    }
  }

  // Update config section
  const updateConfig = <K extends keyof StrategyConfig>(
    section: K,
    value: StrategyConfig[K]
  ) => {
    setEditingConfig((prev) => {
      if (!prev) return prev
      return {
        ...prev,
        [section]: value,
      }
    })
    setHasChanges(true)
  }

  const handleStrategyTypeChange = (
    strategyType: NonNullable<StrategyConfig['strategy_type']>
  ) => {
    if (selectedStrategy?.is_default) return
    if (
      strategyType === 'program_martingale' &&
      !isProgramMartingaleStrategyStudioStrategy(selectedStrategy?.id)
    ) {
      return
    }

    const cachedGridConfig = selectedStrategy?.id
      ? gridConfigCacheRef.current[selectedStrategy.id]
      : null

    setEditingConfig((prev) => {
      if (!prev) return prev

      if (strategyType === 'ai_trading') {
        if (selectedStrategy?.id && prev.grid_config) {
          gridConfigCacheRef.current[selectedStrategy.id] = {
            ...prev.grid_config,
          }
        }

        return {
          ...prev,
          strategy_type: 'ai_trading',
          grid_config: null,
          martingale_program: null,
        }
      }

      if (strategyType === 'program_martingale') {
        return {
          ...prev,
          strategy_type: 'program_martingale',
          grid_config: null,
          martingale_program: prev.martingale_program ?? {
            ...defaultMartingaleProgramConfig,
          },
          coin_source: {
            source_type: 'static',
            static_coins: ['XAUUSDT'],
            use_ai500: false,
            use_oi_top: false,
            use_oi_low: false,
            use_hyper_all: false,
            use_hyper_main: false,
          },
          risk_control: {
            ...prev.risk_control,
            max_positions: 1,
            btc_eth_max_leverage: 20,
            altcoin_max_leverage: 20,
            max_margin_usage: 0.06,
          },
        }
      }

      return {
        ...prev,
        strategy_type: 'grid_trading',
        martingale_program: null,
        grid_config: cachedGridConfig ??
          prev.grid_config ?? { ...defaultGridConfig },
      }
    })

    setHasChanges(true)
  }

  const tr = (key: string) => t(`strategyStudio.${key}`, language)

  if (isLoading) {
    return (
      <div className="flex min-h-[70vh] items-center justify-center bg-nofx-bg">
        <div className="text-center">
          <div className="relative">
            <div className="h-16 w-16 animate-spin rounded-full border-4 border-[#d4ff33]/20 border-t-[#d4ff33]" />
            <Zap className="absolute left-1/2 top-1/2 h-6 w-6 -translate-x-1/2 -translate-y-1/2 text-[#d4ff33]" />
          </div>
        </div>
      </div>
    )
  }

  // Get current strategy type (default to ai_trading if not set)
  const currentStrategyType = editingConfig?.strategy_type || 'ai_trading'

  return (
    <div className="flex min-h-[calc(100vh-4rem)] flex-col bg-surface text-on-surface lg:h-[calc(100vh-4rem)] lg:min-h-0 lg:overflow-hidden">
      {error && (
        <div className="shrink-0 border-b border-error/30 bg-error/10 px-4 py-2 text-center text-xs text-error">
          {error}{' '}
          <button
            type="button"
            onClick={() => setError(null)}
            className="underline"
          >
            关闭
          </button>
        </div>
      )}

      <div className="flex min-h-0 flex-1 flex-col overflow-visible lg:flex-row lg:overflow-hidden">
        <div className="z-10 max-h-44 shrink-0 overflow-y-auto border-b border-[#46484d]/30 bg-nofx-bg-tertiary lg:max-h-none lg:w-52 lg:border-b-0 lg:border-r">
          <div className="p-2">
            <div className="flex items-center justify-between mb-2 px-2">
              <span className="text-xs font-medium text-[#aaabb0]">
                {tr('strategies')}
              </span>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={handleCreateStrategy}
                  className="rounded p-1 text-[#d4ff33] transition-colors hover:bg-[#d4ff33]/10"
                  title={tr('newStrategyTooltip')}
                >
                  <Plus className="w-4 h-4" />
                </button>
              </div>
            </div>
            <div className="flex gap-2 overflow-x-auto pb-1 lg:block lg:space-y-2 lg:overflow-x-visible lg:pb-0">
              {strategies.map((strategy) => (
                <div
                  key={strategy.id}
                  onClick={() => {
                    setSelectedStrategy(strategy)
                    setEditingConfig(strategy.config)
                    setHasChanges(false)
                  }}
                  className={`group min-w-[160px] cursor-pointer rounded-lg px-2 py-2 transition-all lg:min-w-0 ${
                    selectedStrategy?.id === strategy.id
                      ? 'bg-[#d4ff33]/10 shadow-[0_0_12px_rgba(212,255,51,0.12)] ring-1 ring-[#d4ff33]/45'
                      : 'bg-transparent ring-1 ring-[#46484d]/40 hover:bg-white/5 hover:ring-[#d4ff33]/25'
                  }`}
                >
                  <div className="flex items-start justify-between">
                    <span
                      className={`line-clamp-2 text-[#f6f6fc] ${language === 'zh' ? 'text-sm' : 'text-xs'}`}
                    >
                      {strategy.name}
                    </span>
                    {!strategy.is_default && (
                      <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                        <button
                          onClick={(e) => {
                            e.stopPropagation()
                            handleDeleteStrategy(strategy.id)
                          }}
                          disabled={strategy.is_active}
                          className="rounded p-1 text-red-400 hover:bg-red-500/20 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
                          title={
                            strategy.is_active
                              ? tr('cannotDeleteActiveStrategy')
                              : tr('deleteTooltip')
                          }
                        >
                          <Trash2 className="w-3 h-3" />
                        </button>
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-1 mt-1 flex-wrap">
                    {strategy.is_active && (
                      <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] text-emerald-400">
                        {tr('active')}
                      </span>
                    )}
                    {strategy.is_default && (
                      <span className="rounded bg-[#d4ff33]/15 px-1.5 py-0.5 text-[10px] text-[#d4ff33]">
                        {tr('default')}
                      </span>
                    )}
                    {effectiveMarketAccess(strategy) !== 'off' && (
                      <span className="flex items-center gap-0.5 rounded bg-blue-400/15 px-1.5 py-0.5 text-[10px] text-blue-400">
                        <Globe className="h-2.5 w-2.5" />
                        {effectiveMarketAccess(strategy) === 'subscription' &&
                          tr('marketAccessSubscription')}
                        {effectiveMarketAccess(strategy) === 'public' &&
                          tr('marketAccessPublic')}
                        {effectiveMarketAccess(strategy) === 'open_source' &&
                          tr('marketAccessOpenSource')}
                        {effectiveMarketAccess(strategy) === 'private' &&
                          tr('marketAccessPrivate')}
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Middle Column - Config Editor（内部标签切换网格 / AI，本列负责纵向撑满与滚动） */}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-visible bg-surface lg:overflow-hidden">
          {selectedStrategy && editingConfig ? (
            <div className="flex min-h-0 flex-1 flex-col gap-3 p-3 sm:p-4">
              {/* Strategy Name & Actions */}
              <div className="flex shrink-0 flex-col gap-3 border-b border-outline-variant/20 pb-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex-1 min-w-0">
                  <input
                    type="text"
                    value={selectedStrategy.name}
                    onChange={(e) => {
                      setSelectedStrategy({
                        ...selectedStrategy,
                        name: e.target.value,
                      })
                      setHasChanges(true)
                    }}
                    disabled={
                      selectedStrategy.is_default ||
                      isLockedMarketCopy(selectedStrategy)
                    }
                    className="w-full border-none bg-transparent text-lg font-bold text-[#f6f6fc] outline-none placeholder:text-[#aaabb0]/60"
                  />
                  <input
                    type="text"
                    value={selectedStrategy.description || ''}
                    onChange={(e) => {
                      setSelectedStrategy({
                        ...selectedStrategy,
                        description: e.target.value,
                      })
                      setHasChanges(true)
                    }}
                    disabled={
                      selectedStrategy.is_default ||
                      isLockedMarketCopy(selectedStrategy)
                    }
                    placeholder={tr('addDescription')}
                    className="mt-1 w-full border-none bg-transparent text-xs text-[#aaabb0] outline-none placeholder:text-[#aaabb0]/50"
                  />
                  {hasChanges && (
                    <span className="text-xs text-[#d4ff33]">
                      ● {tr('unsaved')}
                    </span>
                  )}
                </div>
                <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:flex-shrink-0">
                  {!selectedStrategy.is_active && (
                    <button
                      onClick={() =>
                        handleActivateStrategy(selectedStrategy.id)
                      }
                      className="flex flex-1 items-center justify-center gap-1 rounded-lg border border-emerald-500/35 bg-emerald-500/10 px-3 py-1.5 text-xs text-emerald-400 transition-colors hover:bg-emerald-500/20 sm:flex-none"
                    >
                      <Check className="w-3 h-3" />
                      {tr('activate')}
                    </button>
                  )}
                  {!selectedStrategy.is_default &&
                    !isLockedMarketCopy(selectedStrategy) && (
                      <>
                        <button
                          type="button"
                          onClick={() => {
                            const cur = editingConfig.market_sale_price_usdt
                            setSalePriceDraft(
                              cur != null && cur > 0 ? String(cur) : ''
                            )
                            setListingDraft(
                              effectiveMarketAccess(selectedStrategy) ===
                                'off' ||
                                effectiveMarketAccess(selectedStrategy) ===
                                  'private'
                                ? 'subscription'
                                : effectiveMarketAccess(selectedStrategy)
                            )
                            setPublishMarketOpen(true)
                          }}
                          className="flex flex-1 items-center justify-center gap-1 rounded-lg border border-primary/40 bg-primary/10 px-3 py-1.5 text-xs font-medium text-primary transition-colors hover:bg-primary/20 sm:flex-none"
                        >
                          <Tag className="h-3 w-3" />
                          {tr('marketListingSection')}
                        </button>
                        {effectiveMarketAccess(selectedStrategy) !== 'off' && (
                          <button
                            type="button"
                            onClick={() => void handlePublishMarketUpdate()}
                            disabled={isPublishingMarket || isSaving}
                            title={tr('marketUpdateHint')}
                            className="flex flex-1 items-center justify-center gap-1 rounded-lg border border-[#d4ff33]/45 bg-[#d4ff33]/10 px-3 py-1.5 text-xs font-medium text-[#d4ff33] transition-colors hover:bg-[#d4ff33]/18 disabled:opacity-50 sm:flex-none"
                          >
                            <RefreshCw
                              className={`h-3 w-3 ${isPublishingMarket ? 'animate-spin' : ''}`}
                            />
                            {isPublishingMarket
                              ? tr('marketUpdating')
                              : tr('marketUpdate')}
                          </button>
                        )}
                        <button
                          onClick={handleSaveStrategy}
                          disabled={
                            isSaving || !hasChanges || isPublishingMarket
                          }
                          className={`flex flex-1 items-center justify-center gap-1 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50 sm:flex-none ${
                            hasChanges
                              ? 'bg-[#d4ff33] text-black hover:bg-[#d4ff33]'
                              : 'cursor-not-allowed bg-nofx-bg-secondary text-[#aaabb0]'
                          }`}
                        >
                          <Save className="w-3 h-3" />
                          {isSaving ? tr('saving') : tr('save')}
                        </button>
                      </>
                    )}
                </div>
              </div>

              {publishMarketOpen && selectedStrategy && editingConfig && (
                <div
                  className="fixed inset-0 z-[200] flex items-start justify-center overflow-y-auto bg-black/60 p-3 backdrop-blur-sm sm:items-center sm:p-4"
                  role="dialog"
                  aria-modal="true"
                  aria-labelledby="publish-market-title"
                >
                  <div className="relative w-full max-w-lg rounded-2xl border border-outline-variant/30 bg-surface-container-high p-5 shadow-2xl">
                    <button
                      type="button"
                      className="absolute right-3 top-3 rounded p-1 text-on-surface-variant hover:bg-surface-container-lowest hover:text-on-surface"
                      onClick={() => setPublishMarketOpen(false)}
                      aria-label="关闭"
                    >
                      <X className="h-4 w-4" />
                    </button>
                    <h3
                      id="publish-market-title"
                      className="pr-8 text-lg font-bold text-on-surface"
                    >
                      {tr('marketListingSection')}
                    </h3>
                    <p className="mt-1 text-xs leading-relaxed text-on-surface-variant">
                      {tr('marketListingHint')}
                    </p>
                    <div className="mt-4 max-h-[52vh] space-y-2 overflow-y-auto pr-1">
                      {(
                        [
                          ['subscription', 'marketAccessSubscription'],
                          ['public', 'marketAccessPublic'],
                          ['open_source', 'marketAccessOpenSource'],
                        ] as const
                      ).map(([value, labelKey]) => (
                        <label
                          key={value}
                          className={`flex cursor-pointer items-start gap-2 rounded-lg border px-3 py-2 text-sm transition-colors ${
                            listingDraft === value
                              ? 'border-primary bg-primary/10 text-on-surface'
                              : 'border-outline-variant/25 bg-surface-container-lowest/80 text-on-surface-variant hover:border-outline-variant/50'
                          }`}
                        >
                          <input
                            type="radio"
                            name="market-listing"
                            className="mt-0.5 accent-primary"
                            checked={listingDraft === value}
                            onChange={() => setListingDraft(value)}
                          />
                          <span className="text-on-surface">
                            {tr(labelKey)}
                          </span>
                        </label>
                      ))}
                    </div>
                    <div className="mt-4 rounded-lg border border-outline-variant/20 bg-surface-container-lowest/60 p-3">
                      <div className="text-xs font-semibold text-on-surface">
                        {tr('marketSaleSection')}
                      </div>
                      <p className="mt-1 text-[11px] leading-relaxed text-on-surface-variant">
                        {tr('marketSaleHint')}
                      </p>
                      <input
                        type="number"
                        min={0}
                        step={0.01}
                        value={salePriceDraft}
                        onChange={(e) => setSalePriceDraft(e.target.value)}
                        placeholder="USDT"
                        className="mt-2 w-full rounded-lg border border-outline-variant/25 bg-surface-container-lowest px-3 py-2 text-sm text-on-surface focus:outline-none"
                      />
                    </div>
                    <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                      <button
                        type="button"
                        className="rounded-lg border border-outline-variant/30 px-4 py-2 text-sm text-on-surface-variant hover:bg-surface-container-lowest"
                        onClick={() => setPublishMarketOpen(false)}
                      >
                        {tr('cancel')}
                      </button>
                      <button
                        type="button"
                        disabled={isSaving}
                        className="rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-black hover:bg-[#d4ff33] disabled:opacity-50"
                        onClick={() => {
                          void (async () => {
                            const raw = salePriceDraft.trim()
                            const p = raw === '' ? NaN : parseFloat(raw)
                            if (listingDraft === 'subscription') {
                              if (!Number.isFinite(p) || p <= 0) {
                                notify.error(
                                  language === 'zh'
                                    ? '选择「需订阅」时请填写大于 0 的售卖价格'
                                    : 'Subscription requires sale price > 0'
                                )
                                return
                              }
                            }
                            if (!token || !selectedStrategy || !editingConfig)
                              return

                            const nextConfig: StrategyConfig = {
                              ...editingConfig,
                            }
                            if (listingDraft === 'subscription') {
                              nextConfig.market_sale_price_usdt = p
                            } else if (Number.isFinite(p) && p >= 0) {
                              nextConfig.market_sale_price_usdt = p
                            } else {
                              delete nextConfig.market_sale_price_usdt
                            }

                            setIsSaving(true)
                            try {
                              await putStrategyToServer(selectedStrategy.id, {
                                name: selectedStrategy.name,
                                description: selectedStrategy.description,
                                config: nextConfig,
                                market_access: listingDraft,
                              })
                              setEditingConfig(nextConfig)
                              setSelectedStrategy((s) => {
                                if (!s) return s
                                return {
                                  ...s,
                                  market_access: listingDraft,
                                  is_public: listingDraft !== 'off',
                                  config_visible:
                                    listingDraft === 'open_source',
                                }
                              })
                              setHasChanges(false)
                              setPublishMarketOpen(false)
                              notify.success(tr('strategySaved'))
                              await fetchStrategies()
                            } catch (err) {
                              notify.error(
                                err instanceof Error ? err.message : '保存失败'
                              )
                            } finally {
                              setIsSaving(false)
                            }
                          })()
                        }}
                      >
                        {isSaving ? tr('saving') : tr('listingDialogConfirm')}
                      </button>
                    </div>
                  </div>
                </div>
              )}

              {/* 必须是 flex 列，子面板里的 flex-1 + overflow-y-auto 才能拿到固定高度并出现纵向滚动 */}
              <div className="flex min-h-0 flex-1 flex-col overflow-visible lg:overflow-hidden">
                {isLockedMarketCopy(selectedStrategy) ? (
                  <div className="flex min-h-0 flex-1 items-center justify-center rounded-2xl border border-[#d4ff33]/20 bg-[#d4ff33]/5 p-8 text-center">
                    <div className="max-w-lg">
                      <Globe className="mx-auto mb-4 h-10 w-10 text-[#d4ff33]" />
                      <h2 className="text-xl font-bold text-white">
                        该作者未公开策略内容
                      </h2>
                      <p className="mt-3 text-sm leading-relaxed text-zinc-400">
                        该策略只可使用，不能查看或修改内容。你仍然可以在创建交易员时选择它运行。
                      </p>
                    </div>
                  </div>
                ) : (
                  <StrategyStudioLuminescentPanels
                    editingConfig={editingConfig}
                    selectedStrategy={selectedStrategy}
                    currentStrategyType={currentStrategyType}
                    updateConfig={updateConfig}
                    editorsDisabled={!!selectedStrategy.is_default}
                    gridEditorDisabled={currentStrategyType !== 'grid_trading'}
                    aiEditorDisabled={
                      currentStrategyType !== 'ai_trading' &&
                      currentStrategyType !== 'program_martingale'
                    }
                    martingaleEditorDisabled={
                      !isProgramMartingaleStrategyStudioStrategy(
                        selectedStrategy.id
                      ) || currentStrategyType !== 'program_martingale'
                    }
                    estimatedTokens={estimatedTokens}
                    tokenBudget={tokenBudget}
                    onStrategyTypeChange={handleStrategyTypeChange}
                    strategyTypeSwitchDisabled={!!selectedStrategy.is_default}
                  />
                )}
              </div>
            </div>
          ) : (
            <div className="flex h-full items-center justify-center">
              <div className="text-center">
                <Activity className="mx-auto mb-2 h-12 w-12 opacity-30 text-[#aaabb0]" />
                <p className="text-sm text-[#aaabb0]">{tr('selectOrCreate')}</p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default StrategyStudioPage
