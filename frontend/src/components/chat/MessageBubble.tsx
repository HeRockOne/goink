import { memo, useState, useCallback, useRef } from 'react'
import { Copy, Check, RotateCcw, Pencil } from 'lucide-react'
import Markdown from '@/components/Markdown'

interface UsageInfo {
  prompt_cache_hit_tokens?: number
  prompt_cache_miss_tokens?: number
  acc_completion_tokens?: number
}

interface Props {
  role: 'user' | 'assistant'
  content: string
  timestamp?: string
  model?: string
  reasoningEffort?: string
  durationMs?: number
  usage?: Record<string, unknown>
  onRetry?: () => void
  onEdit?: () => void
}

export default memo(function MessageBubble({ role, content, timestamp, model, reasoningEffort, durationMs, usage, onRetry, onEdit }: Props) {
  const isUser = role === 'user'
  const [copied, setCopied] = useState(false)
  const [showActions, setShowActions] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const hasMeta = !isUser && Boolean(model || durationMs || usage)
  const u = usage as unknown as UsageInfo | undefined

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
        className="relative overflow-visible max-w-[85%]"
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
      >
        {/* 气泡 */}
        <div
  className={`rounded-xl px-3.5 py-3 break-words ${
    role === 'user'
      ? 'bubble-ai rounded-br-sm'
      : 'bubble-ai rounded-bl-sm shadow-xs'
  }`}
        >
          <Markdown content={content} className={isUser ? 'markdown-user' : undefined} />
          {(timestamp || hasMeta) && (
            <div className={`text-[10px] text-muted-foreground/50 mt-1 flex items-center gap-2 flex-wrap ${isUser ? 'justify-end' : 'justify-start'}`}>
              {timestamp && <span>{formatTime(timestamp)}</span>}
              {hasMeta && (
                <>
                  {model && <span title={model}>{model}</span>}
                  {reasoningEffort && <span>思考:{reasoningEffort}</span>}
                  {!!durationMs && durationMs > 0 && <span>{(durationMs / 1000).toFixed(1)}s</span>}
                  {u && (u.prompt_cache_hit_tokens || u.prompt_cache_miss_tokens || u.acc_completion_tokens) ? (
                    <span title="输入 tokens ↑ / 输出 tokens ↓">
                      ↑{fmtTokens((u.prompt_cache_hit_tokens || 0) + (u.prompt_cache_miss_tokens || 0))}
                      {' '}↓{fmtTokens(u.acc_completion_tokens || 0)}
                    </span>
                  ) : null}
                </>
              )}
            </div>
          )}
        </div>

        {/* 操作按钮 - 气泡边框外居下 */}
        <div
          className={`absolute bottom-0 flex items-center gap-0.5 bg-popover border border-border/30 rounded-lg px-1 py-0.5 shadow-sm transition-all duration-150 z-10 ${
            isUser ? '-left-8' : '-right-8'
          } ${
            showActions ? 'opacity-100' : 'opacity-0 pointer-events-none'
          }`}
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
      </div>
    </div>
  )
})

function fmtTokens(n: number): string {
  if (!n) return '0'
  if (n >= 10000) return (n / 10000).toFixed(1).replace(/\.0$/, '') + 'w'
  if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k'
  return String(n)
}

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
