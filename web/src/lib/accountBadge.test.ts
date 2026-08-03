import { describe, expect, it } from 'vitest'
import { getAccountBadgeName } from './accountBadge'

describe('getAccountBadgeName', () => {
  it('shows the administrator account badge ahead of the rebate role', () => {
    expect(getAccountBadgeName(true, 'retail')).toBe('超级管理员')
  })

  it('keeps the partner role badge for normal accounts', () => {
    expect(getAccountBadgeName(false, 'retail')).toBe('散户')
    expect(getAccountBadgeName(false, 'branch')).toBe('分公司')
  })
})
