import { describe, expect, it } from 'vitest'
import {
  BALIB_API_BASE_URL,
  SUPPORTED_EXCHANGE_TEMPLATES,
  getExchangeCredentialFields,
} from './ExchangeConfigModal'

describe('BALIB trading account configuration', () => {
  it('uses the BALIB name and only asks the user for two API credentials', () => {
    const hz = SUPPORTED_EXCHANGE_TEMPLATES.find(
      (exchange) => exchange.exchange_type === 'hz'
    )
    expect(hz?.name).toBe('BALIB')
    expect(hz?.type).toBe('dex')
    expect(hz?.name).not.toContain('模拟')
    expect(getExchangeCredentialFields('hz')).toEqual({
      apiUrl: false,
      apiKey: true,
      secretKey: true,
      passphrase: false,
    })
    expect(BALIB_API_BASE_URL).toBe('https://trade.kunai.fun/api/v1')
  })
})
