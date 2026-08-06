import { useCallback, useEffect, useRef } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { EditorView } from 'codemirror'

interface Props {
  value: string
  onChange: (value: string) => void
  onMount?: (view: EditorView) => void
  editorTheme?: string
}

const LIGHT_THEME = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'var(--editor-surface)', color: 'var(--foreground)' },
  '.cm-scroller': { flex: '1 1 auto', overflow: 'auto' },
  '.cm-content': { fontFamily: 'var(--font-body)', fontSize: 'var(--font-size, 17px)', lineHeight: '30px', padding: '0 16px' },
  '.cm-gutters': { display: 'none' },
  '.cm-activeLine': { backgroundColor: 'transparent' },
  '.cm-cursor': { borderLeftColor: 'var(--primary)' },
  '.cm-selectionBackground': { backgroundColor: 'var(--accent)' },
  '&.cm-editor.cm-focused': { outline: 'none' },
})

const DARK_THEME = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'var(--editor-surface)', color: 'var(--foreground)' },
  '.cm-scroller': { flex: '1 1 auto', overflow: 'auto' },
  '.cm-content': { fontFamily: 'var(--font-body)', fontSize: 'var(--font-size, 17px)', lineHeight: '30px', padding: '0 16px' },
  '.cm-gutters': { display: 'none' },
  '.cm-activeLine': { backgroundColor: 'transparent' },
  '.cm-cursor': { borderLeftColor: 'var(--primary)' },
  '.cm-selectionBackground': { backgroundColor: 'var(--accent)' },
  '&.cm-editor.cm-focused': { outline: 'none' },
})

export default function ContentEditor({ value, onChange, onMount, editorTheme }: Props) {
  const viewRef = useRef<EditorView | null>(null)

  const handleCreate = useCallback((view: EditorView) => {
    viewRef.current = view
    onMount?.(view)
  }, [onMount])

  // 全局字号/字体变化后强制重新测量（行高/换行/视口高度），
  // 否则编辑器高度停留在旧字号的计算值，面板底部出现空白（最大化/还原时 resize 才触发重算）
  useEffect(() => {
    const target = document.documentElement
    let timer: ReturnType<typeof setTimeout> | null = null
    const observer = new MutationObserver(() => {
      // 多帧重测：CodeMirror 的测量依赖渲染稳定（换行/行高需要字号真正生效后）
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        viewRef.current?.requestMeasure()
        setTimeout(() => viewRef.current?.requestMeasure(), 50)
      }, 30)
    })
    observer.observe(target, { attributes: true, attributeFilter: ['style'] })
    // 兜底：挂载后延迟重测两次，覆盖窗口尺寸恢复/首帧 vh 不稳
    const t1 = setTimeout(() => viewRef.current?.requestMeasure(), 300)
    const t2 = setTimeout(() => viewRef.current?.requestMeasure(), 900)
    return () => { observer.disconnect(); clearTimeout(timer ?? undefined); clearTimeout(t1); clearTimeout(t2) }
  }, [])

  const extensions = [
    markdown({ base: markdownLanguage }),
    EditorView.lineWrapping,
    editorTheme?.includes('dark') ? DARK_THEME : LIGHT_THEME,
  ]

  return (
    <div className="h-full overflow-hidden">
      <CodeMirror
        value={value}
        height="100%"
        style={{ height: '100%' }}
        theme={editorTheme?.includes('dark') ? 'dark' : 'light'}
        extensions={extensions}
        onChange={onChange}
        onCreateEditor={handleCreate}
        basicSetup={{
          lineNumbers: false,
          foldGutter: false,
          highlightActiveLine: false,
          highlightActiveLineGutter: false,
          bracketMatching: false,
          closeBrackets: false,
          autocompletion: false,
          indentOnInput: false,
          syntaxHighlighting: true,
        }}
      />
    </div>
  )
}