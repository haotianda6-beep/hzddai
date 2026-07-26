import { useLanguage } from '../contexts/LanguageContext'
import { MarketBoardView } from './data/MarketBoardView'

/** 行情数据看板：后端 /api/market/board 聚合当前可免费访问的真实公开源 */
export function DataPage() {
  const { language } = useLanguage()
  return <MarketBoardView language={language} />
}
