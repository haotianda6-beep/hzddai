import { ConfirmDialogProvider } from './components/common/ConfirmDialog'
import { LiveChatWidget } from './components/common/LiveChatWidget'
import { FirstLoginNameModal } from './components/common/FirstLoginNameModal'
import { GlobalGsapMotion } from './components/common/GlobalGsapMotion'
import { AccountLockModal } from './components/common/AccountLockModal'
import { AuthProvider } from './contexts/AuthContext'
import { LanguageProvider } from './contexts/LanguageContext'
import { AppRoutes } from './router/AppRoutes'

export default function App() {
  return (
    <LanguageProvider>
      <AuthProvider>
        <ConfirmDialogProvider>
          <GlobalGsapMotion />
          <AppRoutes />
          <FirstLoginNameModal />
          <AccountLockModal />
          <LiveChatWidget />
        </ConfirmDialogProvider>
      </AuthProvider>
    </LanguageProvider>
  )
}
