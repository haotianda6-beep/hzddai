/** 策略编辑器在「币安深色」与 Luminescent 工作室之间的配色切换 */
export type StrategyEditorVisualTheme = 'binance' | 'lum'

export function stratColors(theme: StrategyEditorVisualTheme = 'binance') {
  if (theme === 'lum') {
    return {
      text: '#f6f6fc',
      muted: '#aaabb0',
      dim: '#6b6d73',
      accent: '#d4ff33',
      inputBg: '#131313',
      border: '#46484d',
      sectionBg: '#0b0b0b',
      sectionBgAlt: '#1c1c1c',
      track: '#2a2d33',
    }
  }
  return {
    text: '#EAECEF',
    muted: '#848E9C',
    dim: '#5E6673',
    accent: '#c4cf45',
    inputBg: '#1c1c1c',
    border: '#2B3139',
    sectionBg: '#0b0b0b',
    sectionBgAlt: '#1c1c1c',
    track: '#2B3139',
  }
}
