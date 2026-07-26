/**
 * 浏览器端 UI 防护（仅 deterrent，无法真正阻止技术人员绕过）。
 * - 生产环境拦截常见打开开发者工具 / 查看源代码的快捷键。
 * - 输入框、文本域内允许右键，便于复制粘贴。
 */

function isBlockedShortcut(e: KeyboardEvent): boolean {
  const k = e.key
  const ctrl = e.ctrlKey || e.metaKey
  const shift = e.shiftKey
  const alt = e.altKey

  if (k === 'F12') return true

  // Chrome / Edge：Ctrl+Shift+I / J / C
  if (ctrl && shift && /^[ijc]$/i.test(k)) return true

  // 查看网页源代码（不拦截 Ctrl+S，避免 Monaco 等编辑器内保存失效）
  if (ctrl && !shift && /^u$/i.test(k)) return true

  // macOS：⌘⌥I 等
  if (e.metaKey && alt && /^i$/i.test(k)) return true

  // Firefox：Ctrl+Shift+K 控制台
  if (ctrl && shift && /^k$/i.test(k)) return true

  return false
}

export function installClientUiGuard(): void {
  const onKeyDown = (e: KeyboardEvent) => {
    if (!isBlockedShortcut(e)) return
    e.preventDefault()
    e.stopPropagation()
  }
  window.addEventListener('keydown', onKeyDown, true)

  const onContextMenu = (e: MouseEvent) => {
    const el = e.target as HTMLElement | null
    if (el?.closest?.('input, textarea, select, [contenteditable="true"]')) return
    e.preventDefault()
  }
  document.addEventListener('contextmenu', onContextMenu, true)
}
