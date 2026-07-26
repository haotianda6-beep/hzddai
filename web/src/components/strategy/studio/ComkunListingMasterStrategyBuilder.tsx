/**
 * 跟单开关「主控」上架模板：仅保留 COMKUN-AI 跟单同步相关配置（杠杆、镜像、执行开关等）。
 */
import { lazy, Suspense, type ReactNode } from 'react'
import type { Strategy, StrategyConfig } from '../../../types/strategy'
import '../../../pages/landing/luminescent.css'
import './strategy-quant-lab.css'
import { ComkunFollowListingDataBoard } from './ComkunFollowListingDataBoard'

const StrategyPromptMonacoEditor = lazy(() => import('./StrategyPromptMonacoEditor'))

const periodInputClass =
  'min-w-0 flex-1 rounded border border-outline-variant/25 bg-surface-container-highest px-2 py-1.5 font-mono text-xs text-on-surface focus:outline-none'

export interface ComkunListingMasterStrategyBuilderProps {
  editingConfig: StrategyConfig
  selectedStrategy: Strategy
  updateConfig: <K extends keyof StrategyConfig>(section: K, value: StrategyConfig[K]) => void
  editorsDisabled: boolean
}

function SectionTitle({ icon, children }: { icon: string; children: ReactNode }) {
  return (
    <h2 className="mb-3 flex items-center gap-2 font-['Space_Grotesk',sans-serif] text-lg font-bold text-on-surface">
      <span className="material-symbols-outlined text-primary">{icon}</span>
      {children}
    </h2>
  )
}

export function ComkunListingMasterStrategyBuilder({
  editingConfig,
  selectedStrategy,
  updateConfig,
  editorsDisabled,
}: ComkunListingMasterStrategyBuilderProps) {
  const dis = editorsDisabled

  return (
    <div className="strategy-quant-lab flex min-h-0 flex-1 flex-col gap-3 font-['Inter',sans-serif]">
      <div className="quant-lab-scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden pb-8 pr-1">
        <div className={`mx-auto max-w-4xl space-y-8 ${dis ? 'pointer-events-none opacity-50' : ''}`}>
          <div className="rounded-xl border border-primary/30 bg-primary/5 p-5 text-sm leading-relaxed text-on-surface">
            <p className="mb-2 font-bold text-primary">COMKUN-AI · 主控跟单模板</p>
            <ul className="list-inside list-disc space-y-1 text-on-surface-variant">
              <li>
                主控侧统一走 <strong className="text-on-surface">COMKUN-AI</strong>：按主广播同步到用户账户，无需再配「真实大模型扫描」或长篇策略说明。
              </li>
              <li>
                下面仅保留<strong className="text-on-surface">跟单比例 / 杠杆镜像、是否镜像交易所快照、策略源是否代下单</strong>等与同步相关的项；名称与简介在页面顶部编辑，上架价格在顶部「策略市场」。
              </li>
              <li>
                绑定交易所、API、扫描周期等在<strong className="text-on-surface">交易员</strong>里配置。
              </li>
            </ul>
          </div>

          <section className="rounded-xl border border-outline-variant/25 bg-surface-container-high p-6">
            <SectionTitle icon="hub">跟单用户与广播（实时）</SectionTitle>
            <p className="mb-4 text-xs text-on-surface-variant">
              默认展示绑定本策略的跟单交易员及运行状态；可切换到广播记录、策略源订单。
            </p>
            <ComkunFollowListingDataBoard strategyId={selectedStrategy.id} />
          </section>

          <section className="rounded-xl border border-primary/25 bg-surface-container-high p-6">
            <SectionTitle icon="edit_note">编写策略</SectionTitle>
            <p className="mb-4 text-xs leading-relaxed text-on-surface-variant">
              这里是主控策略给 AI 的完整策略说明，会保存到 strategy_prompt；下面的跟单模板设置只控制广播、镜像和执行方式。
            </p>
            <Suspense
              fallback={
                <textarea
                  className="min-h-[360px] w-full rounded-lg border border-outline-variant/25 bg-surface-container-lowest p-4 font-mono text-xs text-on-surface"
                  value={editingConfig.strategy_prompt ?? ''}
                  disabled
                  readOnly
                />
              }
            >
              <StrategyPromptMonacoEditor
                value={editingConfig.strategy_prompt ?? ''}
                onChange={(v) => updateConfig('strategy_prompt', v)}
                disabled={dis}
              />
            </Suspense>
          </section>

          <section className="rounded-xl border border-primary/25 bg-surface-container-lowest/90 p-6">
            <SectionTitle icon="percent">跟单比例 · 镜像杠杆</SectionTitle>
            <p className="mb-4 text-xs leading-relaxed text-on-surface-variant">
              用户账户按主广播里的持仓/挂单对齐时，用「初始保证金 ÷ 主控权益」的比例换算数量；这里的<strong className="text-on-surface">假定杠杆</strong>只参与该比例估算，可与交易所实际杠杆不同。
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="block space-y-1.5">
                <span className="text-xs font-medium text-on-surface">主控假定杠杆（写入广播 mirror_margin）</span>
                <input
                  type="number"
                  min={1}
                  max={125}
                  disabled={dis}
                  className={periodInputClass}
                  value={(editingConfig.comkun_mirror_master_margin_leverage ?? 20) || 20}
                  onChange={(e) => {
                    const n = parseInt(e.target.value, 10)
                    const v = Number.isNaN(n) ? 20 : Math.min(125, Math.max(1, n))
                    updateConfig('comkun_mirror_master_margin_leverage', v)
                  }}
                />
                <span className="text-[10px] text-on-surface-variant">默认 20。</span>
              </label>
              <label className="block space-y-1.5">
                <span className="text-xs font-medium text-on-surface">跟单侧杠杆（镜像 SetLeverage / 名义换算）</span>
                <input
                  type="number"
                  min={1}
                  max={125}
                  disabled={dis}
                  className={periodInputClass}
                  value={(editingConfig.comkun_mirror_follower_margin_leverage ?? 20) || 20}
                  onChange={(e) => {
                    const n = parseInt(e.target.value, 10)
                    const v = Number.isNaN(n) ? 20 : Math.min(125, Math.max(1, n))
                    updateConfig('comkun_mirror_follower_margin_leverage', v)
                  }}
                />
                <span className="text-[10px] text-on-surface-variant">
                  模板给主控参考；客户复制后的子策略里可再改用户侧杠杆。
                </span>
              </label>
            </div>
          </section>

          <section className="rounded-xl border border-outline-variant/25 bg-surface-container-high p-6">
            <SectionTitle icon="token">COMKUN 跟单消耗</SectionTitle>
            <p className="mb-3 text-xs text-on-surface-variant">每轮扫描消耗的虚拟 Token；填 0 或未填则走后端默认。</p>
            <label className="block max-w-xs space-y-1.5">
              <span className="text-xs font-medium text-on-surface">每轮虚拟 Token</span>
              <input
                type="number"
                min={0}
                max={999999}
                disabled={dis}
                className={periodInputClass}
                value={editingConfig.comkun_follow_tokens_per_scan ?? 0}
                onChange={(e) => {
                  const n = parseInt(e.target.value, 10)
                  updateConfig(
                    'comkun_follow_tokens_per_scan',
                    Number.isNaN(n) ? 0 : Math.max(0, n)
                  )
                }}
              />
            </label>
          </section>

          <section className="rounded-xl border border-outline-variant/25 bg-surface-container-high p-6">
            <SectionTitle icon="sync_alt">镜像范围</SectionTitle>
            <label className="flex cursor-pointer items-start gap-3 rounded-lg border border-outline-variant/20 bg-surface-container-lowest/80 p-4">
              <input
                type="checkbox"
                className="mt-1 h-4 w-4 accent-primary"
                checked={!!editingConfig.comkun_follow_mirror_master_exchange}
                disabled={dis}
                onChange={(e) => updateConfig('comkun_follow_mirror_master_exchange', e.target.checked)}
              />
              <span className="text-sm leading-relaxed text-on-surface">
                <span className="font-medium">按主广播里的交易所持仓与挂单镜像跟单</span>
                <span className="mt-1 block text-xs text-on-surface-variant">
                  关闭后请确认业务仍符合预期；一般跟单场景建议保持开启。
                </span>
              </span>
            </label>
          </section>

          <section className="rounded-xl border border-primary/30 bg-primary/5 p-6">
            <SectionTitle icon="bolt">SOL 主控 · 手动持仓广播</SectionTitle>
            <p className="mb-3 text-xs leading-relaxed text-on-surface-variant">
              开启后：围绕 SOLUSDT；无持仓时对外决策为观望；有持仓时广播会弱化开仓类指令（适合手动挂单后再由 COMKUN 同步解读）。请确保标的含 SOLUSDT。
            </p>
            <p className="mb-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs font-medium text-amber-200/95">
              重要：开启后写入主广播的持仓/挂单<strong className="text-on-surface">仅保留 SOLUSDT</strong>。你在所里开的
              <strong className="text-on-surface">其它合约（如 LTC、BTC 等）不会进广播</strong>，跟单端也
              <strong className="text-on-surface">不会跟单</strong>。需要跟非 SOL 合约时请关闭本项。
            </p>
            <label className="flex cursor-pointer items-start gap-3 rounded-lg border border-outline-variant/20 bg-surface-container-lowest/80 p-3">
              <input
                type="checkbox"
                className="mt-1 h-4 w-4 accent-primary"
                checked={!!editingConfig.comkun_listing_master_sol_manual_broadcast_mode}
                disabled={dis}
                onChange={(e) =>
                  updateConfig('comkun_listing_master_sol_manual_broadcast_mode', e.target.checked)
                }
              />
              <span className="text-sm text-on-surface">启用 SOL 手动广播模式</span>
            </label>
          </section>

          <section className="rounded-xl border border-outline-variant/25 bg-surface-container-lowest/80 p-6">
            <SectionTitle icon="smart_display">策略源执行</SectionTitle>
            <p className="text-xs leading-relaxed text-on-surface-variant">
              默认：绑定本策略的主控交易员启动后，可按 COMKUN 决策在交易所执行并写入广播。若你只在所里手动挂单、不希望程序代下单，请勾选「仅分析」。
            </p>
            <label className="mt-4 flex cursor-pointer items-start gap-3 rounded-lg border border-outline-variant/30 bg-surface-container/50 p-4">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 shrink-0 accent-primary"
                disabled={dis}
                checked={!!editingConfig.comkun_listing_master_skip_exchange_execution}
                onChange={(e) => updateConfig('comkun_listing_master_skip_exchange_execution', e.target.checked)}
              />
              <span className="text-xs leading-relaxed text-on-surface-variant">
                <span className="font-medium text-on-surface">仅分析、不在策略源账户代下单</span>
                ：不向交易所执行开/平仓与撤单，仍可向用户广播快照供跟单同步。
              </span>
            </label>
          </section>
        </div>
      </div>
    </div>
  )
}
