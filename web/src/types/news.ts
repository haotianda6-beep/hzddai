export interface GoldImpact {
  direction: 'bullish' | 'bearish' | 'neutral'
  risk_score: number
  risk_level: 'none' | 'low' | 'medium' | 'high' | 'extreme'
  horizon: string
  confirmation: 'pending' | 'confirmed' | 'divergent' | 'not_required'
  reason: string
  drivers: string[]
}

export interface CryptoNewsItem {
  id: string
  title: string
  title_zh?: string
  source: string
  url: string
  summary: string
  summary_zh?: string
  published_at: string
  tags: string[]
  importance: 'normal' | 'risk' | string
  gold_impact?: GoldImpact
}

export interface CryptoNewsSourceStatus {
  name: string
  status: 'ok' | 'error' | string
  detail?: string
}

export interface GoldMarketQuote {
  symbol: string
  price: number
  change_5m?: number
  change_15m?: number
  change_60m?: number
}

export interface Treasury10YQuote {
  status: 'daily' | 'offline' | string
  date?: string
  value?: number
  change_bp?: number
  source: string
}

export interface GoldMarketStatus {
  status: 'live' | 'warming_up' | 'offline'
  updated_at?: string
  xau?: GoldMarketQuote
  dollar?: GoldMarketQuote
  us10y: Treasury10YQuote
}

export interface CryptoNewsPayload {
  updated_at: string
  items: CryptoNewsItem[]
  sources: CryptoNewsSourceStatus[]
  gold_market?: GoldMarketStatus
}
