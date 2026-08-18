import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Save, Plus, Trash2, RefreshCw, Loader2, ChevronDown, ChevronUp } from 'lucide-react'
import { toast } from 'sonner'
import { useApp } from '@/hooks/useApp'
import { Button } from '@/components/ui/button'

interface Props {
  novelId: number
}

interface OutlineData {
  core_conflict: string
  growth_arc: string
  ending_direction: string
  word_count_plan: number
}

interface BeatData {
  id: number
  chapter: number
  description: string
  beat_type: string
  importance: number
  _status?: 'new' | 'modified' | 'deleted'
}

const BEAT_TYPES = [
  { value: 'shuangdian', zh: '大爽点', en: 'Shuangdian' },
  { value: 'turning', zh: '转折点', en: 'Turning Point' },
  { value: 'climax', zh: '高潮', en: 'Climax' },
]

export default function OutlineEditor({ novelId }: Props) {
  const { i18n } = useTranslation()
  const app = useApp()
  const isZh = i18n.language.startsWith('zh')

  const [outline, setOutline] = useState<OutlineData>({
    core_conflict: '',
    growth_arc: '',
    ending_direction: '',
    word_count_plan: 0,
  })
  const [beats, setBeats] = useState<BeatData[]>([])
  const [deletedBeatIds, setDeletedBeatIds] = useState<number[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [o, b] = await Promise.all([
        app.GetOutline(novelId),
        app.GetOutlineBeats(novelId),
      ])
      if (o) {
        setOutline({
          core_conflict: o.core_conflict ?? '',
          growth_arc: o.growth_arc ?? '',
          ending_direction: o.ending_direction ?? '',
          word_count_plan: o.word_count_plan ?? 0,
        })
      }
      setBeats((b ?? []).map((beat: any) => ({ ...beat, _status: undefined })))
      setDeletedBeatIds([])
    } catch (e) {
      toast.error(String(e))
    } finally {
      setLoading(false)
    }
  }, [novelId, app])

  useEffect(() => { loadData() }, [loadData])

  const handleSave = async () => {
    setSaving(true)
    try {
      // Save outline fields
      await app.SaveOutline(novelId, outline)

      // Process beats: create new, update modified, delete removed
      for (const beat of beats) {
        if (beat._status === 'new') {
          await app.CreateOutlineBeat(novelId, {
            chapter: beat.chapter,
            description: beat.description,
            beat_type: beat.beat_type,
            importance: beat.importance,
          })
        } else if (beat._status === 'modified') {
          await app.UpdateOutlineBeat(novelId, {
            id: beat.id,
            chapter: beat.chapter,
            description: beat.description,
            beat_type: beat.beat_type,
            importance: beat.importance,
          })
        }
      }
      for (const id of deletedBeatIds) {
        await app.DeleteOutlineBeat(id)
      }

      toast.success(isZh ? '总纲已保存' : 'Outline saved')
      await loadData()
    } catch (e) {
      toast.error(String(e))
    } finally {
      setSaving(false)
    }
  }

  const addBeat = () => {
    const maxChapter = beats.length > 0 ? Math.max(...beats.map(b => b.chapter)) : 0
    setBeats(prev => [...prev, {
      id: -Date.now(), // temporary negative id for new beats
      chapter: maxChapter + 10,
      description: '',
      beat_type: 'shuangdian',
      importance: 3,
      _status: 'new',
    }])
  }

  const updateBeat = (idx: number, field: string, value: any) => {
    setBeats(prev => prev.map((b, i) => {
      if (i !== idx) return b
      const updated = { ...b, [field]: value }
      if (updated._status !== 'new') updated._status = 'modified'
      return updated
    }))
  }

  const removeBeat = (idx: number) => {
    const beat = beats[idx]
    if (beat._status !== 'new') {
      setDeletedBeatIds(prev => [...prev, beat.id])
    }
    setBeats(prev => prev.filter((_, i) => i !== idx))
  }

  const toggleBeat = (idx: number) => {
    setExpanded(prev => ({ ...prev, [idx]: !prev[idx] }))
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const fieldClass = "w-full bg-background border border-border rounded-lg px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary/50 resize-y min-h-[60px]"
  const smallFieldClass = "w-full bg-background border border-border rounded-lg px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary/50"

  return (
    <div className="h-full overflow-auto p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between sticky top-0 bg-editor-surface/80 backdrop-blur-sm z-10 pb-2 border-b border-border/30">
        <h3 className="text-sm font-medium text-foreground">
          {isZh ? '全书总纲' : 'Book Outline'}
        </h3>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon-xs" onClick={loadData} disabled={saving}>
            <RefreshCw className="w-3.5 h-3.5" />
          </Button>
          <Button variant="default" size="sm" onClick={handleSave} disabled={saving}>
            {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin mr-1" /> : <Save className="w-3.5 h-3.5 mr-1" />}
            {isZh ? '保存' : 'Save'}
          </Button>
        </div>
      </div>

      {/* Outline fields */}
      <div className="space-y-4">
        <div>
          <label className="block text-xs text-muted-foreground mb-1.5">
            {isZh ? '核心矛盾' : 'Core Conflict'}
          </label>
          <textarea
            value={outline.core_conflict}
            onChange={e => setOutline(prev => ({ ...prev, core_conflict: e.target.value }))}
            placeholder={isZh ? '主角与最终对手的根本冲突（一句话说清）' : 'Core conflict between protagonist and antagonist (one sentence)'}
            className={fieldClass}
            rows={2}
          />
        </div>

        <div>
          <label className="block text-xs text-muted-foreground mb-1.5">
            {isZh ? '主角成长弧线' : 'Growth Arc'}
          </label>
          <textarea
            value={outline.growth_arc}
            onChange={e => setOutline(prev => ({ ...prev, growth_arc: e.target.value }))}
            placeholder={isZh ? '从哪里出发，最终变成什么样的人' : 'Where the protagonist starts, who they become'}
            className={fieldClass}
            rows={3}
          />
        </div>

        <div>
          <label className="block text-xs text-muted-foreground mb-1.5">
            {isZh ? '结局方向' : 'Ending Direction'}
          </label>
          <textarea
            value={outline.ending_direction}
            onChange={e => setOutline(prev => ({ ...prev, ending_direction: e.target.value }))}
            placeholder={isZh ? '大结局基本形态' : 'Basic form of the grand finale'}
            className={fieldClass}
            rows={2}
          />
        </div>

        <div>
          <label className="block text-xs text-muted-foreground mb-1.5">
            {isZh ? '篇幅规划（万字）' : 'Word Count Plan (10k chars)'}
          </label>
          <input
            type="number"
            value={outline.word_count_plan || ''}
            onChange={e => setOutline(prev => ({ ...prev, word_count_plan: parseInt(e.target.value) || 0 }))}
            placeholder={isZh ? '目标字数' : 'Target word count'}
            className={smallFieldClass}
            min={0}
          />
        </div>
      </div>

      {/* Beats section */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
            {isZh ? '大爽点' : 'Key Beats'}
            <span className="ml-2 text-muted-foreground/50 normal-case">({beats.length})</span>
          </h4>
          <Button variant="ghost" size="xs" onClick={addBeat}>
            <Plus className="w-3 h-3 mr-1" />
            {isZh ? '添加' : 'Add'}
          </Button>
        </div>

        {beats.length === 0 && (
          <p className="text-xs text-muted-foreground/50 py-4 text-center">
            {isZh ? '暂无大爽点，点击上方按钮添加' : 'No beats yet, click Add to create one'}
          </p>
        )}

        {beats.map((beat, idx) => (
          <div key={beat.id} className="border border-border/50 rounded-lg overflow-hidden">
            {/* Beat header - always visible */}
            <div
              className="flex items-center gap-2 px-3 py-2 hover:bg-muted/30 cursor-pointer select-none"
              onClick={() => toggleBeat(idx)}
            >
              {expanded[idx] ? <ChevronUp className="w-3.5 h-3.5 text-muted-foreground shrink-0" /> : <ChevronDown className="w-3.5 h-3.5 text-muted-foreground shrink-0" />}
              <span className="text-xs font-mono text-muted-foreground tabular-nums shrink-0">
                {isZh ? `第${beat.chapter}章` : `Ch.${beat.chapter}`}
              </span>
              <span className="text-xs text-foreground truncate flex-1">
                {beat.description || (isZh ? '(未填写)' : '(empty)')}
              </span>
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground shrink-0">
                {BEAT_TYPES.find(bt => bt.value === beat.beat_type)?.[isZh ? 'zh' : 'en'] ?? beat.beat_type}
              </span>
              <button
                onClick={(e) => { e.stopPropagation(); removeBeat(idx) }}
                className="text-muted-foreground/50 hover:text-destructive shrink-0 p-0.5"
              >
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* Beat detail - expandable */}
            {expanded[idx] && (
              <div className="px-3 pb-3 space-y-2 border-t border-border/30">
                <div className="grid grid-cols-4 gap-2 pt-2">
                  <div>
                    <label className="block text-[10px] text-muted-foreground mb-0.5">
                      {isZh ? '章节号' : 'Chapter'}
                    </label>
                    <input
                      type="number"
                      value={beat.chapter}
                      onChange={e => updateBeat(idx, 'chapter', parseInt(e.target.value) || 0)}
                      className={smallFieldClass}
                      min={1}
                    />
                  </div>
                  <div>
                    <label className="block text-[10px] text-muted-foreground mb-0.5">
                      {isZh ? '类型' : 'Type'}
                    </label>
                    <select
                      value={beat.beat_type}
                      onChange={e => updateBeat(idx, 'beat_type', e.target.value)}
                      className={smallFieldClass}
                    >
                      {BEAT_TYPES.map(bt => (
                        <option key={bt.value} value={bt.value}>{isZh ? bt.zh : bt.en}</option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label className="block text-[10px] text-muted-foreground mb-0.5">
                      {isZh ? '重要度' : 'Importance'}
                    </label>
                    <select
                      value={beat.importance}
                      onChange={e => updateBeat(idx, 'importance', parseInt(e.target.value))}
                      className={smallFieldClass}
                    >
                      {[1, 2, 3, 4, 5].map(v => (
                        <option key={v} value={v}>{v}</option>
                      ))}
                    </select>
                  </div>
                </div>
                <div>
                  <label className="block text-[10px] text-muted-foreground mb-0.5">
                    {isZh ? '描述' : 'Description'}
                  </label>
                  <textarea
                    value={beat.description}
                    onChange={e => updateBeat(idx, 'description', e.target.value)}
                    placeholder={isZh ? '大爽点描述' : 'Beat description'}
                    className={fieldClass}
                    rows={2}
                  />
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
