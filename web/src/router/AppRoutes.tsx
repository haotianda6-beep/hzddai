import {
  Suspense,
  lazy,
  type ReactNode,
  useEffect,
  useMemo,
  useState,
} from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import useSWR from 'swr'
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useSearchParams,
} from 'react-router-dom'
import HeaderBar from '../components/common/HeaderBar'
import { SiteFooter } from '../components/common/SiteFooter'
import { LoginPage } from '../components/auth/LoginPage'
import { RegisterPage } from '../components/auth/RegisterPage'
import { ResetPasswordPage } from '../components/auth/ResetPasswordPage'
import { SetupPage } from '../components/modals/SetupPage'
import { LandingPage } from '../pages/LandingPage'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import { useSystemConfig } from '../hooks/useSystemConfig'
import { t } from '../i18n/translations'
import { api } from '../lib/api'
import type {
  AccountInfo,
  DecisionRecord,
  Exchange,
  Position,
  Statistics,
  SystemStatus,
  TraderInfo,
} from '../types'
import {
  buildDashboardPath,
  LEGACY_HASH_ROUTES,
  ROUTES,
  TRADERS_WIZARD_ENTRY,
  type Page,
} from './paths'

const ThemePreviewPage = lazy(() =>
  import('../pages/ThemePreviewPage').then((mod) => ({
    default: mod.ThemePreviewPage,
  }))
)
const CompetitionPage = lazy(() =>
  import('../components/trader/CompetitionPage').then((mod) => ({
    default: mod.CompetitionPage,
  }))
)
const TraderDeployWizardPage = lazy(() =>
  import('../pages/TraderDeployWizardPage').then((mod) => ({
    default: mod.TraderDeployWizardPage,
  }))
)
const FAQPage = lazy(() =>
  import('../pages/FAQPage').then((mod) => ({ default: mod.FAQPage }))
)
const DataPage = lazy(() =>
  import('../pages/DataPage').then((mod) => ({ default: mod.DataPage }))
)
const CryptoNewsPage = lazy(() =>
  import('../pages/CryptoNewsPage').then((mod) => ({
    default: mod.CryptoNewsPage,
  }))
)
const SettingsPage = lazy(() =>
  import('../pages/SettingsPage').then((mod) => ({ default: mod.SettingsPage }))
)
const ProfilePage = lazy(() =>
  import('../pages/ProfilePage').then((mod) => ({ default: mod.ProfilePage }))
)
const InviteFissionPage = lazy(() =>
  import('../pages/InviteFissionPage').then((mod) => ({
    default: mod.InviteFissionPage,
  }))
)
const StrategyMarketPage = lazy(() =>
  import('../pages/StrategyMarketPage').then((mod) => ({
    default: mod.StrategyMarketPage,
  }))
)
const StrategyMarketDetailPage = lazy(() =>
  import('../pages/StrategyMarketDetailPage').then((mod) => ({
    default: mod.StrategyMarketDetailPage,
  }))
)
const AdminHubPage = lazy(() =>
  import('../pages/AdminHubPage').then((mod) => ({ default: mod.AdminHubPage }))
)
const AdminPartnerLedgerPage = lazy(() =>
  import('../pages/AdminPartnerLedgerPage').then((mod) => ({
    default: mod.AdminPartnerLedgerPage,
  }))
)
const AdminTradersPage = lazy(() =>
  import('../pages/AdminTradersPage').then((mod) => ({
    default: mod.AdminTradersPage,
  }))
)
const AdminProxyPoolPage = lazy(() =>
  import('../pages/AdminProxyPoolPage').then((mod) => ({
    default: mod.AdminProxyPoolPage,
  }))
)
const AdminUsersPage = lazy(() =>
  import('../pages/AdminUsersPage').then((mod) => ({
    default: mod.AdminUsersPage,
  }))
)
const AdminAIBillingPage = lazy(() =>
  import('../pages/AdminAIBillingPage').then((mod) => ({
    default: mod.AdminAIBillingPage,
  }))
)
const FinanceDashboardPage = lazy(() =>
  import('../pages/FinanceDashboardPage').then((mod) => ({
    default: mod.FinanceDashboardPage,
  }))
)
const RechargePage = lazy(() =>
  import('../pages/RechargePage').then((mod) => ({ default: mod.RechargePage }))
)
const WalletPage = lazy(() =>
  import('../pages/WalletPage').then((mod) => ({ default: mod.WalletPage }))
)
const StrategyStudioPage = lazy(() =>
  import('../pages/StrategyStudioPage').then((mod) => ({
    default: mod.StrategyStudioPage,
  }))
)
const TraderDashboardPage = lazy(() =>
  import('../pages/TraderDashboardPage').then((mod) => ({
    default: mod.TraderDashboardPage,
  }))
)
const AutoArbitragePage = lazy(() =>
  import('../pages/AutoArbitragePage').then((mod) => ({
    default: mod.AutoArbitragePage,
  }))
)

function getTraderSlug(trader: TraderInfo) {
  const idPrefix = trader.trader_id.slice(0, 4)
  return `${trader.trader_name}-${idPrefix}`
}

function findTraderBySlug(slug: string, traderList: TraderInfo[]) {
  const lastDashIndex = slug.lastIndexOf('-')
  if (lastDashIndex === -1) {
    return traderList.find((trader) => trader.trader_name === slug)
  }

  const name = slug.slice(0, lastDashIndex)
  const idPrefix = slug.slice(lastDashIndex + 1)
  return traderList.find(
    (trader) =>
      trader.trader_name === name && trader.trader_id.startsWith(idPrefix)
  )
}

function LoadingScreen() {
  const { language } = useLanguage()
  const [slowHint, setSlowHint] = useState(false)
  useEffect(() => {
    const id = window.setTimeout(() => setSlowHint(true), 10_000)
    return () => window.clearTimeout(id)
  }, [])

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-nofx-bg text-nofx-text">
      <div className="flex flex-col items-center text-center">
        <div className="mb-3 flex items-center justify-center gap-2.5 text-lg leading-none text-nofx-text-main">
          <img
            src="/icons/comkun-logo.png"
            alt=""
            className="h-8 w-8 shrink-0 animate-pulse rounded-lg object-cover"
            aria-hidden
          />
          <p className="font-medium leading-none">{t('loading', language)}</p>
        </div>
        {slowHint ? (
          <p className="max-w-sm px-4 text-center text-[11px] leading-relaxed text-nofx-text-muted">
            {language === 'zh'
              ? '若长时间停在此页，多半是浏览器访问不到后端的 /api（例如反代未转发、或 nofx 容器未启动）。可先在本机 curl 域名下的 /api/config 排查。'
              : 'If this stays too long, the browser may be unable to reach /api (reverse proxy or backend down). Try curling /api/config on this host.'}
          </p>
        ) : null}
      </div>
    </div>
  )
}

function PageLoader({ children }: { children: ReactNode }) {
  return <Suspense fallback={<LoadingScreen />}>{children}</Suspense>
}

function LegacyHashRedirect() {
  const location = useLocation()
  const navigate = useNavigate()

  useEffect(() => {
    const hashRoute = LEGACY_HASH_ROUTES[location.hash.slice(1)]
    if (!hashRoute) {
      return
    }

    if (hashRoute === location.pathname && location.hash === '') {
      return
    }

    navigate(
      {
        pathname: hashRoute,
        search: location.search,
      },
      { replace: true }
    )
  }, [location.hash, location.pathname, location.search, navigate])

  return null
}

interface AppChromeProps {
  children: ReactNode
  currentPage?: Page
  showFooter?: boolean
  wrapInMain?: boolean
  animateContent?: boolean
  extraContent?: ReactNode
}

function AppChrome({
  children,
  currentPage,
  showFooter = true,
  wrapInMain = true,
  animateContent = false,
  extraContent,
}: AppChromeProps) {
  const location = useLocation()
  const navigate = useNavigate()
  const { language, setLanguage } = useLanguage()
  const { user, logout } = useAuth()

  const handleLoginRequired = (_featureLabel?: string) => {
    navigate(ROUTES.login, { replace: true })
  }

  const content = animateContent ? (
    <AnimatePresence initial={false}>
      <motion.div
        key={`${location.pathname}${location.search}`}
        className="w-full min-w-0"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        transition={{ duration: 0.16, ease: 'easeOut' }}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  ) : (
    children
  )

  return (
    <div className="flex min-h-screen flex-col bg-nofx-bg text-nofx-text">
      <HeaderBar
        isLoggedIn={!!user}
        currentPage={currentPage}
        language={language}
        onLanguageChange={setLanguage}
        user={user}
        onLogout={logout}
        onLoginRequired={handleLoginRequired}
      />

      {wrapInMain ? (
        <main className="min-h-0 w-full min-w-0 flex-1">{content}</main>
      ) : (
        content
      )}

      {showFooter ? <SiteFooter language={language} /> : null}

      {extraContent}
    </div>
  )
}

function TradersRoute() {
  return (
    <AppChrome currentPage="traders" animateContent showFooter={false}>
      <PageLoader>
        <TraderDeployWizardPage />
      </PageLoader>
    </AppChrome>
  )
}

/** 仪表盘右侧「最近决策」拉取条数（界面不再提供切换，固定值） */
const DASHBOARD_DECISIONS_LIMIT = 20

function DashboardRoute() {
  const { language } = useLanguage()
  const { user, token } = useAuth()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const selectedTraderSlug = searchParams.get('trader') || undefined
  const [selectedTraderId, setSelectedTraderId] = useState<string | undefined>()
  const [accountPollOff, setAccountPollOff] = useState(false)
  const [positionsPollOff, setPositionsPollOff] = useState(false)
  const [decisionsPollOff, setDecisionsPollOff] = useState(false)

  const { data: traders, error: tradersError } = useSWR<TraderInfo[]>(
    user && token ? 'traders-dashboard' : null,
    () => api.getTraders(true),
    {
      refreshInterval: 10000,
      shouldRetryOnError: false,
      keepPreviousData: true,
    }
  )

  /** 列表刷新、slug 失效时仍能对上交易员，避免整页骨架把「看板」卡没 */
  const stableSelectedId = useMemo(() => {
    if (!traders?.length) return undefined
    if (selectedTraderSlug) {
      const by = findTraderBySlug(selectedTraderSlug, traders)
      return (by ?? traders[0]).trader_id
    }
    if (
      selectedTraderId &&
      traders.some((t) => t.trader_id === selectedTraderId)
    ) {
      return selectedTraderId
    }
    return traders[0].trader_id
  }, [traders, selectedTraderId, selectedTraderSlug])

  useEffect(() => {
    setAccountPollOff(false)
    setPositionsPollOff(false)
    setDecisionsPollOff(false)
  }, [stableSelectedId])

  useEffect(() => {
    if (stableSelectedId && stableSelectedId !== selectedTraderId) {
      setSelectedTraderId(stableSelectedId)
    }
  }, [stableSelectedId, selectedTraderId])

  useEffect(() => {
    if (!traders?.length || !selectedTraderSlug) return
    if (!findTraderBySlug(selectedTraderSlug, traders)) {
      navigate(buildDashboardPath(getTraderSlug(traders[0])), { replace: true })
    }
  }, [traders, selectedTraderSlug, navigate])

  const { data: exchanges } = useSWR<Exchange[]>(
    user && token ? 'exchanges-dashboard' : null,
    api.getExchangeConfigs,
    {
      refreshInterval: 60000,
      shouldRetryOnError: false,
    }
  )

  const { data: status } = useSWR<SystemStatus>(
    stableSelectedId ? `status-${stableSelectedId}` : null,
    () => api.getStatus(stableSelectedId!, true),
    {
      refreshInterval: 15000,
      revalidateOnFocus: false,
      dedupingInterval: 10000,
    }
  )

  const { data: account } = useSWR<AccountInfo>(
    stableSelectedId ? `account-${stableSelectedId}` : null,
    () => api.getAccount(stableSelectedId!, true),
    {
      refreshInterval: 15000,
      revalidateOnFocus: true,
      dedupingInterval: 10000,
      onErrorRetry: (_err, _key, _config, revalidate, { retryCount }) => {
        if (retryCount >= 2) {
          setAccountPollOff(true)
          return
        }
        setTimeout(() => revalidate({ retryCount }), 500)
      },
      onSuccess: () => {
        if (accountPollOff) {
          setAccountPollOff(false)
        }
      },
    }
  )

  const { data: positions } = useSWR<Position[]>(
    stableSelectedId ? `positions-${stableSelectedId}` : null,
    () => api.getPositions(stableSelectedId!, true),
    {
      refreshInterval: 15000,
      revalidateOnFocus: true,
      dedupingInterval: 10000,
      onErrorRetry: (_err, _key, _config, revalidate, { retryCount }) => {
        if (retryCount >= 2) {
          setPositionsPollOff(true)
          return
        }
        setTimeout(() => revalidate({ retryCount }), 500)
      },
      onSuccess: () => {
        if (positionsPollOff) {
          setPositionsPollOff(false)
        }
      },
    }
  )

  const { data: decisions } = useSWR<DecisionRecord[]>(
    stableSelectedId
      ? `decisions/latest-${stableSelectedId}-${DASHBOARD_DECISIONS_LIMIT}`
      : null,
    () =>
      api.getLatestDecisions(
        stableSelectedId!,
        DASHBOARD_DECISIONS_LIMIT,
        true
      ),
    {
      refreshInterval: decisionsPollOff ? 0 : 10000,
      revalidateOnFocus: false,
      dedupingInterval: 20000,
      onErrorRetry: (_err, _key, _config, revalidate, { retryCount }) => {
        if (retryCount >= 2) {
          setDecisionsPollOff(true)
          return
        }
        setTimeout(() => revalidate({ retryCount }), 500)
      },
      onSuccess: () => {
        if (decisionsPollOff) {
          setDecisionsPollOff(false)
        }
      },
    }
  )

  const { data: stats } = useSWR<Statistics>(
    stableSelectedId ? `statistics-${stableSelectedId}` : null,
    () => api.getStatistics(stableSelectedId!, true),
    {
      refreshInterval: 30000,
      revalidateOnFocus: false,
      dedupingInterval: 20000,
    }
  )

  const selectedTrader = traders?.find(
    (trader) => trader.trader_id === stableSelectedId
  )

  useEffect(() => {
    if (!stableSelectedId) return
    try {
      localStorage.setItem('nofx_notification_trader_id', stableSelectedId)
    } catch {
      /* ignore */
    }
  }, [stableSelectedId])

  return (
    <AppChrome currentPage="trader" animateContent>
      <PageLoader>
        <TraderDashboardPage
          selectedTrader={selectedTrader}
          status={status}
          account={account}
          accountFailed={accountPollOff}
          positions={positions}
          positionsFailed={positionsPollOff}
          decisions={decisions}
          decisionsFailed={decisionsPollOff}
          stats={stats}
          language={language}
          traders={traders}
          tradersError={tradersError}
          selectedTraderId={stableSelectedId}
          onTraderSelect={(traderId) => {
            try {
              localStorage.setItem('nofx_notification_trader_id', traderId)
            } catch {
              /* ignore */
            }
            setSelectedTraderId(traderId)
            const trader = traders?.find((item) => item.trader_id === traderId)
            navigate(
              buildDashboardPath(trader ? getTraderSlug(trader) : undefined),
              {
                replace: true,
              }
            )
          }}
          onNavigateToTraders={() => navigate(TRADERS_WIZARD_ENTRY)}
          exchanges={exchanges}
        />
      </PageLoader>
    </AppChrome>
  )
}

export function AppRoutes() {
  const { user, token, isLoading } = useAuth()
  const { config: systemConfig, loading: configLoading } = useSystemConfig()
  const isAuthenticated = !!user && !!token

  if (isLoading || configLoading) {
    return <LoadingScreen />
  }

  if (systemConfig && !systemConfig.initialized && !user) {
    return <SetupPage />
  }

  const adminElement = (page: ReactNode) =>
    isAuthenticated ? (
      user?.is_admin ? (
        <AppChrome showFooter={false} wrapInMain={false}>
          <PageLoader>{page}</PageLoader>
        </AppChrome>
      ) : (
        <Navigate to={ROUTES.profile} replace />
      )
    ) : (
      <Navigate to={ROUTES.login} replace />
    )

  return (
    <>
      <LegacyHashRedirect />
      <Routes>
        <Route path={ROUTES.home} element={<LandingPage />} />
        <Route
          path={ROUTES.themePreview}
          element={
            <Suspense fallback={<LoadingScreen />}>
              <ThemePreviewPage />
            </Suspense>
          }
        />
        <Route path={ROUTES.login} element={<LoginPage />} />
        <Route path={ROUTES.register} element={<RegisterPage />} />
        <Route path={ROUTES.resetPassword} element={<ResetPasswordPage />} />
        <Route
          path={ROUTES.setup}
          element={
            systemConfig?.initialized ? (
              <Navigate to={ROUTES.login} replace />
            ) : (
              <SetupPage />
            )
          }
        />
        <Route
          path={ROUTES.faq}
          element={
            <AppChrome currentPage="faq" showFooter={false} wrapInMain={false}>
              <PageLoader>
                <FAQPage />
              </PageLoader>
            </AppChrome>
          }
        />
        <Route
          path={ROUTES.data}
          element={
            <AppChrome currentPage="data" showFooter={false}>
              <PageLoader>
                <DataPage />
              </PageLoader>
            </AppChrome>
          }
        />
        <Route
          path={ROUTES.news}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="news" showFooter={false}>
                <PageLoader>
                  <CryptoNewsPage />
                </PageLoader>
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.settings}
          element={
            isAuthenticated ? (
              <AppChrome showFooter={false}>
                <PageLoader>
                  <SettingsPage />
                </PageLoader>
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.profile}
          element={
            isAuthenticated ? (
              <AppChrome showFooter={false}>
                <PageLoader>
                  <ProfilePage />
                </PageLoader>
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.invite}
          element={
            isAuthenticated ? (
              <AppChrome showFooter={false}>
                <PageLoader>
                  <InviteFissionPage />
                </PageLoader>
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.welcome}
          element={
            isAuthenticated ? (
              <TradersRoute />
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.competition}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="competition" animateContent>
                <PageLoader>
                  <CompetitionPage />
                </PageLoader>
              </AppChrome>
            ) : (
              <LandingPage />
            )
          }
        />
        <Route
          path={`${ROUTES.strategyMarket}/:strategyId`}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="strategy-market" animateContent>
                <PageLoader>
                  <StrategyMarketDetailPage />
                </PageLoader>
              </AppChrome>
            ) : (
              <LandingPage />
            )
          }
        />
        <Route
          path={ROUTES.strategyMarket}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="strategy-market" animateContent>
                <PageLoader>
                  <StrategyMarketPage />
                </PageLoader>
              </AppChrome>
            ) : (
              <LandingPage />
            )
          }
        />
        <Route
          path={ROUTES.wallet}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="wallet" showFooter={false}>
                <PageLoader>
                  <WalletPage />
                </PageLoader>
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.recharge}
          element={
            isAuthenticated ? (
              <AppChrome showFooter={false}>
                <PageLoader>
                  <RechargePage />
                </PageLoader>
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.comkunOfficialToken}
          element={
            <Navigate
              to={isAuthenticated ? ROUTES.recharge : ROUTES.login}
              replace
            />
          }
        />
        <Route path={ROUTES.admin} element={adminElement(<AdminHubPage />)} />
        <Route
          path={ROUTES.adminPartners}
          element={adminElement(<AdminPartnerLedgerPage />)}
        />
        <Route
          path={ROUTES.adminTraders}
          element={adminElement(<AdminTradersPage />)}
        />
        <Route
          path={ROUTES.adminProxies}
          element={adminElement(<AdminProxyPoolPage />)}
        />
        <Route
          path={ROUTES.adminUsers}
          element={adminElement(<AdminUsersPage />)}
        />
        <Route
          path={ROUTES.adminAIBilling}
          element={adminElement(<AdminAIBillingPage />)}
        />
        <Route
          path={ROUTES.finance}
          element={
            isAuthenticated ? (
              user?.is_finance ? (
                <AppChrome showFooter={false} wrapInMain={false}>
                  <PageLoader>
                    <FinanceDashboardPage />
                  </PageLoader>
                </AppChrome>
              ) : (
                <Navigate to={ROUTES.profile} replace />
              )
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path="/traders/create"
          element={<Navigate to={TRADERS_WIZARD_ENTRY} replace />}
        />
        <Route
          path={ROUTES.traders}
          element={isAuthenticated ? <TradersRoute /> : <LandingPage />}
        />
        <Route
          path={ROUTES.dashboard}
          element={isAuthenticated ? <DashboardRoute /> : <LandingPage />}
        />
        <Route
          path={ROUTES.strategy}
          element={
            isAuthenticated ? (
              <AppChrome
                currentPage="strategy"
                animateContent
                showFooter={false}
              >
                <PageLoader>
                  <StrategyStudioPage />
                </PageLoader>
              </AppChrome>
            ) : (
              <LandingPage />
            )
          }
        />
        <Route
          path={ROUTES.autoArbitrage}
          element={
            isAuthenticated ? (
              <AppChrome
                currentPage="auto-arbitrage"
                animateContent
                showFooter={false}
              >
                <PageLoader>
                  <AutoArbitragePage />
                </PageLoader>
              </AppChrome>
            ) : (
              <LandingPage />
            )
          }
        />
        <Route path="*" element={<Navigate to={ROUTES.home} replace />} />
      </Routes>
    </>
  )
}
