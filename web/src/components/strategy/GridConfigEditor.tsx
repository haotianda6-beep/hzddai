import { Grid, DollarSign, TrendingUp, Shield, Compass } from 'lucide-react'
import type { GridStrategyConfig } from '../../types'
import { gridConfig, ts } from '../../i18n/strategy-translations'
import type { StrategyEditorVisualTheme } from '../../lib/strategy-editor-theme'
import { stratColors } from '../../lib/strategy-editor-theme'
import { NofxSelect } from '../ui/select'

interface GridConfigEditorProps {
  config: GridStrategyConfig
  onChange: (config: GridStrategyConfig) => void
  disabled?: boolean
  language: string
  /** 策略工作室使用 lum，与外层深色卡片一致 */
  visualTheme?: StrategyEditorVisualTheme
}

// Default grid configuration
export const defaultGridConfig: GridStrategyConfig = {
  symbol: 'BTCUSDT',
  grid_count: 10,
  total_investment: 1000,
  leverage: 5,
  upper_price: 0,
  lower_price: 0,
  use_atr_bounds: true,
  atr_multiplier: 2.0,
  distribution: 'gaussian',
  max_drawdown_pct: 15,
  stop_loss_pct: 5,
  daily_loss_limit_pct: 10,
  use_maker_only: true,
  enable_direction_adjust: false,
  direction_bias_ratio: 0.7,
}

export function GridConfigEditor({
  config,
  onChange,
  disabled,
  language,
  visualTheme = 'binance',
}: GridConfigEditorProps) {
  const c = stratColors(visualTheme)
  const updateField = <K extends keyof GridStrategyConfig>(
    key: K,
    value: GridStrategyConfig[K]
  ) => {
    if (!disabled) {
      onChange({ ...config, [key]: value })
    }
  }

  const inputStyle = {
    background: c.inputBg,
    border: `1px solid ${c.border}`,
    color: c.text,
  }

  const sectionStyle = {
    background: c.sectionBg,
    border: `1px solid ${c.border}`,
  }

  const toggleSliderClass = `w-11 h-6 bg-gray-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all ${
    visualTheme === 'lum' ? 'peer-checked:bg-[#d4ff33]' : 'peer-checked:bg-[#c4cf45]'
  }`

  return (
    <div className="space-y-6">
      {/* Trading Setup */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <DollarSign className="w-5 h-5" style={{ color: c.accent }} />
          <h3 className="font-medium" style={{ color: c.text }}>
            {ts(gridConfig.tradingPair, language)}
          </h3>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {/* Symbol */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.symbol, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.symbolDesc, language)}
            </p>
            <NofxSelect
              value={config.symbol}
              onChange={(val) => updateField('symbol', val)}
              disabled={disabled}
              visualTheme={visualTheme}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
              options={[
                { value: 'BTCUSDT', label: 'BTC/USDT' },
                { value: 'ETHUSDT', label: 'ETH/USDT' },
                { value: 'SOLUSDT', label: 'SOL/USDT' },
                { value: 'BNBUSDT', label: 'BNB/USDT' },
                { value: 'XRPUSDT', label: 'XRP/USDT' },
                { value: 'DOGEUSDT', label: 'DOGE/USDT' },
              ]}
            />
          </div>

          {/* Investment */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.totalInvestment, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.totalInvestmentDesc, language)}
            </p>
            <input
              type="number"
              value={config.total_investment}
              onChange={(e) => updateField('total_investment', parseFloat(e.target.value) || 1000)}
              disabled={disabled}
              min={100}
              step={100}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>

          {/* Leverage */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.leverage, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.leverageDesc, language)}
            </p>
            <input
              type="number"
              value={config.leverage}
              onChange={(e) => updateField('leverage', parseInt(e.target.value) || 5)}
              disabled={disabled}
              min={1}
              max={5}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>
        </div>
      </div>

      {/* Grid Parameters */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Grid className="w-5 h-5" style={{ color: c.accent }} />
          <h3 className="font-medium" style={{ color: c.text }}>
            {ts(gridConfig.gridParameters, language)}
          </h3>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* Grid Count */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.gridCount, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.gridCountDesc, language)}
            </p>
            <input
              type="number"
              value={config.grid_count}
              onChange={(e) => updateField('grid_count', parseInt(e.target.value) || 10)}
              disabled={disabled}
              min={5}
              max={50}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>

          {/* Distribution */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.distribution, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.distributionDesc, language)}
            </p>
            <NofxSelect
              value={config.distribution}
              onChange={(val) => updateField('distribution', val as 'uniform' | 'gaussian' | 'pyramid')}
              disabled={disabled}
              visualTheme={visualTheme}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
              options={[
                { value: 'uniform', label: ts(gridConfig.uniform, language) },
                { value: 'gaussian', label: ts(gridConfig.gaussian, language) },
                { value: 'pyramid', label: ts(gridConfig.pyramid, language) },
              ]}
            />
          </div>
        </div>
      </div>

      {/* Price Bounds */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <TrendingUp className="w-5 h-5" style={{ color: c.accent }} />
          <h3 className="font-medium" style={{ color: c.text }}>
            {ts(gridConfig.priceBounds, language)}
          </h3>
        </div>

        {/* ATR Toggle */}
        <div className="p-4 rounded-lg mb-4" style={sectionStyle}>
          <div className="flex items-center justify-between">
            <div>
              <label className="block text-sm" style={{ color: c.text }}>
                {ts(gridConfig.useAtrBounds, language)}
              </label>
              <p className="text-xs" style={{ color: c.muted }}>
                {ts(gridConfig.useAtrBoundsDesc, language)}
              </p>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={config.use_atr_bounds}
                onChange={(e) => updateField('use_atr_bounds', e.target.checked)}
                disabled={disabled}
                className="sr-only peer"
              />
              <div className={toggleSliderClass} />
            </label>
          </div>
        </div>

        {config.use_atr_bounds ? (
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.atrMultiplier, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.atrMultiplierDesc, language)}
            </p>
            <input
              type="number"
              value={config.atr_multiplier}
              onChange={(e) => updateField('atr_multiplier', parseFloat(e.target.value) || 2.0)}
              disabled={disabled}
              min={1}
              max={5}
              step={0.5}
              className="w-32 px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="p-4 rounded-lg" style={sectionStyle}>
              <label className="block text-sm mb-1" style={{ color: c.text }}>
                {ts(gridConfig.upperPrice, language)}
              </label>
              <p className="text-xs mb-2" style={{ color: c.muted }}>
                {ts(gridConfig.upperPriceDesc, language)}
              </p>
              <input
                type="number"
                value={config.upper_price}
                onChange={(e) => updateField('upper_price', parseFloat(e.target.value) || 0)}
                disabled={disabled}
                min={0}
                step={0.01}
                className="w-full px-3 py-2 rounded"
                style={inputStyle}
              />
            </div>
            <div className="p-4 rounded-lg" style={sectionStyle}>
              <label className="block text-sm mb-1" style={{ color: c.text }}>
                {ts(gridConfig.lowerPrice, language)}
              </label>
              <p className="text-xs mb-2" style={{ color: c.muted }}>
                {ts(gridConfig.lowerPriceDesc, language)}
              </p>
              <input
                type="number"
                value={config.lower_price}
                onChange={(e) => updateField('lower_price', parseFloat(e.target.value) || 0)}
                disabled={disabled}
                min={0}
                step={0.01}
                className="w-full px-3 py-2 rounded"
                style={inputStyle}
              />
            </div>
          </div>
        )}
      </div>

      {/* Risk Control */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Shield className="w-5 h-5" style={{ color: c.accent }} />
          <h3 className="font-medium" style={{ color: c.text }}>
            {ts(gridConfig.riskControl, language)}
          </h3>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.maxDrawdown, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.maxDrawdownDesc, language)}
            </p>
            <input
              type="number"
              value={config.max_drawdown_pct}
              onChange={(e) => updateField('max_drawdown_pct', parseFloat(e.target.value) || 15)}
              disabled={disabled}
              min={5}
              max={50}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>

          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.stopLoss, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.stopLossDesc, language)}
            </p>
            <input
              type="number"
              value={config.stop_loss_pct}
              onChange={(e) => updateField('stop_loss_pct', parseFloat(e.target.value) || 5)}
              disabled={disabled}
              min={1}
              max={20}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>

          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1" style={{ color: c.text }}>
              {ts(gridConfig.dailyLossLimit, language)}
            </label>
            <p className="text-xs mb-2" style={{ color: c.muted }}>
              {ts(gridConfig.dailyLossLimitDesc, language)}
            </p>
            <input
              type="number"
              value={config.daily_loss_limit_pct}
              onChange={(e) => updateField('daily_loss_limit_pct', parseFloat(e.target.value) || 10)}
              disabled={disabled}
              min={1}
              max={30}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>
        </div>

        {/* Maker Only Toggle */}
        <div className="p-4 rounded-lg" style={sectionStyle}>
          <div className="flex items-center justify-between">
            <div>
              <label className="block text-sm" style={{ color: c.text }}>
                {ts(gridConfig.useMakerOnly, language)}
              </label>
              <p className="text-xs" style={{ color: c.muted }}>
                {ts(gridConfig.useMakerOnlyDesc, language)}
              </p>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={config.use_maker_only}
                onChange={(e) => updateField('use_maker_only', e.target.checked)}
                disabled={disabled}
                className="sr-only peer"
              />
              <div className={toggleSliderClass} />
            </label>
          </div>
        </div>
      </div>

      {/* Direction Auto-Adjust */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Compass className="w-5 h-5" style={{ color: c.accent }} />
          <h3 className="font-medium" style={{ color: c.text }}>
            {ts(gridConfig.directionAdjust, language)}
          </h3>
        </div>

        {/* Enable Toggle */}
        <div className="p-4 rounded-lg mb-4" style={sectionStyle}>
          <div className="flex items-center justify-between">
            <div>
              <label className="block text-sm" style={{ color: c.text }}>
                {ts(gridConfig.enableDirectionAdjust, language)}
              </label>
              <p className="text-xs" style={{ color: c.muted }}>
                {ts(gridConfig.enableDirectionAdjustDesc, language)}
              </p>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={config.enable_direction_adjust ?? false}
                onChange={(e) => updateField('enable_direction_adjust', e.target.checked)}
                disabled={disabled}
                className="sr-only peer"
              />
              <div className={toggleSliderClass} />
            </label>
          </div>
        </div>

        {config.enable_direction_adjust && (
          <>
            {/* Direction Modes Explanation */}
            <div
              className="mb-4 rounded-lg border p-4"
              style={{ background: c.sectionBgAlt, borderColor: `${c.accent}40` }}
            >
              <p className="mb-2 text-xs font-medium" style={{ color: c.accent }}>
                📊 {ts(gridConfig.directionModes, language)}
              </p>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs" style={{ color: c.muted }}>
                <div>• {ts(gridConfig.modeNeutral, language)}</div>
                <div>• <span style={{ color: '#0ECB81' }}>{ts(gridConfig.modeLongBias, language)}</span></div>
                <div>• <span style={{ color: '#0ECB81' }}>{ts(gridConfig.modeLong, language)}</span></div>
                <div>• <span style={{ color: '#F6465D' }}>{ts(gridConfig.modeShortBias, language)}</span></div>
                <div>• <span style={{ color: '#F6465D' }}>{ts(gridConfig.modeShort, language)}</span></div>
              </div>
              <p className="text-xs mt-3 pt-2 border-t border-zinc-700" style={{ color: c.muted }}>
                💡 {ts(gridConfig.directionExplain, language)}
              </p>
            </div>

            {/* Bias Strength */}
            <div className="p-4 rounded-lg" style={sectionStyle}>
              <label className="block text-sm mb-1" style={{ color: c.text }}>
                {ts(gridConfig.directionBiasRatio, language)} (X)
              </label>
              <p className="text-xs mb-1" style={{ color: c.muted }}>
                {ts(gridConfig.directionBiasRatioDesc, language)}
              </p>
              <p className="text-xs mb-3" style={{ color: c.accent }}>
                {ts(gridConfig.directionBiasExplain, language)}
              </p>
              <div className="flex items-center gap-3">
                <input
                  type="range"
                  value={(config.direction_bias_ratio ?? 0.7) * 100}
                  onChange={(e) => updateField('direction_bias_ratio', parseInt(e.target.value) / 100)}
                  disabled={disabled}
                  min={55}
                  max={90}
                  step={5}
                  className="flex-1 h-2 rounded-lg appearance-none cursor-pointer"
                  style={{ background: c.track }}
                />
                <span className="text-sm font-mono w-20 text-right" style={{ color: c.accent }}>
                  X = {Math.round((config.direction_bias_ratio ?? 0.7) * 100)}%
                </span>
              </div>
              <div className="mt-2 grid grid-cols-2 gap-2 text-xs">
                <div className="p-2 rounded" style={{ background: '#0ECB8115', border: '1px solid #0ECB8130' }}>
                  <span style={{ color: '#0ECB81' }}>Long Bias: </span>
                  <span style={{ color: c.text }}>{Math.round((config.direction_bias_ratio ?? 0.7) * 100)}% {ts(gridConfig.buy, language)} + {Math.round((1 - (config.direction_bias_ratio ?? 0.7)) * 100)}% {ts(gridConfig.sell, language)}</span>
                </div>
                <div className="p-2 rounded" style={{ background: '#F6465D15', border: '1px solid #F6465D30' }}>
                  <span style={{ color: '#F6465D' }}>Short Bias: </span>
                  <span style={{ color: c.text }}>{Math.round((1 - (config.direction_bias_ratio ?? 0.7)) * 100)}% {ts(gridConfig.buy, language)} + {Math.round((config.direction_bias_ratio ?? 0.7) * 100)}% {ts(gridConfig.sell, language)}</span>
                </div>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
