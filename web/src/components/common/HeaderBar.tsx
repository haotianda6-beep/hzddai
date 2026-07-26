import { useState, useEffect, useRef } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'
import useSWR from 'swr'
import { Menu, X, ChevronDown, Gift, Wallet as WalletIcon } from 'lucide-react'
import { AiTradeNotificationDropdown } from './AiTradeNotificationDropdown'
import { VipTierBadge } from './VipTierBadge'
import { api } from '../../lib/api'
import { t, type Language } from '../../i18n/translations'
import {
  getCurrentPageForPath,
  ROUTES,
  TRADERS_WIZARD_ENTRY,
  type Page,
} from '../../router/paths'
import '../../pages/landing/luminescent.css'
import '../../pages/landing/static-home.css'

interface HeaderBarProps {
  onLoginClick?: () => void
  isLoggedIn?: boolean
  isHomePage?: boolean
  currentPage?: Page
  language?: Language
  onLanguageChange?: (lang: Language) => void
  user?: {
    email: string
    display_name?: string
    avatar_url?: string
    /** 策略市场站内余额（USDT） */
    balance_usdt?: number
    is_admin?: boolean
    is_finance?: boolean
  } | null
  onLogout?: () => void
  onPageChange?: (page: Page) => void
  onLoginRequired?: (featureName: string) => void
}

const lang: Language = 'zh'

export default function HeaderBar({
  isLoggedIn = false,
  isHomePage = false,
  currentPage,
  user,
  onLogout,
  onPageChange,
  onLoginRequired,
}: HeaderBarProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [userDropdownOpen, setUserDropdownOpen] = useState(false)
  const userDropdownRef = useRef<HTMLDivElement>(null)
  const resolvedCurrentPage = currentPage ?? getCurrentPageForPath(location.pathname)

  const { data: rebateBal } = useSWR(
    isLoggedIn && user ? 'header-agent-rebate-vip' : null,
    () => api.getAgentRebateBalance(),
    { refreshInterval: 120_000, revalidateOnFocus: true }
  )

  const vipLevel =
    rebateBal &&
    rebateBal.configured &&
    rebateBal.synced &&
    typeof rebateBal.rebate_vip_level === 'number'
      ? rebateBal.rebate_vip_level
      : undefined

  const userDisplayName =
    (user?.display_name && user.display_name.trim()) ||
    user?.email?.split('@')[0] ||
    ''
  const walletBalance = user?.balance_usdt ?? 0
  const walletBalanceClass = walletBalance < 0 ? 'text-red-400' : 'text-[#d4ff33]'

  const navigateInApp = (path: string) => {
    navigate(path)
  }

  const tryNav = (path: string, page: Page, requiresAuth: boolean, featureLabel: string) => {
    if (requiresAuth && !isLoggedIn) {
      onLoginRequired?.(featureLabel)
      return
    }
    onPageChange?.(page)
    navigateInApp(path)
    setMobileMenuOpen(false)
  }

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (
        userDropdownRef.current &&
        !userDropdownRef.current.contains(event.target as Node)
      ) {
        setUserDropdownOpen(false)
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [])

  const marketActive =
    resolvedCurrentPage === 'strategy-market' ||
    resolvedCurrentPage === 'auto-arbitrage'
  const boardActive =
    resolvedCurrentPage === 'trader' || resolvedCurrentPage === 'data' || resolvedCurrentPage === 'news'
  const tradersActive = resolvedCurrentPage === 'traders' || resolvedCurrentPage === 'strategy'

  return (
    <header className="static-home-header z-50 font-lumbody">
      <nav className="static-home-nav-inner flex items-center justify-between">
        <Link to={ROUTES.home} className="sh-nav-brand">
          <img
            src="/icons/comkun-whale-logo.png"
            alt=""
            className="h-[30px] w-[30px] shrink-0 rounded-lg object-cover"
            width={30}
            height={30}
            aria-hidden
          />
          <span className="sh-nav-name hidden lg:inline">COMKUN-AI</span>
        </Link>

        <div className="hidden items-center lg:flex sh-nav-center">
          <div className="sh-nav-item">
            <Link
              to={ROUTES.strategyMarket}
              className={marketActive ? '!text-[#d4ff33]' : undefined}
            >
              策略市场
              <ChevronDown className="h-3.5 w-3.5 shrink-0 opacity-80" aria-hidden strokeWidth={2} />
            </Link>
            <div className="sh-nav-dropdown">
              <button
                type="button"
                onClick={() =>
                  tryNav(ROUTES.strategyMarket, 'strategy-market', true, '策略市场')
                }
              >
                策略市场
              </button>
              <button
                type="button"
                onClick={() =>
                  tryNav(ROUTES.autoArbitrage, 'auto-arbitrage', true, '全自动套利系统')
                }
              >
                全自动套利系统
              </button>
            </div>
          </div>

          <div className="sh-nav-item">
            <Link to={ROUTES.data} className={boardActive ? '!text-[#d4ff33]' : undefined}>
              数据看板
              <ChevronDown className="h-3.5 w-3.5 shrink-0 opacity-80" aria-hidden strokeWidth={2} />
            </Link>
            <div className="sh-nav-dropdown">
              <button
                type="button"
                onClick={() =>
                  tryNav(ROUTES.dashboard, 'trader', true, t('dashboardNav', lang))
                }
              >
                AI策略数据看板
              </button>
              <button type="button" onClick={() => tryNav(ROUTES.data, 'data', false, '数据')}>
                行情数据
              </button>
              <button
                type="button"
                onClick={() => tryNav(ROUTES.news, 'news', true, '新闻信息监控')}
              >
                新闻信息监控
              </button>
            </div>
          </div>

          <div className="sh-nav-item">
            <Link to={ROUTES.traders} className={tradersActive ? '!text-[#d4ff33]' : undefined}>
              AI交易员配置
              <ChevronDown className="h-3.5 w-3.5 shrink-0 opacity-80" aria-hidden strokeWidth={2} />
            </Link>
            <div className="sh-nav-dropdown">
              <button
                type="button"
                onClick={() => tryNav(TRADERS_WIZARD_ENTRY, 'traders', true, t('configNav', lang))}
              >
                AI交易员配置
              </button>
              <button
                type="button"
                onClick={() => tryNav(ROUTES.strategy, 'strategy', true, '策略构建器')}
              >
                策略构建器
              </button>
              <button
                type="button"
                onClick={() => {
                  if (!isLoggedIn) {
                    onLoginRequired?.('AI模型配置')
                    return
                  }
                  navigateInApp(ROUTES.settings)
                }}
              >
                AI模型配置
              </button>
            </div>
          </div>
        </div>

        <div className="sh-nav-right flex min-w-0 shrink items-center justify-end gap-1.5 sm:gap-3">
          {isLoggedIn && user && (
            <Link
              to={ROUTES.invite}
              title="邀请奖励 · 复制链接与下级列表"
              className="hidden items-center gap-1.5 rounded-full border border-[#d4ff33]/25 bg-[#d4ff33]/10 px-3 py-1.5 text-xs font-bold text-[#d4ff33] transition-colors hover:border-[#d4ff33]/45 hover:bg-[#d4ff33]/15 xl:inline-flex"
            >
              <Gift className="h-3.5 w-3.5" />
              邀请奖励
            </Link>
          )}
          {isLoggedIn && user && (
            <Link
              to={ROUTES.wallet}
              title={`钱包总览：${walletBalance.toFixed(4)} USDT`}
              className={`hidden items-center rounded-full border border-outline-variant/30 bg-surface-container-high/50 px-3 py-1.5 text-xs font-semibold tabular-nums hover:border-primary-container/40 xl:inline-flex ${walletBalanceClass}`}
            >
              <WalletIcon className="mr-1.5 h-3.5 w-3.5" aria-hidden />
              钱包
            </Link>
          )}
          {isLoggedIn && (
            <>
              <div className="sh-header-bell hidden shrink-0 lg:block [&_button]:text-[#9d9daa] [&_button]:hover:text-[#d4ff33]">
                <AiTradeNotificationDropdown />
              </div>
            </>
          )}

          {isLoggedIn && user ? (
            <div className="relative" ref={userDropdownRef}>
              <button
                type="button"
                onClick={() => {
                  setMobileMenuOpen(false)
                  setUserDropdownOpen(!userDropdownOpen)
                }}
                className="flex h-10 w-10 items-center justify-center rounded-full border border-[rgba(197,216,62,0.28)] bg-[#111114]/95 p-1 text-[#e8e8ec] transition-colors hover:border-[#d4ff33]/55 lg:h-auto lg:w-auto lg:gap-2 lg:px-2 lg:py-1.5 lg:pr-3"
              >
                {user.avatar_url ? (
                  <img
                    src={user.avatar_url}
                    alt=""
                    className="h-8 w-8 shrink-0 rounded-full object-cover ring-1 ring-primary-container/30"
                  />
                ) : (
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary-container text-xs font-bold text-black">
                    {(userDisplayName || user.email)[0].toUpperCase()}
                  </div>
                )}
                {/* 名字与 VIP 徽章同一组：徽章紧跟在名字后面 */}
                <div className="hidden min-w-0 max-w-[130px] items-center gap-1.5 lg:flex xl:max-w-[200px]">
                  <span className="truncate text-sm text-[#d4ff33]/90">
                    {userDisplayName || user.email}
                  </span>
                  {vipLevel != null && vipLevel >= 0 ? (
                    <>
                      <VipTierBadge level={vipLevel} />
                      {rebateBal?.configured === true &&
                      rebateBal.synced === true &&
                      rebateBal.rebate_is_studio === true ? (
                        <span
                          title="工作室：直推名义分成额外 +5%"
                          className="shrink-0 rounded border border-amber-400/45 bg-amber-500/15 px-1 py-0.5 text-[9px] font-bold uppercase tracking-wide text-amber-100"
                        >
                          工作室
                        </span>
                      ) : null}
                    </>
                  ) : null}
                </div>
                {/* 窄屏不显示昵称时：仅在头像与箭头之间保留徽章 */}
                <div className="hidden shrink-0 items-center gap-1.5 sm:hidden">
                  {vipLevel != null && vipLevel >= 0 ? (
                    <>
                      <VipTierBadge level={vipLevel} />
                      {rebateBal?.configured === true &&
                      rebateBal.synced === true &&
                      rebateBal.rebate_is_studio === true ? (
                        <span
                          title="工作室：直推名义分成额外 +5%"
                          className="shrink-0 rounded border border-amber-400/45 bg-amber-500/15 px-1 py-0.5 text-[9px] font-bold text-amber-100"
                        >
                          工作室
                        </span>
                      ) : null}
                    </>
                  ) : null}
                </div>
                <ChevronDown className="hidden h-4 w-4 shrink-0 text-[#9d9daa] lg:block" />
              </button>

              {userDropdownOpen && (
                <div className="absolute right-0 top-full z-50 mt-2 w-[min(92vw,260px)] min-w-[200px] overflow-hidden rounded-2xl border border-outline-variant/25 bg-surface-container-high/95 py-1 shadow-2xl backdrop-blur-md">
                  <div className="border-b border-outline-variant/20 px-3 py-2">
                    <div className="text-[11px] text-on-surface-variant/80">{t('loggedInAs', lang)}</div>
                    <div className="truncate text-sm font-medium text-on-surface">
                      {userDisplayName || user.email}
                    </div>
                    <div className="mt-0.5 truncate text-[11px] text-on-surface-variant/70">{user.email}</div>
                  </div>
                  <button
                    type="button"
                    onClick={() => {
                      navigateInApp(ROUTES.profile)
                      setUserDropdownOpen(false)
                    }}
                    className="block w-full px-3 py-2.5 text-left text-sm text-on-surface-variant transition-colors hover:bg-surface-container-highest hover:text-[#d4ff33]"
                  >
                    个人资料
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      navigateInApp(ROUTES.wallet)
                      setUserDropdownOpen(false)
                    }}
                    className="block w-full px-3 py-2.5 text-left text-sm text-on-surface-variant transition-colors hover:bg-surface-container-highest hover:text-[#d4ff33]"
                  >
                    钱包
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      navigateInApp(ROUTES.invite)
                      setUserDropdownOpen(false)
                    }}
                    className="block w-full px-3 py-2.5 text-left text-sm text-on-surface-variant transition-colors hover:bg-surface-container-highest hover:text-[#d4ff33]"
                  >
                    邀请奖励
                  </button>
                  {user.is_admin && (
                    <button
                      type="button"
                      onClick={() => {
                        navigateInApp(ROUTES.admin)
                        setUserDropdownOpen(false)
                      }}
                      className="block w-full px-3 py-2.5 text-left text-sm font-semibold text-[#d4ff33] transition-colors hover:bg-surface-container-highest"
                    >
                      管理后台
                    </button>
                  )}
                  {user.is_finance && (
                    <button
                      type="button"
                      onClick={() => {
                        navigateInApp(ROUTES.finance)
                        setUserDropdownOpen(false)
                      }}
                      className="block w-full px-3 py-2.5 text-left text-sm font-semibold text-emerald-300 transition-colors hover:bg-surface-container-highest"
                    >
                      财务台
                    </button>
                  )}
                  {onLogout && (
                    <button
                      type="button"
                      onClick={() => {
                        onLogout()
                        setUserDropdownOpen(false)
                      }}
                      className="w-full px-3 py-2.5 text-left text-sm font-semibold text-error transition-colors hover:bg-error/10"
                    >
                      {t('exitLogin', lang)}
                    </button>
                  )}
                </div>
              )}
            </div>
          ) : (
            resolvedCurrentPage !== 'login' &&
            resolvedCurrentPage !== 'register' && (
              <div className="hidden items-center gap-2 lg:flex lg:gap-3">
                <Link to={ROUTES.login} className="sh-nav-cta ghost">
                  登录
                </Link>
                <Link to={ROUTES.register} className="sh-nav-cta solid">
                  注册
                </Link>
              </div>
            )
          )}

          <motion.button
            type="button"
            onClick={() => {
              setUserDropdownOpen(false)
              setMobileMenuOpen(!mobileMenuOpen)
            }}
            className="sh-mobile-menu-button shrink-0 rounded-lg p-2 text-[#9d9daa] transition-colors hover:bg-white/5 hover:text-[#d4ff33] lg:hidden"
            whileTap={{ scale: 0.9 }}
            aria-label={mobileMenuOpen ? '关闭菜单' : '打开菜单'}
          >
            {mobileMenuOpen ? <X className="h-6 w-6" /> : <Menu className="h-6 w-6" />}
          </motion.button>
        </div>
      </nav>

      <AnimatePresence>
        {mobileMenuOpen && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="absolute inset-x-0 top-full z-[9999] h-[calc(100dvh-62px)] bg-black/95 backdrop-blur-md lg:hidden"
          >
            <motion.div
              initial={{ y: -12, opacity: 0 }}
              animate={{ y: 0, opacity: 1 }}
              exit={{ y: -12, opacity: 0 }}
              transition={{ delay: 0.05, duration: 0.25 }}
              className="flex h-full flex-col overflow-y-auto overscroll-contain px-3 py-3 pb-[calc(16px+env(safe-area-inset-bottom))] font-lumbody sm:px-5 sm:py-5"
            >
              <div className="mb-4 grid grid-cols-2 gap-2 border-b border-white/10 pb-4">
                <p className="col-span-2 text-xs font-medium tracking-widest text-on-surface-variant/70">策略市场</p>
                <button
                  type="button"
                  onClick={() =>
                    tryNav(ROUTES.strategyMarket, 'strategy-market', true, '策略市场')
                  }
                  className="rounded-xl border border-white/10 bg-white/[0.03] px-3 py-3 text-left text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                >
                  策略市场
                </button>
                <button
                  type="button"
                  onClick={() =>
                    tryNav(ROUTES.autoArbitrage, 'auto-arbitrage', true, '全自动套利系统')
                  }
                  className="rounded-xl border border-white/10 bg-white/[0.03] px-3 py-3 text-left text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                >
                  全自动套利系统
                </button>
              </div>
              <div className="mb-4 grid grid-cols-2 gap-2 border-b border-white/10 pb-4">
                <p className="col-span-2 text-xs font-medium tracking-widest text-on-surface-variant/70">数据看板</p>
                <button
                  type="button"
                  onClick={() =>
                    tryNav(ROUTES.dashboard, 'trader', true, t('dashboardNav', lang))
                  }
                  className="rounded-xl border border-white/10 bg-white/[0.03] px-3 py-3 text-left text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                >
                  AI策略数据看板
                </button>
                <button
                  type="button"
                  onClick={() => tryNav(ROUTES.data, 'data', false, '数据')}
                  className="rounded-xl border border-white/10 bg-white/[0.03] px-3 py-3 text-left text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                >
                  行情数据
                </button>
                <button
                  type="button"
                  onClick={() => tryNav(ROUTES.news, 'news', true, '新闻信息监控')}
                  className="rounded-xl border border-white/10 bg-white/[0.03] px-3 py-3 text-left text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                >
                  新闻信息监控
                </button>
              </div>
              <div className="mb-4 grid grid-cols-2 gap-2 border-b border-white/10 pb-4">
                <p className="col-span-2 text-xs font-medium tracking-widest text-on-surface-variant/70">AI交易员配置</p>
                <button
                  type="button"
                  onClick={() => tryNav(TRADERS_WIZARD_ENTRY, 'traders', true, t('configNav', lang))}
                  className="rounded-xl border border-white/10 bg-white/[0.03] px-3 py-3 text-left text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                >
                  AI交易员配置
                </button>
                <button
                  type="button"
                  onClick={() => tryNav(ROUTES.strategy, 'strategy', true, '策略构建器')}
                  className="rounded-xl border border-white/10 bg-white/[0.03] px-3 py-3 text-left text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                >
                  策略构建器
                </button>
                <button
                  type="button"
                  onClick={() => {
                    if (!isLoggedIn) {
                      onLoginRequired?.('AI模型配置')
                      return
                    }
                    navigateInApp(ROUTES.settings)
                    setMobileMenuOpen(false)
                  }}
                  className="rounded-xl border border-white/10 bg-white/[0.03] px-3 py-3 text-left text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                >
                  AI模型配置
                </button>
              </div>

              {isHomePage && (
                <div className="mb-auto space-y-3 border-t border-white/10 pt-6">
                  {[
                    { key: 'features', label: t('features', lang), href: '#features' },
                    { key: 'howItWorks', label: t('howItWorks', lang), href: '#how-it-works' },
                  ].map((item) => (
                    <a
                      key={item.key}
                      href={item.href}
                      className="block text-sm text-on-surface-variant/80 transition-colors hover:text-[#d4ff33]"
                      onClick={() => setMobileMenuOpen(false)}
                    >
                      {item.label}
                    </a>
                  ))}
                </div>
              )}

              {isLoggedIn && user && (
                <div className="mt-2 space-y-3 border-t border-white/10 pt-4">
                  <Link
                    to={ROUTES.invite}
                    onClick={() => setMobileMenuOpen(false)}
                    className="flex items-center gap-2 rounded-xl border border-[#d4ff33]/35 bg-[#d4ff33]/10 px-4 py-3 text-sm font-bold text-[#d4ff33] transition-colors hover:bg-[#d4ff33]/15"
                  >
                    <Gift className="h-4 w-4 shrink-0" />
                    邀请奖励（复制链接）
                  </Link>
                  <Link
                    to={ROUTES.wallet}
                    onClick={() => setMobileMenuOpen(false)}
                    className={`block rounded-xl border border-outline-variant/20 bg-surface-container-high/40 px-4 py-3 text-sm tabular-nums transition-colors hover:border-primary-container/40 ${walletBalanceClass}`}
                  >
                    钱包总览：{walletBalance.toFixed(4)} USDT（点按查看）
                  </Link>
                  {user.is_admin && (
                    <button
                      type="button"
                      onClick={() => {
                        navigateInApp(ROUTES.admin)
                        setMobileMenuOpen(false)
                      }}
                      className="w-full rounded-xl border border-[#d4ff33]/40 py-3 text-sm font-bold text-[#d4ff33] transition-colors hover:bg-[#d4ff33]/10"
                    >
                      管理后台
                    </button>
                  )}
                  {user.is_finance && (
                    <button
                      type="button"
                      onClick={() => {
                        navigateInApp(ROUTES.finance)
                        setMobileMenuOpen(false)
                      }}
                      className="w-full rounded-xl border border-emerald-500/40 py-3 text-sm font-bold text-emerald-300 transition-colors hover:bg-emerald-500/10"
                    >
                      财务台
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => {
                      onLogout?.()
                      setMobileMenuOpen(false)
                    }}
                    className="w-full rounded-xl border border-error/30 bg-error/10 py-3 text-sm font-bold text-error transition-colors hover:bg-error/20"
                  >
                    {t('exitLogin', lang)}
                  </button>
                </div>
              )}
              {!isLoggedIn && resolvedCurrentPage !== 'login' && resolvedCurrentPage !== 'register' && (
                <div className="mt-2 grid grid-cols-2 gap-2 border-t border-white/10 pt-4">
                  <Link
                    to={ROUTES.login}
                    onClick={() => setMobileMenuOpen(false)}
                    className="rounded-xl border border-white/10 bg-white/[0.03] px-4 py-3 text-center text-sm font-bold text-on-surface-variant transition-colors hover:border-[#d4ff33]/35 hover:text-[#d4ff33]"
                  >
                    登录
                  </Link>
                  <Link
                    to={ROUTES.register}
                    onClick={() => setMobileMenuOpen(false)}
                    className="rounded-xl bg-[#d4ff33] px-4 py-3 text-center text-sm font-bold text-black transition-colors hover:bg-[#d4e64a]"
                  >
                    注册
                  </Link>
                </div>
              )}
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </header>
  )
}
