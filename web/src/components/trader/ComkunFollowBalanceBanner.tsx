import useSWR from 'swr'
import { getComkunFollowBalance } from '../../lib/api/comkunFollow'
import type { Language } from '../../i18n/translations'

function formatUSDT(value: number, language: Language): string {
  return value.toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', {
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  })
}

/** COMKUN 同步策略：展示站内账户余额，扣费统一从这里走。 */
export function ComkunFollowBalanceBanner({
  traderId,
  language,
}: {
  traderId: string
  language: Language
}) {
  const { data, error } = useSWR(
    traderId ? ['comkun-follow-balance', traderId] : null,
    () => getComkunFollowBalance(traderId),
    { refreshInterval: 15000 }
  )
  const bal = data?.balance_usdt
  const fee = data?.scan_fee_usdt
  const feeMin = data?.scan_fee_min_usdt
  const feeMax = data?.scan_fee_max_usdt
  const balLabel =
    error || bal === undefined || bal === null ? '—' : `${formatUSDT(bal, language)} USDT`
  const feeLabel =
    error
      ? '—'
      : feeMin !== undefined && feeMax !== undefined
        ? `${formatUSDT(feeMin, language)}-${formatUSDT(feeMax, language)} USDT / 次`
        : fee === undefined || fee === null
          ? '—'
          : `${formatUSDT(fee, language)} USDT / 次`

  return (
    <div className="rounded-xl border border-amber-500/35 bg-gradient-to-r from-amber-950/50 to-[#1a2210]/80 px-4 py-3">
      <div className="flex items-end justify-between gap-3">
        <div>
          <div className="text-xs font-semibold text-amber-100/70">
            {language === 'zh' ? '站内账户余额' : 'Wallet balance'}
          </div>
          <span className="font-mono text-2xl font-bold tabular-nums leading-none text-[#d4ff33]">{balLabel}</span>
        </div>
        <div className="text-right text-xs font-medium text-amber-100/80">
          <div>{language === 'zh' ? '同步扣费' : 'Sync fee'}</div>
          <div className="font-mono text-sm text-amber-100">{feeLabel}</div>
        </div>
      </div>
    </div>
  )
}
