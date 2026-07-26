import './strategyMonacoSetup'
import { useCallback, useEffect, useRef } from 'react'
import Editor, { type OnMount } from '@monaco-editor/react'
import type * as Monaco from 'monaco-editor'

const THEME_ID = 'quant-lum-dark'

function defineQuantTheme(monaco: typeof Monaco) {
  monaco.editor.defineTheme(THEME_ID, {
    base: 'vs-dark',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': '#131313',
      'editor.foreground': '#e8e8ec',
      'editorLineNumber.foreground': '#4b5058',
      'editorLineNumber.activeForeground': '#d4ff33',
      'editorCursor.foreground': '#d4ff33',
      'editor.lineHighlightBackground': '#1a1f2888',
      'editor.selectionBackground': '#d4ff3333',
      'editor.inactiveSelectionBackground': '#d4ff3318',
      'editorWhitespace.foreground': '#353941',
      'scrollbarSlider.background': '#46484d66',
      'scrollbarSlider.hoverBackground': '#d4ff3344',
      'scrollbarSlider.activeBackground': '#d4ff3366',
      'editorIndentGuide.background': '#252830',
      'editorIndentGuide.activeBackground': '#d4ff3340',
      'editorBracketMatch.background': '#d4ff3314',
      'editorBracketMatch.border': '#d4ff3355',
    },
  })
}

export interface StrategyPromptMonacoEditorProps {
  value: string
  onChange: (next: string) => void
  disabled?: boolean
  /** 编辑器可视高度（px） */
  heightPx?: number
  /** 语法模式：策略正文用 markdown；交易对列表用 plaintext */
  language?: 'markdown' | 'plaintext'
}

export default function StrategyPromptMonacoEditor({
  value,
  onChange,
  disabled = false,
  heightPx = 420,
  language = 'markdown',
}: StrategyPromptMonacoEditorProps) {
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  const disabledRef = useRef(disabled)
  disabledRef.current = disabled

  const editorRef = useRef<Monaco.editor.IStandaloneCodeEditor | null>(null)

  const handleMount: OnMount = useCallback((editor, monaco) => {
    editorRef.current = editor
    defineQuantTheme(monaco)
    monaco.editor.setTheme(THEME_ID)
    editor.updateOptions({ readOnly: disabledRef.current })
  }, [])

  useEffect(() => {
    editorRef.current?.updateOptions({ readOnly: disabled })
  }, [disabled])

  return (
    <div className="strategy-prompt-editor overflow-hidden rounded-lg border border-outline-variant/20 bg-surface-container-lowest">
      <Editor
        height={heightPx}
        language={language}
        theme={THEME_ID}
        value={value}
        onChange={(v) => onChangeRef.current(v ?? '')}
        beforeMount={(monaco) => {
          defineQuantTheme(monaco)
        }}
        onMount={handleMount}
        loading={
          <div
            className="flex items-center justify-center rounded-lg bg-surface-container-lowest text-xs text-on-surface-variant"
            style={{ height: heightPx }}
          >
            加载编辑器…
          </div>
        }
        options={{
          readOnly: disabled,
          minimap: { enabled: false },
          fontSize: 13,
          fontFamily: "JetBrains Mono, Menlo, Monaco, Consolas, monospace",
          fontLigatures: true,
          lineNumbers: 'on',
          lineNumbersMinChars: 3,
          scrollBeyondLastLine: false,
          wordWrap: 'on',
          wrappingIndent: 'same',
          padding: { top: 12, bottom: 12 },
          smoothScrolling: true,
          cursorBlinking: 'smooth',
          cursorSmoothCaretAnimation: 'on',
          renderLineHighlight: 'line',
          renderLineHighlightOnlyWhenFocus: true,
          selectionHighlight: true,
          bracketPairColorization: { enabled: true },
          guides: { bracketPairs: true, indentation: true },
          folding: true,
          automaticLayout: true,
          scrollbar: {
            verticalScrollbarSize: 8,
            horizontalScrollbarSize: 8,
            useShadows: false,
          },
          overviewRulerLanes: 0,
          hideCursorInOverviewRuler: true,
          overviewRulerBorder: false,
          glyphMargin: false,
          contextmenu: true,
          quickSuggestions: false,
          parameterHints: { enabled: false },
          suggestOnTriggerCharacters: false,
          acceptSuggestionOnEnter: 'off',
          tabSize: 2,
          insertSpaces: true,
          detectIndentation: false,
          scrollPredominantAxis: true,
          mouseWheelScrollSensitivity: 1.2,
          fastScrollSensitivity: 5,
        }}
      />
    </div>
  )
}
