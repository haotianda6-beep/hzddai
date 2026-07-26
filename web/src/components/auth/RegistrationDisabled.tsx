import { useNavigate } from 'react-router-dom'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'

export function RegistrationDisabled() {
  const { language } = useLanguage()
  const navigate = useNavigate()

  const handleBackToLogin = () => {
    navigate('/login')
  }

  return (
    <div
      className="min-h-screen flex items-center justify-center"
      style={{ background: '#0b0b0b', color: '#EAECEF' }}
    >
      <div className="max-w-md px-6 text-center">
        <div className="mb-4 flex items-center justify-center gap-2.5 text-2xl font-semibold leading-none">
          <img
            src="/icons/comkun-logo.png"
            alt=""
            data-testid="brand-logo"
            className="h-10 w-10 shrink-0 rounded-lg object-cover sm:h-11 sm:w-11"
            aria-hidden
          />
          <h1 className="font-semibold leading-none">{t('registrationClosed', language)}</h1>
        </div>
        <p className="text-sm text-gray-400">
          {t('registrationClosedMessage', language)}
        </p>
        <button
          className="mt-6 px-4 py-2 rounded text-sm font-semibold transition-colors hover:opacity-90"
          style={{ background: '#F0B90B', color: '#000' }}
          onClick={handleBackToLogin}
        >
          {t('backToLogin', language)}
        </button>
      </div>
    </div>
  )
}
