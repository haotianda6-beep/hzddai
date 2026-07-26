/**
 * 邀请返利 VIP 角标（V0～V5）：黑铁 / 青铜 / 白银 / 黄金 / 翡翠 / 钻石（渐变 + 内高光 + 边框）
 */
export function VipTierBadge({ level }: { level: number }) {
  if (level < 0 || level > 5) return null

  const styles: Record<
    number,
    { label: string; title: string; className: string }
  > = {
    0: {
      label: 'V0',
      title: 'VIP0 · 黑铁',
      className:
        'border-zinc-600/90 bg-gradient-to-br from-[#0a0a0a] via-[#2a2a2a] to-[#050505] text-[#c4c4c4] shadow-[inset_0_1px_1px_rgba(255,255,255,0.12)] ring-1 ring-black/70',
    },
    1: {
      label: 'V1',
      title: 'VIP1 · 青铜',
      className:
        'border-[#6b4f2b]/80 bg-gradient-to-br from-[#5c3d1e] via-[#a06628] to-[#4a3220] text-[#fff5e0] shadow-[inset_0_1px_1px_rgba(255,255,255,0.28)] ring-1 ring-amber-950/40',
    },
    2: {
      label: 'V2',
      title: 'VIP2 · 白银',
      className:
        'border-white/35 bg-gradient-to-br from-[#6b7280] via-[#d1d5db] to-[#9ca3af] text-[#111827] shadow-[inset_0_1px_1px_rgba(255,255,255,0.65)] ring-1 ring-slate-400/50',
    },
    3: {
      label: 'V3',
      title: 'VIP3 · 黄金',
      className:
        'border-[#fbbf24]/55 bg-gradient-to-br from-[#ca8a04] via-[#facc15] to-[#a16207] text-[#422006] shadow-[inset_0_1px_1px_rgba(255,255,255,0.45)] ring-1 ring-amber-300/60',
    },
    4: {
      label: 'V4',
      title: 'VIP4 · 翡翠',
      className:
        'border-emerald-300/45 bg-gradient-to-br from-[#065f46] via-[#059669] to-[#064e3b] text-[#ecfdf5] shadow-[inset_0_1px_1px_rgba(255,255,255,0.22),0_0_14px_rgba(16,185,129,0.35)] ring-1 ring-emerald-400/45',
    },
    5: {
      label: 'V5',
      title: 'VIP5 · 钻石',
      className:
        'border-cyan-100/70 bg-gradient-to-br from-[#f0f9ff] via-[#7dd3fc] to-[#38bdf8] text-[#0c4a6e] shadow-[inset_0_1px_1px_rgba(255,255,255,0.85),0_0_16px_rgba(125,211,252,0.55)] ring-1 ring-sky-200/80',
    },
  }

  const s = styles[level]
  const diamond = level === 5 ? ' vip-tier-badge--diamond' : ''

  return (
    <span
      title={s.title}
      className={`vip-tier-badge relative inline-flex shrink-0 items-center justify-center rounded-md border px-1.5 py-0.5 text-[10px] font-bold leading-none tracking-tight ${s.className}${diamond}`}
    >
      {/* 斜向高光，增强金属 / 宝石质感 */}
      <span
        className="pointer-events-none absolute inset-0 rounded-md opacity-[0.22]"
        style={{
          background:
            'linear-gradient(125deg, transparent 35%, rgba(255,255,255,0.85) 48%, transparent 62%)',
        }}
        aria-hidden
      />
      <span className="relative z-[1]">{s.label}</span>
    </span>
  )
}
