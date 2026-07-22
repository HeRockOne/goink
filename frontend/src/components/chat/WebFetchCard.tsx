import { memo, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Globe, ExternalLink, ChevronDown, ChevronRight, Copy, Check } from 'lucide-react'
import Markdown from '@/components/Markdown'
import './WebFetchCard.css'

interface Props {
  result: Record<string, unknown>
  displayText: string
}

function openExternal(url: string, t: TFunction) {
  if (window.confirm(`${t('chat.openInBrowser')}\n${url}`)) {
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}

export default memo(function WebFetchCard({ result, displayText }: Props) {
  const { t } = useTranslation()
  const [contentOpen, setContentOpen] = useState(false)
  const [copied, setCopied] = useState(false)

  const url = (result.url as string) || ''
  const title = (result.title as string) || ''
  const text = (result.text as string) || ''
  const wordCount = text.replace(/\s/g, '').length

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }, [text])

  return (
    <div className="fetch-card completed">
      <div className="fetch-card-row">
        <span className="fetch-card-icon"><Globe size={14} /></span>
        <span className="fetch-card-label">{displayText}</span>
        <span className="fetch-card-badge fetch-card-badge-done">{t('chat.done')}</span>
      </div>

      <div className="fetch-card-meta">
        <div className="fetch-card-title-line">
          <span className="fetch-card-title">{title || url}</span>
          <button
            className="fetch-card-ext-btn"
            onClick={() => openExternal(url, t)}
            title={url}
          >
            <ExternalLink size={12} />
          </button>
        </div>
        {url && (
          <span className="fetch-card-url">{url}</span>
        )}
      </div>

      {text && (
        <div className="fetch-card-content">
          <div className="flex items-center">
            <button
              className="fetch-card-content-toggle flex-1"
              onClick={() => setContentOpen(!contentOpen)}
            >
              {contentOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
              {t('chat.pageContent', { count: wordCount })}
            </button>
            <button
              onClick={handleCopy}
              className="p-1 rounded hover:bg-muted/50 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              title={copied ? '已复制' : '复制内容'}
            >
              {copied ? <Check size={12} className="text-green-500" /> : <Copy size={12} />}
            </button>
          </div>
          {contentOpen && (
            <div className="fetch-card-content-body">
              <Markdown content={text} />
            </div>
          )}
        </div>
      )}
    </div>
  )
})
