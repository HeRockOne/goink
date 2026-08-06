import ReactDiffViewer from 'react-diff-viewer-continued'
import { FileCode, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useTheme } from '@/hooks/useTheme'
import type { git } from '@/lib/wailsjs/go/models'

const FONT_STYLE = {
  fontFamily: "'Noto Serif SC', 'Source Han Serif SC', serif",
  fontSize: '15px',
  lineHeight: '26px',
  wordWrap: 'break-word' as const,
  whiteSpace: 'pre-wrap' as const,
}

const DIFF_STYLES = {
  variables: {
    light: {
      diffViewerBackground: 'var(--background)',
      diffViewerColor: 'var(--foreground)',
      addedBackground: 'var(--diff-add-bg, #d4e8d4)',
      removedBackground: 'var(--diff-remove-bg, #f0d4d4)',
      addedColor: 'var(--foreground)',
      removedColor: 'var(--foreground)',
      emptyBlockBackground: 'var(--muted)',
      gutterBackground: 'var(--muted)',
      gutterColor: 'var(--foreground)',
    },
    dark: {
      diffViewerBackground: 'var(--background)',
      diffViewerColor: 'var(--foreground)',
      addedBackground: 'var(--diff-add-bg-dark, #1a3020)',
      removedBackground: 'var(--diff-remove-bg-dark, #3a1818)',
      addedColor: 'var(--foreground)',
      removedColor: 'var(--foreground)',
      emptyBlockBackground: 'var(--muted)',
      gutterBackground: 'var(--muted)',
      gutterColor: 'var(--foreground)',
    },
  },
  contentText: FONT_STYLE,
  lineNumber: { ...FONT_STYLE, opacity: 0.6 },
}

interface Props {
  file: git.FileDiff | null
}

export default function GitCommitView({ file }: Props) {
  const { t } = useTranslation()
  const { theme } = useTheme()

  if (!file) {
    return (
      <main className="flex-1 bg-background flex items-center justify-center border-r">
        <div className="text-center">
          <FileCode className="w-10 h-10 text-muted-foreground/30 mx-auto mb-3" />
          <p className="text-sm text-muted-foreground">{t('git.selectFileToDiff')}</p>
        </div>
      </main>
    )
  }

  return (
    <main className="flex-1 bg-background flex flex-col min-w-0 min-h-0 border-r overflow-hidden">
      <div className="flex items-center gap-2 px-4 py-1.5 border-b shrink-0 bg-muted/10">
        <FileText className="w-3.5 h-3.5 text-muted-foreground" />
        <span className="text-xs text-muted-foreground truncate">{file.path}</span>
      </div>

      <div className="flex-1 overflow-auto">
        <ReactDiffViewer
          oldValue={file.original}
          newValue={file.modified}
          splitView={false}
          useDarkTheme={theme === 'dark'}
          styles={DIFF_STYLES}
        />
      </div>
    </main>
  )
}