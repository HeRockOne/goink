import { Shield, CheckCircle, Circle, AlertTriangle } from 'lucide-react'
import type { PhaseStatus } from './types'

interface Props {
  status: PhaseStatus | null
  error?: string
}

const PHASE_LABELS: Record<string, string> = {
  init: '初始化',
  prepare: '准备',
  outline: '大纲',
  write: '正文',
  review: '审读',
  maintain: '维护',
}

const PHASE_ORDER = ['init', 'prepare', 'outline', 'write', 'review', 'maintain']

export default function PhaseGateBar({ status, error }: Props) {
  if (!status || !status.phase) return null

  const currentIndex = PHASE_ORDER.indexOf(status.phase)

  return (
    <div className="flex items-center gap-2 px-3 py-1.5 text-xs border-b bg-muted/30">
      <Shield className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
      <span className="text-muted-foreground font-medium">阶段门禁</span>

      {/* 阶段进度条 */}
      <div className="flex items-center gap-1 ml-2">
        {PHASE_ORDER.map((phase, i) => {
          const isCurrent = phase === status.phase
          const isPast = i < currentIndex
          const label = PHASE_LABELS[phase] || phase

          return (
            <div key={phase} className="flex items-center gap-1">
              {i > 0 && (
                <div className={`w-3 h-px ${isPast || isCurrent ? 'bg-primary' : 'bg-muted-foreground/30'}`} />
              )}
              <div
                className={`flex items-center gap-1 px-1.5 py-0.5 rounded ${
                  isCurrent
                    ? 'bg-primary/10 text-primary font-medium'
                    : isPast
                    ? 'text-muted-foreground'
                    : 'text-muted-foreground/50'
                }`}
              >
                {isPast ? (
                  <CheckCircle className="w-3 h-3" />
                ) : isCurrent ? (
                  <AlertTriangle className="w-3 h-3" />
                ) : (
                  <Circle className="w-3 h-3" />
                )}
                <span>{label}</span>
              </div>
            </div>
          )
        })}
      </div>

      {/* require 状态 */}
      {status.ready && status.next && (
        <span className="ml-auto text-green-600 dark:text-green-400">
          ✓ 可切换到 {PHASE_LABELS[status.next] || status.next}
        </span>
      )}

      {/* 错误信息 */}
      {error && (
        <span className="ml-auto text-destructive max-w-[300px] truncate">
          {error}
        </span>
      )}
    </div>
  )
}
