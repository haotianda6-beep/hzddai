import React, { createContext, useContext, useState, useEffect, useCallback } from 'react'
import { flushSync } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { getSystemConfig, invalidateSystemConfig } from '../lib/config'
import { reset401Flag, httpClient } from '../lib/httpClient'
import { setUserMode, type UserMode } from '../lib/onboarding'
import { ROUTES } from '../router/paths'
import { t, type Language } from '../i18n/translations'
import { useLanguage } from './LanguageContext'

export interface User {
  id: string
  email: string
  display_name?: string
  avatar_url?: string
  /** 策略市场站内余额（USDT） */
  balance_usdt?: number
  /** 管理后台白名单 */
  is_admin?: boolean
  /** 财务台：为客户正数入账 */
  is_finance?: boolean
  invite_code?: string
  profile_named?: boolean
}

interface AuthContextType {
  user: User | null
  token: string | null
  login: (
    email: string,
    password: string,
    mode?: UserMode
  ) => Promise<{
    success: boolean
    message?: string
  }>
  loginAdmin: (password: string) => Promise<{
    success: boolean
    message?: string
  }>
  register: (
    email: string,
    password: string,
    emailCode: string,
    betaCode?: string,
    inviteCode?: string,
    mode?: UserMode
  ) => Promise<{ success: boolean; message?: string }>
  resetPassword: (
    email: string,
    newPassword: string,
    emailCode: string
  ) => Promise<{ success: boolean; message?: string }>
  sendRegisterEmailCode: (
    email: string
  ) => Promise<{
    success: boolean
    message?: string
    retryAfterSec?: number
    hint?: string
  }>
  sendResetPasswordEmailCode: (
    email: string
  ) => Promise<{
    success: boolean
    message?: string
    retryAfterSec?: number
    hint?: string
  }>
  logout: () => void
  /** 保存资料后同步更新内存与 localStorage */
  applyUserProfile: (
    partial: Partial<
      Pick<
        User,
        | 'display_name'
        | 'avatar_url'
        | 'balance_usdt'
        | 'is_admin'
        | 'is_finance'
        | 'invite_code'
        | 'profile_named'
      >
    >
  ) => void
  isLoading: boolean
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function loginErrorMessage(
  status: number,
  fallback: string | undefined,
  language: Language
) {
  return status === 401
    ? t('invalidCredentials', language)
    : fallback || t('loginFailed', language)
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const { language } = useLanguage()
  const navigate = useNavigate()
  const [user, setUser] = useState<User | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    // Reset 401 flag on page load to allow fresh 401 handling
    reset401Flag()

    // Check if admin mode is active (uses cached system config)
    const syncProfileFromServer = async (tok: string) => {
      try {
        const r = await fetch('/api/user/me', { headers: { Authorization: `Bearer ${tok}` } })
        if (!r.ok) return
        const data = (await r.json()) as {
          id?: string
          email?: string
          display_name?: string
          avatar_url?: string
          balance_usdt?: number
          is_admin?: boolean
          is_finance?: boolean
          invite_code?: string
          profile_named?: boolean
        }
        setUser((prev) => {
          const base =
            prev ||
            ({
              id: data.id || '',
              email: data.email || '',
            } as User)
          return {
            ...base,
            display_name: data.display_name ?? base.display_name,
            avatar_url: data.avatar_url ?? base.avatar_url,
            balance_usdt:
              typeof data.balance_usdt === 'number' ? data.balance_usdt : base.balance_usdt,
            is_admin: typeof data.is_admin === 'boolean' ? data.is_admin : base.is_admin,
            is_finance: typeof data.is_finance === 'boolean' ? data.is_finance : base.is_finance,
            invite_code: data.invite_code ?? base.invite_code,
            profile_named:
              typeof data.profile_named === 'boolean' ? data.profile_named : base.profile_named,
          }
        })
        const raw = localStorage.getItem('auth_user')
        if (raw) {
          try {
            const merged = { ...JSON.parse(raw), ...data }
            localStorage.setItem('auth_user', JSON.stringify(merged))
          } catch {
            /* ignore */
          }
        }
      } catch {
        /* ignore */
      }
    }

    getSystemConfig()
      .then(() => {
        // No longer simulate login in admin mode; check local storage uniformly
        const savedToken = localStorage.getItem('auth_token')
        const savedUser = localStorage.getItem('auth_user')
        if (savedToken && savedUser) {
          setToken(savedToken)
          setUser(JSON.parse(savedUser))
          void syncProfileFromServer(savedToken)
        }

        setIsLoading(false)
      })
      .catch((err) => {
        console.error('Failed to fetch system config:', err)
        // On error, continue checking local storage
        const savedToken = localStorage.getItem('auth_token')
        const savedUser = localStorage.getItem('auth_user')

        if (savedToken && savedUser) {
          setToken(savedToken)
          setUser(JSON.parse(savedUser))
          void syncProfileFromServer(savedToken)
        }
        setIsLoading(false)
      })
  }, [])

  // Listen for unauthorized events from httpClient (401 responses)
  useEffect(() => {
    const handleUnauthorized = () => {
      console.log('Unauthorized event received - clearing auth state')
      // Clear auth state when 401 is detected
      setUser(null)
      setToken(null)
      // Note: localStorage cleanup is already done in httpClient
    }

    window.addEventListener('unauthorized', handleUnauthorized)

    return () => {
      window.removeEventListener('unauthorized', handleUnauthorized)
    }
  }, [])

  const handlePostAuthSuccess = (
    authToken: string,
    userInfo: User,
    _mode?: UserMode
  ) => {
    reset401Flag()
    setUserMode('beginner')

    localStorage.setItem('auth_token', authToken)
    localStorage.setItem('auth_user', JSON.stringify(userInfo))
    localStorage.setItem('user_id', userInfo.id)
    flushSync(() => {
      setToken(authToken)
      setUser(userInfo)
    })

    const returnUrl = sessionStorage.getItem('returnUrl')
    const nextPath = returnUrl || ROUTES.strategyMarket
    if (returnUrl) {
      sessionStorage.removeItem('returnUrl')
    }

    navigate(nextPath)
  }

  const login = async (email: string, password: string, mode?: UserMode) => {
    try {
      const response = await fetch('/api/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ email, password }),
      })

      const data = await response.json()

      if (response.ok) {
        if (data.token) {
          const userInfo: User = {
            id: data.user_id,
            email: data.email,
            display_name: data.display_name,
            avatar_url: data.avatar_url,
            balance_usdt:
              typeof (data as { balance_usdt?: number }).balance_usdt === 'number'
                ? (data as { balance_usdt?: number }).balance_usdt
                : 0,
            is_admin: Boolean((data as { is_admin?: boolean }).is_admin),
            is_finance: Boolean((data as { is_finance?: boolean }).is_finance),
            invite_code: (data as { invite_code?: string }).invite_code,
            profile_named: Boolean((data as { profile_named?: boolean }).profile_named),
          }
          handlePostAuthSuccess(data.token, userInfo, mode)

          return { success: true, message: data.message }
        }

        // Unexpected success response
        return {
          success: false,
          message: data.message || 'Unexpected login response',
        }
      } else {
        return {
          success: false,
          message: loginErrorMessage(response.status, data.error, language),
        }
      }
    } catch (error) {
      return { success: false, message: t('loginFailed', language) }
    }
  }

  const loginAdmin = async (password: string) => {
    try {
      const response = await fetch('/api/admin-login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password }),
      })
      const data = await response.json()
      if (response.ok) {
        // Reset 401 flag on successful login
        reset401Flag()

        const userInfo = {
          id: data.user_id || 'admin',
          email: data.email || 'admin@localhost',
        }
        localStorage.setItem('auth_token', data.token)
        localStorage.setItem('auth_user', JSON.stringify(userInfo))
        flushSync(() => {
          setToken(data.token)
          setUser(userInfo)
        })

        // Check and redirect to returnUrl if exists
        const returnUrl = sessionStorage.getItem('returnUrl')
        if (returnUrl) {
          sessionStorage.removeItem('returnUrl')
          navigate(returnUrl)
        } else {
          navigate(ROUTES.strategyMarket)
        }
        return { success: true }
      } else {
        return { success: false, message: data.error || 'Login failed' }
      }
    } catch (e) {
      return { success: false, message: 'Login failed, please try again' }
    }
  }

  const sendRegisterEmailCode = async (email: string) => {
    try {
      const res = await fetch('/api/auth/send-register-code', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      })
      const data = (await res.json()) as {
        error?: string
        retry_after?: number
        hint?: string
        message?: string
      }
      if (res.ok) {
        return {
          success: true as const,
          message: data.message,
          hint: data.hint,
        }
      }
      return {
        success: false as const,
        message: data.error || '发送失败',
        retryAfterSec: data.retry_after,
      }
    } catch {
      return { success: false as const, message: '网络错误，请稍后重试' }
    }
  }

  const sendResetPasswordEmailCode = async (email: string) => {
    try {
      const res = await fetch('/api/auth/send-reset-password-code', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      })
      const data = (await res.json()) as {
        error?: string
        retry_after?: number
        hint?: string
        message?: string
      }
      if (res.ok) {
        return {
          success: true as const,
          message: data.message,
          hint: data.hint,
        }
      }
      return {
        success: false as const,
        message: data.error || '发送失败',
        retryAfterSec: data.retry_after,
      }
    } catch {
      return { success: false as const, message: '网络错误，请稍后重试' }
    }
  }

  const register = async (
    email: string,
    password: string,
    emailCode: string,
    betaCode?: string,
    inviteCode?: string,
    mode?: UserMode
  ) => {
    const requestBody: {
      email: string
      password: string
      email_code: string
      beta_code?: string
      invite_code?: string
      lang?: string
    } = { email, password, email_code: emailCode.trim(), lang: language }
    if (betaCode) {
      requestBody.beta_code = betaCode
    }
    if (inviteCode) {
      requestBody.invite_code = inviteCode
    }

    try {
      const result = await httpClient.post<{
        token: string
        user_id: string
        email: string
        display_name?: string
        avatar_url?: string
        balance_usdt?: number
        is_admin?: boolean
        is_finance?: boolean
        invite_code?: string
        profile_named?: boolean
        message: string
      }>('/api/register', requestBody)

      if (result.success && result.data) {
        // Clear stale onboarding state so new users always see the welcome flow
        localStorage.removeItem('nofx_beginner_onboarding_completed')
        localStorage.removeItem('nofx_beginner_wallet_address')

        const userInfo: User = {
          id: result.data.user_id,
          email: result.data.email,
          display_name: result.data.display_name,
          avatar_url: result.data.avatar_url,
          balance_usdt:
            typeof result.data.balance_usdt === 'number' ? result.data.balance_usdt : 0,
          is_admin: Boolean(result.data.is_admin),
          is_finance: Boolean(result.data.is_finance),
          invite_code: result.data.invite_code,
          profile_named: Boolean(result.data.profile_named),
        }
        handlePostAuthSuccess(result.data.token, userInfo, mode)

        return {
          success: true,
          message: result.message || result.data.message,
        }
      }

      // Only business errors reach here (system/network errors were intercepted)
      return {
        success: false,
        message: result.message || 'Registration failed',
      }
    } catch (error) {
      console.error('Auth register error:', error)
      // Re-throw if it's a critical error, or return structured error
      // Since httpClient throws on 500, we should return a structured error response
      // to let the UI display it gracefully without crashing.
      return {
        success: false,
        message:
          error instanceof Error ? error.message : 'Detailed server error',
      }
    }
  }

  const resetPassword = async (
    email: string,
    newPassword: string,
    emailCode: string
  ) => {
    try {
      const response = await fetch('/api/reset-password', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          email,
          new_password: newPassword,
          email_code: emailCode.trim(),
        }),
      })

      const data = await response.json()

      if (response.ok) {
        return { success: true, message: data.message }
      } else {
        return { success: false, message: data.error }
      }
    } catch (error) {
      return {
        success: false,
        message: 'Password reset failed, please try again',
      }
    }
  }

  const applyUserProfile = useCallback(
    (partial: Partial<
      Pick<
        User,
        | 'display_name'
        | 'avatar_url'
        | 'balance_usdt'
        | 'is_admin'
        | 'is_finance'
        | 'invite_code'
        | 'profile_named'
      >
    >) => {
      setUser((prev) => (prev ? { ...prev, ...partial } : prev))
      const raw = localStorage.getItem('auth_user')
      if (raw) {
        try {
          const prev = JSON.parse(raw) as User
          localStorage.setItem('auth_user', JSON.stringify({ ...prev, ...partial }))
        } catch {
          /* ignore */
        }
      }
    },
    []
  )

  const logout = () => {
    const savedToken = localStorage.getItem('auth_token')
    if (savedToken) {
      fetch('/api/logout', {
        method: 'POST',
        headers: { Authorization: `Bearer ${savedToken}` },
      }).catch(() => {
        /* ignore network errors on logout */
      })
    }
    setUser(null)
    setToken(null)
    localStorage.removeItem('auth_token')
    localStorage.removeItem('auth_user')
    invalidateSystemConfig()
    navigate(ROUTES.home)
  }

  return (
    <AuthContext.Provider
      value={{
        user,
        token,
        login,
        loginAdmin,
        register,
        resetPassword,
        sendRegisterEmailCode,
        sendResetPasswordEmailCode,
        logout,
        applyUserProfile,
        isLoading,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}
