import { Component, type ErrorInfo, type ReactNode } from 'react'

type Props = { children: ReactNode }

type State = { error: Error | null }

/**
 * 根级错误边界：任意子组件渲染/生命周期抛错时，避免整页只剩黑底无任何提示。
 */
export class RootErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('[RootErrorBoundary]', error, info.componentStack)
  }

  render(): ReactNode {
    if (this.state.error) {
      const msg = this.state.error.message || String(this.state.error)
      return (
        <div
          className="flex min-h-screen flex-col items-center justify-center gap-4 px-6 text-center"
          style={{ background: '#0b0b0b', color: '#EAECEF', fontFamily: 'system-ui, sans-serif' }}
        >
          <h1 className="text-xl font-bold" style={{ color: '#d4ff33' }}>
            页面加载出错
          </h1>
          <p className="max-w-lg text-sm leading-relaxed opacity-90">
            界面在渲染时发生了未捕获的异常。请先看下面一行技术信息（可复制给开发者），然后点刷新；若仍黑屏，请强制清缓存：
            <span className="whitespace-nowrap"> Ctrl+Shift+R </span>
            （Mac：<span className="whitespace-nowrap"> Cmd+Shift+R </span>）。
          </p>
          <pre
            className="max-h-40 max-w-full overflow-auto rounded-lg border px-3 py-2 text-left text-xs"
            style={{ borderColor: '#2b3139', background: '#131313', color: '#848E9C' }}
          >
            {msg}
          </pre>
          <button
            type="button"
            className="rounded-lg px-5 py-2.5 text-sm font-semibold transition-opacity hover:opacity-90"
            style={{ background: '#d4ff33', color: '#0b0b0b' }}
            onClick={() => window.location.reload()}
          >
            重新加载
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
