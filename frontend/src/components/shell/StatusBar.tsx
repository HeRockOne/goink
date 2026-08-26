import { useMemo, useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import type { PhaseStatus } from '@/components/chat/types'
import ContextRing, { type UsageInfo } from '@/components/chat/ContextRing'

interface Props {
  content: string
  isDirty?: boolean
  gateStatus?: PhaseStatus | null
  usage?: UsageInfo | null
  tps?: number | null
  selectedModel?: string
  onCompress?: () => void
  phaseMode: string
  onPhaseModeChange: (mode: string) => void
}

// v2 左下角门禁阶段序列
const GATE_STEPS = ['init', 'prepare', 'outline', 'write', 'review', 'maintain', 'done']
const GATE_LABELS: Record<string, string> = {
  init: '初始化', prepare: '准备', outline: '大纲',
  write: '正文', review: '审读', maintain: '维护', done: '完成',
}

interface DetailedStats {
  wordCount: number
  lineCount: number
  chineseChars: number
  englishWords: number
  charCountSpace: number
  charCountNoSpace: number
  paragraphCount: number
}

function computeStats(text: string): DetailedStats {
  if (!text) {
    return { wordCount: 0, lineCount: 0, chineseChars: 0, englishWords: 0, charCountSpace: 0, charCountNoSpace: 0, paragraphCount: 0 }
  }

  let chineseChars = 0
  let spaces = 0
  let paragraphCount = 0
  let inPara = false

  for (const ch of text) {
    const cp = ch.codePointAt(0)!
    if ((cp >= 0x4E00 && cp <= 0x9FFF) || (cp >= 0x3400 && cp <= 0x4DBF) || (cp >= 0x20000 && cp <= 0x2A6DF) || (cp >= 0xF900 && cp <= 0xFAFF)) {
      chineseChars++
    } else if (ch === ' ' || ch === '\t' || ch === '\n' || ch === '\r') {
      spaces++
    }

    if (ch === '\n') {
      if (inPara) { paragraphCount++; inPara = false }
    } else if (ch !== ' ' && ch !== '\t' && ch !== '\r') {
      inPara = true
    }
  }
  if (inPara) paragraphCount++

  const englishWords = (text.match(/[a-zA-Z]+(?:'[a-zA-Z]+)?/g) || []).length
  const lineCount = text ? text.split('\n').length : 0

  return {
    wordCount: chineseChars + englishWords,
    lineCount,
    chineseChars,
    englishWords,
    charCountSpace: [...text].length,
    charCountNoSpace: [...text].length - spaces,
    paragraphCount,
  }
}

export default function StatusBar({ content, isDirty, gateStatus, usage, tps, selectedModel, onCompress, phaseMode, onPhaseModeChange }: Props) {
  const { t } = useTranslation()
  const stats = useMemo(() => computeStats(content), [content])
  const [showDetail, setShowDetail] = useState(false)
  const hoverTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // 门禁模式循环：single → batch3 → batch6 → batch9 → batch12 → single
  const MODE_CYCLE = ['single', 'batch3', 'batch6', 'batch9', 'batch12']
  const MODE_LABELS: Record<string, string> = {
    single: '单章', batch3: '批量3', batch6: '批量6', batch9: '批量9', batch12: '批量12',
  }
  const [modeToast, setModeToast] = useState('')
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  function cycleMode() {
    const idx = MODE_CYCLE.indexOf(phaseMode)
    const next = MODE_CYCLE[(idx + 1) % MODE_CYCLE.length]
    onPhaseModeChange(next)
    setModeToast(MODE_LABELS[next] || next)
    if (toastTimer.current) clearTimeout(toastTimer.current)
    toastTimer.current = setTimeout(() => setModeToast(''), 2000)
  }
  useEffect(() => () => { if (toastTimer.current) clearTimeout(toastTimer.current) }, [])

  function handleMouseEnter() {
    hoverTimer.current = setTimeout(() => setShowDetail(true), 150)
  }

  function handleMouseLeave() {
    if (hoverTimer.current) clearTimeout(hoverTimer.current)
    setShowDetail(false)
  }

  const currentIdx = gateStatus?.phase ? GATE_STEPS.indexOf(gateStatus.phase) : -1

  return (
    <div className="relative h-7 flex items-center justify-between px-4 border-t bg-background text-xs text-muted-foreground select-none z-20">
      {/* 左区：字数 / 行数 */}
      <div className="flex items-center gap-4 min-w-0 shrink-0">
        <span
          className="cursor-default"
          onMouseEnter={handleMouseEnter}
          onMouseLeave={handleMouseLeave}
        >
          {t('shell.wordCount')} {stats.wordCount}
        </span>
        <span>{t('shell.lineCount')} {stats.lineCount}</span>
      </div>

      {/* 中区：门禁模式切换 + 阶段条 */}
      <div className="flex-1 flex items-center justify-center min-w-0 px-3 gap-2">
        <button
          onClick={cycleMode}
          className={`gate-mode-badge shrink-0 cursor-pointer transition-colors hover:opacity-80 ${phaseMode === 'single' ? 'single' : 'batch'}`}
          title="点击切换门禁模式"
        >
          {MODE_LABELS[phaseMode] || '单章'}
        </button>
        <span className="gate-steps whitespace-nowrap overflow-hidden">
          {GATE_STEPS.map((p, i) => (
            <span key={p} className="flex items-center">
              {i > 0 && <span className="gate-step-sep">·</span>}
              <span className={`gate-step ${i < currentIdx ? 'past' : i === currentIdx ? 'current' : ''}`}>
                {GATE_LABELS[p] || p}
              </span>
            </span>
          ))}
        </span>
      </div>

      {showDetail && (
        <div
          className="absolute bottom-0 left-0 mb-7 ml-4 bg-popover border rounded-lg shadow-lg p-4 text-sm space-y-1.5 z-50 min-w-[220px]"
          onMouseEnter={() => { if (hoverTimer.current) clearTimeout(hoverTimer.current); setShowDetail(true) }}
          onMouseLeave={handleMouseLeave}
        >
          <div className="font-medium text-foreground mb-2">{t('shell.wordStats')}</div>
          <div className="flex justify-between gap-8">
            <span>{t('shell.wordCount')}</span>
            <span className="tabular-nums">{stats.wordCount}</span>
          </div>
          <div className="flex justify-between gap-8">
            <span className="pl-3">{t('shell.chineseChars')}</span>
            <span className="tabular-nums">{stats.chineseChars}</span>
          </div>
          <div className="flex justify-between gap-8">
            <span className="pl-3">{t('shell.englishWords')}</span>
            <span className="tabular-nums">{stats.englishWords}</span>
          </div>
          <div className="border-t my-1.5" />
          <div className="flex justify-between gap-8">
            <span>{t('shell.charsNoSpace')}</span>
            <span className="tabular-nums">{stats.charCountNoSpace}</span>
          </div>
          <div className="flex justify-between gap-8">
            <span>{t('shell.charsWithSpace')}</span>
            <span className="tabular-nums">{stats.charCountSpace}</span>
          </div>
          <div className="border-t my-1.5" />
          <div className="flex justify-between gap-8">
            <span>{t('shell.lineCount')}</span>
            <span className="tabular-nums">{stats.lineCount}</span>
          </div>
          <div className="flex justify-between gap-8">
            <span>{t('shell.paragraphCount')}</span>
            <span className="tabular-nums">{stats.paragraphCount}</span>
          </div>
        </div>
      )}

      <span className="flex items-center gap-2 shrink-0">
        {tps != null && (
          <span className="text-xs font-semibold tabular-nums text-muted-foreground" title="实时输出速率（估算，含思考；回合结束定格为平均值）">
            {tps.toFixed(1)} t/s
          </span>
        )}
        <span className={`w-1.5 h-1.5 rounded-full ${isDirty ? 'bg-status-warning' : 'bg-status-ok'}`} />
        {/* 最右：token 用量条状统计（ContextRing bar 模式） */}
        <ContextRing usage={usage ?? null} selectedModel={selectedModel} onCompress={onCompress} bar />
      </span>

      {modeToast && (
        <div className="absolute bottom-full mb-2 left-1/2 -translate-x-1/2 z-50 animate-in fade-in slide-in-from-bottom-2 duration-200">
          <div className="px-3 py-1.5 rounded-lg bg-primary/10 border border-primary/30 text-primary text-xs font-medium shadow-lg whitespace-nowrap">
            门禁模式：{modeToast}
          </div>
        </div>
      )}
    </div>
  )
}
