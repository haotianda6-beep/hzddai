import { describe, expect, it } from 'vitest'
import {
  formatUserFacingFetchError,
  formatUserFacingError,
  isHtmlLikeBody,
} from './userFacingFetchError'

describe('userFacingFetchError', () => {
  it('detects nginx/html bodies', () => {
    expect(isHtmlLikeBody('<html><head><title>502</title></head></html>')).toBe(true)
    expect(isHtmlLikeBody('{"error":"not found"}')).toBe(false)
  })

  it('maps 502 html to friendly zh message', () => {
    const html = '<!DOCTYPE html><html><body>502 Bad Gateway</body></html>'
    expect(formatUserFacingFetchError(502, html, 'zh')).toBe('服务暂时不可用，请稍后重试')
  })

  it('uses json error when present', () => {
    expect(formatUserFacingFetchError(400, '{"error":"策略不存在"}', 'zh')).toBe('策略不存在')
  })

  it('strips dev hints from displayed errors', () => {
    const err = new Error('请把 VITE_API_BASE 设为同源 /api')
    expect(formatUserFacingError(err, 'zh')).toBe('加载失败，请稍后重试')
  })
})
