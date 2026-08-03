import {
  ArrowRight,
  Bot,
  HandCoins,
  Network,
  Shield,
  Users,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { ROUTES } from '../router/paths'

const modules = [
  {
    title: '合作伙伴总账',
    description: '身份分配、充值返佣、工作室申请与提现审核',
    route: ROUTES.adminPartners,
    icon: HandCoins,
  },
  {
    title: '交易员管理',
    description: '查看本机快照、持仓并代客户启动或停止交易员',
    route: ROUTES.adminTraders,
    icon: Bot,
  },
  {
    title: 'SOCKS5 出口代理池',
    description: '导入、绑定、回收并维护交易所专用出口',
    route: ROUTES.adminProxies,
    icon: Network,
  },
  {
    title: '用户与余额',
    description: '查看全部用户、交易所绑定并执行站内余额调账',
    route: ROUTES.adminUsers,
    icon: Users,
  },
  {
    title: 'AI 调用账单',
    description: '核对模型真实成本、服务费和用户余额变化',
    route: ROUTES.adminAIBilling,
    icon: Shield,
  },
] as const

export function AdminHubPage() {
  return (
    <div className="min-h-screen bg-nofx-bg-tertiary px-4 pb-16 pt-20 text-[#EAECEF] sm:px-8">
      <main className="mx-auto max-w-6xl">
        <header className="border-b border-white/10 pb-6">
          <div className="flex items-center gap-3">
            <Shield className="h-8 w-8 text-[#d4ff33]" aria-hidden />
            <div>
              <h1 className="text-2xl font-bold">管理后台</h1>
              <p className="mt-1 text-sm text-[#848E9C]">
                选择一个管理板块进入，各板块独立操作。
              </p>
            </div>
          </div>
        </header>

        <section className="mt-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {modules.map((item) => {
            const Icon = item.icon
            return (
              <Link
                key={item.route}
                to={item.route}
                className="group flex min-h-36 flex-col justify-between rounded-md border border-white/10 bg-nofx-bg-secondary p-5 transition-colors hover:border-[#d4ff33]/45 hover:bg-white/[0.04] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[#d4ff33]"
              >
                <div className="flex items-start justify-between gap-4">
                  <Icon className="h-6 w-6 text-[#d4ff33]" aria-hidden />
                  <ArrowRight className="h-5 w-5 text-[#5e6673] transition-transform group-hover:translate-x-1 group-hover:text-[#d4ff33]" />
                </div>
                <div>
                  <h2 className="text-base font-bold text-white">
                    {item.title}
                  </h2>
                  <p className="mt-1 text-xs leading-5 text-[#848E9C]">
                    {item.description}
                  </p>
                </div>
              </Link>
            )
          })}
        </section>
      </main>
    </div>
  )
}
