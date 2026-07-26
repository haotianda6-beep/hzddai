import type { AIModel } from '../types/config'
import type { StrategyConfig } from '../types/strategy'
import { isProgramMartingaleStrategyStudioStrategy } from '../types/strategy'

/** 合规跟单、上架模板或程序化马丁：推荐优先绑定 COMKUN-AI（与后端 StrategyRequiresComkunAIModel 一致） */
export function strategyConfigRequiresComkunAI(
  cfg: StrategyConfig | undefined | null,
  strategyId?: string | null
): boolean {
  if (!cfg) return false
  if (cfg.comkun_follow_listing_template) {
    return (
      !cfg.comkun_listing_template_master_allow_execute &&
      !!cfg.comkun_listing_master_skip_exchange_execution
    )
  }
  if (cfg.comkun_market_follow && !cfg.comkun_follow_listing_template) return true
  if (
    isProgramMartingaleStrategyStudioStrategy(strategyId, cfg.strategy_type) &&
    cfg.martingale_program
  ) {
    return true
  }
  return false
}

/** 用户模型是否为 COMKUN-AI 占位通道（不含 comkun_proxy） */
export function isComkunAIModel(model: AIModel | undefined | null): boolean {
  if (!model) return false
  return model.provider === 'comkun_ai' || model.id === 'comkun_ai' || model.provider === 'ai'
}

/** 按策略过滤可选模型：跟单/马丁类展示全部并把 COMKUN-AI 前置；普通策略隐藏 COMKUN-AI */
export function filterModelsForStrategyConfig(
  models: AIModel[],
  cfg: StrategyConfig | undefined | null,
  strategyId?: string | null
): AIModel[] {
  const req = strategyConfigRequiresComkunAI(cfg, strategyId)
  if (req) {
    return [...models].sort((a, b) => {
      const ar = isComkunAIModel(a) ? 0 : 1
      const br = isComkunAIModel(b) ? 0 : 1
      return ar - br
    })
  }
  return models.filter((m) => !isComkunAIModel(m))
}
