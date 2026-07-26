import { describe, expect, it } from 'vitest'
import { loginErrorMessage } from './AuthContext'

describe('loginErrorMessage', () => {
  it('localizes invalid credentials and keeps other server errors', () => {
    expect(loginErrorMessage(401, 'Email or password incorrect', 'zh')).toBe(
      '邮箱或密码错误'
    )
    expect(loginErrorMessage(429, 'Too many attempts', 'zh')).toBe(
      'Too many attempts'
    )
  })
})
