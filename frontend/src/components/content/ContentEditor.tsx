import { useCallback } from 'react'
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
  '&': { backgroundColor: 'var(--editor-surface)', color: 'var(--foreground)' },
  '.cm-content': { fontFamily: 'var(--font-body)', fontSize: 'var(--font-size, 17px)', lineHeight: '30px', padding: '0 16px' },
  '.cm-gutters': { display: 'none' },
  '.cm-activeLine': { backgroundColor: 'transparent' },
  '.cm-cursor': { borderLeftColor: 'var(--primary)' },
  '.cm-selectionBackground': { backgroundColor: 'var(--accent)' },
  '&.cm-editor.cm-focused': { outline: 'none' },
})

const DARK_THEME = EditorView.theme({
  '&': { backgroundColor: 'var(--editor-surface)', color: 'var(--foreground)' },
  '.cm-content': { fontFamily: 'var(--font-body)', fontSize: 'var(--font-size, 17px)', lineHeight: '30px', padding: '0 16px' },
  '.cm-gutters': { display: 'none' },
  '.cm-activeLine': { backgroundColor: 'transparent' },
  '.cm-cursor': { borderLeftColor: 'var(--primary)' },
  '.cm-selectionBackground': { backgroundColor: 'var(--accent)' },
  '&.cm-editor.cm-focused': { outline: 'none' },
})

export default function ContentEditor({ value, onChange, onMount, editorTheme }: Props) {
  const handleCreate = useCallback((view: EditorView) => {
    onMount?.(view)
  }, [onMount])

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