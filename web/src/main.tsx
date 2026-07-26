import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.tsx'
import { Toaster } from 'sonner'
import './index.css'
import { BrowserRouter } from 'react-router-dom'
import { RootErrorBoundary } from './components/common/RootErrorBoundary'
import { installClientUiGuard } from './lib/clientGuard'

/** 生产环境：拦截常见 F12 / 开发者工具快捷键（无法彻底禁止，仅降低误触与随意改前端） */
if (import.meta.env.PROD) {
  installClientUiGuard()
}

declare global {
  interface Window {
    __COMKUN_BOOT_OK?: () => void
  }
}

function showFatalBootError(err: unknown) {
  const rootEl = document.getElementById('root')
  const msg = err instanceof Error ? err.message : String(err)
  console.error('[boot]', err)
  if (rootEl) {
    rootEl.innerHTML =
      '<div style="min-height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:24px;font-family:system-ui,sans-serif;background:#0b0b0b;color:#eaecef;text-align:center">' +
      '<h1 style="color:#d4ff33;font-size:18px">应用启动失败</h1>' +
      '<pre style="margin-top:12px;max-width:520px;text-align:left;font-size:12px;white-space:pre-wrap;word-break:break-word;opacity:.9">' +
      msg.replace(/</g, '&lt;') +
      '</pre>' +
      '<button type="button" style="margin-top:20px;padding:10px 18px;border-radius:10px;border:0;background:#d4ff33;color:#111;font-weight:600;cursor:pointer" onclick="location.reload()">重新加载</button></div>'
  }
}

/**
 * 必须使用静态 import App，不要 dynamic import。
 * 否则 Vite 会单独生成 `assets/App-*.js`；在域名反代、CDN 或部分缓存下，
 * 主包与分片版本不一致时会出现「Failed to fetch dynamically imported module」整站挂掉。
 */
const rootEl = document.getElementById('root')
if (!rootEl) {
  console.error('#root 不存在')
} else {
  try {
    ReactDOM.createRoot(rootEl).render(
      <React.StrictMode>
        <RootErrorBoundary>
          <BrowserRouter>
            <Toaster
              theme="dark"
              richColors
              closeButton
              position="top-center"
              duration={2200}
              toastOptions={{
                className: 'nofx-toast',
                style: {
                  background: '#131313',
                  border: '1px solid var(--panel-border)',
                  color: 'var(--text-primary)',
                },
              }}
            />
            <App />
          </BrowserRouter>
        </RootErrorBoundary>
      </React.StrictMode>
    )
    // 必须在 render 调用后立即标记：React 18 提交 DOM 可能晚于首个宏任务，
    // 若仅用 setTimeout+children.length，index.html 的 10s  watchdog 会误判「长时间未显示」。
    window.__COMKUN_BOOT_OK?.()
    window.requestAnimationFrame(() => {
      window.__COMKUN_BOOT_OK?.()
    })
  } catch (err) {
    showFatalBootError(err)
  }
}
