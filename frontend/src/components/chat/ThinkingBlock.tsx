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

  // 思考完成后自动收回：流式结束（isStreaming true → false）时折叠，避免思考过程长期占据界面
  const wasStreaming = useRef(isStreaming)
  useEffect(() => {
    if (wasStreaming.current && !isStreaming) {
      setExpanded(false)
    }
    wasStreaming.current = isStreaming
  }, [isStreaming])

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
