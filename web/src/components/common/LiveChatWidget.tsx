import { useEffect, useMemo, useState } from 'react'
import { Headphones, X } from 'lucide-react'
import {
  openLiveChatPanel,
  prepareTawkEmbedBeforeScript,
  startTawkDefaultLauncherHideLoop,
} from '../../lib/liveChatOpen'

/**
 * 垂直居中用 top-0 bottom-0 my-auto + 固定高度，不用 translateY(-50%)。
 * 否则 hover 时子像素/字体渲染导致高度微变，50% 位移会跟着变，看起来会上下抖。
 */
const EDGE_LAUNCHER_CLASS =
  'fixed right-3 top-[74px] z-[95] flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-[#d4ff33]/55 bg-[#161a14] text-[#d4ff33] shadow-[-6px_0_20px_rgba(0,0,0,0.28)] hover:bg-[#1c2218] hover:text-[#f7ffcc] focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-[#d4ff33] focus-visible:outline-offset-0 sm:right-0 sm:top-0 sm:bottom-0 sm:my-auto sm:h-[172px] sm:w-12 sm:flex-col sm:rounded-l-xl sm:rounded-r-none sm:border-y sm:border-l sm:border-r-0 sm:text-[14px] sm:font-bold sm:leading-none sm:[text-orientation:mixed] sm:[writing-mode:vertical-rl]'

/** 右侧竖条入口（萤火配色），替代各厂商默认悬浮气泡 */
function EdgeLiveChatLauncher() {
  return (
    <button
      type="button"
      onClick={() => openLiveChatPanel()}
      className={EDGE_LAUNCHER_CLASS}
      aria-label="打开在线客服"
      title="在线客服"
    >
      <Headphones className="h-5 w-5 sm:hidden" aria-hidden />
      <span className="hidden sm:inline">在线客服</span>
    </button>
  )
}

/**
 * 全站在线客服
 *
 * 优先级：Crisp > Tawk.to > 自定义 iframe
 *
 * Tawk：在仓库根目录 `.env` 填写 VITE_TAWK_PROPERTY_ID、VITE_TAWK_WIDGET_ID（与嵌入代码
 * `https://embed.tawk.to/{property}/{widget}` 两段一致），然后重建 nofx-frontend。
 * Crisp：VITE_CRISP_WEBSITE_ID
 * 自定义 iframe：未配 Crisp 且未配齐 Tawk 两项时，可用 VITE_LIVECHAT_IFRAME_URL
 */

function IframeLiveChat({ url }: { url: string }) {
  const [open, setOpen] = useState(false)

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={EDGE_LAUNCHER_CLASS}
        aria-label="打开在线客服"
        title="在线客服"
      >
        <Headphones className="h-5 w-5 sm:hidden" aria-hidden />
        <span className="hidden sm:inline">在线客服</span>
      </button>

      {open && (
        <div
          className="fixed inset-0 z-[100] flex items-end justify-end bg-black/50 p-4 sm:items-center sm:justify-end sm:p-6"
          role="dialog"
          aria-modal="true"
          aria-label="在线客服对话窗口"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="flex h-[min(560px,85vh)] w-full max-w-md flex-col overflow-hidden rounded-2xl border border-[#2b3139] bg-nofx-bg-tertiary shadow-2xl">
            <div className="flex items-center justify-between border-b border-[#2b3139] px-4 py-3">
              <span className="text-sm font-bold text-[#EAECEF]">在线客服</span>
              <button
                type="button"
                onClick={() => setOpen(false)}
                className="rounded-lg p-1.5 text-[#848E9C] transition-colors hover:bg-white/10 hover:text-[#EAECEF]"
                aria-label="关闭"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <iframe
              title="在线客服"
              src={url}
              className="min-h-0 flex-1 w-full border-0 bg-nofx-bg-tertiary"
              allow="microphone; camera; clipboard-write"
            />
          </div>
        </div>
      )}
    </>
  )
}

export function LiveChatWidget() {
  const crispId = useMemo(() => (import.meta.env.VITE_CRISP_WEBSITE_ID as string | undefined)?.trim(), [])
  const tawkP = useMemo(() => (import.meta.env.VITE_TAWK_PROPERTY_ID as string | undefined)?.trim() ?? '', [])
  const tawkW = useMemo(() => (import.meta.env.VITE_TAWK_WIDGET_ID as string | undefined)?.trim() ?? '', [])
  const iframeUrl = useMemo(() => (import.meta.env.VITE_LIVECHAT_IFRAME_URL as string | undefined)?.trim(), [])

  const mode = useMemo(() => {
    if (crispId) return 'crisp' as const
    if (tawkP && tawkW) return 'tawk' as const
    if (iframeUrl) return 'iframe' as const
    return 'none' as const
  }, [crispId, tawkP, tawkW, iframeUrl])

  useEffect(() => {
    if (mode === 'iframe' || mode === 'none') return undefined

    if (mode === 'crisp' && crispId) {
      const w = window as Window & { $crisp?: unknown[]; CRISP_WEBSITE_ID?: string }
      w.$crisp = w.$crisp || []
      w.CRISP_WEBSITE_ID = crispId
      w.$crisp.push(['do', 'chat:hide'])
      const s = document.createElement('script')
      s.src = 'https://client.crisp.chat/l.js'
      s.async = true
      s.id = 'comkun-crisp-chat'
      document.head.appendChild(s)
      const hideAgain = [400, 1600, 3200].map((ms) =>
        window.setTimeout(() => {
          try {
            w.$crisp?.push(['do', 'chat:hide'])
          } catch {
            /* ignore */
          }
        }, ms),
      )
      return () => {
        hideAgain.forEach((t) => window.clearTimeout(t))
        try {
          document.getElementById('comkun-crisp-chat')?.remove()
        } catch {
          /* ignore */
        }
      }
    }

    if (mode === 'tawk' && tawkP && tawkW) {
      prepareTawkEmbedBeforeScript()
      const s = document.createElement('script')
      s.async = true
      s.id = 'comkun-tawk-chat'
      s.src = `https://embed.tawk.to/${tawkP}/${tawkW}`
      s.charset = 'UTF-8'
      document.head.appendChild(s)
      const stopHideLoop = startTawkDefaultLauncherHideLoop()
      return () => {
        stopHideLoop()
        try {
          document.getElementById('comkun-tawk-chat')?.remove()
        } catch {
          /* ignore */
        }
      }
    }

    return undefined
  }, [mode, crispId, tawkP, tawkW])

  if (mode === 'iframe' && iframeUrl) {
    return <IframeLiveChat url={iframeUrl} />
  }

  if (mode === 'crisp' || mode === 'tawk') {
    return <EdgeLiveChatLauncher />
  }

  return null
}
