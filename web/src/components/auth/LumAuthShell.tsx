import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ROUTES } from '../../router/paths'
import '../../pages/landing/luminescent.css'

type LumAuthShellProps = {
  title: string
  subtitle?: string
  children: ReactNode
  /** 卡片下方区域，例如「去注册」链接 */
  footer?: ReactNode
  /** 右上角额外控件（如语言切换） */
  topRight?: ReactNode
}

/**
 * 与首页 Luminescent 一致的深色底 + 芥末黄强调 + 网格背景
 */
export function LumAuthShell({
  title,
  subtitle,
  children,
  footer,
  topRight,
}: LumAuthShellProps) {
  return (
    <div className="lum-page relative min-h-screen overflow-x-hidden bg-surface font-lumbody text-on-surface selection:bg-primary-container/30 selection:text-black">
      <div
        className="pointer-events-none absolute inset-0 lum-bg-grid-subtle opacity-90"
        aria-hidden
      />
      <div
        className="pointer-events-none absolute -top-40 left-1/2 h-[420px] w-[min(90vw,720px)] -translate-x-1/2 rounded-full bg-primary-container/[0.07] blur-3xl"
        aria-hidden
      />

      <header className="relative z-10 mx-auto flex max-w-lg items-center justify-between px-4 pt-6 sm:px-6">
        <Link
          to={ROUTES.home}
          className="flex items-center gap-2.5 font-lumheadline text-lg font-bold leading-none tracking-tight text-primary-container transition-colors hover:text-[#d4ff33]"
        >
          <img
            src="/icons/comkun-logo.png"
            alt=""
            className="h-9 w-9 shrink-0 rounded-lg object-cover"
            aria-hidden
          />
          <span className="leading-none">COMKUN-AI</span>
        </Link>
        {topRight ? (
          <div className="flex shrink-0 items-center gap-2">{topRight}</div>
        ) : null}
      </header>

      <main className="relative z-10 mx-auto flex max-w-lg flex-col px-4 pb-16 pt-8 sm:px-6">
        <div className="mb-8 text-center">
          <h1 className="font-lumheadline text-2xl font-bold tracking-tight text-on-surface sm:text-3xl">
            {title}
          </h1>
          {subtitle ? (
            <p className="mt-2 text-sm text-on-surface-variant">{subtitle}</p>
          ) : null}
        </div>

        <div className="lum-glass-panel rounded-3xl border border-outline-variant/30 p-6 shadow-2xl sm:p-8">
          {children}
        </div>

        {footer ? (
          <div className="mt-8 text-center text-sm text-on-surface-variant">
            {footer}
          </div>
        ) : null}
      </main>
    </div>
  )
}
