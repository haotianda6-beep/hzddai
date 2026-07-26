/**
 * Turn raw fetch/HTTP failures into short, user-safe messages (no HTML, no dev hints).
 */

const HTML_MARKERS = /<!doctype\s+html|<html[\s>]/i

const DEV_HINT_PATTERN =
  /vite_api_base|同源\s*\/api|reverse\s+proxy|network\s+tab|docker\s+internal/i

export function isHtmlLikeBody(text: string): boolean {
  const t = text.trim()
  if (!t) return false
  if (HTML_MARKERS.test(t)) return true
  if (t.startsWith('<') && t.includes('</')) return true
  return false
}

function messageForStatus(status: number, lang: 'zh' | 'en'): string {
  if (status === 502 || status === 503 || status === 504) {
    return lang === 'zh' ? '服务暂时不可用，请稍后重试' : 'Service temporarily unavailable. Please try again.'
  }
  if (status === 401) {
    return lang === 'zh' ? '请先登录后再试' : 'Please sign in and try again.'
  }
  if (status === 403) {
    return lang === 'zh' ? '暂无权限访问' : 'You do not have permission to access this.'
  }
  if (status === 404) {
    return lang === 'zh' ? '请求的资源不存在' : 'The requested resource was not found.'
  }
  if (status >= 500) {
    return lang === 'zh' ? '服务器异常，请稍后重试' : 'Server error. Please try again later.'
  }
  if (status >= 400) {
    return lang === 'zh' ? '请求失败，请稍后重试' : 'Request failed. Please try again.'
  }
  return lang === 'zh' ? '加载失败，请稍后重试' : 'Failed to load. Please try again.'
}

function tryParseJsonMessage(text: string): string | null {
  const t = text.trim()
  if (!t.startsWith('{') && !t.startsWith('[')) return null
  try {
    const data = JSON.parse(t) as { error?: string; message?: string }
    const msg = (data.error || data.message || '').trim()
    if (!msg || isHtmlLikeBody(msg) || DEV_HINT_PATTERN.test(msg)) return null
    if (msg.length > 160) return `${msg.slice(0, 157)}…`
    return msg
  } catch {
    return null
  }
}

function sanitizePlainMessage(text: string): string | null {
  const t = text.trim()
  if (!t || isHtmlLikeBody(t) || DEV_HINT_PATTERN.test(t)) return null
  if (t.length > 160) return `${t.slice(0, 157)}…`
  return t
}

/**
 * @param status HTTP status (0 if unknown)
 * @param body optional response body text
 */
export function formatUserFacingFetchError(
  status: number,
  body?: string,
  lang: 'zh' | 'en' = 'zh'
): string {
  const raw = (body ?? '').trim()
  if (raw) {
    const jsonMsg = tryParseJsonMessage(raw)
    if (jsonMsg) return jsonMsg
    const plain = sanitizePlainMessage(raw)
    if (plain) return plain
  }
  return messageForStatus(status, lang)
}

export function formatUserFacingError(
  err: unknown,
  lang: 'zh' | 'en' = 'zh'
): string {
  if (!(err instanceof Error)) {
    return lang === 'zh' ? '加载失败，请稍后重试' : 'Failed to load. Please try again.'
  }
  const msg = err.message.trim()
  if (!msg || isHtmlLikeBody(msg) || DEV_HINT_PATTERN.test(msg)) {
    const statusMatch = /HTTP\s+(\d{3})/i.exec(msg)
    const status = statusMatch ? Number(statusMatch[1]) : 0
    return messageForStatus(status, lang)
  }
  if (msg.length > 160) return `${msg.slice(0, 157)}…`
  return msg
}

export function throwUserFacingFetchError(
  status: number,
  body?: string,
  lang: 'zh' | 'en' = 'zh'
): never {
  const friendly = formatUserFacingFetchError(status, body, lang)
  if (import.meta.env.DEV && body?.trim()) {
    console.warn('[fetch] non-ok response', { status, bodyPreview: body.trim().slice(0, 400) })
  }
  throw new Error(friendly)
}
