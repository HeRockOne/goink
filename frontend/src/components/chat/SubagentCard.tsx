import { useState, useEffect, useRef, memo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Loader2, CheckCircle2, XCircle, ChevronDown, Copy, Check } from 'lucide-react'
import type { TurnSegment } from './types'
import ThinkingBlock from './ThinkingBlock'
import MessageBubble from './MessageBubble'
import ToolCallCard from './ToolCallCard'
import CompressionBlock from './CompressionBlock'
import './SubagentCard.css'

interface Props {
  agentType: 'memory' | 'review'
  segments: TurnSegment[]
  status: 'streaming' | 'done' | 'failed'
}

function getAgentMeta(t: TFunction): Record<string, { label: string; emoji: string }> {
  return {
    memory: { label: t('chat.memoryAnalyst'), emoji: '📝' },
    review: { label: t('chat.reviewEditor'), emoji: '🔍' },
  }
}

export default memo(function SubagentCard({ agentType, segments, status }: Props) {
  const { t } = useTranslation()
  const [collapsed, setCollapsed] = useState(status !== 'streaming')
  const [copiedSegId, setCopiedSegId] = useState<string | null>(null)
  const autoExpanded = useRef(false)
  const meta = getAgentMeta(t)[agentType]
  const isStreaming = status === 'streaming'
  const isDone = status === 'done'
  const isFailed = status === 'failed'

  const accentCls = agentType === 'review' ? 'subagent-review' : 'subagent-memory'

  const handleCopySeg = useCallback((segId: string, content: string) => {
    navigator.clipboard.writeText(content).then(() => {
      setCopiedSegId(segId)
      setTimeout(() => setCopiedSegId(null), 1500)
    })
  }, [])

  const prevStatusRef = useRef(status)

  useEffect(() => {
    const prev = prevStatusRef.current
    prevStatusRef.current = status

    if (isStreaming && !autoExpanded.current) {
      setCollapsed(false)
      autoExpanded.current = true
    }
    if (prev !== 'streaming') {
      autoExpanded.current = false
    }
    if (prev === 'streaming' && isDone) {
      const timer = setTimeout(() => setCollapsed(true), 1000)
      return () => clearTimeout(timer)
    }
  }, [status, isStreaming, isDone])

  return (
    <div className="flex justify-start">
    <div className={`subagent-card max-w-[85%] ${accentCls} ${isStreaming ? 'subagent-streaming' : ''}`}>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="subagent-header"
      >
        <ChevronDown className={`shrink-0 transition-transform duration-200 text-muted-foreground/50 ${collapsed ? '' : 'rotate-180'}`} size={12} />
        <span className="subagent-icon">{meta.emoji}</span>
        <span className="subagent-label">{meta.label}</span>

        <span className="flex-1" />

        {isStreaming && (
          <span className="subagent-badge subagent-badge-running">
            <Loader2 size={10} className="animate-spin" /> {t('chat.executing')}
          </span>
        )}
        {isDone && (
          <span className="subagent-badge subagent-badge-done">
            <CheckCircle2 size={10} /> {t('chat.done')}
          </span>
        )}
        {isFailed && (
          <span className="subagent-badge subagent-badge-failed">
            <XCircle size={10} /> {t('chat.failed')}
          </span>
        )}
      </button>

      <div
        className={`grid transition-all duration-300 ease-out ${
          collapsed ? 'grid-rows-[0fr] opacity-0' : 'grid-rows-[1fr] opacity-100'
        }`}
      >
        <div className="overflow-hidden border-t border-border/30">
          <div className="px-3 pb-3 space-y-2 pt-2">
            {segments.length === 0 && isStreaming && (
              <div className="flex items-center gap-2 text-xs text-muted-foreground py-2">
                <Loader2 size={12} className="animate-spin" /> {t('chat.analyzing')}
              </div>
            )}
            {segments.length === 0 && !isStreaming && (
              <div className="text-xs text-muted-foreground py-2">{t('chat.noContent')}</div>
            )}

            {segments.map(seg => {
              if (seg.type === 'compression') {
                return <CompressionBlock key={seg.id} phase={seg.compressionPhase || 'done'} />
              }
              if (seg.type === 'text') {
                return (
                  <div key={seg.id} className="space-y-1 relative group/subseg">
                    {seg.thinkingContent && (
                      <ThinkingBlock content={seg.thinkingContent} isStreaming={isStreaming && !seg.thinkingDone} />
                    )}
                    {seg.content && (
                      <div className="text-xs">
                        <MessageBubble role="assistant" content={seg.content} />
                      </div>
                    )}
                    {seg.content && (
                      <button
                        onClick={() => handleCopySeg(seg.id, seg.content)}
                        className="absolute top-0 right-0 p-1 rounded opacity-0 group-hover/subseg:opacity-100 hover:bg-muted/50 text-muted-foreground hover:text-foreground transition-all cursor-pointer"
                        title={copiedSegId === seg.id ? '已复制' : '复制'}
                      >
                        {copiedSegId === seg.id ? <Check size={10} className="text-green-500" /> : <Copy size={10} />}
                      </button>
                    )}
                  </div>
                )
              }
              if (seg.type === 'tool') {
                return (
                  <ToolCallCard
                    key={seg.id}
                    toolName={seg.toolName}
                    displayText={seg.displayText}
                    status={seg.toolStatus}
                    activityKind={seg.activityKind}
                    error={seg.error}
                    compact
                  />
                )
              }
              return null
            })}
          </div>
        </div>
      </div>
    </div>
    </div>
  )
})
