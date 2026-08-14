/** 策略市场展示时保留的产品后缀（不参与剥离） */
const PRESERVED_TITLE_SUFFIXES = ['（客户用）', '(客户用)']

type HistoricalOnlyStrategy = {
  performance_only?: boolean
  performance_source?: string
  realtime_follow_available?: boolean
}

export function isHistoricalOnlyStrategy(
  strategy: HistoricalOnlyStrategy | null | undefined
): boolean {
  return Boolean(
    strategy?.performance_only || strategy?.realtime_follow_available === false
  )
}

export function isSimulatedPerformance(
  strategy: HistoricalOnlyStrategy | null | undefined
): boolean {
  return strategy?.performance_source === 'historical_simulation'
}

export function recentBalanceSeries(
  values: number[] | null | undefined
): number[] {
  return (values ?? [])
    .filter((value) => Number.isFinite(value) && value > 0)
    .slice(-40)
}

/**
 * 策略市场展示用：去掉标题里半角/全角括号及其中的说明文字（如「xxx（只能用COMKUN-AI跑）」→「xxx」）
 * 「（客户用）」等产品后缀保留。
 */
export function stripStrategyTitleParenthetical(name: string): string {
  let s = name.trim()
  let preserved = ''
  for (const suf of PRESERVED_TITLE_SUFFIXES) {
    if (s.endsWith(suf)) {
      preserved = suf
      s = s.slice(0, -suf.length).trim()
      break
    }
  }
  let prev = ''
  while (s !== prev) {
    prev = s
    s = s
      .replace(/\([^()]*\)/g, '')
      .replace(/（[^（）]*）/g, '')
      .trim()
  }
  const base = s.replace(/\s+/g, ' ').trim()
  return preserved ? `${base}${preserved}` : base
}
