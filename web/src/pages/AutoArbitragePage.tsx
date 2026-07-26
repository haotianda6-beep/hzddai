import { useEffect, useMemo, useRef, useState } from 'react'
import gsap from 'gsap'
import useSWR from 'swr'
import {
  Activity,
  ArrowRightLeft,
  BarChart3,
  CandlestickChart,
  Factory,
  Globe2,
  LockKeyhole,
  Plus,
  ShieldCheck,
  X,
} from 'lucide-react'
import { useAuth } from '../contexts/AuthContext'
import { api } from '../lib/api'

const ALLOWED_EMAIL = 'haotianda6@gmail.com'
const RATE_90D = 0.3
const PRODUCTS = [
  { id: 'commodity', title: '大宗商品套利', icon: Factory },
  { id: 'crossExchange', title: '五所跨平台套利', icon: Globe2 },
  { id: 'basis', title: '单平台期现套利', icon: CandlestickChart },
  { id: 'usStockBasis', title: '美股基差套利', icon: BarChart3 },
] as const

type ProductId = (typeof PRODUCTS)[number]['id']
type Allocations = Record<ProductId, number>
type TransferDirection = 'in' | 'out'

const emptyAllocations = PRODUCTS.reduce(
  (acc, product) => ({ ...acc, [product.id]: 0 }),
  {} as Allocations
)

function fmtU(value: number) {
  return `${value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })} U`
}

function clampMoney(value: number, max: number) {
  if (!Number.isFinite(value)) return 0
  return Math.min(Math.max(value, 0), Math.max(max, 0))
}

export function AutoArbitragePage() {
  const rootRef = useRef<HTMLDivElement>(null)
  const { user } = useAuth()
  const { data: wallet } = useSWR(user ? 'auto-arbitrage-wallet' : null, () => api.getWallet(), {
    refreshInterval: 15000,
    revalidateOnFocus: true,
  })
  const walletBalance = wallet?.balance_usdt ?? user?.balance_usdt ?? 0
  const canUseInternalPage = user?.email?.toLowerCase() === ALLOWED_EMAIL
  const storageKey = `auto_arbitrage_${user?.email || 'guest'}`
  const [financeTotal, setFinanceTotal] = useState(0)
  const [allocations, setAllocations] = useState<Allocations>(emptyAllocations)
  const [deployingProduct, setDeployingProduct] = useState<ProductId | null>(null)
  const [deployAmount, setDeployAmount] = useState('')
  const [transferOpen, setTransferOpen] = useState(false)
  const [transferDirection, setTransferDirection] = useState<TransferDirection>('in')
  const [transferAmount, setTransferAmount] = useState('')

  const investedTotal = useMemo(
    () => PRODUCTS.reduce((sum, product) => sum + (allocations[product.id] || 0), 0),
    [allocations]
  )
  const available = Math.max(financeTotal - investedTotal, 0)
  const transferInMax = Math.max(walletBalance - financeTotal, 0)
  const selectedProduct = PRODUCTS.find((item) => item.id === deployingProduct)
  const transferMax = transferDirection === 'in' ? transferInMax : available

  useEffect(() => {
    if (!user) return
    try {
      const raw = localStorage.getItem(storageKey)
      if (raw) {
        const saved = JSON.parse(raw) as Partial<{ financeTotal: number; allocations: Partial<Allocations> }>
        setFinanceTotal(clampMoney(Number(saved.financeTotal ?? 0), walletBalance))
        setAllocations({ ...emptyAllocations, ...saved.allocations })
        return
      }
    } catch {
      /* ignore */
    }
    setFinanceTotal(0)
    setAllocations(emptyAllocations)
  }, [storageKey, user, walletBalance])

  useEffect(() => {
    if (!canUseInternalPage) return
    // ponytail: frontend-only wallet partition; replace with backend ledger when real transfers go live.
    localStorage.setItem(storageKey, JSON.stringify({ financeTotal, allocations }))
  }, [allocations, canUseInternalPage, financeTotal, storageKey])

  useEffect(() => {
    const root = rootRef.current
    if (!root) return
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const ctx = gsap.context(() => {
      gsap.set('.arb-reveal', { autoAlpha: 0, y: reduceMotion ? 0 : 18 })
      gsap.timeline({ defaults: { ease: 'power3.out', duration: reduceMotion ? 0 : 0.65 } })
        .to('.arb-reveal', { autoAlpha: 1, y: 0, stagger: 0.055 })
      if (!reduceMotion) {
        gsap.to('.arb-orbit', { rotation: 360, duration: 28, repeat: -1, ease: 'none' })
        gsap.to('.arb-pulse', { scale: 1.08, autoAlpha: 0.65, duration: 1.8, repeat: -1, yoyo: true, ease: 'sine.inOut' })
      }
    }, root)
    return () => ctx.revert()
  }, [canUseInternalPage])

  useEffect(() => {
    if (!transferOpen && !selectedProduct) return
    const root = rootRef.current
    if (!root) return
    const ctx = gsap.context(() => {
      gsap.fromTo('.arb-modal', { autoAlpha: 0, y: 18, scale: 0.98 }, { autoAlpha: 1, y: 0, scale: 1, duration: 0.24, ease: 'power2.out' })
    }, root)
    return () => ctx.revert()
  }, [selectedProduct, transferOpen])

  const confirmDeploy = () => {
    if (!deployingProduct) return
    const otherInvested = investedTotal - (allocations[deployingProduct] || 0)
    setAllocations((prev) => ({ ...prev, [deployingProduct]: clampMoney(Number(deployAmount), financeTotal - otherInvested) }))
    setDeployingProduct(null)
    setDeployAmount('')
  }

  const confirmTransfer = () => {
    const safeAmount = clampMoney(Number(transferAmount), transferMax)
    if (safeAmount <= 0) return
    setFinanceTotal((prev) => prev + (transferDirection === 'in' ? safeAmount : -safeAmount))
    setTransferOpen(false)
    setTransferAmount('')
  }

  const openDeploy = (id: ProductId) => {
    setDeployingProduct(id)
    setDeployAmount(String(allocations[id] || ''))
  }

  const gated = (
    <div className="arb-reveal overflow-hidden rounded-[28px] border border-[#d4ff33]/20 bg-[#111114]/95 shadow-[0_24px_90px_rgba(0,0,0,0.45)]">
      <div className="relative px-6 py-10 sm:px-10">
        <div className="arb-pulse absolute right-0 top-0 h-56 w-56 rounded-full bg-[#d4ff33]/10 blur-3xl" />
        <div className="relative max-w-2xl">
          <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-[#d4ff33]/25 bg-[#d4ff33]/10 px-3 py-1 text-xs font-bold text-[#d4ff33]">
            <LockKeyhole className="h-3.5 w-3.5" /> 实验室内测模块
          </div>
          <h1 className="text-3xl font-black text-white sm:text-5xl">全自动套利系统</h1>
          <p className="mt-4 text-sm leading-7 text-[#9d9daa]">正在开发中，后续陆续上线。当前仅开放内部测试账户查看套利账户与策略池分配面板。</p>
        </div>
      </div>
      <div className="grid gap-3 border-t border-white/8 p-5 sm:grid-cols-2 lg:grid-cols-4">
        {PRODUCTS.map((product) => <ProductMini key={product.id} product={product} />)}
      </div>
    </div>
  )

  return (
    <div ref={rootRef} className="min-h-[calc(100vh-64px)] overflow-hidden bg-[#07080a] px-3 py-6 text-[#e8e8ec] sm:px-4 sm:py-8">
      <div className="pointer-events-none fixed inset-0 bg-[linear-gradient(rgba(212,255,51,0.035)_1px,transparent_1px),linear-gradient(90deg,rgba(212,255,51,0.03)_1px,transparent_1px)] bg-[size:56px_56px]" />
      <div className="relative mx-auto max-w-7xl space-y-5">
        {!canUseInternalPage ? gated : (
          <>
            <section className="arb-reveal relative overflow-hidden rounded-[32px] border border-[#d4ff33]/20 bg-[#101114]/95 p-6 shadow-[0_28px_100px_rgba(0,0,0,0.46)] sm:p-8">
              <div className="arb-pulse absolute -right-20 -top-24 h-80 w-80 rounded-full bg-[#d4ff33]/12 blur-3xl" />
              <div className="relative grid gap-8 lg:grid-cols-[1.15fr_0.85fr] lg:items-center">
                <div>
                  <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-[#d4ff33]/25 bg-[#d4ff33]/10 px-3 py-1 text-xs font-black text-[#d4ff33]">
                    <Activity className="h-3.5 w-3.5" /> Arbitrage Command Center
                  </div>
                  <h1 className="max-w-3xl text-4xl font-black tracking-[-0.02em] text-white sm:text-6xl">全自动套利系统</h1>
                  <div className="mt-6 flex flex-wrap gap-2">
                    <Badge icon={ShieldCheck} label="账户隔离" />
                    <Badge icon={ArrowRightLeft} label="资金划转" />
                    <Badge icon={BarChart3} label="90天测算" />
                  </div>
                </div>
                <div className="relative min-h-[220px] rounded-[28px] border border-white/10 bg-black/25 p-4 sm:p-5">
                  <div className="arb-orbit absolute left-1/2 top-1/2 h-44 w-44 -translate-x-1/2 -translate-y-1/2 rounded-full border border-[#d4ff33]/20" />
                  <div className="absolute left-1/2 top-1/2 h-28 w-28 -translate-x-1/2 -translate-y-1/2 rounded-full border border-[#d4ff33]/30 bg-[#d4ff33]/8 blur-[0.2px]" />
                  <div className="relative z-10 grid h-full grid-cols-2 gap-3">
                    <HeroMetric label="理财总览" value={fmtU(financeTotal)} tone="text-[#d4ff33]" />
                    <HeroMetric label="可用余额" value={fmtU(available)} tone="text-white" />
                    <HeroMetric label="已部署" value={fmtU(investedTotal)} tone="text-white" />
                    <HeroMetric label="90天预估" value={`+${fmtU(investedTotal * RATE_90D)}`} tone="text-emerald-300" />
                  </div>
                </div>
              </div>
            </section>

            <section className="grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
              <div className="arb-reveal rounded-[28px] border border-white/10 bg-[#111114]/95 p-5">
                <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <h2 className="text-lg font-black text-white">钱包资产</h2>
                    <div className="mt-1 text-xs text-[#7b8190]">各板块独立占用，已部署资金不能跨板块使用。</div>
                  </div>
                  <button type="button" onClick={() => setTransferOpen(true)} className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-xl border border-[#d4ff33]/30 bg-[#d4ff33]/10 px-3 text-xs font-black text-[#d4ff33] hover:bg-[#d4ff33]/15 sm:w-auto">
                    <ArrowRightLeft className="h-4 w-4" /> 资金划转
                  </button>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <AssetTile label="理财账户" value={financeTotal} hint={`可用 ${fmtU(available)}`} active />
                  {PRODUCTS.map((product) => <AssetTile key={product.id} label={product.title} value={allocations[product.id] || 0} />)}
                </div>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                {PRODUCTS.map((product) => {
                  const invested = allocations[product.id] || 0
                  return <ProductCard key={product.id} product={product} invested={invested} financeTotal={financeTotal} onDeploy={openDeploy} />
                })}
              </div>
            </section>
          </>
        )}
      </div>

      {transferOpen && (
        <Modal title="资金划转" subtitle="理财账户与平台钱包隔离显示。" onClose={() => setTransferOpen(false)}>
          <div className="mb-4 grid grid-cols-2 gap-2 rounded-2xl bg-black/20 p-1">
            {(['in', 'out'] as TransferDirection[]).map((id) => (
              <button key={id} type="button" onClick={() => setTransferDirection(id)} className={`rounded-xl py-2 text-sm font-bold ${transferDirection === id ? 'bg-[#d4ff33] text-black' : 'text-[#9d9daa] hover:text-white'}`}>
                {id === 'in' ? '转入理财' : '转出理财'}
              </button>
            ))}
          </div>
          <AmountInput label="划转金额" value={transferAmount} max={transferMax} onChange={setTransferAmount} />
          {transferDirection === 'out' ? <div className="mt-2 text-xs text-[#7b8190]">理财可用 {fmtU(transferMax)}</div> : null}
          <PrimaryButton onClick={confirmTransfer}>确认划转</PrimaryButton>
        </Modal>
      )}

      {selectedProduct && (
        <Modal title={selectedProduct.title} subtitle="输入该板块独立投入金额。" onClose={() => setDeployingProduct(null)}>
          <AmountInput label="投入金额" value={deployAmount} max={financeTotal} onChange={setDeployAmount} />
          <div className="mt-2 text-xs text-[#7b8190]">理财可用 {fmtU(available)}</div>
          <PrimaryButton onClick={confirmDeploy}>确认部署</PrimaryButton>
        </Modal>
      )}
    </div>
  )
}

function Badge({ icon: Icon, label }: { icon: typeof ShieldCheck; label: string }) {
  return <span className="inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1.5 text-xs font-bold text-[#d9dee7]"><Icon className="h-3.5 w-3.5 text-[#d4ff33]" />{label}</span>
}

function HeroMetric({ label, value, tone }: { label: string; value: string; tone: string }) {
  return <div className="rounded-2xl border border-white/10 bg-[#111114]/80 p-4"><div className="text-xs text-[#7b8190]">{label}</div><div className={`mt-2 break-words text-base font-black sm:text-lg ${tone}`}>{value}</div></div>
}

function AssetTile({ label, value, hint, active = false }: { label: string; value: number; hint?: string; active?: boolean }) {
  return <div className={`arb-reveal rounded-2xl border p-4 ${active ? 'border-[#d4ff33]/25 bg-[#d4ff33]/10' : 'border-white/10 bg-black/20'}`}><div className="text-xs text-[#9d9daa]">{label}</div><div className={`mt-2 break-words text-lg font-black sm:text-xl ${active ? 'text-[#d4ff33]' : 'text-white'}`}>{fmtU(value)}</div>{hint ? <div className="mt-1 text-xs text-[#7b8190]">{hint}</div> : null}</div>
}

function ProductMini({ product }: { product: (typeof PRODUCTS)[number] }) {
  const Icon = product.icon
  return <div className="arb-reveal rounded-2xl border border-white/10 bg-white/[0.03] p-4"><Icon className="mb-4 h-5 w-5 text-[#d4ff33]" /><div className="font-semibold text-white">{product.title}</div><div className="mt-2 text-xs text-[#7b8190]">预估90天收益率 30%</div></div>
}

function ProductCard({ product, invested, financeTotal, onDeploy }: { product: (typeof PRODUCTS)[number]; invested: number; financeTotal: number; onDeploy: (id: ProductId) => void }) {
  const Icon = product.icon
  const pct = financeTotal > 0 ? (invested / financeTotal) * 100 : 0
  return <article className="arb-reveal group rounded-[24px] border border-white/10 bg-[#111114]/95 p-5 shadow-[0_14px_50px_rgba(0,0,0,0.24)] transition-colors hover:border-[#d4ff33]/30"><div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><div className="flex items-center gap-3"><div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl border border-[#d4ff33]/25 bg-[#d4ff33]/10"><Icon className="h-5 w-5 text-[#d4ff33]" /></div><div className="min-w-0"><h2 className="text-lg font-black text-white">{product.title}</h2><div className="mt-1 text-xs text-[#7b8190]">预估90天收益率 30%</div></div></div><button type="button" onClick={() => onDeploy(product.id)} className="inline-flex h-9 w-full items-center justify-center gap-1.5 rounded-xl bg-[#d4ff33] px-3 text-xs font-black text-black hover:bg-[#e4ff64] sm:w-auto"><Plus className="h-3.5 w-3.5" />{invested > 0 ? '调整' : '部署'}</button></div><div className="mt-5 grid gap-3 sm:grid-cols-3"><Metric label="投入金额" value={fmtU(invested)} /><Metric label="90天预估" value={`+${fmtU(invested * RATE_90D)}`} green /><Metric label="账户占比" value={`${pct.toFixed(1)}%`} yellow /></div><div className="mt-4 h-1.5 overflow-hidden rounded-full bg-white/8"><div className="h-full rounded-full bg-[#d4ff33]" style={{ width: `${Math.min(pct, 100)}%` }} /></div></article>
}

function Metric({ label, value, green, yellow }: { label: string; value: string; green?: boolean; yellow?: boolean }) {
  return <div className="rounded-2xl bg-black/20 p-3"><div className="text-xs text-[#7b8190]">{label}</div><div className={`mt-1 break-words text-sm font-black sm:text-base ${green ? 'text-emerald-300' : yellow ? 'text-[#d4ff33]' : 'text-white'}`}>{value}</div></div>
}

function Modal({ title, subtitle, onClose, children }: { title: string; subtitle: string; onClose: () => void; children: React.ReactNode }) {
  return <div className="fixed inset-0 z-[70] flex items-start justify-center overflow-y-auto bg-black/70 px-3 py-6 backdrop-blur-sm sm:items-center sm:px-4"><div className="arb-modal w-full max-w-md rounded-3xl border border-white/10 bg-[#111114] p-5 shadow-2xl"><div className="mb-5 flex items-start justify-between gap-3"><div><h3 className="text-xl font-black text-white">{title}</h3><p className="mt-1 text-xs text-[#7b8190]">{subtitle}</p></div><button type="button" onClick={onClose} className="shrink-0 rounded-full p-2 text-[#9d9daa] hover:bg-white/10 hover:text-white"><X className="h-4 w-4" /></button></div>{children}</div></div>
}

function AmountInput({ label, value, max, onChange }: { label: string; value: string; max: number; onChange: (value: string) => void }) {
  return <label className="block text-xs font-semibold text-[#9d9daa]">{label}<input value={value} onChange={(e) => onChange(e.target.value)} type="number" min="0" max={max} step="1" autoFocus className="mt-2 h-12 w-full rounded-xl border border-white/10 bg-black/30 px-3 text-sm font-bold text-white outline-none focus:border-[#d4ff33]/60" /></label>
}

function PrimaryButton({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} className="mt-5 h-11 w-full rounded-xl bg-[#d4ff33] text-sm font-black text-black hover:bg-[#e4ff64]">{children}</button>
}
