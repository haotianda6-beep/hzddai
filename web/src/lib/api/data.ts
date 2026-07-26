import type {
  SystemStatus,
  AccountInfo,
  Position,
  ExchangeOpenOrder,
  DecisionRecord,
  Statistics,
  CompetitionData,
  PositionHistoryResponse,
  MarketBoardPayload,
  CryptoNewsPayload,
} from '../../types'
import { API_BASE, httpClient } from './helpers'

export const dataApi = {
  async getStatus(traderId?: string, silent?: boolean): Promise<SystemStatus> {
    const url = traderId
      ? `${API_BASE}/status?trader_id=${traderId}`
      : `${API_BASE}/status`
    const result = await httpClient.request<SystemStatus>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch system status')
    return result.data!
  },

  async getAccount(traderId?: string, silent?: boolean): Promise<AccountInfo> {
    const url = traderId
      ? `${API_BASE}/account?trader_id=${traderId}`
      : `${API_BASE}/account`
    const result = await httpClient.request<AccountInfo>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch account info')
    return result.data!
  },

  async getPositions(traderId?: string, silent?: boolean): Promise<Position[]> {
    const url = traderId
      ? `${API_BASE}/positions?trader_id=${traderId}`
      : `${API_BASE}/positions`
    const result = await httpClient.request<Position[]>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch positions')
    return result.data!
  },

  /**
   * 交易所未成交挂单。不传 symbol 时拉取账户全部挂单（限价、止盈止损等），用于 AI 策略数据看板。
   */
  async getOpenOrders(
    traderId: string,
    symbol?: string,
    silent?: boolean
  ): Promise<ExchangeOpenOrder[]> {
    const q = new URLSearchParams({ trader_id: traderId })
    if (symbol && symbol.trim()) {
      q.set('symbol', symbol.trim())
    }
    const result = await httpClient.request<ExchangeOpenOrder[]>(
      `${API_BASE}/open-orders?${q.toString()}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch open orders')
    return Array.isArray(result.data) ? result.data : []
  },

  async getDecisions(traderId?: string): Promise<DecisionRecord[]> {
    const url = traderId
      ? `${API_BASE}/decisions?trader_id=${traderId}`
      : `${API_BASE}/decisions`
    const result = await httpClient.get<DecisionRecord[]>(url)
    if (!result.success) throw new Error('Failed to fetch decision logs')
    return result.data!
  },

  async getLatestDecisions(
    traderId?: string,
    limit: number = 5,
    silent?: boolean
  ): Promise<DecisionRecord[]> {
    const params = new URLSearchParams()
    if (traderId) {
      params.append('trader_id', traderId)
    }
    params.append('limit', limit.toString())

    const result = await httpClient.request<DecisionRecord[]>(
      `${API_BASE}/decisions/latest?${params}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch latest decisions')
    return result.data!
  },

  async getMarketBoard(opts?: { silent?: boolean; quick?: boolean }): Promise<MarketBoardPayload> {
    const silent = opts?.silent ?? true
    const quick = opts?.quick ?? false
    const qs = quick ? '?quick=1' : ''
    const result = await httpClient.request<MarketBoardPayload>(`${API_BASE}/market/board${qs}`, {
      method: 'GET',
      silent,
    })
    if (!result.success || !result.data) throw new Error('获取行情看板失败')
    return result.data
  },

  async getCryptoNews(silent?: boolean): Promise<CryptoNewsPayload> {
    const result = await httpClient.request<CryptoNewsPayload>(`${API_BASE}/news/crypto`, {
      method: 'GET',
      silent: silent ?? true,
    })
    if (!result.success || !result.data) throw new Error('Failed to fetch crypto news')
    return result.data
  },

  async getStatistics(traderId?: string, silent?: boolean): Promise<Statistics> {
    const url = traderId
      ? `${API_BASE}/statistics?trader_id=${traderId}`
      : `${API_BASE}/statistics`
    const result = await httpClient.request<Statistics>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch statistics')
    return result.data!
  },

  async getEquityHistory(traderId?: string, silent?: boolean): Promise<any[]> {
    const url = traderId
      ? `${API_BASE}/equity-history?trader_id=${traderId}&limit=300`
      : `${API_BASE}/equity-history?limit=300`
    const result = await httpClient.request<any[]>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch equity history')
    return result.data!
  },

  async getEquityHistoryBatch(traderIds: string[], hours?: number): Promise<any> {
    const result = await httpClient.post<any>(
      `${API_BASE}/equity-history-batch`,
      { trader_ids: traderIds, hours: hours || 0 }
    )
    if (!result.success) throw new Error('Failed to fetch batch equity history')
    return result.data!
  },

  async getTopTraders(): Promise<any[]> {
    const result = await httpClient.get<any[]>(`${API_BASE}/top-traders`)
    if (!result.success) throw new Error('Failed to fetch top traders')
    return result.data!
  },

  async getPublicTraderConfig(traderId: string): Promise<any> {
    const result = await httpClient.get<any>(
      `${API_BASE}/trader/${traderId}/config`
    )
    if (!result.success) throw new Error('Failed to fetch public trader config')
    return result.data!
  },

  async getCompetition(): Promise<CompetitionData> {
    const result = await httpClient.get<CompetitionData>(
      `${API_BASE}/competition`
    )
    if (!result.success) throw new Error('Failed to fetch competition data')
    return result.data!
  },

  async getPositionHistory(
    traderId: string,
    limit: number = 100,
    silent?: boolean
  ): Promise<PositionHistoryResponse> {
    const result = await httpClient.request<PositionHistoryResponse>(
      `${API_BASE}/positions/history?trader_id=${traderId}&limit=${limit}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch position history')
    return result.data!
  },
}
