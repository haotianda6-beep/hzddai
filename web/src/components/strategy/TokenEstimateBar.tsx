import { useState, useEffect, useRef } from 'react'
import { Loader2, Info } from 'lucide-react'
import type { StrategyConfig } from '../../types'
import { t, type Language } from '../../i18n/translations'
import type { StrategyEditorVisualTheme } from '../../lib/strategy-editor-theme'
import { stratColors } from '../../lib/strategy-editor-theme'
import { cn } from '../../lib/cn'

const API_BASE = import.meta.env.VITE_API_BASE || ''

interface ModelLimit {
  name: string
  context_limit: number
  usage_pct: number
  level: string
}

interface TokenEstimateResult {
  total: number
  model_limits: ModelLimit[]
  suggestions: string[]
}

interface TokenEstimateBarProps {
  config: StrategyConfig | null
  language: Language
  onTokenCountChange?: (total: number) => void
  visualTheme?: StrategyEditorVisualTheme
}

export function TokenEstimateBar({
  config,
  language,
  onTokenCountChange,
  visualTheme = 'binance',
}: TokenEstimateBarProps) {
  const c = stratColors(visualTheme)
  const [estimate, setEstimate] = useState<TokenEstimateResult | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const tr = (key: string) => t(`strategyStudio.${key}`, language)

  useEffect(() => {
    if (!config) {
      setEstimate(null)
      return
    }

    if (debounceRef.current) {
      clearTimeout(debounceRef.current)
    }

    debounceRef.current = setTimeout(async () => {
      setIsLoading(true)
      try {
        const response = await fetch(`${API_BASE}/api/strategies/estimate-tokens`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ config }),
        })
        if (response.ok) {
          const data = await response.json()
          setEstimate(data)
          onTokenCountChange?.(data.total)
        }
      } catch {
        // silently ignore — non-critical UI element
      } finally {
        setIsLoading(false)
      }
    }, 800)

    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current)
      }
    }
  }, [config])

  if (!config) return null

  if (isLoading && !estimate) {
    return (
      <div
        className={cn('flex items-center gap-1.5 text-xs', visualTheme !== 'lum' && 'text-nofx-text-muted')}
        style={visualTheme === 'lum' ? { color: c.muted } : undefined}
      >
        <Loader2 className="h-3 w-3 animate-spin" />
        <span>{tr('tokenEstimating')}</span>
      </div>
    )
  }

  if (!estimate) return null

  // Display based on 200K reference
  const pct = Math.round(estimate.total * 100 / 200000)
  const barWidth = Math.min(pct, 100)

  let barColor = '#0ECB81' // green
  let textColor = visualTheme === 'lum' ? c.muted : '#848E9C'
  if (pct >= 100) {
    barColor = '#F6465D' // red
    textColor = '#F6465D'
  } else if (pct >= 80) {
    barColor = visualTheme === 'lum' ? c.accent : '#F0B90B'
    textColor = visualTheme === 'lum' ? c.accent : '#F0B90B'
  }

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <div
          className="flex-1 h-1.5 rounded-full overflow-hidden"
          style={{ background: visualTheme === 'lum' ? c.track : '#1c1c1c' }}
        >
          <div
            className="h-full rounded-full transition-all duration-500"
            style={{ width: `${barWidth}%`, background: barColor }}
          />
        </div>
        <span className="text-xs font-mono whitespace-nowrap" style={{ color: textColor }}>
          {isLoading ? <Loader2 className="w-3 h-3 animate-spin inline" /> : `${pct}%`}
        </span>
        <div className="relative group">
          <Info
            className="h-3 w-3 cursor-help"
            style={{ color: visualTheme === 'lum' ? c.muted : undefined }}
          />
          <div
            className={cn(
              'pointer-events-none absolute bottom-full right-0 z-50 mb-1.5 whitespace-nowrap rounded-lg border px-2.5 py-1.5 text-[10px] opacity-0 shadow-lg transition-opacity group-hover:opacity-100',
              visualTheme !== 'lum' && 'border-nofx-border bg-nofx-bg-lighter text-nofx-text-muted',
            )}
            style={
              visualTheme === 'lum'
                ? {
                    background: c.sectionBgAlt,
                    borderColor: c.border,
                    color: c.muted,
                  }
                : undefined
            }
          >
            {tr('tokenTooltip')} (~{estimate.total.toLocaleString()} / 200K)
          </div>
        </div>
      </div>
    </div>
  )
}
