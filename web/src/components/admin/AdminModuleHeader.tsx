import { ArrowLeft, RefreshCw, type LucideIcon } from 'lucide-react'
import { Link } from 'react-router-dom'
import { ROUTES } from '../../router/paths'

export function AdminModuleHeader({
  icon: Icon,
  title,
  description,
  onRefresh,
}: {
  icon: LucideIcon
  title: string
  description: string
  onRefresh: () => void
}) {
  return (
    <header>
      <Link
        to={ROUTES.admin}
        className="inline-flex items-center gap-1 text-sm text-[#848E9C] hover:text-[#d4ff33]"
      >
        <ArrowLeft className="h-4 w-4" /> 管理后台
      </Link>
      <div className="mt-5 flex flex-wrap items-center justify-between gap-4 border-b border-white/10 pb-5">
        <div className="flex items-center gap-3">
          <Icon className="h-8 w-8 text-[#d4ff33]" aria-hidden />
          <div>
            <h1 className="text-2xl font-bold text-white">{title}</h1>
            <p className="mt-1 text-sm text-[#848E9C]">{description}</p>
          </div>
        </div>
        <button
          type="button"
          onClick={onRefresh}
          className="inline-flex items-center gap-2 rounded border border-white/10 px-3 py-2 text-sm hover:bg-white/5"
        >
          <RefreshCw className="h-4 w-4" /> 刷新
        </button>
      </div>
    </header>
  )
}

export function AdminModuleShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-nofx-bg-tertiary px-4 pb-16 pt-20 text-[#EAECEF] sm:px-8">
      <main className="mx-auto max-w-[1400px]">{children}</main>
    </div>
  )
}
