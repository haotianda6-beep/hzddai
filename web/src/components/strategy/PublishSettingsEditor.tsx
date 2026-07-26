import { Globe, Lock, Eye, EyeOff } from 'lucide-react'
import { publishSettings, ts } from '../../i18n/strategy-translations'
import type { StrategyEditorVisualTheme } from '../../lib/strategy-editor-theme'
import { stratColors } from '../../lib/strategy-editor-theme'

interface PublishSettingsEditorProps {
  isPublic: boolean
  configVisible: boolean
  onIsPublicChange: (value: boolean) => void
  onConfigVisibleChange: (value: boolean) => void
  disabled?: boolean
  language: string
  visualTheme?: StrategyEditorVisualTheme
}

export function PublishSettingsEditor({
  isPublic,
  configVisible,
  onIsPublicChange,
  onConfigVisibleChange,
  disabled = false,
  language,
  visualTheme = 'binance',
}: PublishSettingsEditorProps) {
  const c = stratColors(visualTheme)
  const lum = visualTheme === 'lum'
  const privateCardBg = lum
    ? 'linear-gradient(135deg, #1c1c1c 0%, #131313 100%)'
    : 'linear-gradient(135deg, #1c1c1c 0%, #0b0b0b 100%)'
  const detailAccent = lum ? '#d4ff33' : '#a855f7'

  return (
    <div className="space-y-3">
      {/* Publish toggle */}
      <div
        className={`relative overflow-hidden rounded-lg transition-all duration-300 ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
        style={{
          background: isPublic
            ? 'linear-gradient(135deg, rgba(14, 203, 129, 0.15) 0%, rgba(14, 203, 129, 0.05) 100%)'
            : privateCardBg,
          border: isPublic ? '1px solid rgba(14, 203, 129, 0.4)' : `1px solid ${c.border}`,
          boxShadow: isPublic ? '0 0 20px rgba(14, 203, 129, 0.1)' : 'none',
        }}
        onClick={() => !disabled && onIsPublicChange(!isPublic)}
      >
        {/* Top glow line */}
        <div
          className="absolute top-0 left-0 w-full h-[1px] transition-opacity duration-300"
          style={{
            background: isPublic
              ? 'linear-gradient(90deg, transparent, #0ECB81, transparent)'
              : `linear-gradient(90deg, transparent, ${c.border}, transparent)`,
            opacity: isPublic ? 1 : 0.5
          }}
        />

        <div className="p-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div
              className="p-2.5 rounded-lg transition-all duration-300"
              style={{
                background: isPublic ? 'rgba(14, 203, 129, 0.2)' : c.sectionBg,
                border: isPublic ? '1px solid rgba(14, 203, 129, 0.3)' : `1px solid ${c.border}`,
              }}
            >
              {isPublic ? (
                <Globe className="w-5 h-5" style={{ color: '#0ECB81' }} />
              ) : (
                <Lock className="h-5 w-5" style={{ color: c.muted }} />
              )}
            </div>
            <div>
              <div className="text-sm font-medium" style={{ color: c.text }}>
                {ts(publishSettings.publishToMarket, language)}
              </div>
              <div className="mt-0.5 text-xs" style={{ color: c.muted }}>
                {ts(publishSettings.publishDesc, language)}
              </div>
            </div>
          </div>

          {/* Toggle with status */}
          <div className="flex items-center gap-3">
            <span
              className="text-[10px] font-mono font-bold tracking-wider"
              style={{ color: isPublic ? '#0ECB81' : c.muted }}
            >
              {isPublic ? ts(publishSettings.public, language) : ts(publishSettings.private, language)}
            </span>
            <div
              className="relative h-6 w-12 rounded-full transition-all duration-300"
              style={{
                background: isPublic
                  ? 'linear-gradient(90deg, #0ECB81, #4ade80)'
                  : c.border,
                boxShadow: isPublic ? '0 0 10px rgba(14, 203, 129, 0.4)' : 'none',
              }}
            >
              <div
                className="absolute top-1 h-4 w-4 rounded-full transition-all duration-300"
                style={{
                  background: c.text,
                  left: isPublic ? '28px' : '4px',
                  boxShadow: '0 2px 4px rgba(0,0,0,0.3)',
                }}
              />
            </div>
          </div>
        </div>
      </div>

      {/* Config visibility toggle - only shown when public */}
      {isPublic && (
        <div
          className={`relative overflow-hidden rounded-lg transition-all duration-300 ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
          style={{
            background: configVisible
              ? lum
                ? 'linear-gradient(135deg, rgba(212, 255, 51, 0.12) 0%, rgba(212, 255, 51, 0.04) 100%)'
                : 'linear-gradient(135deg, rgba(168, 85, 247, 0.15) 0%, rgba(168, 85, 247, 0.05) 100%)'
              : privateCardBg,
            border: configVisible
              ? lum
                ? '1px solid rgba(212, 255, 51, 0.35)'
                : '1px solid rgba(168, 85, 247, 0.4)'
              : `1px solid ${c.border}`,
            boxShadow: configVisible
              ? lum
                ? '0 0 20px rgba(212, 255, 51, 0.08)'
                : '0 0 20px rgba(168, 85, 247, 0.1)'
              : 'none',
          }}
          onClick={() => !disabled && onConfigVisibleChange(!configVisible)}
        >
          {/* Top glow line */}
          <div
            className="absolute top-0 left-0 w-full h-[1px] transition-opacity duration-300"
            style={{
              background: configVisible
                ? `linear-gradient(90deg, transparent, ${detailAccent}, transparent)`
                : `linear-gradient(90deg, transparent, ${c.border}, transparent)`,
              opacity: configVisible ? 1 : 0.5,
            }}
          />

          <div className="p-4 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div
                className="p-2.5 rounded-lg transition-all duration-300"
                style={{
                  background: configVisible
                    ? lum
                      ? 'rgba(212, 255, 51, 0.18)'
                      : 'rgba(168, 85, 247, 0.2)'
                    : c.sectionBg,
                  border: configVisible
                    ? lum
                      ? '1px solid rgba(212, 255, 51, 0.35)'
                      : '1px solid rgba(168, 85, 247, 0.3)'
                    : `1px solid ${c.border}`,
                }}
              >
                {configVisible ? (
                  <Eye className="h-5 w-5" style={{ color: detailAccent }} />
                ) : (
                  <EyeOff className="h-5 w-5" style={{ color: c.muted }} />
                )}
              </div>
              <div>
                <div className="text-sm font-medium" style={{ color: c.text }}>
                  {ts(publishSettings.showConfig, language)}
                </div>
                <div className="mt-0.5 text-xs" style={{ color: c.muted }}>
                  {ts(publishSettings.showConfigDesc, language)}
                </div>
              </div>
            </div>

            {/* Toggle with status */}
            <div className="flex items-center gap-3">
              <span
                className="text-[10px] font-mono font-bold tracking-wider"
                style={{ color: configVisible ? detailAccent : c.muted }}
              >
                {configVisible ? ts(publishSettings.visible, language) : ts(publishSettings.hidden, language)}
              </span>
              <div
                className="relative w-12 h-6 rounded-full transition-all duration-300"
                style={{
                  background: configVisible
                    ? lum
                      ? 'linear-gradient(90deg, #d4ff33, #9fcc00)'
                      : 'linear-gradient(90deg, #a855f7, #c084fc)'
                    : c.border,
                  boxShadow: configVisible
                    ? lum
                      ? '0 0 10px rgba(212, 255, 51, 0.25)'
                      : '0 0 10px rgba(168, 85, 247, 0.4)'
                    : 'none',
                }}
              >
                <div
                  className="absolute top-1 h-4 w-4 rounded-full transition-all duration-300"
                  style={{
                    background: c.text,
                    left: configVisible ? '28px' : '4px',
                    boxShadow: '0 2px 4px rgba(0,0,0,0.3)',
                  }}
                />
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default PublishSettingsEditor
