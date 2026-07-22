import { memo, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Search, ExternalLink, ChevronDown, ChevronRight, Copy, Check } from 'lucide-react'
import Markdown from '@/components/Markdown'
import './WebSearchCard.css'

interface SourceItem {
  title: string
  url: string
}

interface Props {
  result: Record<string, unknown>
}

function openExternal(url: string, t: TFunction) {
  if (window.confirm(`${t('chat.openInBrowser')}\n${url}`)) {
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}

export default memo(function WebSearchCard({ result }: Props) {
  const { t } = useTranslation()
  const [summaryOpen, setSummaryOpen] = useState(false)
  const [copied, setCopied] = useState(false)

  const queries = (result.queries as string[]) || []
  const summary = (result.summary as string) || ''
  const sources = (result.sources as SourceItem[]) || []

  const handleCopySummary = useCallback(() => {
    navigator.clipboard.writeText(summary).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }, [summary])

  return (
    <div className="web-card completed">
      <div className="web-card-row">
        <span className="web-card-icon"><Search size={14} /></span>
        <span className="web-card-label">{t('chat.searchComplete')}</span>
        <span className="web-card-badge web-card-badge-done">{t('chat.done')}</span>
      </div>

      {queries.length > 0 && (
        <div className="web-card-queries">
          <span className="web-card-queries-label">{t('chat.searchQuery')}</span>
          {queries.map((q, i) => (
            <span key={i} className="web-card-query-tag">{q}</span>
          ))}
        </div>
      )}

      {sources.length > 0 && (
        <div className="web-card-sources">
          {sources.map((s, i) => (
            <div
              key={i}
              className="web-card-source"
              onClick={() => openExternal(s.url, t)}
              title={s.url}
            >
              <span className="web-card-source-index">{i + 1}</span>
              <div className="web-card-source-body">
                <span className="web-card-source-title">{s.title || s.url}</span>
                <span className="web-card-source-url">{s.url}</span>
              </div>
              <ExternalLink size={12} className="web-card-source-ext" />
            </div>
          ))}
        </div>
      )}

      {summary && (
        <div className="web-card-summary">
          <div className="flex items-center">
            <button
              className="web-card-summary-toggle flex-1"
              onClick={() => setSummaryOpen(!summaryOpen)}
            >
              {summaryOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
              {t('chat.searchResultSummary')}
            </button>
            <button
              onClick={handleCopySummary}
              className="p-1 rounded hover:bg-muted/50 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              title={copied ? '已复制' : '复制摘要'}
            >
              {copied ? <Check size={12} className="text-green-500" /> : <Copy size={12} />}
            </button>
          </div>
          {summaryOpen && (
            <div className="web-card-summary-body">
              <Markdown content={summary} />
            </div>
          )}
        </div>
      )}
    </div>
  )
})
