import { useState, useRef, useCallback, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import './ThinkingBlock.css'

interface Props {
  content: string
  isStreaming: boolean
}

export default function ThinkingBlock({ content, isStreaming }: Props) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)
  const [contentHeight, setContentHeight] = useState(0)

  useEffect(() => {
    if (contentRef.current) {
      setContentHeight(contentRef.current.scrollHeight)
    }
  }, [content])

  const toggle = useCallback(() => setExpanded(prev => !prev), [])

  if (!content) return null

  return (
    <div className="thinking-block-animated">
      <button
        className="thinking-toggle"
        onClick={toggle}
      >
        <ChevronRight
          className={`thinking-chevron-icon transition-transform duration-200 ${expanded ? 'rotate-90' : ''}`}
          size={12}
        />
        {isStreaming ? (
          <span className="thinking-shimmer">{t('chat.thinking')}</span>
        ) : (
          <span>{t('chat.thinkingProcess')}</span>
        )}
      </button>
      <div
        className="thinking-expand-wrapper"
        style={{
          maxHeight: expanded ? `${Math.min(contentHeight, 400)}px` : '0px',
          opacity: expanded ? 1 : 0,
        }}
      >
        <div ref={contentRef} className="thinking-content-animated">
          {content}
        </div>
      </div>
    </div>
  )
}
