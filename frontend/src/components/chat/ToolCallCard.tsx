import { Loader2, CheckCircle2, XCircle, Eye, Plus, Pencil, Brain, FileText, Wrench, Check, AlertTriangle, Trash2, ChevronDown, ChevronRight } from 'lucide-react'
import { memo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import './ToolCallCard.css'

interface Props {
  toolName: string
  displayText: string
  status: 'executing' | 'awaiting_approval' | 'completed' | 'failed'
  activityKind?: string
  error?: string
  compact?: boolean
  // approval
  approvalType?: string
  approvalPayload?: Record<string, unknown>
  onApprove?: (feedback: string) => void
  onReject?: (feedback: string) => void
}

function activityIcon(kind?: string) {
  switch (kind) {
    case 'view': case 'browse': return Eye
    case 'create': return Plus
    case 'write': case 'edit': return Pencil
    case 'memory': return Brain
    case 'review': return CheckCircle2
    case 'delete': return Trash2
    case 'plan': return FileText
    default: return Wrench
  }
}

function activityBadge(kind: string | undefined, t: TFunction): string {
  switch (kind) {
    case 'view': case 'browse': return t('chat.viewing')
    case 'create': return t('chat.creating')
    case 'write': return t('chat.writing')
    case 'edit': return t('chat.editing')
    case 'delete': return t('chat.deleting')
    case 'memory': return t('chat.retrieving')
    case 'review': return t('chat.reviewing')
    case 'plan': return t('chat.planning')
    default: return t('chat.processing')
  }
}

function getTypeLabels(t: TFunction): Record<string, string> {
  return {
    character: t('chat.toolCharacter'), character_relation: t('chat.toolCharacterRelation'),
    location: t('chat.toolLocation'), location_relation: t('chat.toolLocationRelation'),
    timeline_entry: t('chat.toolTimelineEntry'), story_arc: t('chat.toolStoryArc'),
    arc_node: t('chat.toolArcNode'), reader_perspective_entry: t('chat.toolReaderEntry'),
    preference: t('chat.toolPreference'),
  }
}

function ApprovalBody({ type, payload }: { type?: string; payload?: Record<string, unknown> }) {
  const { t } = useTranslation()
  const typeLabels = getTypeLabels(t)
  if (type === 'delete' && payload?.deleted) {
    const d = payload.deleted as Record<string, unknown>
    const label = typeLabels[String(d.type)] ?? String(d.type ?? t('chat.record'))
    const nameOrTitle = (d.name ?? d.title) as string | undefined
    const title = nameOrTitle ?? `#${d.id}`

    if (d.type === 'character_relation') {
      return <span>{t('chat.confirmDeleteCharacterRelation', { source: String(d.source), target: String(d.target), relation: String(d.relation) })}</span>
    }
    if (d.type === 'location_relation') {
      return <span>{t('chat.confirmDeleteLocationRelation', { locationA: String(d.location_a), locationB: String(d.location_b), relation: String(d.relation) })}</span>
    }
    if (d.type === 'arc_node') {
      return <span>{t('chat.confirmDeleteArcNode', { title, storyArc: String(d.story_arc) })}</span>
    }
    if (d.type === 'reader_perspective_entry') {
      return <span>{t('chat.confirmDeleteReaderEntry', { id: String(d.id), entryType: String(d.entry_type), plantedChapter: String(d.planted_chapter) })}</span>
    }
    if (d.type === 'preference') {
      return <span>{t('chat.confirmDeletePreference', { category: String(d.category), id: String(d.id) })}</span>
    }
    if (d.type === 'timeline_entry') {
      return <span>{t('chat.confirmDeleteTimelineEntry', { title })}</span>
    }
    return <span>{t('chat.confirmDeleteGeneric', { label, title })}</span>
  }

  if (type === 'file_edit' && payload) {
    const changeTypeMap: Record<string, string> = {
      full_replace: t('chat.fullReplace'),
      search_replace: t('chat.findReplace'),
      line_range_replace: t('chat.lineRangeReplace'),
    }
    const rawType = (payload.change_type as string) || ''
    const changeType = changeTypeMap[rawType] || rawType || t('chat.modify')
    const reason = (payload.reason as string) || ''
    return (
      <div>
        <div className="approval-summary">{changeType}</div>
        {reason && <div className="approval-reason">{reason}</div>}
      </div>
    )
  }

  return <span>{t('chat.waitingApproval')}</span>
}

function ApprovalView({ displayText, compact, approvalType, approvalPayload, onApprove, onReject }: { displayText: string; compact?: boolean; approvalType?: string; approvalPayload?: Record<string, unknown>; onApprove: (feedback: string) => void; onReject: (feedback: string) => void }) {
  const [feedback, setFeedback] = useState('')
  const handleInput = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setFeedback(e.target.value)
  }
  const { t } = useTranslation()

  return (
    <div className={`tool-card awaiting-approval ${compact ? 'compact' : ''}`}>
      <div className="tool-row">
        <span className="tool-icon"><AlertTriangle size={compact ? 12 : 14} /></span>
        <span className="tool-label">{displayText}</span>
        <span className="tool-badge tool-badge-approval">
          <Loader2 size={10} className="animate-spin" /> {t('chat.waitingForApproval')}
        </span>
      </div>
      <div className="approval-body">
        <ApprovalBody type={approvalType} payload={approvalPayload} />
        <textarea
          value={feedback}
          onChange={handleInput}
          placeholder={t('chat.feedbackOptional')}
          rows={1}
          className="approval-feedback"
        />
        <div className="approval-actions">
          <button
            onClick={() => { onReject(feedback); setFeedback('') }}
            className="approval-reject-btn cursor-pointer select-none"
          >
            <XCircle size={13} /> {t('chat.reject')}
          </button>
          <button
            onClick={() => { onApprove(feedback); setFeedback('') }}
            className="approval-accept-btn cursor-pointer select-none"
          >
            <Check size={13} /> {t('chat.approve')}
          </button>
        </div>
      </div>
    </div>
  )
}

function ToolErrorDisplay({ error }: { error: string }) {
  const [expanded, setExpanded] = useState(false)
  const isLong = error.length > 120
  const displayText = expanded || !isLong ? error : error.slice(0, 120) + '...'

  return (
    <div className="tool-error">
      <span className="flex-1">{displayText}</span>
      {isLong && (
        <button
          onClick={(e) => { e.stopPropagation(); setExpanded(!expanded) }}
          className="shrink-0 ml-1 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
        >
          {expanded ? <ChevronDown size={10} /> : <ChevronRight size={10} />}
        </button>
      )}
    </div>
  )
}

export default memo(function ToolCallCard({ displayText, status, activityKind, error, compact, approvalType, approvalPayload, onApprove, onReject }: Props) {
  const { t } = useTranslation()

  // 审批中状态
  if (status === 'awaiting_approval' && onApprove && onReject) {
    return <ApprovalView displayText={displayText} compact={compact} approvalType={approvalType} approvalPayload={approvalPayload} onApprove={onApprove} onReject={onReject} />
  }

  const isExecuting = status === 'executing'
  const isCompleted = status === 'completed'
  const isFailed = status === 'failed'

  return (
    <div className={`tool-card ${isExecuting ? 'executing' : isCompleted ? 'completed' : 'failed'} ${compact ? 'compact' : ''}`}>
      <div className={`tool-row ${compact ? 'compact' : ''}`}>
        <span className="tool-icon">
          {isExecuting ? (
            <Loader2 className="animate-spin" size={compact ? 12 : 14} />
          ) : isFailed ? (
            <XCircle size={compact ? 12 : 14} />
          ) : (
            (() => { const I = activityIcon(activityKind); return <I size={compact ? 12 : 14} /> })()
          )}
        </span>

        <span className="tool-label">{displayText}</span>

        <span className={`tool-badge ${isCompleted ? 'tool-badge-done' : isFailed ? 'tool-badge-failed' : ''}`}>
          {isExecuting ? activityBadge(activityKind, t) : isCompleted ? t('chat.done') : t('chat.failed')}
        </span>
      </div>

      {isFailed && error && (
        <ToolErrorDisplay error={error} />
      )}
    </div>
  )
})
