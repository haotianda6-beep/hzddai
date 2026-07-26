import { useNavigate } from 'react-router-dom'
import HeaderBar from '../components/common/HeaderBar'
import { useAuth } from '../contexts/AuthContext'
import { LuminescentHome } from './landing/LuminescentHome'
import { ROUTES } from '../router/paths'
import './landing/luminescent.css'

/** 首页：与静态 crypto-home.html 1:1（芥末绿暗色 + 顶栏固定） */
export function LandingPage() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  return (
    <div className="min-h-screen bg-[#0a0a0a] font-lumbody selection:bg-[#c5d83e]/30 selection:text-black">
      <HeaderBar
        isHomePage
        isLoggedIn={!!user}
        user={user}
        onLogout={logout}
        onLoginRequired={() => navigate(ROUTES.login)}
      />
      <LuminescentHome />
    </div>
  )
}
