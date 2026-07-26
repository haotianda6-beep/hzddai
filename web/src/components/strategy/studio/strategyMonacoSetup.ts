/**
 * Monaco + Vite：注册 Worker 并绑定本地 monaco 包（避免默认 CDN 路径在生产失效）
 * 在任意使用 @monaco-editor/react 的模块中最先 import 本文件一次即可。
 */
import { loader } from '@monaco-editor/react'
import * as monaco from 'monaco-editor'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'

const g = globalThis as typeof globalThis & {
  MonacoEnvironment?: { getWorker: (_workerId: string, label: string) => Worker }
}

if (!g.MonacoEnvironment) {
  g.MonacoEnvironment = {
    /** 策略正文只用 Markdown：统一走 editor worker，避免拉取 6MB 的 TS worker */
    getWorker() {
      return new EditorWorker()
    },
  }
}

loader.config({ monaco })

export { monaco }
