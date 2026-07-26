import React, { useEffect, useId, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { useAuth } from '../../contexts/AuthContext'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { ROUTES } from '../../router/paths'
import './crypto-login.css'

export function LoginPage() {
  const uid = useId()
  const { language } = useLanguage()
  const { login } = useAuth()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [shake, setShake] = useState(false)
  const [expiredToastId, setExpiredToastId] = useState<string | number | null>(null)

  const nodes = useMemo(() => {
    return Array.from({ length: 18 }, (_, i) => ({
      id: `${uid}-n-${i}`,
      x: 5 + Math.random() * 90,
      y: 5 + Math.random() * 90,
      delay: Math.random() * 3,
      dur: 2 + Math.random() * 4,
    }))
  }, [uid])

  const candles = useMemo(() => {
    return Array.from({ length: 24 }, (_, i) => ({
      id: `${uid}-c-${i}`,
      left: 2 + Math.random() * 96,
      h: 20 + Math.random() * 80,
      dur: 6 + Math.random() * 12,
      delay: Math.random() * 8,
    }))
  }, [uid])

  useEffect(() => {
    document.title = 'COMKUN-AI · 登录'
  }, [])

  useEffect(() => {
    localStorage.removeItem('auth_token')
    localStorage.removeItem('auth_user')
    localStorage.removeItem('user_id')
  }, [])

  useEffect(() => {
    if (sessionStorage.getItem('from401') === 'true') {
      const id = toast.warning(t('sessionExpired', language), { duration: Infinity })
      setExpiredToastId(id)
      sessionStorage.removeItem('from401')
    }
  }, [language])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    const em = email.trim()
    if (!em || !password || password.length < 6) {
      setShake(true)
      window.setTimeout(() => setShake(false), 500)
      return
    }
    setLoading(true)
    const result = await login(em, password)
    setLoading(false)
    if (result.success) {
      if (expiredToastId) toast.dismiss(expiredToastId)
    } else {
      const msg = result.message || t('loginFailed', language)
      setError(msg)
      toast.error(msg)
      setShake(true)
      window.setTimeout(() => setShake(false), 500)
    }
  }

  return (
    <div className="crypto-login-root">
      <div className="crypto-login-grid-bg" aria-hidden />
      <div className="crypto-login-scanline" aria-hidden />

      <div className="crypto-login-nodes-layer" aria-hidden>
        {nodes.map((n) => (
          <div
            key={n.id}
            className="crypto-login-node"
            style={{
              left: `${n.x}%`,
              top: `${n.y}%`,
              animationDelay: `${n.delay}s`,
              animationDuration: `${n.dur}s`,
            }}
          />
        ))}
      </div>
      <div className="crypto-login-chart-bars" aria-hidden>
        {candles.map((b) => (
          <div
            key={b.id}
            className="crypto-login-candle"
            style={{
              left: `${b.left}%`,
              height: `${b.h}px`,
              animationDuration: `${b.dur}s`,
              animationDelay: `${b.delay}s`,
            }}
          />
        ))}
      </div>

      <div className={`crypto-login-card${shake ? ' crypto-login-shake' : ''}`}>
        <div className="crypto-login-card-accent" />
        <div className="crypto-login-card-body">
          <div className="crypto-login-logo-area">
            <div className="crypto-login-coin-logo" />
            <span className="crypto-login-logo-text">COMKUN-AI</span>
          </div>

          <div className="crypto-login-title">欢迎回来</div>
          <div className="crypto-login-subtitle">新时代 · AI量化 · 交易系统</div>

          <form onSubmit={handleSubmit} autoComplete="on">
            <div className="crypto-login-input-group">
              <label htmlFor="crypto-login-email">邮箱</label>
              <div className="crypto-login-input-wrap">
                <input
                  id="crypto-login-email"
                  type="email"
                  name="email"
                  inputMode="email"
                  autoComplete="username"
                  autoCapitalize="none"
                  spellCheck={false}
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="请输入邮箱地址"
                  required
                />
              </div>
            </div>

            <div className="crypto-login-input-group">
              <label htmlFor="crypto-login-password">密码</label>
              <div className="crypto-login-input-wrap">
                <input
                  id="crypto-login-password"
                  type="password"
                  name="password"
                  autoComplete="current-password"
                  autoCapitalize="none"
                  spellCheck={false}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="请输入登录密码"
                  required
                  minLength={6}
                />
              </div>
            </div>

            {error ? <div className="crypto-login-err">{error}</div> : null}

            <button type="submit" className="crypto-login-btn" disabled={loading}>
              <span>{loading ? t('loggingIn', language) || '登录中…' : '登录'}</span>
            </button>
          </form>

          <div className="crypto-login-extras">
            <Link to={ROUTES.register}>注册</Link>
            <Link to={ROUTES.resetPassword}>忘记密码</Link>
          </div>

          <div className="crypto-login-terminal-line">
            COMKUN-AI-交易系统-就绪<span className="crypto-login-terminal-blink" />
          </div>
        </div>
      </div>
    </div>
  )
}
