import { useState } from 'react'
import { ChevronDown, ChevronRight, RotateCcw, FileText } from 'lucide-react'
import type { PromptSectionsConfig } from '../../types'
import { promptSections as promptSectionsI18n, ts } from '../../i18n/strategy-translations'
import type { StrategyEditorVisualTheme } from '../../lib/strategy-editor-theme'
import { stratColors } from '../../lib/strategy-editor-theme'

interface PromptSectionsEditorProps {
  config: PromptSectionsConfig | undefined
  onChange: (config: PromptSectionsConfig) => void
  disabled?: boolean
  language: string
  visualTheme?: StrategyEditorVisualTheme
}

// Default prompt sections (same as backend defaults)
const defaultSections: PromptSectionsConfig = {
  role_definition: `# 你是专业的加密货币交易AI

你专注于技术分析和风险管理，基于市场数据做出理性的交易决策。
你的目标是在控制风险的前提下，捕捉高概率的交易机会。`,

  trading_frequency: `# ⏱️ 交易频率认知

- 优秀交易员：每天2-4笔 ≈ 每小时0.1-0.2笔
- 每小时>2笔 = 过度交易
- 单笔持仓时间≥30-60分钟
如果你发现自己每个周期都在交易 → 标准过低；若持仓<30分钟就平仓 → 过于急躁。`,

  entry_standards: `# 🎯 开仓标准（严格）

只在多重信号共振时开仓：
- 趋势方向明确（EMA排列、价格位置）
- 动量确认（MACD、RSI协同）
- 波动率适中（ATR合理范围）
- 量价配合（成交量支持方向）

避免：单一指标、信号矛盾、横盘震荡、刚平仓即重启。`,

  decision_process: `# 📋 决策流程

1. 检查持仓 → 是否该止盈/止损
2. 扫描候选币 + 多时间框 → 是否存在强信号
3. 评估风险回报比 → 是否满足最小要求
4. 先写思维链，再输出结构化JSON`,
}

export function PromptSectionsEditor({
  config,
  onChange,
  disabled,
  language,
  visualTheme = 'binance',
}: PromptSectionsEditorProps) {
  const c = stratColors(visualTheme)
  const accentIcon = visualTheme === 'lum' ? '#d4ff33' : '#a855f7'
  const [expandedSections, setExpandedSections] = useState<Record<string, boolean>>({
    role_definition: false,
    trading_frequency: false,
    entry_standards: false,
    decision_process: false,
  })

  const sections = [
    { key: 'role_definition', label: ts(promptSectionsI18n.roleDefinition, language), desc: ts(promptSectionsI18n.roleDefinitionDesc, language) },
    { key: 'trading_frequency', label: ts(promptSectionsI18n.tradingFrequency, language), desc: ts(promptSectionsI18n.tradingFrequencyDesc, language) },
    { key: 'entry_standards', label: ts(promptSectionsI18n.entryStandards, language), desc: ts(promptSectionsI18n.entryStandardsDesc, language) },
    { key: 'decision_process', label: ts(promptSectionsI18n.decisionProcess, language), desc: ts(promptSectionsI18n.decisionProcessDesc, language) },
  ]

  const currentConfig = config || {}

  const updateSection = (key: keyof PromptSectionsConfig, value: string) => {
    if (!disabled) {
      onChange({ ...currentConfig, [key]: value })
    }
  }

  const resetSection = (key: keyof PromptSectionsConfig) => {
    if (!disabled) {
      onChange({ ...currentConfig, [key]: defaultSections[key] })
    }
  }

  const toggleSection = (key: string) => {
    setExpandedSections((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const getValue = (key: keyof PromptSectionsConfig): string => {
    return currentConfig[key] || defaultSections[key] || ''
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start gap-2 mb-4">
        <FileText className="mt-0.5 h-5 w-5" style={{ color: accentIcon }} />
        <div>
          <h3 className="font-medium" style={{ color: c.text }}>
            {ts(promptSectionsI18n.promptSections, language)}
          </h3>
          <p className="mt-1 text-xs" style={{ color: c.muted }}>
            {ts(promptSectionsI18n.promptSectionsDesc, language)}
          </p>
        </div>
      </div>

      <div className="space-y-2">
        {sections.map(({ key, label, desc }) => {
          const sectionKey = key as keyof PromptSectionsConfig
          const isExpanded = expandedSections[key]
          const value = getValue(sectionKey)
          const isModified = currentConfig[sectionKey] !== undefined && currentConfig[sectionKey] !== defaultSections[sectionKey]

          return (
            <div
              key={key}
              className="rounded-lg overflow-hidden"
              style={{ background: c.sectionBg, border: `1px solid ${c.border}` }}
            >
              <button
                onClick={() => toggleSection(key)}
                className="w-full flex items-center justify-between px-3 py-2.5 hover:bg-white/5 transition-colors text-left"
              >
                <div className="flex items-center gap-2">
                  {isExpanded ? (
                    <ChevronDown className="h-4 w-4" style={{ color: c.muted }} />
                  ) : (
                    <ChevronRight className="h-4 w-4" style={{ color: c.muted }} />
                  )}
                  <span className="text-sm font-medium" style={{ color: c.text }}>
                    {label}
                  </span>
                  {isModified && (
                    <span
                      className="px-1.5 py-0.5 text-[10px] rounded"
                      style={{
                        background:
                          visualTheme === 'lum' ? 'rgba(212, 255, 51, 0.12)' : 'rgba(168, 85, 247, 0.15)',
                        color: visualTheme === 'lum' ? c.accent : '#a855f7',
                      }}
                    >
                      {ts(promptSectionsI18n.modified, language)}
                    </span>
                  )}
                </div>
                <span className="text-[10px]" style={{ color: c.muted }}>
                  {value.length} {ts(promptSectionsI18n.chars, language)}
                </span>
              </button>

              {isExpanded && (
                <div className="px-3 pb-3">
                  <p className="mb-2 text-xs" style={{ color: c.muted }}>
                    {desc}
                  </p>
                  <textarea
                    value={value}
                    onChange={(e) => updateSection(sectionKey, e.target.value)}
                    disabled={disabled}
                    rows={6}
                    className="w-full px-3 py-2 rounded-lg resize-y font-mono text-xs"
                    style={{
                      background: c.inputBg,
                      border: `1px solid ${c.border}`,
                      color: c.text,
                      minHeight: '120px',
                    }}
                  />
                  <div className="flex justify-end mt-2">
                    <button
                      onClick={() => resetSection(sectionKey)}
                      disabled={disabled || !isModified}
                      className="flex items-center gap-1 px-2 py-1 rounded text-xs transition-colors hover:bg-white/5 disabled:opacity-30"
                      style={{ color: c.muted }}
                    >
                      <RotateCcw className="w-3 h-3" />
                      {ts(promptSectionsI18n.resetToDefault, language)}
                    </button>
                  </div>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
