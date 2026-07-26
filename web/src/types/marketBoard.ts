export interface MarketQuote {
  symbol: string
  name: string
  category: string
  price: number
  change_pct: number
  volume?: number
  extra?: string
  up: boolean
  source?: string
}

export interface MarketTableRow {
  rank: number
  symbol: string
  name: string
  price: number
  change_pct: number
  volume?: number
  volume_str: string
  signal: string
  up: boolean
}

export interface MarketCardBlock {
  id: string
  title_zh: string
  title_en: string
  items: MarketQuote[]
}

export interface MarketMetric {
  id: string
  label_zh: string
  label_en: string
  value: string
  detail_zh?: string
  detail_en?: string
  tone?: 'up' | 'down' | 'neutral' | 'warn' | string
}

export interface MarketSourceStatus {
  name: string
  status: 'ok' | 'error' | string
  detail?: string
}

export interface MarketBoardPayload {
  updated_at: string
  ticker: MarketQuote[]
  cards: MarketCardBlock[]
  metrics?: MarketMetric[]
  sources?: MarketSourceStatus[]
  table_rows: Record<string, MarketTableRow[]>
  disclaimer: string
}
