import { t, type Language } from '../../i18n/translations'

interface SiteFooterProps {
  language: Language
}

export function SiteFooter({ language }: SiteFooterProps) {
  return (
    <footer className="mt-auto border-t border-white/[0.06] bg-nofx-bg-tertiary">
      <div className="mx-auto max-w-[1920px] px-6 py-8 text-center text-sm text-nofx-text-muted">
        <p className="text-nofx-text-muted">{t('footerTitle', language)}</p>
        <p className="mt-1 text-nofx-text-muted/90">{t('footerWarning', language)}</p>
      </div>
    </footer>
  )
}
