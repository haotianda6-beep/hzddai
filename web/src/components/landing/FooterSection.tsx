import { ExternalLink } from 'lucide-react'
import { t, Language } from '../../i18n/translations'

interface FooterSectionProps {
  language: Language
}

export default function FooterSection({ language }: FooterSectionProps) {
  const links = {
    resources: [
      {
        name: language === 'zh' ? '文档' : 'Documentation',
        href: 'https://github.com/NoFxAiOS/nofx/blob/main/README.md',
      },
      { name: 'Issues', href: 'https://github.com/NoFxAiOS/nofx/issues' },
      { name: 'Pull Requests', href: 'https://github.com/NoFxAiOS/nofx/pulls' },
    ],
    supporters: [
      { name: 'Binance', href: 'https://www.binance.com/join?ref=COMKUNENG' },
      { name: 'Bybit', href: 'https://partner.bybit.com/b/83856' },
      { name: 'OKX', href: 'https://www.okx.com/join/1865360' },
      { name: 'Bitget', href: 'https://www.bitget.com/referral/register?from=referral&clacCode=c8a43172' },
      { name: 'Gate.io', href: 'https://www.gatenode.xyz/share/VQBGUAxY' },
      { name: 'KuCoin', href: 'https://www.kucoin.com/r/broker/CXEV7XKK' },
      { name: 'Hyperliquid', href: 'https://app.hyperliquid.xyz/join/AITRADING' },
      { name: 'Aster DEX', href: 'https://www.asterdex.com/en/referral/fdfc0e' },
      { name: 'Lighter', href: 'https://app.lighter.xyz/?referral=68151432' },
    ],
  }

  return (
    <footer style={{ background: '#131313', borderTop: '1px solid rgba(255, 255, 255, 0.06)' }}>
      <div className="max-w-6xl mx-auto px-4 py-8 md:py-12">
        {/* Top Section */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-8 md:gap-10 mb-8 md:mb-12">
          {/* Brand */}
          <div className="md:col-span-1">
            <div
              className="mb-4 flex items-center gap-2.5 text-xl font-bold leading-none"
              style={{ color: '#EAECEF' }}
            >
              <img
                src="/icons/comkun-logo.png"
                alt=""
                className="h-9 w-9 shrink-0 rounded-lg object-cover"
                aria-hidden
              />
              <span>COMKUN-AI</span>
            </div>
            <p className="text-sm mb-6" style={{ color: '#5E6673' }}>
              {t('futureStandardAI', language)}
            </p>
          </div>

          {/* Resources */}
          <div>
            <h4 className="text-sm font-semibold mb-4" style={{ color: '#EAECEF' }}>
              {t('resources', language)}
            </h4>
            <ul className="space-y-3">
              {links.resources.map((link) => (
                <li key={link.name}>
                  <a
                    href={link.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-sm transition-colors hover:text-[#F0B90B] inline-flex items-center gap-1"
                    style={{ color: '#5E6673' }}
                  >
                    {link.name}
                    <ExternalLink className="w-3 h-3 opacity-50" />
                  </a>
                </li>
              ))}
            </ul>
          </div>

          {/* Supporters */}
          <div>
            <h4 className="text-sm font-semibold mb-4" style={{ color: '#EAECEF' }}>
              {t('supporters', language)}
            </h4>
            <div className="flex flex-wrap gap-2">
              {links.supporters.map((link) => (
                <a
                  key={link.name}
                  href={link.href}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-xs border border-zinc-800 bg-zinc-900/50 rounded px-3 py-1.5 transition-all hover:border-[#F0B90B] hover:text-[#F0B90B] hover:bg-[#F0B90B]/10 hover:shadow-[0_0_10px_rgba(240,185,11,0.2)]"
                  style={{ color: '#848E9C' }}
                >
                  {link.name}
                </a>
              ))}
            </div>
          </div>
        </div>

        {/* Bottom Section */}
        <div
          className="pt-6 text-center text-xs"
          style={{ color: '#5E6673', borderTop: '1px solid rgba(255, 255, 255, 0.06)' }}
        >
          <p className="mb-2">{t('footerTitle', language)}</p>
          <p style={{ color: '#3C4249' }}>{t('footerWarning', language)}</p>
        </div>
      </div>
    </footer>
  )
}
