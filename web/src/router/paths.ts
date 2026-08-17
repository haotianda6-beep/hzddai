export type Page =
  | 'competition'
  | 'traders'
  | 'trader'
  | 'strategy'
  | 'strategy-market'
  | 'auto-arbitrage'
  | 'wallet'
  | 'data'
  | 'news'
  | 'faq'
  | 'login'
  | 'register'

export const ROUTES = {
  home: '/',
  login: '/login',
  register: '/register',
  setup: '/setup',
  welcome: '/welcome',
  faq: '/faq',
  resetPassword: '/reset-password',
  settings: '/settings',
  profile: '/profile',
  /** 主站邀请奖励（邀请码 / 链接 / 下级列表，GET /api/invite/me） */
  invite: '/invite',
  data: '/data',
  news: '/news',
  competition: '/competition',
  /** AI 交易员：四步部署向导（原「交易员配置」页已合并至此） */
  traders: '/traders',
  dashboard: '/dashboard',
  strategy: '/strategy',
  strategyMarket: '/strategy-market',
  autoArbitrage: '/auto-arbitrage',
  wallet: '/wallet',
  /** 主站管理后台（仅白名单邮箱；邀请用户列表等）。邀请「返佣」独立服务见 agent_rebate_backend。 */
  admin: '/admin',
  adminPartners: '/admin/partners',
  adminTraders: '/admin/traders',
  adminProxies: '/admin/proxies',
  adminUsers: '/admin/users',
  adminAIBilling: '/admin/ai-billing',
  adminTeam: '/admin/team',
  adminFollowingStats: '/admin/following-stats',
  /** 财务台：为客户正数入账（COMKUN_FINANCE_EMAILS 白名单） */
  finance: '/finance',
  /** 站内余额充值说明（人工客服） */
  recharge: '/recharge',
  /** 新配色静态全站预览，不进入上线导航 */
  themePreview: '/theme-preview',
  /** 历史旧入口：官方 API Token 已废弃，统一跳转站内余额充值 */
  comkunOfficialToken: '/comkun-official-token',
} as const

export const PAGE_PATHS: Record<Page, string> = {
  competition: ROUTES.competition,
  traders: ROUTES.traders,
  trader: ROUTES.dashboard,
  strategy: ROUTES.strategy,
  'strategy-market': ROUTES.strategyMarket,
  'auto-arbitrage': ROUTES.autoArbitrage,
  wallet: ROUTES.wallet,
  data: ROUTES.data,
  news: ROUTES.news,
  faq: ROUTES.faq,
  login: ROUTES.login,
  register: ROUTES.register,
}

export const LEGACY_HASH_ROUTES: Record<string, string> = {
  competition: ROUTES.competition,
  traders: ROUTES.traders,
  trader: ROUTES.dashboard,
  details: ROUTES.dashboard,
  strategy: ROUTES.strategy,
  'strategy-market': ROUTES.strategyMarket,
  'auto-arbitrage': ROUTES.autoArbitrage,
  data: ROUTES.data,
  news: ROUTES.news,
}

/** 从主导航进入时带此参数，向导从第 1 步重新开始并清空草稿 */
export const TRADERS_WIZARD_ENTRY = `${ROUTES.traders}?fresh=1` as const

/** 策略市场详情页路径（列表项点击进入） */
export function strategyMarketDetailPath(strategyId: string): string {
  return `${ROUTES.strategyMarket}/${encodeURIComponent(strategyId)}`
}

export function getCurrentPageForPath(pathname: string): Page | undefined {
  if (pathname.startsWith(`${ROUTES.strategyMarket}/`)) {
    return 'strategy-market'
  }
  switch (pathname) {
    case ROUTES.welcome:
    case ROUTES.traders:
      return 'traders'
    case ROUTES.dashboard:
      return 'trader'
    case ROUTES.strategy:
      return 'strategy'
    case ROUTES.strategyMarket:
      return 'strategy-market'
    case ROUTES.autoArbitrage:
      return 'auto-arbitrage'
    case ROUTES.wallet:
      return 'wallet'
    case ROUTES.data:
      return 'data'
    case ROUTES.news:
      return 'news'
    case ROUTES.faq:
      return 'faq'
    case ROUTES.login:
      return 'login'
    case ROUTES.register:
      return 'register'
    case ROUTES.competition:
      return 'competition'
    default:
      return undefined
  }
}

export function buildDashboardPath(traderSlug?: string): string {
  if (!traderSlug) {
    return ROUTES.dashboard
  }

  return `${ROUTES.dashboard}?trader=${encodeURIComponent(traderSlug)}`
}
