import { memo, useState, useCallback, useRef } from 'react'
import { Copy, Check, RotateCcw, Pencil } from 'lucide-react'
import Markdown from '@/components/Markdown'

interface Props {
  role: 'user' | 'assistant'
  content: string
  timestamp?: string
  onRetry?: () => void
  onEdit?: () => void
}

export default memo(function MessageBubble({ role, content, timestamp, onRetry, onEdit }: Props) {
  const isUser = role === 'user'
  const [copied, setCopied] = useState(false)
  const [showActions, setShowActions] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(content).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }, [content])

  const handleMouseEnter = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current)
    setShowActions(true)
  }, [])

  const handleMouseLeave = useCallback(() => {
    timerRef.current = setTimeout(() => setShowActions(false), 300)
  }, [])

  return (
    <div className={`group/msg flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`relative max-w-[85%] rounded-xl px-3.5 py-3 break-words ${
          isUser
            ? 'bg-bubble-user text-bubble-user-foreground rounded-br-sm'
            : 'bg-card border border-border/30 text-foreground rounded-bl-sm shadow-xs'
        }`}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
      >
        <Markdown content={content} className={isUser ? 'markdown-user' : undefined} />

        {/* 操作按钮 - 右下角 */}
        <div
          className={`absolute -bottom-8 right-0 flex items-center gap-0.5 bg-popover border border-border/30 rounded-lg px-1 py-0.5 shadow-sm transition-all duration-150 ${
            showActions ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-1 pointer-events-none'
          }`}
          onMouseEnter={handleMouseEnter}
          onMouseLeave={handleMouseLeave}
        >
          <button
            onClick={handleCopy}
            className={`p-1 rounded transition-colors cursor-pointer ${
              copied
                ? 'bg-green-100 dark:bg-green-900/30 text-green-600 dark:text-green-400'
                : 'bg-primary/10 text-primary hover:bg-primary/20'
            }`}
            title={copied ? '已复制' : '复制'}
          >
            {copied ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
          </button>
          {isUser && onEdit && (
            <button
              onClick={onEdit}
              className="p-1 rounded bg-muted/50 hover:bg-muted text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              title="编辑"
            >
              <Pencil className="w-3.5 h-3.5" />
            </button>
          )}
          {!isUser && onRetry && (
            <button
              onClick={onRetry}
              className="p-1 rounded bg-muted/50 hover:bg-muted text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              title="重试"
            >
              <RotateCcw className="w-3.5 h-3.5" />
            </button>
          )}
        </div>

        {/* 时间戳 */}
        {timestamp && (
          <div className={`text-[10px] text-muted-foreground/50 mt-1 ${isUser ? 'text-right' : 'text-left'}`}>
            {formatTime(timestamp)}
          </div>
        )}
      </div>
    </div>
  )
})

function formatTime(ts: string): string {
  try {
    const d = new Date(ts)
    if (isNaN(d.getTime())) return ''
    const now = new Date()
    const isToday = d.toDateString() === now.toDateString()
    const time = d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    return isToday ? time : d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' }) + ' ' + time
  } catch {
    return ''
  }
}
