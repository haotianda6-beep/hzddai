import { describe, expect, it } from 'vitest'
import {
  SUPPORTED_EXCHANGE_TEMPLATES,
  getExchangeCredentialFields,
} from './ExchangeConfigModal'

describe('BALIB trading account configuration', () => {
  it('uses the approved name and exactly the required credential fields', () => {
    const hz = SUPPORTED_EXCHANGE_TEMPLATES.find(
      (exchange) => exchange.exchange_type === 'hz'
    )
    expect(hz?.name).toBe('BALIB 交易账户')
    expect(hz?.name).not.toContain('模拟')
    expect(getExchangeCredentialFields('hz')).toEqual({
      apiUrl: true,
      apiKey: true,
      secretKey: true,
      passphrase: false,
    })
  })
})
