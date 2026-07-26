import React, { useState, useEffect, useCallback } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { toast } from 'sonner'
import { useAuth } from '../../contexts/AuthContext'
import { invalidateSystemConfig } from '../../lib/config'

const labels = {
  zh: {
    welcome: '欢迎使用 COMKUN-AI',
    subtitle: '创建账号开始使用',
    email: '邮箱',
    emailPlaceholder: 'you@example.com',
    emailCode: '邮箱验证码',
    sendCode: '发送验证码',
    sendingCode: '发送中…',
    codeHint: '向邮箱发送 6 位数字验证码，10 分钟内有效',
    password: '密码',
    passwordPlaceholder: '至少 8 个字符',
    passwordError: '密码至少需要 8 个字符',
    codeError: '请填写 6 位数字验证码',
    invalidEmail: '请填写有效邮箱',
    submit: '开始使用',
    submitting: '创建中...',
    setupFailed: '创建失败，请重试',
    singleUser: '每个邮箱仅能注册一个账号；同一设备可使用不同邮箱注册多个账号',
  },
  en: {
    welcome: 'Welcome to COMKUN-AI',
    subtitle: 'Create your account to get started',
    email: 'Email',
    emailPlaceholder: 'you@example.com',
    emailCode: 'Email code',
    sendCode: 'Send code',
    sendingCode: 'Sending…',
    codeHint: 'We will email a 6-digit code (valid 10 minutes)',
    password: 'Password',
    passwordPlaceholder: 'At least 8 characters',
    passwordError: 'Password must be at least 8 characters',
    codeError: 'Enter the 6-digit code from email',
    invalidEmail: 'Please enter a valid email',
    submit: 'Get Started',
    submitting: 'Creating account...',
    setupFailed: 'Setup failed, please try again',
    singleUser: 'One account per email; you may register multiple accounts with different emails on the same device',
  },
  id: {
    welcome: 'Selamat Datang di COMKUN-AI',
    subtitle: 'Buat akun untuk memulai',
    email: 'Email',
    emailPlaceholder: 'you@example.com',
    emailCode: 'Kode email',
    sendCode: 'Kirim kode',
    sendingCode: 'Mengirim…',
    codeHint: 'Kode 6 digit dikirim ke email (berlaku 10 menit)',
    password: 'Kata Sandi',
    passwordPlaceholder: 'Minimal 8 karakter',
    passwordError: 'Kata sandi minimal 8 karakter',
    codeError: 'Masukkan kode 6 digit dari email',
    invalidEmail: 'Masukkan email yang valid',
    submit: 'Mulai',
    submitting: 'Membuat akun...',
    setupFailed: 'Gagal membuat akun, coba lagi',
    singleUser: 'Satu email satu akun; perangkat yang sama bisa mendaftar beberapa akun dengan email berbeda',
  },
} as const

export function SetupPage() {
  const { register, sendRegisterEmailCode } = useAuth()
  const [email, setEmail] = useState('')
  const [emailCode, setEmailCode] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [sendingCode, setSendingCode] = useState(false)
  const [codeCooldown, setCodeCooldown] = useState(0)
  // Clean up any stale auth/onboarding state on setup page load
  useEffect(() => {
    localStorage.removeItem('auth_token')
    localStorage.removeItem('auth_user')
    localStorage.removeItem('user_id')
    localStorage.removeItem('nofx_beginner_onboarding_completed')
    localStorage.removeItem('nofx_beginner_wallet_address')
  }, [])

  useEffect(() => {
    if (codeCooldown <= 0) return
    const tmr = window.setInterval(() => {
      setCodeCooldown((s) => (s <= 1 ? 0 : s - 1))
    }, 1000)
    return () => window.clearInterval(tmr)
  }, [codeCooldown])

  const l = labels.zh

  const handleSendCode = useCallback(async () => {
    setError('')
    const trimmed = email.trim()
    if (!trimmed || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)) {
      setError(l.invalidEmail)
      return
    }
    setSendingCode(true)
    const r = await sendRegisterEmailCode(trimmed)
    setSendingCode(false)
    if (r.success) {
      if (r.hint) toast.info(r.hint)
      setCodeCooldown(60)
    } else {
      if (r.retryAfterSec != null && r.retryAfterSec > 0) {
        setCodeCooldown(r.retryAfterSec)
      }
      setError(r.message || 'Send failed')
    }
  }, [email, l.invalidEmail, sendRegisterEmailCode])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (password.length < 8) {
      setError(l.passwordError)
      return
    }
    const code = emailCode.trim()
    if (code.length !== 6 || !/^\d{6}$/.test(code)) {
      setError(l.codeError)
      return
    }
    setLoading(true)
    const result = await register(email.trim(), password, code, undefined)
    setLoading(false)
    if (result.success) {
      invalidateSystemConfig()
    } else {
      setError(result.message || l.setupFailed)
    }
  }

  return (
    <div className="relative min-h-screen w-full overflow-hidden bg-nofx-bg">
      {/* Decorative background - simulates the main app behind a modal */}

      {/* Grid */}
      <div className="absolute inset-0 pointer-events-none">
        <div className="absolute inset-x-0 bottom-0 h-[60vh] bg-[linear-gradient(to_right,#80808012_1px,transparent_1px),linear-gradient(to_bottom,#80808012_1px,transparent_1px)] bg-[size:40px_40px] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_0%,#000_70%,transparent_100%)] opacity-40" style={{ transform: 'perspective(500px) rotateX(60deg) translateY(80px) scale(2)' }} />
      </div>

      {/* Glow spots */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute top-[10%] left-[15%] w-[500px] h-[500px] bg-nofx-gold/8 rounded-full blur-[150px]" />
        <div className="absolute bottom-[5%] right-[10%] w-[400px] h-[400px] bg-indigo-500/6 rounded-full blur-[140px]" />
        <div className="absolute top-[40%] right-[30%] w-[300px] h-[300px] bg-emerald-500/4 rounded-full blur-[120px]" />
      </div>

      {/* Faux UI elements in background to simulate the app */}
      <div className="absolute inset-0 pointer-events-none opacity-[0.06]">
        {/* Fake header bar */}
        <div className="h-14 border-b border-white/20 flex items-center px-6 gap-4">
          <div className="w-8 h-8 rounded-lg bg-white/40" />
          <div className="h-3 w-20 rounded bg-white/30" />
          <div className="h-3 w-16 rounded bg-white/20 ml-4" />
          <div className="h-3 w-16 rounded bg-white/20" />
          <div className="h-3 w-16 rounded bg-white/20" />
        </div>
        {/* Fake content cards */}
        <div className="p-6 grid grid-cols-4 gap-4 mt-2">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-24 rounded-xl border border-white/15 bg-white/5" />
          ))}
        </div>
        <div className="px-6 mt-2">
          <div className="h-64 rounded-xl border border-white/15 bg-white/5" />
        </div>
      </div>

      {/* Blur overlay */}
      <div className="absolute inset-0 backdrop-blur-md bg-black/60" />

      {/* Modal card */}
      <div className="relative z-10 flex min-h-screen items-center justify-center px-4 py-16">
        <div className="w-full max-w-sm animate-[fadeInUp_0.4s_ease-out]">

          {/* Logo + 标题同一行，图标与文字同字号 */}
          <div className="mb-8 text-center">
            <div className="relative mb-3 flex justify-center">
              <div className="absolute inset-0 flex justify-center">
                <div className="h-12 w-[min(100%,20rem)] bg-nofx-gold/20 blur-2xl" aria-hidden />
              </div>
              <div className="relative z-10 flex items-center justify-center gap-2.5 text-2xl font-bold leading-none text-white">
                <img
                  src="/icons/comkun-logo.png"
                  alt=""
                  className="h-10 w-10 shrink-0 rounded-lg object-cover drop-shadow-[0_0_12px_rgba(240,185,11,0.25)] sm:h-11 sm:w-11"
                  aria-hidden
                />
                <h1 className="font-bold leading-none">{l.welcome}</h1>
              </div>
            </div>
            <p className="text-sm text-zinc-500">{l.subtitle}</p>
          </div>

          {/* Card */}
          <div className="bg-zinc-900/80 backdrop-blur-2xl border border-white/10 rounded-2xl p-8 shadow-[0_25px_60px_-15px_rgba(0,0,0,0.5),0_0_40px_-10px_rgba(240,185,11,0.08)]">
            <form onSubmit={handleSubmit} className="space-y-5">

              {/* Email */}
              <div>
                <label className="block text-xs font-medium text-zinc-400 mb-2">{l.email}</label>
                <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
                  <input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className="w-full flex-1 bg-black/40 border border-white/10 rounded-xl px-4 py-3 text-sm text-white placeholder-zinc-600 focus:outline-none focus:border-nofx-gold/60 focus:ring-1 focus:ring-nofx-gold/30 transition-all"
                    placeholder={l.emailPlaceholder}
                    required
                    autoFocus
                  />
                  <button
                    type="button"
                    onClick={handleSendCode}
                    disabled={sendingCode || codeCooldown > 0}
                    className="shrink-0 rounded-xl border border-white/15 bg-white/5 px-4 py-3 text-xs font-semibold text-zinc-200 transition-colors hover:border-nofx-gold/50 hover:text-nofx-gold disabled:cursor-not-allowed disabled:opacity-45 sm:w-[7.5rem]"
                  >
                    {sendingCode
                      ? l.sendingCode
                      : codeCooldown > 0
                        ? `${codeCooldown}s`
                        : l.sendCode}
                  </button>
                </div>
                <p className="mt-1.5 text-[10px] text-zinc-500">{l.codeHint}</p>
              </div>

              <div>
                <label className="block text-xs font-medium text-zinc-400 mb-2">{l.emailCode}</label>
                <input
                  type="text"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  value={emailCode}
                  onChange={(e) =>
                    setEmailCode(e.target.value.replace(/\D/g, '').slice(0, 6))
                  }
                  className="w-full bg-black/40 border border-white/10 rounded-xl px-4 py-3 text-center font-mono text-lg tracking-[0.35em] text-white placeholder-zinc-600 focus:outline-none focus:border-nofx-gold/60 focus:ring-1 focus:ring-nofx-gold/30 transition-all"
                  placeholder="000000"
                  required
                />
              </div>

              {/* Password */}
              <div>
                <label className="block text-xs font-medium text-zinc-400 mb-2">{l.password}</label>
                <div className="relative">
                  <input
                    type={showPassword ? 'text' : 'password'}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="w-full bg-black/40 border border-white/10 rounded-xl px-4 py-3 pr-11 text-sm text-white placeholder-zinc-600 focus:outline-none focus:border-nofx-gold/60 focus:ring-1 focus:ring-nofx-gold/30 transition-all"
                    placeholder={l.passwordPlaceholder}
                    required
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3.5 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300 transition-colors"
                  >
                    {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
              </div>

              {/* Error */}
              {error && (
                <p className="text-xs text-red-400 bg-red-500/10 border border-red-500/20 rounded-lg px-3 py-2">
                  {error}
                </p>
              )}

              {/* Submit */}
              <button
                type="submit"
                disabled={loading}
                className="w-full bg-nofx-gold hover:bg-yellow-400 active:scale-[0.98] text-black font-semibold py-3 rounded-xl text-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed mt-2 shadow-[0_0_20px_rgba(240,185,11,0.2)]"
              >
                {loading ? l.submitting : l.submit}
              </button>
            </form>
          </div>

          <p className="text-center text-xs text-zinc-600 mt-6">
            {l.singleUser}
          </p>
        </div>
      </div>

      <style>{`
        @keyframes fadeInUp {
          from { opacity: 0; transform: translateY(20px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}
