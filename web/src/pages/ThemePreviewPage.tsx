import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import gsap from 'gsap'
import { ScrollTrigger } from 'gsap/ScrollTrigger'
import {
  BarChart3, Bell, Bot, ChevronDown, CreditCard, Database, FileQuestion,
  Home, LayoutDashboard, Newspaper, Search, Settings, Shield, Sparkles, Store,
  User, Users, WalletCards,
} from 'lucide-react'
import './theme-preview.css'

gsap.registerPlugin(ScrollTrigger)

type PageKey =
  | 'home' | 'auth' | 'market' | 'detail' | 'data' | 'news' | 'faq' | 'traders'
  | 'dashboard' | 'studio' | 'profile' | 'settings' | 'invite' | 'recharge'
  | 'admin' | 'finance'

type PageItem = { key: PageKey; title: string; path: string; icon: typeof Home }

const pages: PageItem[] = [
  { key: 'home', title: '首页', path: '/', icon: Home },
  { key: 'auth', title: '登录注册', path: '/login', icon: User },
  { key: 'market', title: '策略市场', path: '/strategy-market', icon: Store },
  { key: 'detail', title: '策略详情', path: '/strategy-market/:id', icon: BarChart3 },
  { key: 'data', title: '数据看板', path: '/data', icon: Database },
  { key: 'news', title: '币圈新闻', path: '/news', icon: Newspaper },
  { key: 'faq', title: 'FAQ', path: '/faq', icon: FileQuestion },
  { key: 'traders', title: '交易员配置', path: '/traders', icon: Bot },
  { key: 'dashboard', title: '交易员看板', path: '/dashboard', icon: LayoutDashboard },
  { key: 'studio', title: '策略编辑器', path: '/strategy', icon: Sparkles },
  { key: 'profile', title: '个人中心', path: '/profile', icon: WalletCards },
  { key: 'settings', title: '设置', path: '/settings', icon: Settings },
  { key: 'invite', title: '邀请奖励', path: '/invite', icon: Users },
  { key: 'recharge', title: '充值', path: '/recharge', icon: CreditCard },
  { key: 'admin', title: '管理后台', path: '/admin', icon: Shield },
  { key: 'finance', title: '财务台', path: '/finance', icon: WalletCards },
]

const strategies = ['只做主流币,冷静处理行情', 'OKX策略-稳中求胜', '全智能操作,解放双手']
const trades = ['ETHUSDT 做多 +988.02', 'BTCUSDT 做空 +296.70', 'WLDUSDT 做多 -148.60', 'SOLUSDT 做空 +75.41']
const coins = ['BTCUSDT', 'ETHUSDT', 'SOLUSDT', 'WLDUSDT', 'ZECUSDT', 'HYPEUSDT']

function StaticHeader() {
  return (
    <header className="tp-header">
      <div className="tp-brand"><img src="/icons/comkun-whale-logo.png" alt="" />COMKUN-AI</div>
      <nav>
        <span>实验室 <ChevronDown size={13} /></span>
        <span>数据看板 <ChevronDown size={13} /></span>
        <span>AI交易员配置 <ChevronDown size={13} /></span>
      </nav>
      <div className="tp-header-actions">
        <button className="tp-pill">邀请奖励</button>
        <button className="tp-pill">余额 1000.0000 USDT</button>
        <Bell size={17} />
        <button className="tp-user">红中 V5</button>
      </div>
    </header>
  )
}

function Metric({ label, value, tone = 'green' }: { label: string; value: string; tone?: 'green' | 'violet' | 'red' }) {
  return <div className={`tp-metric tp-${tone}`}><span>{label}</span><strong>{value}</strong></div>
}

function ChartPanel() {
  return <div className="tp-chart">{[28, 36, 33, 55, 51, 68, 63, 84, 78, 92].map((h, i) => <i key={i} style={{ height: `${h}%` }} />)}</div>
}

function StrategyCard({ name, i }: { name: string; i: number }) {
  return (
    <article className="tp-strategy-card">
      <div className="tp-card-head"><span className="tp-avatar">{name[0]}</span><div><h3>{name}</h3><p>青衫量化 · BINANCE · 20x</p></div><b>V{(1.1 + i / 10).toFixed(1)}</b></div>
      <ChartPanel />
      <div className="tp-card-stats"><Metric label="收益率" value={i === 1 ? '+64.38%' : '+91.54%'} /><Metric label="最大回撤" value="-3.86%" tone="red" /><Metric label="订阅数" value={String(26 + i * 8)} tone="violet" /></div>
    </article>
  )
}

function TradeTable() {
  return <div className="tp-table">{trades.map((t, i) => <div key={t}><span>{t.split(' ')[0]}</span><span>{t.split(' ')[1]}</span><span>2026-05-{28 - i} 22:0{i}</span><b className={t.includes('-') ? 'tp-loss' : ''}>{t.split(' ').slice(2).join(' ')} USDT</b></div>)}</div>
}

function renderHome() {
  return <><section className="tp-hero-page"><div><p className="tp-kicker">AI QUANT PLATFORM</p><h1>COMKUN-AI<br />新一代量化交易实验室</h1><p>首页按原站首屏逻辑保留品牌、导航、CTA、数据卡和产品流程，只切换为藤紫绿与钛啡紫体系。</p><div className="tp-cta"><button>进入策略市场</button><button>配置AI交易员</button></div></div><div className="tp-hero-board"><ChartPanel /><div className="tp-card-stats"><Metric label="运行策略" value="43" /><Metric label="管理资金" value="769,054" tone="violet" /></div></div></section><Section title="核心流程"><StepGrid /></Section></>
}

function renderAuth() {
  return <section className="tp-auth"><div><h1>欢迎回来</h1><p>保持原登录/注册页的分栏和安全感，按钮、输入框、提示文案统一换新色。</p><ChartPanel /></div><form><label>邮箱<input defaultValue="user@example.com" /></label><label>密码<input defaultValue="••••••••" /></label><button type="button">登录 COMKUN-AI</button><p>还没有账号？使用邀请码注册</p></form></section>
}

function renderMarket() {
  return <><Toolbar title="策略市场" search="搜索策略 / 交易所 / 作者" /><div className="tp-market-grid">{strategies.map((s, i) => <StrategyCard key={s} name={s} i={i} />)}</div></>
}

function renderDetail() {
  return <><div className="tp-detail-hero"><button>← 返回策略市场</button><h1>只做主流币,冷静处理行情</h1><p>红中 · 运行中 Agent: 1 · 更新 2026/05/28</p><button className="tp-subscribe">订阅</button></div><section className="tp-dashboard-grid"><div className="tp-panel tp-wide"><h3>运行数据</h3><ChartPanel /><div className="tp-card-stats"><Metric label="总盈亏" value="+91.54%" /><Metric label="胜率" value="100%" /><Metric label="盈亏比" value="999" tone="violet" /></div></div><div className="tp-panel"><h3>多空比</h3><Donut /></div></section><Section title="做单历史"><TradeTable /></Section></>
}

function renderDataNewsFaq(key: PageKey) {
  const title = key === 'data' ? '行情数据看板' : key === 'news' ? '币圈新闻' : '帮助中心'
  return <><Toolbar title={title} search={key === 'faq' ? '搜索问题' : '搜索 BTC / ETH / SOL'} /><section className="tp-dashboard-grid">{[0, 1, 2, 3].map((i) => <div className="tp-panel" key={i}><h3>{key === 'news' ? ['快讯', '宏观', '交易所', '链上'][i] : key === 'faq' ? ['新手入门', '策略订阅', '账户安全', '费用说明'][i] : coins[i]}</h3><ChartPanel /><p>{key === 'news' ? '市场波动加剧，AI 策略维持低频观察。' : key === 'faq' ? '常见问题以折叠面板展示，保留原帮助中心阅读节奏。' : '24h +3.26% · 成交量 2.8B'}</p></div>)}</section></>
}

function renderTraders() {
  return <><Toolbar title="AI交易员配置" search="选择交易所 / 模型 / 风控" /><div className="tp-wizard">{['连接交易所', '选择模型', '风控参数', '确认部署'].map((s, i) => <div className="tp-step" key={s}><b>{i + 1}</b><h3>{s}</h3><p>沿用原四步向导，控件和状态色替换为新主题。</p></div>)}</div></>
}

function renderDashboard() {
  return <><Toolbar title="AI策略数据看板" search="当前交易员：红中主控" /><section className="tp-dashboard-grid"><aside className="tp-panel"><h3>交易员列表</h3>{strategies.map((s) => <p key={s} className="tp-list-row">{s}<b>运行中</b></p>)}</aside><div className="tp-panel tp-wide"><h3>账户权益</h3><ChartPanel /><div className="tp-card-stats"><Metric label="余额" value="1000.00" /><Metric label="持仓" value="2" tone="violet" /><Metric label="今日收益" value="+643.82" /></div></div><div className="tp-panel tp-wide"><h3>当前仓位</h3><TradeTable /></div></section></>
}

function renderStudio() {
  return <><Toolbar title="策略编辑器" search="策略名称 / 提示词 / 指标" /><section className="tp-studio"><aside>{['基础信息', '币种来源', '指标条件', '风控设置', '发布设置'].map((s) => <button key={s}>{s}</button>)}</aside><main><h3>Prompt Sections</h3><div className="tp-code">你是 COMKUN-AI 策略引擎。只在趋势确认后开仓，控制回撤，避免频繁交易。</div><div className="tp-card-stats"><Metric label="Token" value="2,816" /><Metric label="风险等级" value="中" tone="violet" /></div></main></section></>
}

function renderOps(key: PageKey) {
  const titles: Record<string, string> = { profile: '个人中心', settings: '设置', invite: '邀请奖励', recharge: '余额充值', admin: '管理后台', finance: '财务台' }
  return <><Toolbar title={titles[key]} search="搜索 / 筛选 / 操作" /><section className="tp-dashboard-grid"><div className="tp-panel"><h3>概览</h3><Metric label="状态" value="正常" /><Metric label="余额" value="1000 USDT" tone="violet" /></div><div className="tp-panel tp-wide"><h3>记录</h3><TradeTable /></div><div className="tp-panel"><h3>操作区</h3>{['复制链接', '更新配置', '提交审核'].map((x) => <button className="tp-action" key={x}>{x}</button>)}</div></section></>
}

function Toolbar({ title, search }: { title: string; search: string }) {
  return <div className="tp-toolbar"><div><p className="tp-kicker">STATIC REPLICA</p><h1>{title}</h1></div><label><Search size={16} /><input defaultValue={search} /></label></div>
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return <section className="tp-section"><h2>{title}</h2>{children}</section>
}

function StepGrid() {
  return <div className="tp-wizard">{['订阅策略', 'AI 解析信号', '风控执行', '数据复盘'].map((s, i) => <div className="tp-step" key={s}><b>{String(i + 1).padStart(2, '0')}</b><h3>{s}</h3><p>保留原产品流程模块，换成更有品牌识别度的紫绿视觉。</p></div>)}</div>
}

function Donut() {
  return <div className="tp-donut"><span>多 66%</span></div>
}

function PageBody({ active }: { active: PageKey }) {
  if (active === 'home') return renderHome()
  if (active === 'auth') return renderAuth()
  if (active === 'market') return renderMarket()
  if (active === 'detail') return renderDetail()
  if (active === 'data' || active === 'news' || active === 'faq') return renderDataNewsFaq(active)
  if (active === 'traders') return renderTraders()
  if (active === 'dashboard') return renderDashboard()
  if (active === 'studio') return renderStudio()
  return renderOps(active)
}

export function ThemePreviewPage() {
  const rootRef = useRef<HTMLDivElement>(null)
  const [active, setActive] = useState<PageKey>('market')
  const activePage = useMemo(() => pages.find((p) => p.key === active)!, [active])

  useLayoutEffect(() => {
    const root = rootRef.current
    if (!root) return
    const ctx = gsap.context(() => {
      gsap.fromTo('[data-preview-stage]', { opacity: 0, y: 20 }, { opacity: 1, y: 0, duration: 0.55, ease: 'power3.out' })
      gsap.fromTo('.tp-panel,.tp-strategy-card,.tp-step,.tp-auth form,.tp-hero-board', { opacity: 0, y: 28 }, { opacity: 1, y: 0, stagger: 0.045, duration: 0.62, ease: 'power3.out', delay: 0.08 })
      gsap.to('.tp-chart i', { scaleY: 0.72, transformOrigin: 'bottom', repeat: -1, yoyo: true, stagger: 0.06, duration: 1.2, ease: 'sine.inOut' })
    }, root)
    return () => ctx.revert()
  }, [active])

  return (
    <div ref={rootRef} className="theme-preview-shell">
      <aside className="tp-preview-rail">
        <div className="tp-preview-mark">预览</div>
        {pages.map((page) => {
          const Icon = page.icon
          return <button key={page.key} className={active === page.key ? 'active' : ''} onClick={() => setActive(page.key)}><Icon size={16} /><span>{page.title}</span></button>
        })}
      </aside>
      <div className="tp-app-replica" data-preview-stage>
        <StaticHeader />
        <div className="tp-pathbar"><span>{activePage.path}</span><b>1:1 静态复刻预览 · 新配色</b></div>
        <main><PageBody active={active} /></main>
        <footer>COMKUN-AI · AI 量化 · 交易有风险，请谨慎使用。</footer>
      </div>
    </div>
  )
}
