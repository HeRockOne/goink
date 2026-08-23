import { useState, useEffect } from 'react'
import { X, RefreshCw } from 'lucide-react'
import { GetReviewRecords } from '@/lib/wailsjs/go/app/App'

interface ReviewRecord {
  id: number
  chapter_start: number
  chapter_end: number
  total_score: number
  verdict: string
  fatal_count: number
  dim_structure: number
  dim_character: number
  dim_pacing: number
  dim_prose: number
  dim_scene: number
  instruction: string
  report: string
  created_at: string
}

const VERDICT_LABEL: Record<string, string> = { pass: '通过', revise: '需修改', fail: '不通过', unknown: '未解析' }
const DIMS: Array<{ key: keyof ReviewRecord; label: string }> = [
  { key: 'dim_structure', label: '结构' },
  { key: 'dim_character', label: '角色' },
  { key: 'dim_pacing', label: '节奏' },
  { key: 'dim_prose', label: '散文' },
  { key: 'dim_scene', label: '场景' },
]

function fmtTime(s: string) {
  try { return new Date(s).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) } catch { return s }
}
const fmtScore = (v: number) => v < 0 ? '—' : v.toFixed(1)

export default function ReviewHistoryPanel({ novelId, width, onClose }: { novelId: number; width: number; onClose: () => void }) {
  const [records, setRecords] = useState<ReviewRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState<number | null>(null)

  const load = () => {
    setLoading(true)
    GetReviewRecords(novelId, 100)
      .then(r => setRecords((r || []) as ReviewRecord[]))
      .catch(() => setRecords([]))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [novelId])

  return (
    <div className="review-history-panel" style={{ width, minWidth: 280 }}>
      <div className="review-history-header">
        <span className="review-history-title">审稿记录</span>
        <span className="text-[10px] text-muted-foreground/70">{records.length} 条</span>
        <button onClick={load} className="review-history-btn" title="刷新"><RefreshCw className="w-3.5 h-3.5" /></button>
        <button onClick={onClose} className="review-history-btn" title="关闭"><X className="w-3.5 h-3.5" /></button>
      </div>
      <div className="review-history-body">
        {loading && <div className="p-4 text-xs text-muted-foreground text-center">加载中…</div>}
        {!loading && records.length === 0 && <div className="p-4 text-xs text-muted-foreground text-center">暂无审稿记录<br /><span className="text-[10px] opacity-70">每次审稿子代理完成后自动落库</span></div>}
        {records.map(rec => {
          const open = expanded === rec.id
          return (
            <div key={rec.id} className={`review-record-card verdict-${rec.verdict}`} onClick={() => setExpanded(open ? null : rec.id)}>
              <div className="review-record-top">
                <span className="review-record-chapters">
                  {rec.chapter_start > 0 ? (rec.chapter_start === rec.chapter_end ? `第${rec.chapter_start}章` : `第${rec.chapter_start}-${rec.chapter_end}章`) : '章节未知'}
                </span>
                <span className={`review-verdict-chip v-${rec.verdict}`}>{VERDICT_LABEL[rec.verdict] ?? rec.verdict}</span>
                <span className="review-record-time">{fmtTime(rec.created_at)}</span>
              </div>
              <div className="review-record-score-row">
                <span className="review-total-score">{fmtScore(rec.total_score)}<small>/10</small></span>
                {rec.fatal_count > 0 && <span className="review-fatal">致命 {rec.fatal_count} 项</span>}
                <span className="review-dims">
                  {DIMS.map(d => (
                    <span key={d.key} className="review-dim">{d.label} {fmtScore(rec[d.key] as number)}</span>
                  ))}
                </span>
              </div>
              {open && (
                <div className="review-report-detail">
                  {rec.instruction && <div className="review-instruction">指令：{rec.instruction}</div>}
                  <pre>{rec.report}</pre>
                </div>
              )}
              {!open && rec.instruction && <div className="review-record-hint">{rec.instruction}</div>}
            </div>
          )
        })}
      </div>
    </div>
  )
}
