/**
 * Luminescent Quant 策略实验室 — 全新 UI（对齐 /root/ai_1、/root/ai_2 原型）
 * 不引用旧版 CoinSource / Indicator / Grid 等编辑器组件，仅通过 updateConfig 写回同一套 API 数据结构。
 */
import { lazy, Suspense, useLayoutEffect, useRef, useState } from 'react'
import type {
  Strategy,
  StrategyConfig,
  CoinSourceConfig,
  IndicatorConfig,
  PromptSectionsConfig,
  MartingaleProgramConfig,
} from '../../../types/strategy'
import {
  defaultMartingaleProgramConfig,
  isProgramMartingaleStrategyStudioStrategy,
} from '../../../types/strategy'
import { notify } from '../../../lib/notify'
import { defaultGridConfig } from '../GridConfigEditor'
import '../../../pages/landing/luminescent.css'
import './strategy-quant-lab.css'
import { Activity, LayoutGrid, X } from 'lucide-react'
import { ComkunListingMasterStrategyBuilder } from './ComkunListingMasterStrategyBuilder'

const StrategyPromptMonacoEditor = lazy(
  () => import('./StrategyPromptMonacoEditor')
)

const GRID_SYMBOLS = [
  { value: 'BTCUSDT', label: 'BTC/USDT' },
  { value: 'ETHUSDT', label: 'ETH/USDT' },
  { value: 'SOLUSDT', label: 'SOL/USDT' },
  { value: 'BNBUSDT', label: 'BNB/USDT' },
  { value: 'XRPUSDT', label: 'XRP/USDT' },
]

const TIMEFRAMES = [
  '1m',
  '3m',
  '5m',
  '15m',
  '30m',
  '1h',
  '2h',
  '4h',
  '1d',
] as const
const HZ_INSTRUMENTS = new Set(['XAUUSD', 'XAGUSD', 'WTIUSD'])

function isHZCoinSource(source: CoinSourceConfig): boolean {
  const coins = (source.static_coins || [])
    .map((coin) => coin.trim().toUpperCase())
    .filter(Boolean)
  return (
    coins.length > 0 &&
    coins.every((coin) => HZ_INSTRUMENTS.has(coin)) &&
    !source.use_ai500 &&
    !source.use_oi_top &&
    !source.use_oi_low
  )
}

/** 旧数据只有四段字段时，拼成一段供展示；一旦编辑会写入 strategy_prompt */
function mergedLegacyPromptSections(ps?: PromptSectionsConfig): string {
  if (!ps) return ''
  const parts = [
    ps.role_definition,
    ps.trading_frequency,
    ps.entry_standards,
    ps.decision_process,
  ]
    .map((s) => (s || '').trim())
    .filter(Boolean)
  return parts.join('\n\n')
}

function strategyPromptDisplay(cfg: StrategyConfig): string {
  const sp = cfg.strategy_prompt
  if (sp != null && String(sp).trim() !== '') return sp
  return mergedLegacyPromptSections(cfg.prompt_sections)
}

function syncCoinSourceType(cs: CoinSourceConfig): CoinSourceConfig {
  const c = { ...cs }
  const hasStatic = (c.static_coins?.length ?? 0) > 0
  const dyn = [c.use_ai500, c.use_oi_top, c.use_oi_low].filter(Boolean).length
  if (dyn >= 2 || (dyn >= 1 && hasStatic) || (c.use_ai500 && c.use_oi_top)) {
    c.source_type = 'mixed'
  } else if (c.use_ai500) c.source_type = 'ai500'
  else if (c.use_oi_top) c.source_type = 'oi_top'
  else if (c.use_oi_low) c.source_type = 'oi_low'
  else c.source_type = 'static'
  return c
}

/** 交互统一用同一条缓动曲线，手感更顺 */
const Q_EASE = 'duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]'

function chip(active: boolean) {
  return active
    ? `rounded-full border border-primary/40 bg-surface-container-highest px-3 py-1.5 text-xs font-semibold text-primary transition-[color,background-color,border-color,box-shadow] ${Q_EASE}`
    : `rounded-full border border-outline-variant/20 bg-surface-container-lowest px-3 py-1.5 text-xs text-on-surface-variant transition-[color,background-color,border-color,box-shadow,opacity] ${Q_EASE} hover:border-primary/40`
}

/** 与后端 store 默认一致，用于首次打开某指标时预填 */
const DEFAULT_EMA_PERIODS = [20, 50] as const
const DEFAULT_RSI_PERIODS = [7, 14] as const
const DEFAULT_ATR_PERIODS = [14] as const
const DEFAULT_BOLL_PERIODS = [20] as const

/** 从输入框解析周期；没有合法数字时用 fallback */
function parseIndicatorPeriods(
  raw: string,
  fallback: readonly number[]
): number[] {
  const nums = raw
    .split(/[,，\s]+/)
    .map((s) => parseInt(s.trim(), 10))
    .filter((n) => !Number.isNaN(n) && n > 0 && n <= 500)
  return nums.length > 0 ? nums : [...fallback]
}

const periodInputClass =
  'min-w-0 flex-1 rounded border border-outline-variant/25 bg-surface-container-highest px-2 py-1.5 font-mono text-xs text-on-surface focus:outline-none'

const FLOW_INDICATOR_EXPLAINS = [
  {
    key: 'enable_quant_oi',
    label: 'OI 分析',
    desc: '看持仓量变化，判断资金是在加仓、撤退，还是可能酝酿逼空/砸盘。',
  },
  {
    key: 'enable_quant_netflow',
    label: 'Netflow',
    desc: '看交易所净流入/净流出，辅助判断资金是在进场还是离场。',
  },
  {
    key: 'enable_oi_ranking',
    label: 'OI 排行榜',
    desc: '按持仓量增减排名，快速发现资金关注度突然升高或降低的币种。',
  },
  {
    key: 'enable_quant_data',
    label: '资金流向',
    desc: '汇总机构/散户资金流方向，帮助 AI 判断多空资金偏向。',
  },
  {
    key: 'enable_price_ranking',
    label: '涨跌幅排行',
    desc: '查看强势上涨和弱势下跌币种，辅助识别短线热点与风险。',
  },
] as const

const SENTIMENT_INDICATOR_EXPLAINS = [
  {
    key: 'enable_volume',
    label: '成交量',
    desc: '看市场交易是否活跃，放量通常代表分歧或趋势正在加强。',
  },
  {
    key: 'enable_oi',
    label: '持仓量',
    desc: '看合约未平仓规模，配合价格判断新增资金是在追多还是追空。',
  },
  {
    key: 'enable_funding_rate',
    label: '资金费率',
    desc: '看多空拥挤程度；费率过高可能多头拥挤，过低可能空头拥挤。',
  },
] as const

export interface StrategyStudioLuminescentPanelsProps {
  editingConfig: StrategyConfig
  selectedStrategy: Strategy
  currentStrategyType: 'ai_trading' | 'grid_trading' | 'program_martingale'
  updateConfig: <K extends keyof StrategyConfig>(
    section: K,
    value: StrategyConfig[K]
  ) => void
  editorsDisabled: boolean
  gridEditorDisabled: boolean
  aiEditorDisabled: boolean
  estimatedTokens: number
  /** 进度条 100% 对应的上限（与后端 estimate-tokens 的 model_limits 一致） */
  tokenBudget: number
  onStrategyTypeChange?: (
    type: 'ai_trading' | 'grid_trading' | 'program_martingale'
  ) => void
  strategyTypeSwitchDisabled?: boolean
  martingaleEditorDisabled?: boolean
}

export function StrategyStudioLuminescentPanels({
  editingConfig,
  selectedStrategy,
  currentStrategyType,
  updateConfig,
  editorsDisabled,
  gridEditorDisabled,
  aiEditorDisabled,
  estimatedTokens,
  tokenBudget,
  onStrategyTypeChange,
  strategyTypeSwitchDisabled = false,
  martingaleEditorDisabled: _martingaleEditorDisabled = false,
}: StrategyStudioLuminescentPanelsProps) {
  const gridCfg = editingConfig.grid_config ?? { ...defaultGridConfig }
  const mpCfg = editingConfig.martingale_program ?? {
    ...defaultMartingaleProgramConfig,
  }
  const ind = editingConfig.indicators
  const coin = editingConfig.coin_source
  const rc = editingConfig.risk_control

  /** OHLCV 为必选：若历史数据里被关掉，自动写回 true，与界面锁定一致 */
  useLayoutEffect(() => {
    if (currentStrategyType !== 'ai_trading') return
    if (editingConfig.indicators.enable_raw_klines === false) {
      updateConfig('indicators', {
        ...editingConfig.indicators,
        enable_raw_klines: true,
      })
    }
  }, [
    currentStrategyType,
    selectedStrategy.id,
    editingConfig.indicators.enable_raw_klines,
    editingConfig.indicators,
    updateConfig,
  ])

  const [staticModalOpen, setStaticModalOpen] = useState(false)
  const [staticModalDraft, setStaticModalDraft] = useState('')
  const staticModalCtx = useRef<{ hadCoins: boolean }>({ hadCoins: false })
  const hzMode = isHZCoinSource(coin)

  const setRc = (patch: Partial<typeof rc>) => {
    updateConfig('risk_control', { ...rc, ...patch })
  }

  const setCoin = (patch: Partial<CoinSourceConfig>) => {
    const next = syncCoinSourceType({ ...coin, ...patch })
    updateConfig('coin_source', next)
    if (isHZCoinSource(next)) {
      const leverage =
        rc.altcoin_max_leverage >= 100 && rc.altcoin_max_leverage <= 2000
          ? rc.altcoin_max_leverage
          : rc.btc_eth_max_leverage >= 100 && rc.btc_eth_max_leverage <= 2000
            ? rc.btc_eth_max_leverage
            : 500
      setRc({ btc_eth_max_leverage: leverage, altcoin_max_leverage: leverage })
    }
  }

  const setInd = (patch: Partial<IndicatorConfig>) => {
    updateConfig('indicators', { ...ind, ...patch })
  }

  useLayoutEffect(() => {
    if (!hzMode) return
    const leverage =
      rc.altcoin_max_leverage >= 100 && rc.altcoin_max_leverage <= 2000
        ? rc.altcoin_max_leverage
        : rc.btc_eth_max_leverage >= 100 && rc.btc_eth_max_leverage <= 2000
          ? rc.btc_eth_max_leverage
          : 500
    if (
      rc.btc_eth_max_leverage !== leverage ||
      rc.altcoin_max_leverage !== leverage
    ) {
      setRc({ btc_eth_max_leverage: leverage, altcoin_max_leverage: leverage })
    }
  }, [hzMode, rc.btc_eth_max_leverage, rc.altcoin_max_leverage])

  const tokenPct = Math.min(
    100,
    tokenBudget > 0 ? Math.round((estimatedTokens / tokenBudget) * 100) : 0
  )
  const approxChars = Math.round(
    estimatedTokens * (editingConfig.language === 'zh' ? 2 : 4)
  )

  const tabBtn = (active: boolean) =>
    active
      ? `flex flex-1 items-center justify-center gap-2 rounded-lg bg-primary-container/15 py-2.5 text-sm font-bold text-primary-container ring-1 ring-primary-container/40 transition-[color,background-color,box-shadow,transform] ${Q_EASE}`
      : `flex flex-1 items-center justify-center gap-2 rounded-lg py-2.5 text-sm font-medium text-on-surface-variant transition-[color,background-color,box-shadow,transform] ${Q_EASE} hover:bg-surface-container-highest hover:text-on-surface`

  const dis = editorsDisabled
  const gridDis = dis || gridEditorDisabled
  const aiDis = dis || aiEditorDisabled
  const setMp = (patch: Partial<MartingaleProgramConfig>) => {
    updateConfig('martingale_program', { ...mpCfg, ...patch })
  }

  const martingaleStudioPanel = (
    <div
      className={`mx-auto max-w-3xl space-y-6 ${dis ? 'pointer-events-none opacity-50' : ''}`}
    >
      <section className="quant-lab-glass rounded-xl border border-primary-container/20 p-4 sm:p-6">
        <h3 className="mb-2 text-lg font-bold text-on-surface">
          程序化马丁（COMKUN-AI · 无真实 LLM）
        </h3>
        <p className="mb-4 text-sm text-on-surface-variant">
          4h EMA20/50 判大趋势后开首层；逆势按间距补第 2～7
          层。各层保证金按净值比例拆分，账户大小不同也保持同一占比。
        </p>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className="text-xs text-on-surface-variant">交易对</label>
            <input
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={mpCfg.symbol}
              disabled={dis}
              onChange={(e) => setMp({ symbol: e.target.value.toUpperCase() })}
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              只交易这一个合约，如 XAUUSDT。
            </p>
          </div>
          <div>
            <label className="text-xs text-on-surface-variant">
              杠杆（默认 20x）
            </label>
            <input
              type="number"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={mpCfg.leverage}
              disabled={dis}
              onChange={(e) =>
                setMp({ leverage: parseInt(e.target.value, 10) || 20 })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              开仓前会同步到交易所该交易对杠杆。
            </p>
          </div>
          <div>
            <label className="text-xs text-on-surface-variant">
              最大层数（1～7 手）
            </label>
            <input
              type="number"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={mpCfg.max_layers}
              disabled={dis}
              onChange={(e) =>
                setMp({
                  max_layers: Math.min(
                    12,
                    Math.max(1, parseInt(e.target.value, 10) || 7)
                  ),
                })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              首仓算第 1 层，逆势最多再补到该层数。
            </p>
          </div>
          <div>
            <label className="text-xs text-on-surface-variant">
              合计保证金占净值 %
            </label>
            <input
              type="number"
              step="0.1"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={(mpCfg.margin_budget_pct * 100).toFixed(2)}
              disabled={dis}
              onChange={(e) =>
                setMp({
                  margin_budget_pct: (parseFloat(e.target.value) || 0) / 100,
                })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              7 层保证金总和占账户净值的比例；各层再按权重拆分。
            </p>
          </div>
          <div>
            <label className="text-xs text-on-surface-variant">
              补仓间距 %（相对首仓价×层号）
            </label>
            <input
              type="number"
              step="0.01"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={(mpCfg.add_step_pct * 100).toFixed(2)}
              disabled={dis}
              onChange={(e) =>
                setMp({ add_step_pct: (parseFloat(e.target.value) || 0) / 100 })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              多单：现价 ≤ 首仓价×(1−间距×层号) 才补下一层；空单对称向上。
            </p>
          </div>
          <div>
            <label className="text-xs text-on-surface-variant">
              Basket 止盈 ROE %
            </label>
            <input
              type="number"
              step="0.1"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={(mpCfg.basket_take_profit_roe * 100).toFixed(2)}
              disabled={dis}
              onChange={(e) =>
                setMp({
                  basket_take_profit_roe:
                    (parseFloat(e.target.value) || 0) / 100,
                })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              整篮浮盈 ÷ 已用保证金 ≥ 该值时全部平仓。
            </p>
          </div>
          <div>
            <label className="text-xs text-on-surface-variant">
              趋势最小分离 %
            </label>
            <input
              type="number"
              step="0.01"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={(mpCfg.trend_min_sep_pct * 100).toFixed(3)}
              disabled={dis}
              onChange={(e) =>
                setMp({
                  trend_min_sep_pct: (parseFloat(e.target.value) || 0) / 100,
                })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              4h EMA20 与 EMA50 偏离不够大时视为震荡，不开首仓。
            </p>
          </div>
          <label className="flex items-center gap-2 text-sm sm:col-span-2">
            <input
              type="checkbox"
              checked={mpCfg.allow_short}
              disabled={dis}
              onChange={(e) => setMp({ allow_short: e.target.checked })}
            />
            允许做空（4h 空头趋势）
          </label>
          <div>
            <label className="text-xs text-on-surface-variant">
              止损 Basket ROE %
            </label>
            <input
              type="number"
              step="0.1"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={((mpCfg.max_basket_loss_roe ?? 0.12) * 100).toFixed(1)}
              disabled={dis}
              onChange={(e) =>
                setMp({
                  max_basket_loss_roe: (parseFloat(e.target.value) || 0) / 100,
                })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              浮亏相对保证金达该比例全平（默认 12%）。
            </p>
          </div>
          <div>
            <label className="text-xs text-on-surface-variant">
              日亏熔断 %
            </label>
            <input
              type="number"
              step="0.1"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={mpCfg.daily_loss_limit_pct ?? 8}
              disabled={dis}
              onChange={(e) =>
                setMp({ daily_loss_limit_pct: parseFloat(e.target.value) || 0 })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              当日净值回撤达限暂停开新仓（默认 8%）。
            </p>
          </div>
          <div>
            <label className="text-xs text-on-surface-variant">
              单层最小初始保证金 U（币安 20x≈0.46）
            </label>
            <input
              type="number"
              step="0.01"
              className="mt-1 w-full rounded bg-surface-container-highest p-2 text-sm"
              value={mpCfg.min_layer_margin_usdt ?? 0.46}
              disabled={dis}
              onChange={(e) =>
                setMp({
                  min_layer_margin_usdt: parseFloat(e.target.value) || 0.46,
                })
              }
            />
            <p className="mt-1 text-[11px] text-on-surface-variant">
              按保证金校验，不是名义 12U；20x 时 0.46U 保证金约等于 9.2U 名义。
            </p>
          </div>
          <label className="flex items-center gap-2 text-sm sm:col-span-2">
            <input
              type="checkbox"
              checked={!!mpCfg.budget_use_available_only}
              disabled={dis}
              onChange={(e) =>
                setMp({ budget_use_available_only: e.target.checked })
              }
            />
            保证金预算按可用余额（否则按净值）
          </label>
        </div>
        <div className="mt-4">
          <p className="mb-2 text-xs font-semibold text-on-surface-variant">
            各层权重 %（共 {mpCfg.max_layers} 层，可改，保存后程序会归一化）
          </p>
          <div className="grid grid-cols-4 gap-2 sm:grid-cols-7">
            {Array.from({ length: mpCfg.max_layers }, (_, i) => {
              const weights =
                mpCfg.layer_weights ??
                defaultMartingaleProgramConfig.layer_weights ??
                []
              const w =
                weights[i] ??
                defaultMartingaleProgramConfig.layer_weights?.[i] ??
                0.1
              return (
                <div key={i}>
                  <label className="text-[10px] text-on-surface-variant">
                    L{i + 1}
                  </label>
                  <input
                    type="number"
                    step="0.01"
                    className="mt-0.5 w-full rounded bg-surface-container-highest p-1.5 text-xs"
                    value={(w * 100).toFixed(2)}
                    disabled={dis}
                    onChange={(e) => {
                      const next = [...weights]
                      while (next.length < mpCfg.max_layers) {
                        next.push(
                          defaultMartingaleProgramConfig.layer_weights?.[
                            next.length
                          ] ?? 0.1
                        )
                      }
                      next[i] = (parseFloat(e.target.value) || 0) / 100
                      setMp({ layer_weights: next })
                    }}
                  />
                </div>
              )
            })}
          </div>
        </div>
        <p className="mt-2 text-xs text-on-surface-variant">
          第 1 层最轻、末层最重；预算另受策略风控「最大保证金使用率」封顶。
        </p>
        <p className="mt-2 text-xs text-amber-200/90">
          交易员须绑定 <strong>COMKUN-AI</strong>{' '}
          模型；周期内由程序执行，不调用大模型。
        </p>
      </section>
      <Suspense fallback={null}>
        <div className={dis ? 'opacity-50' : ''}>
          <StrategyPromptMonacoEditor
            value={editingConfig.strategy_prompt ?? ''}
            onChange={(v) => updateConfig('strategy_prompt', v)}
            disabled={dis}
          />
        </div>
      </Suspense>
    </div>
  )

  if (editingConfig.comkun_follow_listing_template) {
    return (
      <ComkunListingMasterStrategyBuilder
        editingConfig={editingConfig}
        selectedStrategy={selectedStrategy}
        updateConfig={updateConfig}
        editorsDisabled={editorsDisabled}
      />
    )
  }

  if (
    isProgramMartingaleStrategyStudioStrategy(
      selectedStrategy.id,
      editingConfig.strategy_type
    )
  ) {
    return (
      <div className="strategy-quant-lab flex flex-1 flex-col gap-3 font-['Inter',sans-serif] lg:min-h-0">
        <div className="quant-lab-scrollbar flex-1 overflow-visible overflow-x-hidden pb-6 pr-0 lg:min-h-0 lg:overflow-y-auto lg:pr-1">
          {martingaleStudioPanel}
        </div>
      </div>
    )
  }

  return (
    <div className="strategy-quant-lab flex flex-1 flex-col gap-3 font-['Inter',sans-serif] lg:min-h-0">
      {/* 模式切换 — 与后端 strategy_type 同步（程序马丁仅 XAU 专用策略可见） */}
      <div className="flex shrink-0 gap-1 rounded-xl border border-outline-variant/25 bg-surface-container-low p-1">
        <button
          type="button"
          disabled={strategyTypeSwitchDisabled}
          onClick={() => onStrategyTypeChange?.('ai_trading')}
          className={tabBtn(currentStrategyType === 'ai_trading')}
        >
          <Activity className="h-4 w-4" />
          AI 策略
        </button>
        <button
          type="button"
          disabled={strategyTypeSwitchDisabled}
          onClick={() => onStrategyTypeChange?.('grid_trading')}
          className={tabBtn(currentStrategyType === 'grid_trading')}
        >
          <LayoutGrid className="h-4 w-4" />
          网格
        </button>
      </div>

      <div className="quant-lab-scrollbar flex-1 overflow-visible overflow-x-hidden pb-6 pr-0 lg:min-h-0 lg:overflow-y-auto lg:pr-1">
        {/* ——— AI 视图（ai_1 栅格） ——— */}
        {currentStrategyType === 'ai_trading' && (
          <div
            className={`mx-auto max-w-7xl space-y-6 ${aiDis ? 'pointer-events-none opacity-50' : ''}`}
          >
            <div className="grid grid-cols-12 gap-4 sm:gap-6">
              <div className="relative col-span-12 overflow-hidden rounded-xl bg-surface-container-high p-4 sm:p-6 lg:col-span-8">
                <div className="pointer-events-none absolute right-4 top-4 opacity-10">
                  <span className="material-symbols-outlined text-6xl text-primary">
                    psychology
                  </span>
                </div>
                <div className="relative z-[1] mb-5 rounded-xl border border-outline-variant/25 bg-surface-container-lowest/90 p-4">
                  <div className="mb-2 flex flex-wrap items-end justify-between gap-2">
                    <span className="text-xs font-semibold tracking-wide text-on-surface-variant">
                      Token 预估预算
                    </span>
                    <span className="text-right font-mono text-sm tabular-nums text-on-surface">
                      <span className="font-bold text-primary">
                        {estimatedTokens.toLocaleString()}
                      </span>
                      <span className="text-on-surface-variant">
                        {' '}
                        / {tokenBudget.toLocaleString()}{' '}
                      </span>
                      <span className="text-[10px] text-on-surface-variant">
                        tokens
                      </span>
                    </span>
                  </div>
                  <div className="h-2.5 w-full overflow-hidden rounded-full bg-surface-container-highest">
                    <div
                      className={`h-full max-w-full rounded-full transition-[width,background-color] ${Q_EASE} ${
                        tokenPct >= 90
                          ? 'bg-error'
                          : tokenPct >= 72
                            ? 'bg-amber-400'
                            : 'bg-primary'
                      }`}
                      style={{ width: `${tokenPct}%` }}
                    />
                  </div>
                  <p className="mt-2 text-[11px] leading-relaxed text-on-surface-variant">
                    约合{' '}
                    <strong className="text-on-surface">
                      {approxChars.toLocaleString()}
                    </strong>{' '}
                    {editingConfig.language === 'zh' ? '字' : 'chars'}（按 token
                    粗算，仅作排版参考）
                  </p>
                </div>
                <h2 className="mb-1 flex items-center gap-2 font-['Space_Grotesk',sans-serif] text-xl font-bold text-on-surface">
                  <span className="material-symbols-outlined text-primary">
                    terminal
                  </span>
                  策略核心
                </h2>
                <p className="mb-3 text-xs leading-relaxed text-on-surface-variant">
                  一整段说明你的交易思路；保存后交给模型。风控与输出格式由系统自动附加。支持
                  Markdown 标题与列表，观感与专业 IDE 一致。
                </p>
                <Suspense
                  fallback={
                    <div
                      className="flex min-h-[420px] w-full items-center justify-center rounded-lg border border-outline-variant/20 bg-surface-container-lowest text-xs text-on-surface-variant"
                      aria-hidden
                    >
                      正在加载专业编辑器…
                    </div>
                  }
                >
                  <StrategyPromptMonacoEditor
                    key={`strategy-prompt-${selectedStrategy.id}`}
                    value={strategyPromptDisplay(editingConfig)}
                    onChange={(v) => updateConfig('strategy_prompt', v)}
                    disabled={aiDis}
                    heightPx={420}
                  />
                </Suspense>
              </div>

              <div className="col-span-12 rounded-xl border-t-4 border-primary/25 bg-surface-container-high p-4 sm:p-6 lg:col-span-4">
                <h3 className="mb-4 flex items-center gap-2 font-['Space_Grotesk',sans-serif] text-lg font-bold text-on-surface">
                  <span className="material-symbols-outlined text-primary">
                    database
                  </span>
                  币种来源
                </h3>
                <div className="space-y-3">
                  <label className="flex cursor-pointer items-center justify-between rounded border border-outline-variant/10 bg-surface-container-low p-3 transition-colors hover:bg-surface-container-highest">
                    <span className="text-sm font-medium text-on-surface">
                      静态列表
                    </span>
                    <input
                      type="checkbox"
                      className="h-4 w-4 rounded border-none bg-surface-container-highest text-primary focus:ring-0"
                      checked={
                        (coin.static_coins?.length ?? 0) > 0 ||
                        coin.source_type === 'static'
                      }
                      disabled={aiDis}
                      onChange={(e) => {
                        if (e.target.checked) {
                          staticModalCtx.current = {
                            hadCoins: (coin.static_coins?.length ?? 0) > 0,
                          }
                          setStaticModalDraft(
                            (coin.static_coins || []).join(', ')
                          )
                          setStaticModalOpen(true)
                        } else {
                          setCoin({ static_coins: [] })
                        }
                      }}
                    />
                  </label>
                  {(coin.static_coins?.length ?? 0) > 0 && (
                    <div className="rounded border border-primary/20 bg-surface-container-lowest/80 p-2">
                      <p className="mb-1 text-[10px] uppercase tracking-wider text-on-surface-variant">
                        当前静态
                      </p>
                      <p className="break-all font-mono text-xs text-on-surface">
                        {(coin.static_coins || []).join(', ')}
                      </p>
                      <button
                        type="button"
                        disabled={aiDis}
                        onClick={() => {
                          staticModalCtx.current = { hadCoins: true }
                          setStaticModalDraft(
                            (coin.static_coins || []).join(', ')
                          )
                          setStaticModalOpen(true)
                        }}
                        className={`mt-2 text-xs font-medium text-primary underline-offset-2 hover:underline ${Q_EASE}`}
                      >
                        编辑交易对
                      </button>
                    </div>
                  )}
                  <div className="overflow-hidden rounded border border-outline-variant/10 bg-surface-container-low">
                    <label className="flex cursor-pointer items-center justify-between p-3 transition-colors hover:bg-surface-container-highest">
                      <span className="text-sm font-medium text-on-surface">
                        AI500 数据源
                      </span>
                      <input
                        type="checkbox"
                        className="h-4 w-4 rounded border-none bg-surface-container-highest text-primary focus:ring-0"
                        checked={coin.use_ai500}
                        disabled={aiDis}
                        onChange={(e) =>
                          setCoin({
                            use_ai500: e.target.checked,
                            ai500_limit: coin.ai500_limit ?? 3,
                          })
                        }
                      />
                    </label>
                    {coin.use_ai500 && (
                      <div className="flex items-center justify-between gap-2 border-t border-outline-variant/10 bg-surface-container-lowest/50 px-3 py-2">
                        <span className="text-xs text-on-surface-variant">
                          扫描数量
                        </span>
                        <input
                          type="number"
                          min={1}
                          max={10}
                          disabled={aiDis}
                          value={coin.ai500_limit ?? 3}
                          onChange={(e) =>
                            setCoin({
                              ai500_limit: Math.min(
                                10,
                                Math.max(1, parseInt(e.target.value, 10) || 1)
                              ),
                            })
                          }
                          className="w-16 rounded border-none bg-surface-container-highest py-1 text-center text-xs font-bold text-primary focus:outline-none"
                        />
                      </div>
                    )}
                  </div>
                  <div className="overflow-hidden rounded border border-outline-variant/10 bg-surface-container-low">
                    <label className="flex cursor-pointer items-center justify-between p-3 transition-colors hover:bg-surface-container-highest">
                      <span className="text-sm font-medium text-on-surface">
                        OI 持仓增加
                      </span>
                      <input
                        type="checkbox"
                        className="h-4 w-4 rounded border-none bg-surface-container-highest text-primary focus:ring-0"
                        checked={coin.use_oi_top}
                        disabled={aiDis}
                        onChange={(e) =>
                          setCoin({
                            use_oi_top: e.target.checked,
                            oi_top_limit: coin.oi_top_limit ?? 3,
                          })
                        }
                      />
                    </label>
                    {coin.use_oi_top && (
                      <div className="flex items-center justify-between gap-2 border-t border-outline-variant/10 bg-surface-container-lowest/50 px-3 py-2">
                        <span className="text-xs text-on-surface-variant">
                          扫描数量
                        </span>
                        <input
                          type="number"
                          min={1}
                          max={10}
                          disabled={aiDis}
                          value={coin.oi_top_limit ?? 3}
                          onChange={(e) =>
                            setCoin({
                              oi_top_limit: Math.min(
                                10,
                                Math.max(1, parseInt(e.target.value, 10) || 1)
                              ),
                            })
                          }
                          className="w-16 rounded border-none bg-surface-container-highest py-1 text-center text-xs font-bold text-primary focus:outline-none"
                        />
                      </div>
                    )}
                  </div>
                  <div className="overflow-hidden rounded border border-outline-variant/10 bg-surface-container-low">
                    <label className="flex cursor-pointer items-center justify-between p-3 transition-colors hover:bg-surface-container-highest">
                      <span className="text-sm font-medium text-on-surface">
                        OI 持仓减少
                      </span>
                      <input
                        type="checkbox"
                        className="h-4 w-4 rounded border-none bg-surface-container-highest text-primary focus:ring-0"
                        checked={coin.use_oi_low}
                        disabled={aiDis}
                        onChange={(e) =>
                          setCoin({
                            use_oi_low: e.target.checked,
                            oi_low_limit: coin.oi_low_limit ?? 3,
                          })
                        }
                      />
                    </label>
                    {coin.use_oi_low && (
                      <div className="flex items-center justify-between gap-2 border-t border-outline-variant/10 bg-surface-container-lowest/50 px-3 py-2">
                        <span className="text-xs text-on-surface-variant">
                          扫描数量
                        </span>
                        <input
                          type="number"
                          min={1}
                          max={10}
                          disabled={aiDis}
                          value={coin.oi_low_limit ?? 3}
                          onChange={(e) =>
                            setCoin({
                              oi_low_limit: Math.min(
                                10,
                                Math.max(1, parseInt(e.target.value, 10) || 1)
                              ),
                            })
                          }
                          className="w-16 rounded border-none bg-surface-container-highest py-1 text-center text-xs font-bold text-primary focus:outline-none"
                        />
                      </div>
                    )}
                  </div>
                  <div className="pt-2">
                    <p className="mb-2 px-1 text-xs text-on-surface-variant">
                      排除币种（逗号分隔）
                    </p>
                    <input
                      value={(coin.excluded_coins || []).join(',')}
                      onChange={(e) =>
                        setCoin({
                          excluded_coins: e.target.value
                            .split(/[,，\s]+/)
                            .map((s) => s.trim().toUpperCase())
                            .filter(Boolean),
                        })
                      }
                      disabled={aiDis}
                      placeholder="例如: DOGEUSDT"
                      className="w-full rounded border-none bg-surface-container-lowest p-2 text-xs text-on-surface focus:outline-none"
                    />
                  </div>
                </div>
              </div>

              <div className="col-span-12 grid grid-cols-1 gap-6 md:grid-cols-3">
                <div className="rounded-xl bg-surface-container-high p-4 sm:p-6">
                  <h3 className="mb-4 flex items-center gap-2 text-sm font-bold uppercase tracking-wide text-on-surface">
                    <span className="material-symbols-outlined text-sm text-primary">
                      waves
                    </span>
                    资金流指标
                  </h3>
                  <div className="space-y-2">
                    {FLOW_INDICATOR_EXPLAINS.map((item) => {
                      const active = !!ind[item.key]
                      return (
                        <div
                          key={item.key}
                          className="rounded-lg border border-outline-variant/15 bg-surface-container-lowest/35 p-2.5"
                        >
                          <button
                            type="button"
                            disabled={aiDis}
                            className={chip(active)}
                            onClick={() =>
                              setInd({
                                [item.key]: !active,
                              } as Partial<IndicatorConfig>)
                            }
                          >
                            {item.label}
                          </button>
                          <p className="mt-1.5 text-[11px] leading-relaxed text-on-surface-variant">
                            {item.desc}
                          </p>
                        </div>
                      )
                    })}
                  </div>
                </div>
                <div className="rounded-xl bg-surface-container-high p-4 sm:p-6">
                  <h3 className="mb-2 flex items-center gap-2 text-sm font-bold uppercase tracking-wide text-on-surface">
                    <span className="material-symbols-outlined text-sm text-primary">
                      legend_toggle
                    </span>
                    技术指标
                  </h3>
                  <p className="mb-3 text-[11px] leading-relaxed text-on-surface-variant">
                    打开指标后可填
                    <strong className="text-on-surface">计算周期</strong>
                    ；多个数字用英文逗号分隔（会写入策略配置）。MACD
                    在后端按经典{' '}
                    <span className="font-mono text-on-surface">
                      12 / 26
                    </span>{' '}
                    固定计算，无需填周期。
                  </p>
                  <div className="space-y-2">
                    <div className="flex flex-col gap-2 rounded-lg border border-outline-variant/15 bg-surface-container-lowest/40 p-2.5 sm:flex-row sm:items-center sm:justify-between">
                      <div className="flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          disabled={aiDis}
                          className={chip(!!ind.enable_ema)}
                          onClick={() =>
                            setInd(
                              ind.enable_ema
                                ? { enable_ema: false }
                                : {
                                    enable_ema: true,
                                    ema_periods:
                                      ind.ema_periods &&
                                      ind.ema_periods.length > 0
                                        ? ind.ema_periods
                                        : [...DEFAULT_EMA_PERIODS],
                                  }
                            )
                          }
                        >
                          EMA
                        </button>
                        <span className="text-[10px] text-on-surface-variant">
                          指数均线
                        </span>
                      </div>
                      {ind.enable_ema && (
                        <div className="flex min-w-0 flex-1 items-center gap-2 sm:max-w-[240px]">
                          <span className="shrink-0 text-[10px] font-medium text-on-surface-variant">
                            周期
                          </span>
                          <input
                            type="text"
                            disabled={aiDis}
                            aria-label="EMA 周期"
                            value={(ind.ema_periods?.length
                              ? ind.ema_periods
                              : [...DEFAULT_EMA_PERIODS]
                            ).join(',')}
                            onChange={(e) =>
                              setInd({
                                ema_periods: parseIndicatorPeriods(
                                  e.target.value,
                                  DEFAULT_EMA_PERIODS
                                ),
                              })
                            }
                            placeholder="20,50"
                            className={periodInputClass}
                          />
                        </div>
                      )}
                    </div>
                    <div className="flex flex-col gap-2 rounded-lg border border-outline-variant/15 bg-surface-container-lowest/40 p-2.5 sm:flex-row sm:items-center sm:justify-between">
                      <div className="flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          disabled={aiDis}
                          className={chip(!!ind.enable_macd)}
                          onClick={() =>
                            setInd({ enable_macd: !ind.enable_macd })
                          }
                        >
                          MACD
                        </button>
                        <span className="text-[10px] text-on-surface-variant">
                          动量
                        </span>
                      </div>
                      {ind.enable_macd && (
                        <p className="text-[10px] text-on-surface-variant sm:text-right">
                          固定参数：快线 12、慢线 26（与常见交易所一致）
                        </p>
                      )}
                    </div>
                    <div className="flex flex-col gap-2 rounded-lg border border-outline-variant/15 bg-surface-container-lowest/40 p-2.5 sm:flex-row sm:items-center sm:justify-between">
                      <div className="flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          disabled={aiDis}
                          className={chip(!!ind.enable_rsi)}
                          onClick={() =>
                            setInd(
                              ind.enable_rsi
                                ? { enable_rsi: false }
                                : {
                                    enable_rsi: true,
                                    rsi_periods:
                                      ind.rsi_periods &&
                                      ind.rsi_periods.length > 0
                                        ? ind.rsi_periods
                                        : [...DEFAULT_RSI_PERIODS],
                                  }
                            )
                          }
                        >
                          RSI
                        </button>
                        <span className="text-[10px] text-on-surface-variant">
                          相对强弱
                        </span>
                      </div>
                      {ind.enable_rsi && (
                        <div className="flex min-w-0 flex-1 items-center gap-2 sm:max-w-[240px]">
                          <span className="shrink-0 text-[10px] font-medium text-on-surface-variant">
                            周期
                          </span>
                          <input
                            type="text"
                            disabled={aiDis}
                            aria-label="RSI 周期"
                            value={(ind.rsi_periods?.length
                              ? ind.rsi_periods
                              : [...DEFAULT_RSI_PERIODS]
                            ).join(',')}
                            onChange={(e) =>
                              setInd({
                                rsi_periods: parseIndicatorPeriods(
                                  e.target.value,
                                  DEFAULT_RSI_PERIODS
                                ),
                              })
                            }
                            placeholder="7,14"
                            className={periodInputClass}
                          />
                        </div>
                      )}
                    </div>
                    <div className="flex flex-col gap-2 rounded-lg border border-outline-variant/15 bg-surface-container-lowest/40 p-2.5 sm:flex-row sm:items-center sm:justify-between">
                      <div className="flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          disabled={aiDis}
                          className={chip(!!ind.enable_atr)}
                          onClick={() =>
                            setInd(
                              ind.enable_atr
                                ? { enable_atr: false }
                                : {
                                    enable_atr: true,
                                    atr_periods:
                                      ind.atr_periods &&
                                      ind.atr_periods.length > 0
                                        ? ind.atr_periods
                                        : [...DEFAULT_ATR_PERIODS],
                                  }
                            )
                          }
                        >
                          ATR
                        </button>
                        <span className="text-[10px] text-on-surface-variant">
                          波动
                        </span>
                      </div>
                      {ind.enable_atr && (
                        <div className="flex min-w-0 flex-1 items-center gap-2 sm:max-w-[240px]">
                          <span className="shrink-0 text-[10px] font-medium text-on-surface-variant">
                            周期
                          </span>
                          <input
                            type="text"
                            disabled={aiDis}
                            aria-label="ATR 周期"
                            value={(ind.atr_periods?.length
                              ? ind.atr_periods
                              : [...DEFAULT_ATR_PERIODS]
                            ).join(',')}
                            onChange={(e) =>
                              setInd({
                                atr_periods: parseIndicatorPeriods(
                                  e.target.value,
                                  DEFAULT_ATR_PERIODS
                                ),
                              })
                            }
                            placeholder="14"
                            className={periodInputClass}
                          />
                        </div>
                      )}
                    </div>
                    <div className="flex flex-col gap-2 rounded-lg border border-outline-variant/15 bg-surface-container-lowest/40 p-2.5 sm:flex-row sm:items-center sm:justify-between">
                      <div className="flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          disabled={aiDis}
                          className={chip(!!ind.enable_boll)}
                          onClick={() =>
                            setInd(
                              ind.enable_boll
                                ? { enable_boll: false }
                                : {
                                    enable_boll: true,
                                    boll_periods:
                                      ind.boll_periods &&
                                      ind.boll_periods.length > 0
                                        ? ind.boll_periods
                                        : [...DEFAULT_BOLL_PERIODS],
                                  }
                            )
                          }
                        >
                          BOLL
                        </button>
                        <span className="text-[10px] text-on-surface-variant">
                          布林带中轨周期
                        </span>
                      </div>
                      {ind.enable_boll && (
                        <div className="flex min-w-0 flex-1 items-center gap-2 sm:max-w-[240px]">
                          <span className="shrink-0 text-[10px] font-medium text-on-surface-variant">
                            周期
                          </span>
                          <input
                            type="text"
                            disabled={aiDis}
                            aria-label="BOLL 周期"
                            value={(ind.boll_periods?.length
                              ? ind.boll_periods
                              : [...DEFAULT_BOLL_PERIODS]
                            ).join(',')}
                            onChange={(e) =>
                              setInd({
                                boll_periods: parseIndicatorPeriods(
                                  e.target.value,
                                  DEFAULT_BOLL_PERIODS
                                ),
                              })
                            }
                            placeholder="20"
                            className={periodInputClass}
                          />
                        </div>
                      )}
                    </div>
                  </div>
                </div>
                <div className="rounded-xl bg-surface-container-high p-4 sm:p-6">
                  <h3 className="mb-4 flex items-center gap-2 text-sm font-bold uppercase tracking-wide text-on-surface">
                    <span className="material-symbols-outlined text-sm text-primary">
                      mood
                    </span>
                    市场情绪
                  </h3>
                  <div className="space-y-2">
                    {SENTIMENT_INDICATOR_EXPLAINS.map((item) => {
                      const active = !!ind[item.key]
                      return (
                        <div
                          key={item.key}
                          className="rounded-lg border border-outline-variant/15 bg-surface-container-lowest/35 p-2.5"
                        >
                          <button
                            type="button"
                            disabled={aiDis}
                            className={chip(active)}
                            onClick={() =>
                              setInd({
                                [item.key]: !active,
                              } as Partial<IndicatorConfig>)
                            }
                          >
                            {item.label}
                          </button>
                          <p className="mt-1.5 text-[11px] leading-relaxed text-on-surface-variant">
                            {item.desc}
                          </p>
                        </div>
                      )
                    })}
                  </div>
                </div>
              </div>

              <div className="col-span-12 rounded-xl bg-surface-container-high p-4 sm:p-6">
                <h3 className="mb-2 flex items-center gap-2 font-['Space_Grotesk',sans-serif] text-lg font-bold text-on-surface">
                  <span className="material-symbols-outlined text-primary">
                    gavel
                  </span>
                  风控参数
                </h3>
                <p className="mb-4 text-[11px] leading-relaxed text-on-surface-variant">
                  单笔名义金额、下单手数等仓位大小请在上方
                  <strong className="text-on-surface">策略核心</strong>
                  提示词里自行约定；此处仅保留杠杆与风险阈值类硬约束。
                </p>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-5">
                  <div className="space-y-2">
                    <label className="text-xs font-medium text-on-surface-variant">
                      最大持仓
                    </label>
                    <input
                      type="number"
                      min={1}
                      max={20}
                      value={rc.max_positions}
                      disabled={aiDis}
                      onChange={(e) =>
                        setRc({
                          max_positions: parseInt(e.target.value, 10) || 1,
                        })
                      }
                      className="w-full rounded border-none bg-surface-container-lowest p-2 text-sm font-bold text-primary focus:outline-none"
                    />
                  </div>
                  {hzMode ? (
                    <div className="space-y-2">
                      <label className="text-xs font-medium text-on-surface-variant">
                        HZ 杠杆
                      </label>
                      <input
                        type="number"
                        min={100}
                        max={2000}
                        step={100}
                        value={rc.altcoin_max_leverage ?? 500}
                        disabled={aiDis}
                        onChange={(e) => {
                          const leverage = Math.min(
                            2000,
                            Math.max(100, parseInt(e.target.value, 10) || 100)
                          )
                          setRc({
                            btc_eth_max_leverage: leverage,
                            altcoin_max_leverage: leverage,
                          })
                        }}
                        className="w-full rounded border-none bg-surface-container-lowest p-2 text-sm font-bold text-primary focus:outline-none"
                      />
                    </div>
                  ) : (
                    <>
                      <div className="space-y-2">
                        <label className="text-xs font-medium text-on-surface-variant">
                          BTC/ETH 杠杆
                        </label>
                        <input
                          type="number"
                          min={1}
                          max={125}
                          value={rc.btc_eth_max_leverage}
                          disabled={aiDis}
                          onChange={(e) =>
                            setRc({
                              btc_eth_max_leverage:
                                parseInt(e.target.value, 10) || 1,
                            })
                          }
                          className="w-full rounded border-none bg-surface-container-lowest p-2 text-sm font-bold text-primary focus:outline-none"
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-xs font-medium text-on-surface-variant">
                          山寨币杠杆
                        </label>
                        <input
                          type="number"
                          min={1}
                          max={125}
                          value={rc.altcoin_max_leverage ?? 5}
                          disabled={aiDis}
                          onChange={(e) =>
                            setRc({
                              altcoin_max_leverage:
                                parseInt(e.target.value, 10) || 1,
                            })
                          }
                          className="w-full rounded border-none bg-surface-container-lowest p-2 text-sm font-bold text-primary focus:outline-none"
                        />
                      </div>
                    </>
                  )}
                  <div className="space-y-2">
                    <label className="text-xs font-medium text-on-surface-variant">
                      最小盈亏比
                    </label>
                    <input
                      type="number"
                      step={0.5}
                      min={1}
                      value={rc.min_risk_reward_ratio}
                      disabled={aiDis}
                      onChange={(e) =>
                        setRc({
                          min_risk_reward_ratio:
                            parseFloat(e.target.value) || 1,
                        })
                      }
                      className="w-full rounded border-none bg-surface-container-lowest p-2 text-sm font-bold text-primary focus:outline-none"
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-medium text-on-surface-variant">
                      AI 信心阈值 %
                    </label>
                    <input
                      type="range"
                      min={50}
                      max={99}
                      value={rc.min_confidence}
                      disabled={aiDis}
                      onChange={(e) =>
                        setRc({ min_confidence: parseInt(e.target.value, 10) })
                      }
                      className="w-full accent-primary"
                    />
                    <span className="text-xs font-bold text-primary">
                      {rc.min_confidence}%
                    </span>
                  </div>
                </div>
              </div>

              <div className="col-span-12 rounded-xl bg-surface-container-high p-4 sm:p-6">
                <h3 className="mb-4 flex items-center gap-2 font-['Space_Grotesk',sans-serif] text-md font-bold text-on-surface">
                  <span className="material-symbols-outlined text-primary">
                    bar_chart
                  </span>
                  市场数据 / K 线
                </h3>
                <div className="mb-6 flex flex-wrap items-center gap-4">
                  <div className="flex min-w-full flex-1 items-center justify-between rounded border border-primary/20 bg-surface-container-lowest p-3 sm:min-w-0">
                    <span className="text-sm font-medium text-on-surface">
                      OHLCV 原始 K 线
                      <span className="ml-1 text-xs font-normal text-on-surface-variant">
                        （必选）
                      </span>
                    </span>
                    <input
                      type="checkbox"
                      checked
                      disabled
                      aria-label="OHLCV 原始 K 线，必选且不可关闭"
                      className="h-4 w-4 cursor-not-allowed accent-primary opacity-90"
                    />
                  </div>
                  <div className="flex min-w-[140px] flex-1 items-center gap-2 rounded bg-surface-container-lowest p-2 px-3">
                    <span className="whitespace-nowrap text-sm text-on-surface">
                      K 线数量
                    </span>
                    <input
                      type="number"
                      className="w-full border-none bg-transparent p-0 text-sm font-bold text-primary focus:ring-0"
                      value={ind.klines.primary_count}
                      disabled={aiDis}
                      onChange={(e) =>
                        setInd({
                          klines: {
                            ...ind.klines,
                            primary_count: parseInt(e.target.value, 10) || 50,
                          },
                        })
                      }
                    />
                  </div>
                </div>
                <p className="mb-3 text-xs font-medium uppercase tracking-widest text-on-surface-variant">
                  时间周期
                </p>
                <div className="flex flex-wrap gap-2">
                  {TIMEFRAMES.map((tf) => {
                    const sel = (
                      ind.klines.selected_timeframes || [
                        ind.klines.primary_timeframe,
                      ]
                    ).includes(tf)
                    return (
                      <button
                        key={tf}
                        type="button"
                        disabled={aiDis}
                        onClick={() => {
                          const cur = ind.klines.selected_timeframes || [
                            ind.klines.primary_timeframe,
                          ]
                          let next = cur.includes(tf)
                            ? cur.filter((x) => x !== tf)
                            : [...cur, tf]
                          if (next.length === 0) next = [tf]
                          setInd({
                            klines: {
                              ...ind.klines,
                              selected_timeframes: next,
                              primary_timeframe: next.includes(
                                ind.klines.primary_timeframe
                              )
                                ? ind.klines.primary_timeframe
                                : next[0],
                              enable_multi_timeframe: next.length > 1,
                            },
                          })
                        }}
                        className={
                          sel
                            ? `h-10 w-10 rounded border border-primary bg-primary/5 text-xs font-bold text-primary transition-[color,background-color,border-color,transform] ${Q_EASE}`
                            : `h-10 w-10 rounded border border-outline-variant/10 text-xs text-on-surface-variant transition-[color,background-color,border-color,transform] ${Q_EASE} hover:border-primary/50`
                        }
                      >
                        {tf.toUpperCase()}
                      </button>
                    )
                  })}
                </div>
              </div>

              {staticModalOpen && (
                <div
                  className="fixed inset-0 z-[100] flex items-start justify-center overflow-y-auto bg-black/55 p-3 backdrop-blur-[2px] sm:items-center sm:p-4"
                  role="dialog"
                  aria-modal="true"
                  aria-labelledby="static-coins-title"
                >
                  <div className="relative w-full max-w-md rounded-2xl border border-outline-variant/30 bg-surface-container-high p-5 shadow-2xl">
                    <button
                      type="button"
                      className="absolute right-3 top-3 rounded p-1 text-on-surface-variant hover:bg-surface-container-lowest hover:text-on-surface"
                      onClick={() => {
                        setStaticModalOpen(false)
                        if (!staticModalCtx.current.hadCoins) {
                          setCoin({ static_coins: [] })
                        }
                      }}
                      aria-label="关闭"
                    >
                      <X className="h-4 w-4" />
                    </button>
                    <h3
                      id="static-coins-title"
                      className="pr-8 text-lg font-bold text-on-surface"
                    >
                      静态交易对
                    </h3>
                    <p className="mt-1 text-xs leading-relaxed text-on-surface-variant">
                      多个交易对请用英文逗号、空格或换行分隔，例如{' '}
                      <span className="font-mono text-on-surface">
                        BTCUSDT, ETHUSDT
                      </span>
                    </p>
                    <div className="mt-3">
                      <Suspense
                        fallback={
                          <div
                            className="flex h-[188px] w-full items-center justify-center rounded-lg border border-outline-variant/25 bg-surface-container-lowest text-xs text-on-surface-variant"
                            aria-hidden
                          >
                            加载编辑器…
                          </div>
                        }
                      >
                        <StrategyPromptMonacoEditor
                          value={staticModalDraft}
                          onChange={setStaticModalDraft}
                          heightPx={188}
                          language="plaintext"
                        />
                      </Suspense>
                    </div>
                    <div className="mt-4 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                      <button
                        type="button"
                        className="rounded-lg border border-outline-variant/30 px-4 py-2 text-sm text-on-surface-variant hover:bg-surface-container-lowest"
                        onClick={() => {
                          setStaticModalOpen(false)
                          if (!staticModalCtx.current.hadCoins) {
                            setCoin({ static_coins: [] })
                          }
                        }}
                      >
                        取消
                      </button>
                      <button
                        type="button"
                        className="rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-black hover:bg-[#d4ff33]"
                        onClick={() => {
                          const parsed = staticModalDraft
                            .split(/[,，\s\n]+/)
                            .map((s) => s.trim().toUpperCase())
                            .filter(Boolean)
                          if (parsed.length === 0) {
                            notify.error('请至少填写一个交易对')
                            return
                          }
                          setCoin({ static_coins: parsed })
                          setStaticModalOpen(false)
                          notify.success('已保存静态列表')
                        }}
                      >
                        保存
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* ——— 网格视图（ai_2 左栏 + 状态，不含右侧风控栏） ——— */}
        {currentStrategyType === 'grid_trading' && (
          <div
            className={`mx-auto max-w-7xl space-y-6 ${gridDis ? 'pointer-events-none opacity-50' : ''}`}
          >
            <section className="relative overflow-hidden rounded-xl bg-surface-container-high p-4 sm:p-6">
              <div className="absolute left-0 top-0 h-1 w-full bg-primary-container/40" />
              <div className="mb-6 flex items-center gap-2">
                <span className="material-symbols-outlined text-primary-container">
                  tune
                </span>
                <h2 className="font-['Space_Grotesk',sans-serif] text-lg font-semibold text-on-surface">
                  交易设置
                </h2>
              </div>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
                <div className="space-y-2">
                  <label className="text-xs uppercase tracking-widest text-on-surface-variant">
                    交易币种
                  </label>
                  <select
                    value={gridCfg.symbol}
                    onChange={(e) =>
                      updateConfig('grid_config', {
                        ...gridCfg,
                        symbol: e.target.value,
                      })
                    }
                    disabled={gridDis}
                    className="w-full cursor-pointer rounded border-none bg-surface-container-highest p-3 font-bold text-on-surface focus:outline-none"
                  >
                    {GRID_SYMBOLS.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="space-y-2">
                  <label className="text-xs uppercase tracking-widest text-on-surface-variant">
                    启用资金 (USDT)
                  </label>
                  <input
                    type="number"
                    min={100}
                    step={100}
                    value={gridCfg.total_investment}
                    disabled={gridDis}
                    onChange={(e) =>
                      updateConfig('grid_config', {
                        ...gridCfg,
                        total_investment: parseFloat(e.target.value) || 0,
                      })
                    }
                    className="w-full rounded border-none bg-surface-container-highest p-3 font-['Space_Grotesk',sans-serif] text-on-surface focus:outline-none"
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-xs uppercase tracking-widest text-on-surface-variant">
                    杠杆倍数
                  </label>
                  <div className="flex items-center gap-4">
                    <input
                      type="range"
                      min={1}
                      max={20}
                      value={gridCfg.leverage}
                      disabled={gridDis}
                      onChange={(e) =>
                        updateConfig('grid_config', {
                          ...gridCfg,
                          leverage: parseInt(e.target.value, 10),
                        })
                      }
                      className="w-full accent-primary-container"
                    />
                    <span className="w-10 font-bold text-primary-container">
                      {gridCfg.leverage}x
                    </span>
                  </div>
                </div>
              </div>
            </section>

            <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
              <section className="rounded-xl bg-surface-container-high p-4 sm:p-6">
                <div className="mb-6 flex items-center gap-2">
                  <span className="material-symbols-outlined text-primary-container">
                    grid_view
                  </span>
                  <h2 className="font-['Space_Grotesk',sans-serif] text-lg font-semibold text-on-surface">
                    网格参数
                  </h2>
                </div>
                <div className="space-y-6">
                  <div className="space-y-2">
                    <label className="text-xs uppercase tracking-widest text-on-surface-variant">
                      网格数量
                    </label>
                    <input
                      type="number"
                      min={5}
                      max={50}
                      value={gridCfg.grid_count}
                      disabled={gridDis}
                      onChange={(e) =>
                        updateConfig('grid_config', {
                          ...gridCfg,
                          grid_count: parseInt(e.target.value, 10) || 5,
                        })
                      }
                      className="w-full rounded border-none bg-surface-container-highest p-3 focus:outline-none"
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs uppercase tracking-widest text-on-surface-variant">
                      资金分配
                    </label>
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                      {(['uniform', 'gaussian', 'pyramid'] as const).map(
                        (d) => (
                          <button
                            key={d}
                            type="button"
                            disabled={gridDis}
                            onClick={() =>
                              updateConfig('grid_config', {
                                ...gridCfg,
                                distribution: d,
                              })
                            }
                            className={
                              gridCfg.distribution === d
                                ? `rounded bg-primary-container py-2 text-xs font-bold text-on-primary-container transition-[color,background-color,transform] ${Q_EASE}`
                                : `rounded bg-surface-container-highest py-2 text-xs font-bold text-on-surface-variant transition-[color,background-color,transform] ${Q_EASE} hover:text-on-surface`
                            }
                          >
                            {d === 'uniform'
                              ? '均匀'
                              : d === 'gaussian'
                                ? '高斯'
                                : '金字塔'}
                          </button>
                        )
                      )}
                    </div>
                  </div>
                </div>
              </section>
              <section className="rounded-xl bg-surface-container-high p-4 sm:p-6">
                <div className="mb-6 flex items-center gap-2">
                  <span className="material-symbols-outlined text-primary-container">
                    auto_graph
                  </span>
                  <h2 className="font-['Space_Grotesk',sans-serif] text-lg font-semibold text-on-surface">
                    价格边界
                  </h2>
                </div>
                <div className="space-y-6">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-on-surface">
                      自动计算 ATR
                    </span>
                    <button
                      type="button"
                      disabled={gridDis}
                      role="switch"
                      aria-checked={gridCfg.use_atr_bounds}
                      onClick={() =>
                        updateConfig('grid_config', {
                          ...gridCfg,
                          use_atr_bounds: !gridCfg.use_atr_bounds,
                        })
                      }
                      className={`relative h-6 w-11 shrink-0 rounded-full transition-[background-color] ${Q_EASE} ${
                        gridCfg.use_atr_bounds
                          ? 'bg-primary-container/25'
                          : 'bg-surface-container-highest'
                      }`}
                    >
                      <span
                        className={`absolute left-0.5 top-0.5 block h-5 w-5 rounded-full bg-primary-container shadow transition-transform ${Q_EASE} will-change-transform ${
                          gridCfg.use_atr_bounds
                            ? 'translate-x-5'
                            : 'translate-x-0'
                        }`}
                      />
                    </button>
                  </div>
                  {gridCfg.use_atr_bounds ? (
                    <div className="space-y-2">
                      <label className="text-xs uppercase tracking-widest text-on-surface-variant">
                        ATR 倍数
                      </label>
                      <input
                        type="number"
                        step={0.5}
                        min={1}
                        max={5}
                        value={gridCfg.atr_multiplier}
                        disabled={gridDis}
                        onChange={(e) =>
                          updateConfig('grid_config', {
                            ...gridCfg,
                            atr_multiplier: parseFloat(e.target.value) || 2,
                          })
                        }
                        className="w-full rounded border-none bg-surface-container-highest p-3 pr-12 focus:outline-none"
                      />
                    </div>
                  ) : (
                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                      <input
                        type="number"
                        placeholder="上界"
                        value={gridCfg.upper_price || ''}
                        disabled={gridDis}
                        onChange={(e) =>
                          updateConfig('grid_config', {
                            ...gridCfg,
                            upper_price: parseFloat(e.target.value) || 0,
                          })
                        }
                        className="rounded border-none bg-surface-container-highest p-2 text-sm text-on-surface"
                      />
                      <input
                        type="number"
                        placeholder="下界"
                        value={gridCfg.lower_price || ''}
                        disabled={gridDis}
                        onChange={(e) =>
                          updateConfig('grid_config', {
                            ...gridCfg,
                            lower_price: parseFloat(e.target.value) || 0,
                          })
                        }
                        className="rounded border-none bg-surface-container-highest p-2 text-sm text-on-surface"
                      />
                    </div>
                  )}
                </div>
              </section>
            </div>

            <section className="relative overflow-hidden rounded-xl bg-surface-container-high p-4 sm:p-6">
              <div
                className="pointer-events-none absolute inset-0 opacity-[0.06]"
                style={{
                  backgroundImage:
                    'radial-gradient(circle at 2px 2px, #46484d 1px, transparent 0)',
                  backgroundSize: '24px 24px',
                }}
              />
              <div className="relative mb-6 flex flex-col items-start gap-2 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-2">
                  <span className="material-symbols-outlined text-primary-container">
                    psychology
                  </span>
                  <h2 className="font-['Space_Grotesk',sans-serif] text-lg font-semibold text-on-surface">
                    方向自动调整
                  </h2>
                </div>
                <span className="rounded bg-primary-container/10 px-2 py-1 font-mono text-xs text-primary-container">
                  Grid Engine
                </span>
              </div>
              <div className="relative grid grid-cols-1 gap-8 md:grid-cols-2">
                <div className="space-y-4">
                  <div className="flex justify-between font-mono text-xs">
                    <span className="text-error">空头偏向</span>
                    <span className="text-primary-container">多头偏向</span>
                  </div>
                  <div className="relative h-2 rounded-full bg-surface-container-highest">
                    <div
                      className="absolute top-0 h-full w-2 rounded-full bg-primary-container shadow-[0_0_10px_#d4ff33]"
                      style={{
                        left: `${Math.min(95, Math.max(5, (gridCfg.direction_bias_ratio ?? 0.7) * 100))}%`,
                        transform: 'translateX(-50%)',
                      }}
                    />
                  </div>
                  <input
                    type="range"
                    min={55}
                    max={90}
                    step={5}
                    disabled={gridDis || !gridCfg.enable_direction_adjust}
                    value={Math.round(
                      (gridCfg.direction_bias_ratio ?? 0.7) * 100
                    )}
                    onChange={(e) =>
                      updateConfig('grid_config', {
                        ...gridCfg,
                        direction_bias_ratio:
                          parseInt(e.target.value, 10) / 100,
                      })
                    }
                    className="w-full accent-primary-container"
                  />
                  <label className="flex cursor-pointer items-center gap-2 text-sm text-on-surface">
                    <input
                      type="checkbox"
                      checked={!!gridCfg.enable_direction_adjust}
                      disabled={gridDis}
                      onChange={(e) =>
                        updateConfig('grid_config', {
                          ...gridCfg,
                          enable_direction_adjust: e.target.checked,
                        })
                      }
                    />
                    启用方向调整
                  </label>
                </div>
                <div className="space-y-2 rounded border border-outline-variant/10 bg-surface-container-lowest p-4 font-mono text-xs text-on-surface-variant">
                  <p className="text-on-surface">
                    网格风控字段仍保存在配置中，含回撤、止损、熔断与仅 Maker
                    等选项。
                  </p>
                  <p className="break-words">
                    max_drawdown: {gridCfg.max_drawdown_pct}% | stop_loss:{' '}
                    {gridCfg.stop_loss_pct}%
                  </p>
                  <p className="break-words">
                    daily_loss_limit: {gridCfg.daily_loss_limit_pct}% |
                    maker_only: {gridCfg.use_maker_only ? 'yes' : 'no'}
                  </p>
                </div>
              </div>
            </section>

            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <section className="rounded-xl bg-surface-container-high p-4 sm:p-6">
                <h3 className="mb-4 flex items-center gap-2 font-['Space_Grotesk',sans-serif] text-lg font-semibold text-on-surface">
                  <span className="material-symbols-outlined text-error">
                    shield_lock
                  </span>
                  网格风控
                </h3>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <div className="space-y-1">
                    <label className="text-xs text-on-surface-variant">
                      最大回撤 %
                    </label>
                    <input
                      type="number"
                      value={gridCfg.max_drawdown_pct}
                      disabled={gridDis}
                      onChange={(e) =>
                        updateConfig('grid_config', {
                          ...gridCfg,
                          max_drawdown_pct: parseFloat(e.target.value) || 0,
                        })
                      }
                      className="w-full rounded border-none bg-surface-container-highest p-2 text-sm text-error"
                    />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-on-surface-variant">
                      止损 %
                    </label>
                    <input
                      type="number"
                      value={gridCfg.stop_loss_pct}
                      disabled={gridDis}
                      onChange={(e) =>
                        updateConfig('grid_config', {
                          ...gridCfg,
                          stop_loss_pct: parseFloat(e.target.value) || 0,
                        })
                      }
                      className="w-full rounded border-none bg-surface-container-highest p-2 text-sm"
                    />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-on-surface-variant">
                      日亏损熔断 %
                    </label>
                    <input
                      type="number"
                      value={gridCfg.daily_loss_limit_pct}
                      disabled={gridDis}
                      onChange={(e) =>
                        updateConfig('grid_config', {
                          ...gridCfg,
                          daily_loss_limit_pct: parseFloat(e.target.value) || 0,
                        })
                      }
                      className="w-full rounded border-none bg-surface-container-highest p-2 text-sm"
                    />
                  </div>
                  <label className="flex items-center gap-2 pt-6 text-sm text-on-surface">
                    <input
                      type="checkbox"
                      checked={gridCfg.use_maker_only}
                      disabled={gridDis}
                      onChange={(e) =>
                        updateConfig('grid_config', {
                          ...gridCfg,
                          use_maker_only: e.target.checked,
                        })
                      }
                    />
                    仅 Maker
                  </label>
                </div>
              </section>
              <div className="quant-lab-glass quant-lab-glow flex flex-col items-center justify-center gap-3 rounded-xl border border-white/5 p-4 text-center sm:p-6">
                <div className="flex h-16 w-16 items-center justify-center rounded-full bg-primary-container/10">
                  <span
                    className="material-symbols-outlined text-3xl text-primary-container"
                    style={{ fontVariationSettings: "'FILL' 1" }}
                  >
                    hub
                  </span>
                </div>
                <p className="text-xs uppercase tracking-widest text-on-surface-variant">
                  核心引擎
                </p>
                <p className="font-['Space_Grotesk',sans-serif] text-xl font-bold text-primary-container">
                  SYNCED & READY
                </p>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
