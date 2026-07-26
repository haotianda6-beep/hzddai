import React, {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import PasswordChecklist from 'react-password-checklist'
import { toast } from 'sonner'
import { useAuth } from '../../contexts/AuthContext'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { getSystemConfig } from '../../lib/config'
import { WhitelistFullPage } from '../common/WhitelistFullPage'
import { ROUTES } from '../../router/paths'
import './crypto-login.css'
import './crypto-register.css'

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

export function RegisterPage() {
  const uid = useId()
  const { language } = useLanguage()
  const { register, sendRegisterEmailCode } = useAuth()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const inviteFromUrl = useMemo(() => {
    const raw = searchParams.get('invite') || searchParams.get('ref') || ''
    return raw.replace(/[^a-z0-9]/gi, '').toUpperCase()
  }, [searchParams])

  const inviteLocked = inviteFromUrl.length > 0
  const [view, setView] = useState<'register' | 'whitelist-full'>('register')
  const [email, setEmail] = useState('')
  const [emailCode, setEmailCode] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [betaCode, setBetaCode] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [betaMode, setBetaMode] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [sendingCode, setSendingCode] = useState(false)
  const [passwordValid, setPasswordValid] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [codeCooldown, setCodeCooldown] = useState(0)
  const [shake, setShake] = useState(false)
  const [celebrate, setCelebrate] = useState(false)
  const [sparks, setSparks] = useState<SparkParticle[]>([])

  const barRef = useRef<HTMLDivElement>(null)
  const thumbRef = useRef<HTMLDivElement>(null)
  const draggingRef = useRef(false)
  const dragStartClientX = useRef(0)
  const dragStartThumb = useRef(2)
  const captchaVerifiedRef = useRef(false)
  const [thumbOffset, setThumbOffset] = useState(2)
  const [captchaActive, setCaptchaActive] = useState(false)
  const [captchaVerified, setCaptchaVerified] = useState(false)

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
    document.title = 'COMKUN-AI · 注册'
  }, [])

  useEffect(() => {
    if (codeCooldown <= 0) return
    const tmr = window.setInterval(() => {
      setCodeCooldown((s) => (s <= 1 ? 0 : s - 1))
    }, 1000)
    return () => window.clearInterval(tmr)
  }, [codeCooldown])

  useEffect(() => {
    getSystemConfig()
      .then((config) => {
        setBetaMode(config.beta_mode || false)
      })
      .catch((err) => {
        console.error('Failed to fetch system config:', err)
      })
  }, [])

  const getMaxTravel = useCallback(() => {
    const bar = barRef.current
    const thumb = thumbRef.current
    if (!bar || !thumb) return 120
    return Math.max(0, bar.clientWidth - thumb.offsetWidth - 4)
  }, [])

  const resetCaptcha = useCallback(() => {
    captchaVerifiedRef.current = false
    setCaptchaVerified(false)
    setThumbOffset(2)
    setCaptchaActive(false)
    draggingRef.current = false
  }, [])

  const triggerShake = useCallback(() => {
    setShake(true)
    window.setTimeout(() => setShake(false), 500)
  }, [])

  const onThumbPointerDown = useCallback(
    (e: React.MouseEvent | React.TouchEvent) => {
      if (captchaVerifiedRef.current || celebrate) return
      draggingRef.current = true
      const clientX = 'touches' in e ? e.touches[0].clientX : e.clientX
      dragStartClientX.current = clientX
      dragStartThumb.current = thumbOffset
      setCaptchaActive(true)
      e.preventDefault()
    },
    [celebrate, thumbOffset]
  )

  useEffect(() => {
    const onMove = (clientX: number) => {
      if (!draggingRef.current || captchaVerifiedRef.current || celebrate) return
      const max = getMaxTravel()
      let next = dragStartThumb.current + (clientX - dragStartClientX.current)
      next = Math.max(2, Math.min(next, max))
      setThumbOffset(next)
      if (next >= max - 2) {
        captchaVerifiedRef.current = true
        setCaptchaVerified(true)
        setThumbOffset(max)
        draggingRef.current = false
        setCaptchaActive(false)
      }
    }
    const onMouseMove = (e: MouseEvent) => {
      onMove(e.clientX)
    }
    const onTouchMove = (e: TouchEvent) => {
      if (e.touches[0]) onMove(e.touches[0].clientX)
    }
    const end = () => {
      if (!draggingRef.current) return
      draggingRef.current = false
      setCaptchaActive(false)
      if (!captchaVerifiedRef.current) setThumbOffset(2)
    }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', end)
    window.addEventListener('touchmove', onTouchMove, { passive: false })
    window.addEventListener('touchend', end)
    return () => {
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', end)
      window.removeEventListener('touchmove', onTouchMove)
      window.removeEventListener('touchend', end)
    }
  }, [celebrate, getMaxTravel])

  useEffect(() => {
    if (celebrate) return
    const onResize = () => {
      if (!captchaVerified) setThumbOffset(2)
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [captchaVerified, celebrate])

  useEffect(() => {
    if (!celebrate) return
    const handles: number[] = []
    for (let i = 0; i < 40; i++) {
      handles.push(
        window.setTimeout(() => {
          const angle = Math.random() * Math.PI * 2
          const dist = 40 + Math.random() * 120
          const id = `sp-${Date.now()}-${i}`
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
      setError('请先填写有效邮箱')
      triggerShake()
      return
    }
    setSendingCode(true)
    const r = await sendRegisterEmailCode(trimmed)
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
  }, [email, sendRegisterEmailCode, triggerShake])

  if (view === 'whitelist-full') {
    return <WhitelistFullPage onBack={() => setView('register')} />
  }

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()
    if (celebrate) return
    setError('')

    if (!captchaVerified) {
      setError('请完成滑块验证')
      triggerShake()
      return
    }

    if (!passwordValid) {
      setError(t('passwordNotMeetRequirements', language))
      triggerShake()
      return
    }

    const code = emailCode.trim()
    if (code.length !== 6 || !/^\d{6}$/.test(code)) {
      setError('请填写 6 位数字邮箱验证码')
      triggerShake()
      return
    }

    if (betaMode && !betaCode.trim()) {
      setError('内测期间，注册需要提供内测码')
      triggerShake()
      return
    }

    setLoading(true)
    try {
      const effectiveInvite = inviteLocked ? inviteFromUrl : inviteCode.trim()
      const result = await register(
        email.trim(),
        password,
        code,
        betaCode.trim() || undefined,
        effectiveInvite || undefined
      )

      const isWhitelistError = (msg: string) => {
        const lowerMsg = msg.toLowerCase()
        return (
          lowerMsg.includes('whitelist') ||
          lowerMsg.includes('capacity') ||
          lowerMsg.includes('limit') ||
          lowerMsg.includes('permission denied') ||
          lowerMsg.includes('not on whitelist')
        )
      }

      if (!result.success) {
        const msg = result.message || t('registrationFailed', language)
        if (isWhitelistError(msg)) {
          setView('whitelist-full')
          return
        }
        setError(msg)
        toast.error(msg)
        resetCaptcha()
      } else {
        toast.success(result.message || '注册成功')
        setCelebrate(true)
      }
    } catch (err) {
      console.error('Registration error:', err)
      const errorMsg =
        err instanceof Error
          ? err.message
          : 'Registration failed due to server error'
      const lowerMsg = errorMsg.toLowerCase()
      if (
        lowerMsg.includes('whitelist') ||
        lowerMsg.includes('capacity') ||
        lowerMsg.includes('limit') ||
        lowerMsg.includes('permission denied') ||
        lowerMsg.includes('not on whitelist')
      ) {
        setView('whitelist-full')
        return
      }
      setError(errorMsg)
      toast.error(errorMsg)
      resetCaptcha()
    } finally {
      setLoading(false)
    }
  }

  const fillWidth = Math.max(0, thumbOffset - 2)
  const submitDisabled =
    loading ||
    celebrate ||
    !passwordValid ||
    !captchaVerified ||
    (betaMode && !betaCode.trim())

  return (
    <div className="crypto-login-root crypto-register-root">
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
          className="crypto-reg-spark"
          style={{
            left: s.left,
            top: s.top,
            width: s.size,
            height: s.size,
            ['--dx' as string]: s.dx,
            ['--dy' as string]: s.dy,
            animation: `crypto-reg-spark-line ${s.dur} ease-out forwards`,
          }}
        />
      ))}

      <div
        className={`crypto-login-card crypto-register-card crypto-register-card-static${shake ? ' crypto-login-shake' : ''}`}
      >
            <div className="crypto-login-card-accent" />
            <div className="crypto-login-card-body">
              <div className={`crypto-login-logo-area${celebrate ? ' crypto-register-coin-fast' : ''}`}>
                <div className="crypto-login-coin-logo" />
                <span className="crypto-login-logo-text">COMKUN-AI</span>
              </div>

              <div className="crypto-login-title">创建账户</div>
              <div className="crypto-login-subtitle">开启你的量化系统部署</div>

              <form onSubmit={handleRegister} autoComplete="on">
                <div className="crypto-login-input-group">
                  <label htmlFor="crypto-reg-email">邮箱</label>
                  <div className="crypto-register-row-email">
                    <div className="crypto-register-email-grow">
                      <div className="crypto-login-input-wrap">
                        <input
                          id="crypto-reg-email"
                          type="email"
                          name="email"
                          value={email}
                          onChange={(e) => setEmail(e.target.value)}
                          placeholder="you@example.com"
                          disabled={celebrate}
                          required
                          autoComplete="email"
                        />
                      </div>
                    </div>
                    <button
                      type="button"
                      className="crypto-register-send-code"
                      onClick={handleSendCode}
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

                <div className="crypto-login-input-group">
                  <label htmlFor="crypto-reg-code">输入验证码</label>
                  <div className="crypto-login-input-wrap crypto-register-code-input">
                    <input
                      id="crypto-reg-code"
                      type="text"
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      maxLength={6}
                      value={emailCode}
                      onChange={(ev) =>
                        setEmailCode(ev.target.value.replace(/\D/g, '').slice(0, 6))
                      }
                      placeholder="000000"
                      disabled={celebrate}
                      required
                    />
                  </div>
                </div>

                <div className="crypto-register-pw-row">
                  <div className="crypto-login-input-group">
                    <label>{t('password', language)}</label>
                    <div className="crypto-register-pw-wrap">
                      <div className="crypto-login-input-wrap">
                        <input
                          type={showPassword ? 'text' : 'password'}
                          value={password}
                          onChange={(e) => setPassword(e.target.value)}
                          placeholder="••••••••"
                          disabled={celebrate}
                          required
                          autoComplete="new-password"
                        />
                      </div>
                      <button
                        type="button"
                        className="crypto-register-pw-toggle"
                        tabIndex={-1}
                        onClick={() => setShowPassword(!showPassword)}
                        aria-label={showPassword ? '隐藏密码' : '显示密码'}
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
                          placeholder="••••••••"
                          disabled={celebrate}
                          required
                          autoComplete="new-password"
                        />
                      </div>
                      <button
                        type="button"
                        className="crypto-register-pw-toggle"
                        tabIndex={-1}
                        onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                        aria-label={showConfirmPassword ? '隐藏密码' : '显示密码'}
                      >
                        {showConfirmPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                      </button>
                    </div>
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
                    value={password}
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

                {betaMode ? (
                  <div className="crypto-login-input-group">
                    <label htmlFor="crypto-reg-beta">内测码</label>
                    <div className="crypto-login-input-wrap">
                      <input
                        id="crypto-reg-beta"
                        type="text"
                        value={betaCode}
                        onChange={(e) =>
                          setBetaCode(e.target.value.replace(/[^a-z0-9]/gi, '').toLowerCase())
                        }
                        placeholder="6 位字母数字"
                        maxLength={6}
                        disabled={celebrate}
                        required={betaMode}
                        autoComplete="off"
                      />
                    </div>
                  </div>
                ) : null}

                <div className="crypto-login-input-group">
                  <label htmlFor="crypto-reg-invite">
                    {inviteLocked ? '邀请码（来自邀请链接）' : '邀请码（选填）'}
                  </label>
                  <div className="crypto-login-input-wrap">
                    <input
                      id="crypto-reg-invite"
                      type="text"
                      name="invite_code"
                      value={inviteLocked ? inviteFromUrl : inviteCode}
                      onChange={(e) => {
                        if (inviteLocked) return
                        setInviteCode(e.target.value.replace(/[^a-z0-9]/gi, '').toUpperCase())
                      }}
                      readOnly={inviteLocked}
                      disabled={celebrate}
                      autoComplete="off"
                      placeholder={inviteLocked ? '' : '朋友给你的邀请码，可不填'}
                      maxLength={16}
                      title={inviteLocked ? '此邀请码由邀请链接带入，不可修改' : undefined}
                    />
                  </div>
                  {inviteLocked ? (
                    <p className="crypto-register-hint">已通过邀请链接绑定，邀请码不可更改。</p>
                  ) : null}
                </div>

                <div className="crypto-reg-captcha-section">
                  <label>安全验证</label>
                  <div
                    ref={barRef}
                    className={`crypto-reg-captcha-bar${
                      captchaActive ? ' crypto-reg-captcha-active' : ''
                    }${captchaVerified ? ' crypto-reg-captcha-verified' : ''}`}
                    role="presentation"
                    onMouseDown={() => {
                      if (!captchaVerified && !celebrate) setCaptchaActive(true)
                    }}
                  >
                    <div className="crypto-reg-captcha-track">
                      <div
                        className="crypto-reg-captcha-fill"
                        style={{ width: `${fillWidth}px` }}
                      />
                      <div className="crypto-reg-captcha-text">
                        {captchaVerified ? '✓ 验证通过' : '→ 拖动滑块完成验证 →'}
                      </div>
                    </div>
                    <div
                      ref={thumbRef}
                      className="crypto-reg-captcha-thumb"
                      style={{ left: `${thumbOffset}px` }}
                      onMouseDown={onThumbPointerDown}
                      onTouchStart={onThumbPointerDown}
                    >
                      {captchaVerified ? '✓' : '⇌'}
                    </div>
                  </div>
                </div>

                {error ? <div className="crypto-login-err">{error}</div> : null}

                <button
                  type="submit"
                  className={`crypto-login-btn${celebrate ? ' crypto-register-btn-success' : ''}`}
                  disabled={submitDisabled}
                >
                  <span>
                    {celebrate
                      ? '✓ 注册成功'
                      : loading
                        ? '提交中…'
                        : '注册'}
                  </span>
                </button>
              </form>

              <div className="crypto-register-switch">
                已有账号？
                <Link to={ROUTES.login}>立即登录 →</Link>
              </div>

              <div className="crypto-register-terminal-static">
                COMKUN-AI-交易系统-就绪
                <span className="crypto-login-terminal-blink" />
              </div>

              <p className="mt-6 text-center text-[11px] text-[#71717a]">
                <button
                  type="button"
                  onClick={() => navigate(ROUTES.home)}
                  className="underline-offset-2 hover:text-[#d4ff33] hover:underline"
                >
                  返回首页
                </button>
              </p>
            </div>
          </div>
    </div>
  )
}
