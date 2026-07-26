import React, {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useState,
} from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import PasswordChecklist from 'react-password-checklist'
import { toast } from 'sonner'
import { useAuth } from '../../contexts/AuthContext'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { ROUTES } from '../../router/paths'
import './crypto-login.css'
import './crypto-register.css'
import './crypto-recover.css'

type SparkParticle = {
  id: string
  left: string
  top: string
  dx: string
  dy: string
  dur: string
  size: string
}

const CELEBRATION_MS = 2400

export function ResetPasswordPage() {
  const uid = useId()
  const { language } = useLanguage()
  const { resetPassword, sendResetPasswordEmailCode } = useAuth()
  const navigate = useNavigate()

  const [step, setStep] = useState(1)
  const [email, setEmail] = useState('')
  const [emailCode, setEmailCode] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [sendingCode, setSendingCode] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [passwordValid, setPasswordValid] = useState(false)
  const [codeCooldown, setCodeCooldown] = useState(0)
  const [shake, setShake] = useState(false)
  const [celebrate, setCelebrate] = useState(false)
  const [sparks, setSparks] = useState<SparkParticle[]>([])

  const nodes = useMemo(
    () =>
      Array.from({ length: 18 }, (_, i) => ({
        id: `${uid}-n-${i}`,
        x: 5 + Math.random() * 90,
        y: 5 + Math.random() * 90,
        delay: Math.random() * 3,
        dur: 2 + Math.random() * 4,
      })),
    [uid]
  )

  const candles = useMemo(
    () =>
      Array.from({ length: 24 }, (_, i) => ({
        id: `${uid}-c-${i}`,
        left: 2 + Math.random() * 96,
        h: 20 + Math.random() * 80,
        dur: 6 + Math.random() * 12,
        delay: Math.random() * 8,
      })),
    [uid]
  )

  useEffect(() => {
    document.title = 'COMKUN-AI · 找回密码'
  }, [])

  useEffect(() => {
    if (codeCooldown <= 0) return
    const tmr = window.setInterval(() => {
      setCodeCooldown((s) => (s <= 1 ? 0 : s - 1))
    }, 1000)
    return () => window.clearInterval(tmr)
  }, [codeCooldown])

  const triggerShake = useCallback(() => {
    setShake(true)
    window.setTimeout(() => setShake(false), 500)
  }, [])

  const goStep = useCallback((n: number) => {
    setStep(n)
    setError('')
  }, [])

  useEffect(() => {
    if (!celebrate) return
    const handles: number[] = []
    for (let i = 0; i < 40; i++) {
      handles.push(
        window.setTimeout(() => {
          const angle = Math.random() * Math.PI * 2
          const dist = 40 + Math.random() * 120
          const id = `rsp-${Date.now()}-${i}`
          const sp: SparkParticle = {
            id,
            left: `${42 + Math.random() * 16}%`,
            top: `${38 + Math.random() * 18}%`,
            dx: `${Math.cos(angle) * dist}px`,
            dy: `${Math.sin(angle) * dist}px`,
            dur: `${0.55 + Math.random() * 0.55}s`,
            size: `${1 + Math.random() * 3}px`,
          }
          setSparks((prev) => [...prev, sp])
          window.setTimeout(() => {
            setSparks((prev) => prev.filter((x) => x.id !== id))
          }, 1300)
        }, i * 15)
      )
    }
    return () => handles.forEach((h) => window.clearTimeout(h))
  }, [celebrate])

  useEffect(() => {
    if (!celebrate) return
    const t = window.setTimeout(() => {
      navigate(ROUTES.login)
    }, CELEBRATION_MS)
    return () => window.clearTimeout(t)
  }, [celebrate, navigate])

  const handleSendCode = useCallback(async () => {
    setError('')
    const trimmed = email.trim()
    if (!trimmed || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)) {
      toast.warning('请先填写有效邮箱')
      goStep(1)
      triggerShake()
      return
    }
    setSendingCode(true)
    const r = await sendResetPasswordEmailCode(trimmed)
    setSendingCode(false)
    if (r.success) {
      toast.success(r.message || '验证码已发送')
      if (r.hint) toast.info(r.hint)
      setCodeCooldown(60)
    } else {
      const msg = r.message || '发送失败'
      if (r.retryAfterSec != null && r.retryAfterSec > 0) {
        setCodeCooldown(r.retryAfterSec)
      }
      setError(msg)
      toast.error(msg)
    }
  }, [email, goStep, sendResetPasswordEmailCode, triggerShake])

  const onStep1Next = () => {
    setError('')
    const trimmed = email.trim()
    if (!trimmed || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)) {
      setError('请先填写有效邮箱')
      triggerShake()
      return
    }
    goStep(2)
  }

  const onStep2Next = () => {
    setError('')
    const code = emailCode.trim()
    if (code.length !== 6 || !/^\d{6}$/.test(code)) {
      setError('请填写 6 位数字邮箱验证码')
      triggerShake()
      return
    }
    goStep(3)
  }

  const submitReset = async () => {
    if (celebrate) return
    setError('')

    if (!passwordValid) {
      setError(t('passwordNotMeetRequirements', language))
      triggerShake()
      return
    }

    const code = emailCode.trim()
    if (code.length !== 6 || !/^\d{6}$/.test(code)) {
      setError('请填写 6 位数字邮箱验证码')
      goStep(2)
      triggerShake()
      return
    }

    setLoading(true)
    const result = await resetPassword(email.trim(), newPassword, code)
    setLoading(false)

    if (result.success) {
      toast.success(t('resetPasswordSuccess', language) || '重置成功')
      setCelebrate(true)
    } else {
      const msg = result.message || t('resetPasswordFailed', language)
      setError(msg)
      toast.error(msg)
    }
  }

  const onFormSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (step !== 3) return
    void submitReset()
  }

  const submitDisabled = loading || celebrate || !passwordValid

  const dotClass = (n: number) => {
    if (step === n) return 'crypto-recover-step-dot crypto-recover-active'
    if (step > n) return 'crypto-recover-step-dot crypto-recover-done'
    return 'crypto-recover-step-dot'
  }

  const lineClass = (afterStep: number) =>
    step > afterStep ? 'crypto-recover-step-line crypto-recover-done' : 'crypto-recover-step-line'

  const labelClass = (n: number) => {
    if (step === n) return 'crypto-recover-step-label crypto-recover-active'
    if (step > n) return 'crypto-recover-step-label crypto-recover-done'
    return 'crypto-recover-step-label'
  }

  return (
    <div className="crypto-login-root crypto-recover-root">
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

      {sparks.map((s) => (
        <div
          key={s.id}
          className="crypto-recover-spark"
          style={{
            left: s.left,
            top: s.top,
            width: s.size,
            height: s.size,
            ['--dx' as string]: s.dx,
            ['--dy' as string]: s.dy,
            animation: `crypto-recover-spark-line ${s.dur} ease-out forwards`,
          }}
        />
      ))}

      <div
        className={`crypto-login-card crypto-recover-card crypto-recover-card-static${shake ? ' crypto-login-shake' : ''}`}
      >
        <div className="crypto-login-card-accent" />
        <div className="crypto-login-card-body">
          <div className={`crypto-login-logo-area${celebrate ? ' crypto-recover-coin-fast' : ''}`}>
            <div className="crypto-login-coin-logo" />
            <span className="crypto-login-logo-text">COMKUN-AI</span>
          </div>

          <div className="crypto-login-title">{t('resetPasswordTitle', language)}</div>
          <div className="crypto-login-subtitle">重置你的安全凭证</div>

          <div className="crypto-recover-steps" aria-hidden={false}>
            <div className={dotClass(1)}>1</div>
            <div className={lineClass(1)} />
            <div className={dotClass(2)}>2</div>
            <div className={lineClass(2)} />
            <div className={dotClass(3)}>3</div>
          </div>
          <div className="crypto-recover-step-labels">
            <span className={labelClass(1)}>验证身份</span>
            <span className={labelClass(2)}>安全校验</span>
            <span className={labelClass(3)}>重置密码</span>
          </div>

          {error ? (
            <div className="crypto-login-err" style={{ marginBottom: 12 }}>
              {error}
            </div>
          ) : null}

          <form onSubmit={onFormSubmit} autoComplete="on">
            <div
              className={`crypto-recover-panel${step === 1 ? ' crypto-recover-panel-active' : ''}`}
            >
              <div className="crypto-login-input-group">
                <label htmlFor="crypto-recover-email">邮箱</label>
                <div className="crypto-login-input-wrap">
                  <input
                    id="crypto-recover-email"
                    type="email"
                    name="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="请输入注册时使用的邮箱"
                    disabled={celebrate}
                    autoComplete="email"
                  />
                </div>
              </div>
              <div className="crypto-recover-btn-row">
                <Link
                  to={ROUTES.login}
                  className="crypto-login-btn crypto-recover-btn-secondary"
                  style={{
                    textDecoration: 'none',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    pointerEvents: celebrate ? 'none' : undefined,
                    opacity: celebrate ? 0.45 : undefined,
                  }}
                >
                  <span>← 返回登录</span>
                </Link>
                <button type="button" className="crypto-login-btn" onClick={onStep1Next} disabled={celebrate}>
                  <span>下一步 →</span>
                </button>
              </div>
            </div>

            <div
              className={`crypto-recover-panel${step === 2 ? ' crypto-recover-panel-active' : ''}`}
            >
              <div className="crypto-login-input-group">
                <label htmlFor="crypto-recover-code">验证码</label>
                <div className="crypto-recover-code-wrap">
                  <input
                    id="crypto-recover-code"
                    type="text"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    value={emailCode}
                    onChange={(e) =>
                      setEmailCode(e.target.value.replace(/\D/g, '').slice(0, 6))
                    }
                    placeholder="请输入 6 位验证码"
                    disabled={celebrate}
                  />
                  <button
                    type="button"
                    className={`crypto-recover-code-btn${codeCooldown > 0 ? ' crypto-recover-code-sent' : ''}`}
                    onClick={() => void handleSendCode()}
                    disabled={celebrate || sendingCode || codeCooldown > 0}
                  >
                    {sendingCode
                      ? '发送中…'
                      : codeCooldown > 0
                        ? `${codeCooldown}s`
                        : '获取验证码'}
                  </button>
                </div>
              </div>
              <div className="crypto-recover-btn-row">
                <button
                  type="button"
                  className="crypto-login-btn crypto-recover-btn-secondary"
                  onClick={() => goStep(1)}
                  disabled={celebrate}
                >
                  <span>← 上一步</span>
                </button>
                <button type="button" className="crypto-login-btn" onClick={onStep2Next} disabled={celebrate}>
                  <span>下一步 →</span>
                </button>
              </div>
            </div>

            <div
              className={`crypto-recover-panel${step === 3 ? ' crypto-recover-panel-active' : ''}`}
            >
              <div className="crypto-login-input-group">
                <label>{t('newPassword', language)}</label>
                <div className="crypto-register-pw-wrap">
                  <div className="crypto-login-input-wrap">
                    <input
                      type={showPassword ? 'text' : 'password'}
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      placeholder={t('newPasswordPlaceholder', language)}
                      disabled={celebrate}
                      autoComplete="new-password"
                    />
                  </div>
                  <button
                    type="button"
                    className="crypto-register-pw-toggle"
                    tabIndex={-1}
                    onClick={() => setShowPassword(!showPassword)}
                    aria-label={showPassword ? '隐藏密码' : '显示密码'}
                    disabled={celebrate}
                  >
                    {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
              </div>
              <div className="crypto-login-input-group">
                <label>{t('confirmPassword', language)}</label>
                <div className="crypto-register-pw-wrap">
                  <div className="crypto-login-input-wrap">
                    <input
                      type={showConfirmPassword ? 'text' : 'password'}
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      placeholder={t('confirmPasswordPlaceholder', language)}
                      disabled={celebrate}
                      autoComplete="new-password"
                    />
                  </div>
                  <button
                    type="button"
                    className="crypto-register-pw-toggle"
                    tabIndex={-1}
                    onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                    aria-label={showConfirmPassword ? '隐藏密码' : '显示密码'}
                    disabled={celebrate}
                  >
                    {showConfirmPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
              </div>

              <div className="crypto-register-checklist">
                <p className="crypto-register-checklist-title">密码要求</p>
                <PasswordChecklist
                  rules={[
                    'minLength',
                    'capital',
                    'lowercase',
                    'number',
                    'specialChar',
                    'match',
                  ]}
                  minLength={8}
                  value={newPassword}
                  valueAgain={confirmPassword}
                  messages={{
                    minLength: t('passwordRuleMinLength', language),
                    capital: t('passwordRuleUppercase', language),
                    lowercase: t('passwordRuleLowercase', language),
                    number: t('passwordRuleNumber', language),
                    specialChar: t('passwordRuleSpecial', language),
                    match: t('passwordRuleMatch', language),
                  }}
                  className="crypto-register-checklist-ul"
                  iconSize={9}
                  validColor="rgba(197, 216, 62, 0.75)"
                  invalidColor="rgba(82, 82, 91, 0.9)"
                  validTextColor="rgba(212, 230, 74, 0.92)"
                  invalidTextColor="#52525b"
                  onChange={(isValid) => setPasswordValid(isValid)}
                />
              </div>

              <div className="crypto-recover-btn-row" style={{ marginTop: 12 }}>
                <button
                  type="button"
                  className="crypto-login-btn crypto-recover-btn-secondary"
                  onClick={() => goStep(2)}
                  disabled={celebrate}
                >
                  <span>← 上一步</span>
                </button>
                <button
                  type="submit"
                  className={`crypto-login-btn${celebrate ? ' crypto-recover-btn-success' : ''}`}
                  disabled={submitDisabled}
                >
                  <span>
                    {celebrate ? '✓ 密码已重置' : loading ? t('loading', language) : '→ 重置密码'}
                  </span>
                </button>
              </div>
            </div>
          </form>

          <div className="crypto-recover-switch">
            想起密码了？
            <Link to={ROUTES.login}>立即登录 →</Link>
          </div>

          <div className="crypto-recover-terminal">
            COMKUN-AI-交易系统-就绪
            <span className="crypto-login-terminal-blink" />
          </div>
        </div>
      </div>
    </div>
  )
}
