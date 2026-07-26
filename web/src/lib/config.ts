export interface SystemConfig {
  initialized: boolean
  beta_mode?: boolean
}

let configPromise: Promise<SystemConfig> | null = null
let cachedConfig: SystemConfig | null = null

/** 避免 /api/config 长时间无响应时整站卡在「加载中」 */
const CONFIG_FETCH_TIMEOUT_MS = 12_000

function fetchConfigWithTimeout(): Promise<Response> {
  const ctrl = new AbortController()
  const id = window.setTimeout(() => ctrl.abort(), CONFIG_FETCH_TIMEOUT_MS)
  return fetch('/api/config', { signal: ctrl.signal }).finally(() => {
    window.clearTimeout(id)
  })
}

export function getSystemConfig(): Promise<SystemConfig> {
  if (cachedConfig) {
    return Promise.resolve(cachedConfig)
  }
  if (configPromise) {
    return configPromise
  }
  configPromise = fetchConfigWithTimeout()
    .then((res) => {
      if (!res.ok) {
        throw new Error(`配置接口异常 HTTP ${res.status}`)
      }
      return res.json() as Promise<SystemConfig>
    })
    .then((data: SystemConfig) => {
      cachedConfig = data
      return data
    })
    .catch((err: unknown) => {
      configPromise = null
      if (err instanceof Error && err.name === 'AbortError') {
        throw new Error(
          `拉取系统配置超时（${CONFIG_FETCH_TIMEOUT_MS / 1000}s），请检查网络与后端 /api 是否可达`
        )
      }
      throw err instanceof Error ? err : new Error(String(err))
    })
  return configPromise
}

/** Call after first-time setup completes so next check reflects initialized=true */
export function invalidateSystemConfig() {
  cachedConfig = null
  configPromise = null
  window.dispatchEvent(new Event('system-config-invalidated'))
}
