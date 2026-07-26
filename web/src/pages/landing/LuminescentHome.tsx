import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../../contexts/AuthContext'
import {
  fetchHomeMarketPageData,
  type HomeAssetCard,
  type HomeTickerItem,
} from '../../lib/binanceHomeMarkets'
import { ROUTES } from '../../router/paths'
import './static-home.css'

const FEATURE_ITEMS: { title: string; body: string }[] = [
  {
    title: '极速交易',
    body: '自研撮合引擎，毫秒级订单执行。支持高并发场景，保障交易体验始终流畅。',
  },
  {
    title: '机构级流动性',
    body: '接入通常仅供一级基金使用的深度流动性池。交易数百万资金而不影响市场。',
  },
  {
    title: '多链支持',
    body: '覆盖 Bitcoin、Ethereum、Solana 等主流公链，一站式管理所有链上资产。',
  },
  {
    title: '专业数据',
    body: '实时 K 线、深度图、链上数据聚合。为专业交易者提供决策所需的全维度信息。',
  },
  {
    title: 'AI 策略',
    body: 'COMKUN-AI 智能分析市场趋势，提供量化策略建议与自动化交易工具。',
  },
  {
    title: '24/7 全天候监控',
    body: '持续的自主监管。我们的代理从不休息，确保您的策略在所有交易时段保持最优。',
  },
]

/** 与 /root/index/crypto-home.html 结构、样式 1:1；行情数据走币安公共接口 */
export function LuminescentHome() {
  const navigate = useNavigate()
  const { user } = useAuth()
  const [homeAssets, setHomeAssets] = useState<HomeAssetCard[]>([])
  const [tickerItems, setTickerItems] = useState<HomeTickerItem[]>([])
  const [assetsErr, setAssetsErr] = useState<string | null>(null)

  const nodes = useMemo(
    () =>
      Array.from({ length: 22 }, () => ({
        left: `${3 + Math.random() * 94}%`,
        top: `${3 + Math.random() * 94}%`,
        delay: `${Math.random() * 3}s`,
        duration: `${2 + Math.random() * 4}s`,
      })),
    []
  )

  useEffect(() => {
    document.title = 'COMKUN-AI · 智能交易平台'
  }, [])

  const loadMarket = useCallback(async () => {
    try {
      setAssetsErr(null)
      const { cards, ticker } = await fetchHomeMarketPageData()
      setHomeAssets(cards)
      setTickerItems(ticker)
    } catch (e) {
      setHomeAssets([])
      setTickerItems([])
      setAssetsErr(e instanceof Error ? e.message : '加载失败')
    }
  }, [])

  useEffect(() => {
    void loadMarket()
    const id = window.setInterval(() => void loadMarket(), 20_000)
    return () => window.clearInterval(id)
  }, [loadMarket])

  const goStrategy = () => {
    if (!user) {
      navigate(ROUTES.login)
      return
    }
    navigate(ROUTES.strategy)
  }

  const showAssetSkeleton = !assetsErr && homeAssets.length === 0

  const tickerScroll =
    tickerItems.length > 0 ? [...tickerItems, ...tickerItems] : []

  return (
    <div className="static-home-content min-h-0">
      <div className="grid-bg" aria-hidden />
      <div className="scanline" aria-hidden />
      <div className="nodes-layer" aria-hidden>
        {nodes.map((n, i) => (
          <div
            key={i}
            className="node"
            style={{
              left: n.left,
              top: n.top,
              animationDelay: n.delay,
              animationDuration: n.duration,
            }}
          />
        ))}
      </div>

      <section className="hero">
        <div className="hero-grid">
          <div>
            <div className="hero-badge">
              <span className="dot" /> 实时行情 · 极速交易
            </div>
            <h1>
              智能加密资产
              <br />
              <span className="hl">交易与管理平台</span>
            </h1>
            <p className="hero-desc">
              毫秒级交易执行，多层安全架构。支持主流公链资产，一站式管理你的数字财富。由
              COMKUN-AI 驱动的新一代交易体验。
            </p>
            <div className="hero-btns">
              <Link to={ROUTES.register} className="sh-btn">
                <span>创建账户 →</span>
              </Link>
              <button type="button" className="sh-btn ghost" onClick={goStrategy}>
                <span>探索实验室</span>
              </button>
            </div>
          </div>
          <div className="hero-stats">
            <div className="stat-card">
              <div className="stat-label">24h 交易量</div>
              <div className="stat-value">
                $302,847<span className="small">.62</span>
              </div>
              <div className="stat-change up">+12.4%</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">活跃用户</div>
              <div className="stat-value">
                186<span className="small">个</span>
              </div>
              <div className="stat-change up">+8.7%</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">支持币种</div>
              <div className="stat-value">
                1,200<span className="small">+</span>
              </div>
              <div className="stat-change up">+32</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">累计交易额</div>
              <div className="stat-value">
                $128<span className="small">B</span>
              </div>
              <div className="stat-change up">+18.2%</div>
            </div>
          </div>
        </div>
      </section>

      <div className="ticker-bar">
        <div
          className="ticker-inner"
          style={tickerScroll.length === 0 ? { animation: 'none' } : undefined}
        >
          {tickerScroll.length > 0 ? (
            tickerScroll.map((c, i) => (
              <span key={`${c.sym}-${i}`} className="ticker-item">
                <span className="ticker-sym">{c.sym}</span> ${c.price}{' '}
                <span className={c.dir}>{c.chg}%</span>
              </span>
            ))
          ) : (
            <span className="ticker-item">加载行情…</span>
          )}
        </div>
      </div>

      <section className="section">
        <div className="section-header">
          <div className="section-title">
            <span className="hl">#</span> 热门资产
          </div>
          <Link to={ROUTES.data} className="section-link">
            查看更多 →
          </Link>
        </div>
        {assetsErr ? (
          <p style={{ color: '#f87171', fontSize: 13, marginBottom: 16 }}>
            行情暂不可用（{assetsErr}）
          </p>
        ) : null}
        <div className="asset-grid">
          {(showAssetSkeleton ? Array.from({ length: 8 }, (_, i) => i) : homeAssets).map(
            (item, idx) => {
              if (typeof item === 'number') {
                return (
                  <div key={`sk-${idx}`} className="asset-card">
                    <div className="asset-top">
                      <div
                        className="asset-icon gen"
                        style={{ opacity: 0.35 }}
                      />
                      <div>
                        <div className="asset-sym">···</div>
                        <div className="asset-name">加载中</div>
                      </div>
                    </div>
                    <div className="asset-price">—</div>
                    <div className="asset-chg up">—</div>
                  </div>
                )
              }
              const a = item
              return (
                <div key={`${a.sym}-${idx}`} className="asset-card">
                  <div className="asset-top">
                    <div className={`asset-icon ${a.accent}`}>{a.icon}</div>
                    <div>
                      <div className="asset-sym">{a.sym}</div>
                      <div className="asset-name">{a.name}</div>
                    </div>
                  </div>
                  <div className="asset-price">{a.price}</div>
                  <div className={`asset-chg ${a.dir}`}>{a.chg}</div>
                </div>
              )
            }
          )}
        </div>
      </section>

      <section className="section">
        <div className="section-header">
          <div className="section-title">
            <span className="hl">#</span> 平台优势
          </div>
        </div>
        <div className="feature-grid">
          {FEATURE_ITEMS.map((f) => (
            <div key={f.title} className="feature-card">
              <div className="feature-icon">
                <div className="orbit" />
                <div className="orbit" />
                <div className="glow" />
              </div>
              <h3>{f.title}</h3>
              <p>{f.body}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="cta-section">
        <div className="cta-box">
          <h2>
            加入 <span className="hl">COMKUN-AI</span>
          </h2>
          <div className="cta-btns">
            <Link to={ROUTES.register} className="sh-btn">
              <span>立即注册 →</span>
            </Link>
            <Link to={ROUTES.login} className="sh-btn ghost">
              <span>已有账号？登录</span>
            </Link>
          </div>
          <div className="terminal-note">
            {'>'} COMKUN-AI-交易系统-就绪<span className="blink" />
          </div>
        </div>
      </section>

      <div className="footer">
        © {new Date().getFullYear()}{' '}
        <Link to={ROUTES.home}>COMKUN-AI</Link> · 去中心化智能交易平台 ·{' '}
        <a href="#">服务协议</a> · <a href="#">隐私政策</a>
      </div>
    </div>
  )
}
