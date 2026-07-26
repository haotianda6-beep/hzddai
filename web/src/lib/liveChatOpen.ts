import { toast } from 'sonner'

/** 萤火强调色（与顶栏、充值页一致） */
export const FIREFLY_ACCENT = '#d4ff33'

type TawkApiPartial = {
  customStyle?: Record<string, unknown>
  maximize?: () => void
  showWidget?: () => void
  hideWidget?: () => void
  isChatMaximized?: () => boolean
  onLoad?: (...args: unknown[]) => void
}

/** 用户点开聊天后短时间内不要 auto-hide，否则会和 maximize 打架 */
let tawkHideSuppressUntil = 0

export function suppressTawkAutoHide(ms: number): void {
  tawkHideSuppressUntil = Math.max(tawkHideSuppressUntil, Date.now() + ms)
}

/**
 * 定时压掉 Tawk 默认小圆球（仅在小窗未展开时）；展开对话后由 isChatMaximized 跳过。
 * 返回 stop 函数供 useEffect cleanup。
 */
export function startTawkDefaultLauncherHideLoop(): () => void {
  const id = window.setInterval(() => {
    if (Date.now() < tawkHideSuppressUntil) return
    const win = window as Window & { Tawk_API?: TawkApiPartial }
    const api = win.Tawk_API
    if (!api?.hideWidget) return
    try {
      if (typeof api.isChatMaximized === 'function' && api.isChatMaximized()) return
      api.hideWidget()
    } catch {
      /* ignore */
    }
  }, 550)
  return () => window.clearInterval(id)
}

export function isTawkConfigured(): boolean {
  const p = (import.meta.env.VITE_TAWK_PROPERTY_ID as string | undefined)?.trim()
  const w = (import.meta.env.VITE_TAWK_WIDGET_ID as string | undefined)?.trim()
  return Boolean(p && w)
}

/**
 * Tawk 要求：customStyle 必须在下载 embed 脚本之前就挂在 window.Tawk_API 上。
 * 这里用萤火站点的 z-index、右下角位置；聊天窗「里层」配色需在 tawk 后台
 * Administration → Chat Widget → Widget Appearance 把 Header 等设为 #d4ff33 系。
 */
export function prepareTawkEmbedBeforeScript(): void {
  if (!isTawkConfigured()) return
  const win = window as Window & { Tawk_API?: TawkApiPartial }
  win.Tawk_API = win.Tawk_API || {}
  win.Tawk_API.customStyle = {
    zIndex: 2147483000,
    visibility: {
      desktop: { position: 'br', xOffset: 18, yOffset: 22 },
      mobile: { position: 'br', xOffset: 10, yOffset: 20 },
      bubble: { rotate: '0deg', xOffset: 0, yOffset: 0 },
    },
  }
  // 隐藏 Tawk 默认圆形气泡，改用站内右侧「在线客服」条
  const prevOnLoad = win.Tawk_API.onLoad
  win.Tawk_API.onLoad = function (this: unknown, ...args: unknown[]) {
    if (typeof prevOnLoad === 'function') {
      prevOnLoad.apply(this, args)
    }
    try {
      win.Tawk_API?.hideWidget?.()
    } catch {
      /* ignore */
    }
  }
}

function openTawkMaximized(): void {
  suppressTawkAutoHide(4500)
  let cleared = false
  const stop = (timer: number) => {
    if (!cleared) {
      cleared = true
      window.clearInterval(timer)
    }
  }
  let n = 0
  const id = window.setInterval(() => {
    n += 1
    const api = (window as Window & { Tawk_API?: Pick<TawkApiPartial, 'maximize' | 'showWidget'> }).Tawk_API
    if (api?.maximize) {
      try {
        api.showWidget?.()
        api.maximize()
      } catch {
        /* ignore */
      }
      stop(id)
      return
    }
    if (n >= 45) {
      stop(id)
      toast.message('在线客服还在加载，请等页面右下角图标出现后点一下，或稍后重试')
    }
  }, 160)
}

/** 打开当前站点已接入的网页客服（Crisp 优先，其次 Tawk） */
export function openLiveChatPanel(): void {
  const crisp = (import.meta.env.VITE_CRISP_WEBSITE_ID as string | undefined)?.trim()
  if (crisp) {
    try {
      const w = window as Window & { $crisp?: unknown[] }
      w.$crisp = w.$crisp || []
      w.$crisp.push(['do', 'chat:show'])
      w.$crisp.push(['do', 'chat:open'])
    } catch {
      toast.error('无法打开客服窗口')
    }
    return
  }
  if (!isTawkConfigured()) {
    toast.info('未配置网页在线客服，请使用下方邮箱联系')
    return
  }
  openTawkMaximized()
}
