import { OKX_STABLE_RAW_TRADE_HISTORY } from './okxStableTradeHistoryData'
import { HZ_STAR_RAW_TRADE_HISTORY } from './hzStarTradeHistoryData'
import { BN_SMART_OPERATION_RAW_TRADE_HISTORY } from './bnSmartOperationTradeHistoryData'
import { ULTIMATE_SOL_RAW_TRADE_HISTORY } from './ultimateSolTradeHistoryData'

type TradeDirection = '多' | '空'
type MarginMode = '全仓' | '逐仓'

export interface TradeHistoryRow {
  id: string
  symbol: string
  contractLabel: '永续'
  leverage: string
  marginMode: MarginMode
  direction: TradeDirection
  status: '已平仓'
  opened: string
  entryPrice: string
  maxOpenInterest: string
  closingPnl: string
  closed: string
  avgClosePrice: string
  closedVol: string
}

export const MAINSTREAM_CALM_STRATEGY_ID = 'bn-screen-mirror-test03-8c402b25'
const MAINSTREAM_CALM_INITIAL_CAPITAL = 10000
export const OKX_STABLE_STRATEGY_ID = 'okx-screen-mirror-01-archive'
const OKX_STABLE_INITIAL_CAPITAL = 30000
export const HZ_STAR_STRATEGY_ID = 'b1d9c5f2-c818-4701-8931-962a73b78274'
const HZ_STAR_INITIAL_CAPITAL = 10000
export const BN_SMART_OPERATION_STRATEGY_ID = 'bn-screen-mirror-test02-5a3b7c1d'
const BN_SMART_OPERATION_INITIAL_CAPITAL = 25000
export const ULTIMATE_SOL_STRATEGY_ID = 'cff474d3-2d32-44ed-a2a4-813fd142e373'
const ULTIMATE_SOL_INITIAL_CAPITAL = 20000

type RawTradeHistoryRow = [
  string,
  string,
  '全仓' | '逐仓',
  '多' | '空',
  string,
  string,
  string,
  string,
  string,
  string,
  string,
]

const RAW_MAINSTREAM_CALM_TRADE_HISTORY: RawTradeHistoryRow[] = [
  ['ETHUSDT', '19x', '全仓', '空', '2026-05-26 18:23:22', '2,095.59 USDT', '24.784 ETH', '+988.02 USDT', '2026-05-28 05:40:39', '2,062.56 USDT', '29.916 ETH'],
  ['ETHUSDT', '19x', '全仓', '多', '2026-05-23 15:48:09', '2,036.00 USDT', '12.572 ETH', '+510.74 USDT', '2026-05-24 22:01:26', '2,076.63 USDT', '12.572 ETH'],
  ['ETHUSDT', '19x', '全仓', '多', '2026-05-19 18:05:16', '2,124.93 USDT', '17.454 ETH', '+280.09 USDT', '2026-05-22 08:47:50', '2,140.98 USDT', '17.454 ETH'],
  ['ETHUSDT', '19x', '全仓', '多', '2026-05-16 15:13:05', '2,128.33 USDT', '35.589 ETH', '+562.86 USDT', '2026-05-19 04:04:35', '2,138.08 USDT', '57.740 ETH'],
  ['ETHUSDT', '19x', '全仓', '空', '2026-05-14 23:49:58', '2,294.00 USDT', '4.727 ETH', '+128.57 USDT', '2026-05-15 12:33:26', '2,266.80 USDT', '4.727 ETH'],
  ['ETHUSDT', '19x', '全仓', '多', '2026-05-11 22:31:40', '2,280.26 USDT', '21.818 ETH', '+831.67 USDT', '2026-05-14 23:19:40', '2,296.79 USDT', '50.290 ETH'],
  ['ETHUSDT', '19x', '全仓', '空', '2026-05-10 23:46:44', '2,346.00 USDT', '12.847 ETH', '+104.24 USDT', '2026-05-11 11:25:30', '2,337.89 USDT', '12.847 ETH'],
  ['BTCUSDT', '15x', '全仓', '空', '2026-05-07 15:08:43', '81,500.00 USDT', '0.331 BTC', '+296.70 USDT', '2026-05-07 21:43:28', '80,603.61 USDT', '0.331 BTC'],
  ['ETHUSDT', '19x', '全仓', '空', '2026-05-01 21:55:30', '2,360.25 USDT', '31.486 ETH', '+774.30 USDT', '2026-05-07 21:48:10', '2,346.76 USDT', '57.370 ETH'],
  ['ETHUSDT', '19x', '全仓', '多', '2026-04-30 02:07:13', '2,241.83 USDT', '21.678 ETH', '+455.77 USDT', '2026-04-30 22:59:44', '2,262.85 USDT', '21.678 ETH'],
  ['ETHUSDT', '19x', '全仓', '空', '2026-04-30 01:07:57', '2,268.58 USDT', '9.356 ETH', '+22.26 USDT', '2026-04-30 01:16:01', '2,266.20 USDT', '9.356 ETH'],
  ['ETHUSDT', '20x', '全仓', '空', '2026-04-25 23:36:32', '2,353.61 USDT', '18.475 ETH', '+1,144.29 USDT', '2026-04-29 08:23:42', '2,304.90 USDT', '23.492 ETH'],
  ['ETHUSDT', '15x', '全仓', '空', '2026-04-23 22:46:02', '2,333.95 USDT', '8.995 ETH', '+156.71 USDT', '2026-04-25 11:15:31', '2,316.53 USDT', '8.995 ETH'],
  ['ETHUSDT', '20x', '全仓', '空', '2026-04-22 10:53:21', '2,389.38 USDT', '21.180 ETH', '+390.87 USDT', '2026-04-23 10:48:07', '2,370.92 USDT', '21.180 ETH'],
  ['ETHUSDT', '20x', '全仓', '多', '2026-04-21 09:21:47', '2,313.16 USDT', '28.468 ETH', '+836.94 USDT', '2026-04-22 10:36:10', '2,335.93 USDT', '36.748 ETH'],
  ['ETHUSDT', '20x', '全仓', '空', '2026-04-17 21:08:18', '2,422.25 USDT', '38.383 ETH', '+1,749.63 USDT', '2026-04-20 01:17:01', '2,376.67 USDT', '38.383 ETH'],
  ['ETHUSDT', '20x', '全仓', '多', '2026-04-16 23:28:25', '2,325.38 USDT', '35.170 ETH', '+1,294.33 USDT', '2026-04-17 19:28:02', '2,347.65 USDT', '58.134 ETH'],
  ['ETHUSDT', '20x', '逐仓', '空', '2026-04-15 21:57:05', '2,332.23 USDT', '46.182 ETH', '-1,062.14 USDT', '2026-04-16 14:26:51', '2,355.23 USDT', '46.182 ETH'],
  ['ETHUSDT', '20x', '逐仓', '空', '2026-04-14 22:32:33', '2,416.00 USDT', '22.618 ETH', '+1,421.77 USDT', '2026-04-14 23:02:58', '2,353.14 USDT', '22.618 ETH'],
  ['ETHUSDT', '19x', '逐仓', '空', '2026-04-14 16:36:51', '2,392.00 USDT', '15.394 ETH', '+187.34 USDT', '2026-04-14 22:18:21', '2,379.83 USDT', '15.394 ETH'],
  ['ETHUSDT', '19x', '逐仓', '空', '2026-04-14 07:10:09', '2,369.89 USDT', '37.130 ETH', '+219.07 USDT', '2026-04-14 12:02:09', '2,363.99 USDT', '37.130 ETH'],
  ['ETHUSDT', '19x', '逐仓', '多', '2026-04-13 23:49:26', '2,204.50 USDT', '20.230 ETH', '+1,233.02 USDT', '2026-04-14 03:56:39', '2,265.45 USDT', '20.230 ETH'],
  ['ETHUSDT', '19x', '逐仓', '空', '2026-04-10 11:24:40', '2,184.18 USDT', '15.485 ETH', '+17.34 USDT', '2026-04-10 11:26:31', '2,183.06 USDT', '15.485 ETH'],
  ['ETHUSDT', '19x', '逐仓', '多', '2026-04-10 10:00:20', '2,193.60 USDT', '14.979 ETH', '+782.90 USDT', '2026-04-10 22:20:08', '2,245.87 USDT', '14.979 ETH'],
  ['ETHUSDT', '19x', '逐仓', '空', '2026-04-10 06:17:16', '2,235.00 USDT', '23.965 ETH', '+630.52 USDT', '2026-04-10 06:48:31', '2,208.69 USDT', '23.965 ETH'],
  ['ETHUSDT', '15x', '逐仓', '空', '2026-04-08 10:11:56', '2,246.85 USDT', '21.734 ETH', '+1,398.60 USDT', '2026-04-09 09:09:09', '2,182.50 USDT', '21.734 ETH'],
  ['ETHUSDT', '9x', '逐仓', '空', '2026-04-08 07:20:30', '2,245.00 USDT', '23.923 ETH', '+90.36 USDT', '2026-04-08 08:27:52', '2,241.22 USDT', '23.923 ETH'],
  ['ETHUSDT', '14x', '逐仓', '空', '2026-04-06 06:58:44', '2,098.76 USDT', '35.145 ETH', '-33.36 USDT', '2026-04-07 20:30:26', '2,099.71 USDT', '35.145 ETH'],
  ['ETHUSDT', '15x', '逐仓', '空', '2026-04-03 11:14:08', '2,054.00 USDT', '47.134 ETH', '+263.48 USDT', '2026-04-04 10:53:50', '2,048.41 USDT', '47.134 ETH'],
  ['ETHUSDT', '9x', '逐仓', '空', '2026-04-02 22:21:56', '2,043.50 USDT', '24.951 ETH', '-132.28 USDT', '2026-04-02 23:23:04', '2,048.80 USDT', '24.951 ETH'],
  ['ETHUSDT', '9x', '逐仓', '空', '2026-04-01 21:34:10', '2,130.00 USDT', '21.941 ETH', '+1,107.02 USDT', '2026-04-02 10:44:12', '2,079.55 USDT', '21.941 ETH'],
  ['ETHUSDT', '23x', '逐仓', '多', '2026-04-01 11:12:37', '2,100.50 USDT', '40.084 ETH', '+1,715.83 USDT', '2026-04-01 14:26:16', '2,143.31 USDT', '40.084 ETH'],
  ['TAOUSDT', '10x', '全仓', '空', '2026-03-30 22:18:18', '309.33 USDT', '10.975 TAO', '+30.19 USDT', '2026-04-02 14:42:17', '306.58 USDT', '10.975 TAO'],
  ['ETHUSDT', '10x', '逐仓', '空', '2026-03-30 21:38:01', '2,075.00 USDT', '3.720 ETH', '+94.75 USDT', '2026-03-30 22:38:33', '2,049.53 USDT', '3.720 ETH'],
]

export const MAINSTREAM_CALM_TRADE_HISTORY: TradeHistoryRow[] = RAW_MAINSTREAM_CALM_TRADE_HISTORY.map((row, index) => ({
  id: `mainstream-calm-${index}`,
  symbol: row[0],
  contractLabel: '永续',
  leverage: row[1].replace('x', '倍'),
  marginMode: row[2] === '全仓' ? '全仓' : '逐仓',
  direction: row[3] === '多' ? '多' : '空',
  status: '已平仓',
  opened: row[4],
  entryPrice: row[5],
  maxOpenInterest: row[6],
  closingPnl: row[7],
  closed: row[8],
  avgClosePrice: row[9],
  closedVol: row[10],
}))

export const OKX_STABLE_TRADE_HISTORY: TradeHistoryRow[] = OKX_STABLE_RAW_TRADE_HISTORY.map((row, index) => ({
  id: `okx-stable-${index}`,
  symbol: row[0],
  contractLabel: '永续',
  leverage: row[1].replace('x', '倍'),
  marginMode: row[2] === '全仓' ? '全仓' : '逐仓',
  direction: row[3] === '多' ? '多' : '空',
  status: '已平仓',
  opened: row[4],
  entryPrice: row[5],
  maxOpenInterest: row[6],
  closingPnl: row[7],
  closed: row[8],
  avgClosePrice: row[9],
  closedVol: row[10],
}))

export const HZ_STAR_TRADE_HISTORY: TradeHistoryRow[] = HZ_STAR_RAW_TRADE_HISTORY.map((row, index) => ({
  id: `hz-star-${index}`,
  symbol: row[0],
  contractLabel: '永续',
  leverage: row[1].replace('x', '倍'),
  marginMode: row[2] === '全仓' ? '全仓' : '逐仓',
  direction: row[3] === '多' ? '多' : '空',
  status: '已平仓',
  opened: row[4],
  entryPrice: row[5],
  maxOpenInterest: row[6],
  closingPnl: row[7],
  closed: row[8],
  avgClosePrice: row[9],
  closedVol: row[10],
}))

export const BN_SMART_OPERATION_TRADE_HISTORY: TradeHistoryRow[] = BN_SMART_OPERATION_RAW_TRADE_HISTORY.map((row, index) => ({
  id: `bn-smart-operation-${index}`,
  symbol: row[0],
  contractLabel: '永续',
  leverage: row[1].replace('x', '倍'),
  marginMode: row[2] === '全仓' ? '全仓' : '逐仓',
  direction: row[3] === '多' ? '多' : '空',
  status: '已平仓',
  opened: row[4],
  entryPrice: row[5],
  maxOpenInterest: row[6],
  closingPnl: row[7],
  closed: row[8],
  avgClosePrice: row[9],
  closedVol: row[10],
}))

export const ULTIMATE_SOL_TRADE_HISTORY: TradeHistoryRow[] = ULTIMATE_SOL_RAW_TRADE_HISTORY.map((row, index) => ({
  id: `ultimate-sol-${index}`,
  symbol: row[0],
  contractLabel: '永续',
  leverage: row[1].replace('x', '倍'),
  marginMode: row[2] === '全仓' ? '全仓' : '逐仓',
  direction: row[3] === '多' ? '多' : '空',
  status: '已平仓',
  opened: row[4],
  entryPrice: row[5],
  maxOpenInterest: row[6],
  closingPnl: row[7],
  closed: row[8],
  avgClosePrice: row[9],
  closedVol: row[10],
}))

function parseUsdtAmount(value: string): number {
  const normalized = value.replace(/USDT/g, '').replace(/,/g, '').trim()
  const parsed = Number(normalized)
  return Number.isFinite(parsed) ? parsed : 0
}

function parseLocalDateMs(value: string): number {
  const match = value.match(
    /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/
  )
  if (!match) return Number.NaN
  const [, year, month, day, hour, minute, second] = match
  return new Date(
    Number(year),
    Number(month) - 1,
    Number(day),
    Number(hour),
    Number(minute),
    Number(second)
  ).getTime()
}

function round2(value: number): number {
  return Math.round(value * 100) / 100
}

function calculateSharpeRatio(pnls: number[], initialCapital: number): number {
  if (pnls.length <= 1 || initialCapital <= 0) return 0
  const returns = pnls.map((pnl) => pnl / initialCapital)
  const mean = returns.reduce((sum, item) => sum + item, 0) / returns.length
  const variance =
    returns.reduce((sum, item) => sum + (item - mean) ** 2, 0) / (returns.length - 1)
  if (variance <= 0) return 0
  return round2((mean / Math.sqrt(variance)) * Math.sqrt(returns.length))
}

function buildTradeHistoryPerformance(rows: TradeHistoryRow[], initialCapital: number) {
  const trades = rows.map((row) => ({
    ...row,
    pnl: parseUsdtAmount(row.closingPnl),
    openedMs: parseLocalDateMs(row.opened),
    closedMs: parseLocalDateMs(row.closed),
  }))
  const totalTrades = trades.length
  const winTrades = trades.filter((row) => row.pnl > 0).length
  const lossTrades = trades.filter((row) => row.pnl < 0).length
  const longTrades = trades.filter((row) => row.direction === '多').length
  const shortTrades = trades.filter((row) => row.direction === '空').length
  const grossProfit = round2(
    trades.filter((row) => row.pnl > 0).reduce((sum, row) => sum + row.pnl, 0)
  )
  const grossLoss = round2(
    Math.abs(trades.filter((row) => row.pnl < 0).reduce((sum, row) => sum + row.pnl, 0))
  )
  const totalPnl = round2(trades.reduce((sum, row) => sum + row.pnl, 0))
  const validHoldTimes = trades
    .map((row) => row.closedMs - row.openedMs)
    .filter((ms) => Number.isFinite(ms) && ms > 0)
  const avgHoldMs =
    validHoldTimes.length > 0
      ? validHoldTimes.reduce((sum, ms) => sum + ms, 0) / validHoldTimes.length
      : 0

  let equity = initialCapital > 0 ? initialCapital : 0
  let peak = equity
  let maxDrawdownPct = 0
  const trend = trades
    .slice()
    .sort((a, b) => a.closedMs - b.closedMs)
    .map((row) => {
      equity += row.pnl
      if (equity > peak) peak = equity
      const drawdownPct = peak > 0 ? ((peak - equity) / peak) * 100 : 0
      if (drawdownPct > maxDrawdownPct) maxDrawdownPct = drawdownPct
      return round2(equity - (initialCapital > 0 ? initialCapital : 0))
    })

  return {
    initialCapital,
    returnPct: initialCapital > 0 ? round2((totalPnl / initialCapital) * 100) : undefined,
    trend,
    aggregate: {
      avg_hold_ms: avgHoldMs,
      gross_profit: grossProfit,
      gross_loss: grossLoss,
      long_trades: longTrades,
      short_trades: shortTrades,
      stats: {
        total_trades: totalTrades,
        win_trades: winTrades,
        loss_trades: lossTrades,
        win_rate: totalTrades > 0 ? round2((winTrades / totalTrades) * 100) : 0,
        profit_factor: grossLoss > 0 ? round2(grossProfit / grossLoss) : grossProfit > 0 ? 999 : 0,
        sharpe_ratio: calculateSharpeRatio(
          trades.map((row) => row.pnl),
          initialCapital
        ),
        total_pnl: totalPnl,
        total_fee: Number.NaN,
        avg_win: winTrades > 0 ? round2(grossProfit / winTrades) : 0,
        avg_loss: lossTrades > 0 ? round2(grossLoss / lossTrades) : 0,
        max_drawdown_pct: round2(maxDrawdownPct),
      },
    },
  }
}

export function buildMainstreamCalmPerformance(strategyId: string) {
  if (strategyId !== MAINSTREAM_CALM_STRATEGY_ID) return undefined
  return buildTradeHistoryPerformance(MAINSTREAM_CALM_TRADE_HISTORY, MAINSTREAM_CALM_INITIAL_CAPITAL)
}

export function buildOkxStablePerformance(strategyId: string) {
  if (strategyId !== OKX_STABLE_STRATEGY_ID) return undefined
  return buildTradeHistoryPerformance(OKX_STABLE_TRADE_HISTORY, OKX_STABLE_INITIAL_CAPITAL)
}

export function buildHzStarPerformance(strategyId: string) {
  if (strategyId !== HZ_STAR_STRATEGY_ID) return undefined
  return buildTradeHistoryPerformance(HZ_STAR_TRADE_HISTORY, HZ_STAR_INITIAL_CAPITAL)
}

export function buildBnSmartOperationPerformance(strategyId: string) {
  if (strategyId !== BN_SMART_OPERATION_STRATEGY_ID) return undefined
  return buildTradeHistoryPerformance(BN_SMART_OPERATION_TRADE_HISTORY, BN_SMART_OPERATION_INITIAL_CAPITAL)
}

export function buildUltimateSolPerformance(strategyId: string) {
  if (strategyId !== ULTIMATE_SOL_STRATEGY_ID) return undefined
  return buildTradeHistoryPerformance(ULTIMATE_SOL_TRADE_HISTORY, ULTIMATE_SOL_INITIAL_CAPITAL)
}

interface TradeHistorySectionProps {
  strategyId: string
  rows?: TradeHistoryRow[]
}

export function TradeHistorySection({ strategyId, rows: overrideRows }: TradeHistorySectionProps) {
  const rows =
    Array.isArray(overrideRows)
      ? overrideRows
      : strategyId === MAINSTREAM_CALM_STRATEGY_ID
      ? MAINSTREAM_CALM_TRADE_HISTORY
      : strategyId === OKX_STABLE_STRATEGY_ID
        ? OKX_STABLE_TRADE_HISTORY
        : strategyId === HZ_STAR_STRATEGY_ID
          ? []
          : strategyId === BN_SMART_OPERATION_STRATEGY_ID
            ? BN_SMART_OPERATION_TRADE_HISTORY
            : strategyId === ULTIMATE_SOL_STRATEGY_ID
              ? ULTIMATE_SOL_TRADE_HISTORY
              : []
  if (rows.length === 0) return null

  return (
    <section className="trade-history-section" aria-labelledby="trade-history-title">
      <div className="trade-history-heading">
        <h2 id="trade-history-title">做单历史</h2>
      </div>

      <div className="trade-history-list">
        {rows.map((row) => {
          const isLong = row.direction === '多'
          const isProfit = !row.closingPnl.startsWith('-')
          return (
            <article className="trade-history-card" key={row.id}>
              <div className="trade-history-card-head">
                <div className="trade-history-symbol">
                  <span className="trade-history-symbol-dot" aria-hidden />
                  <strong>{row.symbol}</strong>
                  <span className="trade-history-chip trade-history-chip-neutral">{row.contractLabel}</span>
                  <span className="trade-history-chip trade-history-chip-neutral">{row.leverage}</span>
                  <span className="trade-history-chip trade-history-chip-neutral">{row.marginMode}</span>
                  <span
                    className={`trade-history-chip ${
                      isLong ? 'trade-history-chip-long' : 'trade-history-chip-short'
                    }`}
                  >
                    {row.direction}
                  </span>
                  <span className="trade-history-chip trade-history-chip-closed">{row.status}</span>
                </div>
              </div>

              <dl className="trade-history-grid">
                <div>
                  <dt>开仓时间</dt>
                  <dd>{row.opened}</dd>
                </div>
                <div>
                  <dt>开仓均价</dt>
                  <dd>{row.entryPrice}</dd>
                </div>
                <div>
                  <dt>最大持仓量</dt>
                  <dd>{row.maxOpenInterest}</dd>
                </div>
                <div>
                  <dt>平仓盈亏</dt>
                  <dd className={isProfit ? 'is-profit' : 'is-loss'}>{row.closingPnl}</dd>
                </div>
                <div>
                  <dt>平仓时间</dt>
                  <dd>{row.closed}</dd>
                </div>
                <div>
                  <dt>平仓均价</dt>
                  <dd>{row.avgClosePrice}</dd>
                </div>
                <div>
                  <dt>平仓量</dt>
                  <dd>{row.closedVol}</dd>
                </div>
              </dl>
            </article>
          )
        })}
      </div>
    </section>
  )
}
