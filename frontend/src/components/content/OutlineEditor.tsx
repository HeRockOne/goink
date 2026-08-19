import { useState, useEffect, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Save, Trash2, RefreshCw, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { useApp } from '@/hooks/useApp'
import { Button } from '@/components/ui/button'
import { parseGrowthArc, serializeGrowthArc, parseKV, serializeKV } from './outlineParse'
import type { GrowthStage, KVItem } from './outlineParse'

interface Props {
  novelId: number
}

interface OutlineData {
  core_conflict: string
  growth_arc: string
  ending_direction: string
  theme: string
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

interface VolumeData {
  id: number
  name: string
  description: string
  start_chapter: number
  end_chapter: number
  detail_json: string
  sort_order: number
  _status?: 'new' | 'modified' | 'deleted'
}

type TabType = 'outline' | 'volumes'

export default function OutlineEditor({ novelId }: Props) {
  const { i18n } = useTranslation()
  const app = useApp()
  const isZh = i18n.language.startsWith('zh')

  const [outline, setOutline] = useState<OutlineData>({
    core_conflict: '',
    growth_arc: '',
    ending_direction: '',
    theme: '',
    word_count_plan: 0,
  })
  const [beats, setBeats] = useState<BeatData[]>([])
  const [deletedBeatIds, setDeletedBeatIds] = useState<number[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const [expandedStages, setExpandedStages] = useState<Record<number, boolean>>({})
  const [genre, setGenre] = useState('')
  const [activeTab, setActiveTab] = useState<TabType>('outline')
  const [volumeList, setVolumeList] = useState<VolumeData[]>([])
  const [expandedVolumes, setExpandedVolumes] = useState<Record<number, boolean>>({})
  const [deletedVolumeIds, setDeletedVolumeIds] = useState<number[]>([])

  // 解析各字段为结构化数据
  const growthStages = useMemo(() => parseGrowthArc(outline.growth_arc), [outline.growth_arc])
  const coreKV = useMemo(() => parseKV(outline.core_conflict), [outline.core_conflict])
  const endingKV = useMemo(() => parseKV(outline.ending_direction), [outline.ending_direction])
  const themeKV = useMemo(() => parseKV(outline.theme), [outline.theme])
  const hasStructuredGrowth = growthStages.length > 0
  const hasStructuredCore = coreKV.length > 0
  const hasStructuredEnding = endingKV.length > 0
  const hasStructuredTheme = themeKV.length > 0

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [o, b, n, vols] = await Promise.all([
        app.GetOutline(novelId),
        app.GetOutlineBeats(novelId),
        app.GetNovel(novelId),
        app.GetVolumes(novelId),
      ])
      if (o) {
        setOutline({
          core_conflict: o.core_conflict ?? '',
          growth_arc: o.growth_arc ?? '',
          ending_direction: o.ending_direction ?? '',
          theme: o.theme ?? '',
          word_count_plan: o.word_count_plan ?? 0,
        })
      }
      if (n) setGenre(n.genre ?? '')
      setVolumeList((vols ?? []).map((v: any) => ({
        id: v.id ?? 0,
        name: v.name ?? '',
        description: v.description ?? '',
        start_chapter: v.start_chapter ?? 0,
        end_chapter: v.end_chapter ?? 0,
        detail_json: v.detail_json ?? '',
        sort_order: v.sort_order ?? 0,
      })))
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

      // Save volumes
      for (const vol of volumeList) {
        if (vol._status === 'new' || vol._status === 'modified') {
          await app.SaveVolume(novelId, {
            id: vol.id > 0 ? vol.id : 0,
            name: vol.name,
            description: vol.description,
            start_chapter: vol.start_chapter,
            end_chapter: vol.end_chapter,
            detail_json: vol.detail_json,
            sort_order: vol.sort_order,
          })
        }
      }
      for (const id of deletedVolumeIds) {
        await app.DeleteVolume(id)
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

  // ── 卷纲操作 ──
  const addVolume = () => {
    const maxEnd = volumeList.length > 0 ? Math.max(...volumeList.map(v => v.end_chapter)) : 0
    setVolumeList(prev => [...prev, {
      id: -Date.now(),
      name: '',
      description: '',
      start_chapter: maxEnd + 1,
      end_chapter: maxEnd + 10,
      detail_json: '',
      sort_order: prev.length + 1,
      _status: 'new',
    }])
  }

  const updateVolume = (idx: number, field: string, value: any) => {
    setVolumeList(prev => prev.map((v, i) => {
      if (i !== idx) return v
      const updated = { ...v, [field]: value }
      if (updated._status !== 'new') updated._status = 'modified'
      return updated
    }))
  }

  const removeVolume = (idx: number) => {
    const vol = volumeList[idx]
    if (vol._status !== 'new') {
      setDeletedVolumeIds(prev => [...prev, vol.id])
    }
    setVolumeList(prev => prev.filter((_, i) => i !== idx))
  }

  const toggleVolume = (idx: number) => {
    setExpandedVolumes(prev => ({ ...prev, [idx]: !prev[idx] }))
  }

  // ── 成长弧线阶段操作 ──
  const updateGrowthArc = (stages: GrowthStage[]) => {
    setOutline(prev => ({ ...prev, growth_arc: serializeGrowthArc(stages, prev.growth_arc) }))
  }

  const addStage = () => {
    const lastEnd = growthStages.length > 0 ? growthStages[growthStages.length - 1].chapterEnd : 0
    const newStage: GrowthStage = { chapterStart: lastEnd + 1, chapterEnd: lastEnd + 10, name: '', description: '' }
    updateGrowthArc([...growthStages, newStage])
    setExpandedStages(prev => ({ ...prev, [growthStages.length]: true }))
  }

  const updateStage = (idx: number, patch: Partial<GrowthStage>) => {
    updateGrowthArc(growthStages.map((s, i) => i === idx ? { ...s, ...patch } : s))
  }

  const removeStage = (idx: number) => {
    updateGrowthArc(growthStages.filter((_, i) => i !== idx))
  }

  const toggleStage = (idx: number) => {
    setExpandedStages(prev => ({ ...prev, [idx]: !prev[idx] }))
  }

  // ── 通用 KV 字段操作 ──
  const updateKVField = (field: 'core_conflict' | 'ending_direction' | 'theme', items: KVItem[]) => {
    setOutline(prev => ({ ...prev, [field]: serializeKV(items, prev[field]) }))
  }

  const addKVItem = (field: 'core_conflict' | 'ending_direction' | 'theme') => {
    const items = field === 'core_conflict' ? coreKV : field === 'ending_direction' ? endingKV : themeKV
    updateKVField(field, [...items, { key: '', value: '' }])
  }

  const updateKVItem = (field: 'core_conflict' | 'ending_direction' | 'theme', idx: number, patch: Partial<KVItem>) => {
    const items = field === 'core_conflict' ? coreKV : field === 'ending_direction' ? endingKV : themeKV
    updateKVField(field, items.map((item, i) => i === idx ? { ...item, ...patch } : item))
  }

  const removeKVItem = (field: 'core_conflict' | 'ending_direction' | 'theme', idx: number) => {
    const items = field === 'core_conflict' ? coreKV : field === 'ending_direction' ? endingKV : themeKV
    updateKVField(field, items.filter((_, i) => i !== idx))
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const fieldClass = "w-full bg-background border border-border/60 rounded px-2 py-1.5 text-sm text-foreground placeholder:text-muted-foreground/40 focus:outline-none focus:ring-1 focus:ring-primary/30 focus:border-primary/50 resize-y min-h-[56px]"

  return (
    <div className="h-full overflow-auto p-4 space-y-3">
      {/* Header with tabs */}
      <div className="flex items-center justify-between sticky top-0 bg-editor-surface/80 backdrop-blur-sm z-10 pb-2 border-b border-border/30">
        <div className="flex items-center gap-1">
          <button
            onClick={() => setActiveTab('outline')}
            className={`text-xs px-2 py-1 rounded transition-colors ${activeTab === 'outline' ? 'bg-primary/10 text-primary font-medium' : 'text-muted-foreground hover:text-foreground'}`}
          >
            📜 {isZh ? '总纲' : 'Outline'}
          </button>
          <button
            onClick={() => setActiveTab('volumes')}
            className={`text-xs px-2 py-1 rounded transition-colors ${activeTab === 'volumes' ? 'bg-primary/10 text-primary font-medium' : 'text-muted-foreground hover:text-foreground'}`}
          >
            📚 {isZh ? '卷纲' : 'Volumes'}
          </button>
        </div>
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

      {/* 类型标签 */}
      {genre && (
        <div className="flex items-center gap-2">
          <span className="text-[10px] px-2 py-0.5 rounded-full bg-primary/10 text-primary border border-primary/20">
            {genre}
          </span>
        </div>
      )}

      {/* Tab content */}
      {activeTab === 'outline' ? (
        <>
      {/* Outline fields — card-sec grouped */}
      <div className="space-y-2">
        {/* 故事核: core_conflict KV 结构化 */}
        <div className="card-sec">
          <div className="card-sec-title">
            <span>{isZh ? '🎯 故事核' : '🎯 Story Core'}</span>
            <button
              onClick={() => addKVItem('core_conflict')}
              className="ml-auto text-[10px] text-primary hover:text-primary/80 transition-colors"
            >
              + {isZh ? '添加' : 'Add'}
            </button>
          </div>
          {hasStructuredCore ? (
            <div className="space-y-1.5">
              {coreKV.map((item, idx) => (
                <div key={idx} className="flex items-center gap-1.5">
                  <input
                    type="text"
                    value={item.key}
                    onChange={e => updateKVItem('core_conflict', idx, { key: e.target.value })}
                    className="w-24 bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground font-medium focus:outline-none focus:ring-1 focus:ring-primary/30 shrink-0"
                    placeholder={isZh ? '键' : 'Key'}
                  />
                  <span className="text-muted-foreground shrink-0">：</span>
                  <input
                    type="text"
                    value={item.value}
                    onChange={e => updateKVItem('core_conflict', idx, { value: e.target.value })}
                    className="flex-1 bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                    placeholder={isZh ? '值' : 'Value'}
                  />
                  <button
                    onClick={() => removeKVItem('core_conflict', idx)}
                    className="text-muted-foreground/30 hover:text-destructive shrink-0 p-0.5"
                    style={{ opacity: 0.4 }}
                  >
                    <Trash2 className="w-3 h-3" />
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <textarea
              value={outline.core_conflict}
              onChange={e => setOutline(prev => ({ ...prev, core_conflict: e.target.value }))}
              placeholder={isZh
                ? '主角与最终对手的根本冲突\n\n提示：用以下格式可显示为结构化视图\n> 主角：林逸\n> 反派：陈默\n> 根本冲突：被至交背叛夺走一切\n> 赌注：六界存亡'
                : 'Core conflict\n\nTip: Use this format for structured view:\n> Protagonist: Lin Yi\n> Antagonist: Chen Mo\n> Conflict: Betrayed by best friend\n> Stakes: Survival of six realms'}
              className={fieldClass}
              rows={3}
            />
          )}
        </div>

        {/* 成长弧线时间线 */}
        <div className="card-sec">
          <div className="card-sec-title">
            <span>{isZh ? '📈 成长弧线' : '📈 Growth Arc'}</span>
            <button
              onClick={addStage}
              className="ml-auto text-[10px] text-primary hover:text-primary/80 transition-colors"
            >
              + {isZh ? '添加阶段' : 'Add Stage'}
            </button>
          </div>

          {hasStructuredGrowth ? (
            <div className="space-y-1.5">
              {growthStages.map((stage, idx) => (
                <div key={idx} className="card-item" style={{ padding: '0.3rem 0.5rem' }}>
                  {/* Collapsed */}
                  <div
                    className="flex items-center gap-1.5 cursor-pointer select-none"
                    onClick={() => toggleStage(idx)}
                  >
                    <span className="text-[10px] text-muted-foreground shrink-0">
                      {expandedStages[idx] ? '▾' : '▸'}
                    </span>
                    <span className="text-xs font-mono text-primary font-semibold tabular-nums shrink-0">
                      Ch.{stage.chapterStart}-{stage.chapterEnd}
                    </span>
                    <span className="text-xs text-foreground font-medium shrink-0">
                      {stage.name || (isZh ? '(未命名)' : '(unnamed)')}
                    </span>
                    <span className="text-xs text-muted-foreground truncate flex-1">
                      {stage.description}
                    </span>
                    <button
                      onClick={(e) => { e.stopPropagation(); removeStage(idx) }}
                      className="text-muted-foreground/30 hover:text-destructive shrink-0 p-0.5"
                      style={{ opacity: 0.4 }}
                    >
                      <Trash2 className="w-3 h-3" />
                    </button>
                  </div>

                  {/* Expanded */}
                  {expandedStages[idx] && (
                    <div className="mt-2 pl-4 space-y-1.5 border-l border-border/40">
                      <div className="grid grid-cols-3 gap-1.5">
                        <div>
                          <label className="block text-[10px] text-muted-foreground mb-0.5">
                            {isZh ? '起始章' : 'Start Ch.'}
                          </label>
                          <input
                            type="number"
                            value={stage.chapterStart}
                            onChange={e => updateStage(idx, { chapterStart: parseInt(e.target.value) || 0 })}
                            className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                            min={1}
                          />
                        </div>
                        <div>
                          <label className="block text-[10px] text-muted-foreground mb-0.5">
                            {isZh ? '结束章' : 'End Ch.'}
                          </label>
                          <input
                            type="number"
                            value={stage.chapterEnd}
                            onChange={e => updateStage(idx, { chapterEnd: parseInt(e.target.value) || 0 })}
                            className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                            min={1}
                          />
                        </div>
                        <div>
                          <label className="block text-[10px] text-muted-foreground mb-0.5">
                            {isZh ? '阶段名' : 'Name'}
                          </label>
                          <input
                            type="text"
                            value={stage.name}
                            onChange={e => updateStage(idx, { name: e.target.value })}
                            className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                            placeholder={isZh ? '如：崛起期' : 'e.g. Rise'}
                          />
                        </div>
                      </div>
                      <textarea
                        value={stage.description}
                        onChange={e => updateStage(idx, { description: e.target.value })}
                        className={fieldClass}
                        rows={2}
                        placeholder={isZh ? '阶段描述：主角状态/关键事件/能力变化' : 'Stage description'}
                      />
                    </div>
                  )}
                </div>
              ))}
            </div>
          ) : (
            /* 降级：旧数据无结构化格式，显示 textarea */
            <textarea
              value={outline.growth_arc}
              onChange={e => setOutline(prev => ({ ...prev, growth_arc: e.target.value }))}
              placeholder={isZh
                ? '从哪里出发，最终变成什么样的人\n\n提示：用以下格式可显示为时间线\n> 第1-8章 废柴期：被欺压的普通弟子\n> 第9-18章 崛起期：获得金手指，修为暴涨'
                : 'Start → End transformation\n\nTip: Use this format for timeline view:\n> Ch.1-8 Weak phase: bullied disciple\n> Ch.9-18 Rise phase: gains power'}
              className={fieldClass}
              rows={4}
            />
          )}
        </div>

        {/* 结局方向: ending_direction KV 结构化 */}
        <div className="card-sec">
          <div className="card-sec-title">
            <span>{isZh ? '🎬 结局方向' : '🎬 Ending Direction'}</span>
            <button
              onClick={() => addKVItem('ending_direction')}
              className="ml-auto text-[10px] text-primary hover:text-primary/80 transition-colors"
            >
              + {isZh ? '添加' : 'Add'}
            </button>
          </div>
          {hasStructuredEnding ? (
            <div className="space-y-1.5">
              {endingKV.map((item, idx) => (
                <div key={idx} className="flex items-center gap-1.5">
                  <input
                    type="text"
                    value={item.key}
                    onChange={e => updateKVItem('ending_direction', idx, { key: e.target.value })}
                    className="w-24 bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground font-medium focus:outline-none focus:ring-1 focus:ring-primary/30 shrink-0"
                    placeholder={isZh ? '键' : 'Key'}
                  />
                  <span className="text-muted-foreground shrink-0">：</span>
                  <input
                    type="text"
                    value={item.value}
                    onChange={e => updateKVItem('ending_direction', idx, { value: e.target.value })}
                    className="flex-1 bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                    placeholder={isZh ? '值' : 'Value'}
                  />
                  <button
                    onClick={() => removeKVItem('ending_direction', idx)}
                    className="text-muted-foreground/30 hover:text-destructive shrink-0 p-0.5"
                    style={{ opacity: 0.4 }}
                  >
                    <Trash2 className="w-3 h-3" />
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <textarea
              value={outline.ending_direction}
              onChange={e => setOutline(prev => ({ ...prev, ending_direction: e.target.value }))}
              placeholder={isZh
                ? '大结局基本形态\n\n提示：用以下格式可显示为结构化视图\n> 类型：逆袭碾压\n> 基调：从最低谷到碾压巅峰\n> 收束：清算所有仇敌'
                : 'Grand finale form\n\nTip: Use this format for structured view:\n> Type: Comeback domination\n> Tone: From rock bottom to crushing peak\n> Resolution: Settle all scores'}
              className={fieldClass}
              rows={3}
            />
          )}
        </div>

        {/* 主题立意: theme KV 结构化 */}
        <div className="card-sec">
          <div className="card-sec-title">
            <span>{isZh ? '💎 主题立意' : '💎 Theme'}</span>
            <button
              onClick={() => addKVItem('theme')}
              className="ml-auto text-[10px] text-primary hover:text-primary/80 transition-colors"
            >
              + {isZh ? '添加' : 'Add'}
            </button>
          </div>
          {hasStructuredTheme ? (
            <div className="space-y-1.5">
              {themeKV.map((item, idx) => (
                <div key={idx} className="flex items-center gap-1.5">
                  <input
                    type="text"
                    value={item.key}
                    onChange={e => updateKVItem('theme', idx, { key: e.target.value })}
                    className="w-24 bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground font-medium focus:outline-none focus:ring-1 focus:ring-primary/30 shrink-0"
                    placeholder={isZh ? '键' : 'Key'}
                  />
                  <span className="text-muted-foreground shrink-0">：</span>
                  <input
                    type="text"
                    value={item.value}
                    onChange={e => updateKVItem('theme', idx, { value: e.target.value })}
                    className="flex-1 bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                    placeholder={isZh ? '值' : 'Value'}
                  />
                  <button
                    onClick={() => removeKVItem('theme', idx)}
                    className="text-muted-foreground/30 hover:text-destructive shrink-0 p-0.5"
                    style={{ opacity: 0.4 }}
                  >
                    <Trash2 className="w-3 h-3" />
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <textarea
              value={outline.theme}
              onChange={e => setOutline(prev => ({ ...prev, theme: e.target.value }))}
              placeholder={isZh
                ? '这本书想表达什么\n\n提示：用以下格式可显示为结构化视图\n> 核心主题：逆天改命\n> 深层追问：人的价值由谁定义'
                : 'What does this book express?\n\nTip: Use this format for structured view:\n> Core theme: Defying fate\n> Deep question: Who defines human worth'}
              className={fieldClass}
              rows={2}
            />
          )}
        </div>

        {/* 规模: word_count_plan 紧凑 */}
        <div className="card-sec" style={{ display: 'inline-block', minWidth: 160 }}>
          <div className="card-sec-title" style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <span>{isZh ? '📏 规模' : '📏 Scale'}</span>
            <input
              type="number"
              value={outline.word_count_plan || ''}
              onChange={e => setOutline(prev => ({ ...prev, word_count_plan: parseInt(e.target.value) || 0 }))}
              placeholder="—"
              className="w-16 bg-background border border-border/60 rounded px-2 py-0.5 text-sm text-foreground text-center focus:outline-none focus:ring-1 focus:ring-primary/30"
              min={0}
            />
            <span className="text-xs text-muted-foreground">{isZh ? '万字' : '10k'}</span>
          </div>
        </div>
      </div>

      {/* Beats section */}
      <div className="card-sec">
        <div className="card-sec-title">
          <span>{isZh ? '⚡ 大爽点' : '⚡ Key Beats'}</span>
          <span className="text-[10px] font-normal text-muted-foreground ml-1">({beats.length})</span>
          <button
            onClick={addBeat}
            className="ml-auto text-[10px] text-primary hover:text-primary/80 transition-colors"
          >
            + {isZh ? '添加' : 'Add'}
          </button>
        </div>

        {beats.length === 0 ? (
          <p className="text-xs text-muted-foreground/40 py-2 text-center">
            {isZh ? '点击 + 添加大爽点' : 'Click + to add a beat'}
          </p>
        ) : (
          <div className="space-y-1.5">
            {beats.map((beat, idx) => (
              <div key={beat.id} className="card-item" style={{ padding: '0.3rem 0.5rem' }}>
                {/* Collapsed: single line */}
                <div
                  className="flex items-center gap-1.5 cursor-pointer select-none"
                  onClick={() => toggleBeat(idx)}
                >
                  <span className="text-[10px] text-muted-foreground shrink-0">
                    {expanded[idx] ? '▾' : '▸'}
                  </span>
                  <span className="text-xs font-mono text-primary font-semibold tabular-nums shrink-0">
                    {isZh ? `第${beat.chapter}章` : `Ch.${beat.chapter}`}
                  </span>
                  <span className="text-xs text-foreground truncate flex-1">
                    {beat.description || (isZh ? '(未填写)' : '(empty)')}
                  </span>
                  <span className="card-item-tag shrink-0">
                    {BEAT_TYPES.find(bt => bt.value === beat.beat_type)?.[isZh ? 'zh' : 'en'] ?? beat.beat_type}
                  </span>
                  <span className="card-item-tag shrink-0">
                    {'★'.repeat(beat.importance)}
                  </span>
                  <button
                    onClick={(e) => { e.stopPropagation(); removeBeat(idx) }}
                    className="text-muted-foreground/30 hover:text-destructive shrink-0 p-0.5 opacity-0 group-hover:opacity-100"
                    style={{ opacity: 0.4 }}
                  >
                    <Trash2 className="w-3 h-3" />
                  </button>
                </div>

                {/* Expanded: edit fields */}
                {expanded[idx] && (
                  <div className="mt-2 pl-4 space-y-1.5 border-l border-border/40">
                    <div className="grid grid-cols-3 gap-1.5">
                      <div>
                        <label className="block text-[10px] text-muted-foreground mb-0.5">
                          {isZh ? '章节' : 'Ch.'}
                        </label>
                        <input
                          type="number"
                          value={beat.chapter}
                          onChange={e => updateBeat(idx, 'chapter', parseInt(e.target.value) || 0)}
                          className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
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
                          className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                        >
                          {BEAT_TYPES.map(bt => (
                            <option key={bt.value} value={bt.value}>{isZh ? bt.zh : bt.en}</option>
                          ))}
                        </select>
                      </div>
                      <div>
                        <label className="block text-[10px] text-muted-foreground mb-0.5">
                          {isZh ? '重要度' : 'Import.'}
                        </label>
                        <select
                          value={beat.importance}
                          onChange={e => updateBeat(idx, 'importance', parseInt(e.target.value))}
                          className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                        >
                          {[1, 2, 3, 4, 5].map(v => (
                            <option key={v} value={v}>{'★'.repeat(v)}</option>
                          ))}
                        </select>
                      </div>
                    </div>
                    <textarea
                      value={beat.description}
                      onChange={e => updateBeat(idx, 'description', e.target.value)}
                      placeholder={isZh ? '大爽点描述' : 'Beat description'}
                      className={fieldClass}
                      rows={2}
                    />
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
        </>
      ) : (
        /* ═══ 卷纲 Tab ═══ */
        <div className="space-y-2">
          <div className="card-sec">
            <div className="card-sec-title">
              <span>{isZh ? '📚 卷结构' : '📚 Volume Structure'}</span>
              <button
                onClick={addVolume}
                className="ml-auto text-[10px] text-primary hover:text-primary/80 transition-colors"
              >
                + {isZh ? '添加卷' : 'Add Volume'}
              </button>
            </div>

            {volumeList.length === 0 ? (
              <p className="text-xs text-muted-foreground/40 py-2 text-center">
                {isZh ? '点击 + 添加卷' : 'Click + to add a volume'}
              </p>
            ) : (
              <div className="space-y-1.5">
                {volumeList.map((vol, idx) => (
                  <div key={vol.id} className="card-item" style={{ padding: '0.3rem 0.5rem' }}>
                    {/* Collapsed */}
                    <div
                      className="flex items-center gap-1.5 cursor-pointer select-none"
                      onClick={() => toggleVolume(idx)}
                    >
                      <span className="text-[10px] text-muted-foreground shrink-0">
                        {expandedVolumes[idx] ? '▾' : '▸'}
                      </span>
                      <span className="text-xs font-mono text-primary font-semibold tabular-nums shrink-0">
                        Ch.{vol.start_chapter}-{vol.end_chapter}
                      </span>
                      <span className="text-xs text-foreground font-medium flex-1 truncate">
                        {vol.name || (isZh ? '(未命名)' : '(unnamed)')}
                      </span>
                      <span className="text-xs text-muted-foreground truncate max-w-[200px]">
                        {vol.description}
                      </span>
                      <button
                        onClick={(e) => { e.stopPropagation(); removeVolume(idx) }}
                        className="text-muted-foreground/30 hover:text-destructive shrink-0 p-0.5"
                        style={{ opacity: 0.4 }}
                      >
                        <Trash2 className="w-3 h-3" />
                      </button>
                    </div>

                    {/* Expanded */}
                    {expandedVolumes[idx] && (
                      <div className="mt-2 pl-4 space-y-1.5 border-l border-border/40">
                        <div className="grid grid-cols-3 gap-1.5">
                          <div>
                            <label className="block text-[10px] text-muted-foreground mb-0.5">
                              {isZh ? '卷名' : 'Name'}
                            </label>
                            <input
                              type="text"
                              value={vol.name}
                              onChange={e => updateVolume(idx, 'name', e.target.value)}
                              className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                              placeholder={isZh ? '如：第一卷·崛起' : 'e.g. Vol.1 Rise'}
                            />
                          </div>
                          <div>
                            <label className="block text-[10px] text-muted-foreground mb-0.5">
                              {isZh ? '起始章' : 'Start Ch.'}
                            </label>
                            <input
                              type="number"
                              value={vol.start_chapter}
                              onChange={e => updateVolume(idx, 'start_chapter', parseInt(e.target.value) || 0)}
                              className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                              min={1}
                            />
                          </div>
                          <div>
                            <label className="block text-[10px] text-muted-foreground mb-0.5">
                              {isZh ? '结束章' : 'End Ch.'}
                            </label>
                            <input
                              type="number"
                              value={vol.end_chapter}
                              onChange={e => updateVolume(idx, 'end_chapter', parseInt(e.target.value) || 0)}
                              className="w-full bg-background border border-border/60 rounded px-1.5 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary/30"
                              min={1}
                            />
                          </div>
                        </div>
                        <div>
                          <label className="block text-[10px] text-muted-foreground mb-0.5">
                            {isZh ? '描述' : 'Description'}
                          </label>
                          <textarea
                            value={vol.description}
                            onChange={e => updateVolume(idx, 'description', e.target.value)}
                            className={fieldClass}
                            rows={2}
                            placeholder={isZh ? '卷纲概述' : 'Volume summary'}
                          />
                        </div>
                        <div>
                          <label className="block text-[10px] text-muted-foreground mb-0.5">
                            {isZh ? '卷纲详情' : 'Detail'}
                          </label>
                          {(() => {
                            // 智能渲染：> 格式 → KV 列表；JSON → KV 列表；其他 → textarea
                            const val = vol.detail_json || ''
                            const kvItems = parseKV(val)
                            let jsonItems: { key: string; value: string }[] = []
                            if (kvItems.length === 0 && val.trim().startsWith('{')) {
                              try {
                                const obj = JSON.parse(val)
                                jsonItems = Object.entries(obj).map(([k, v]) => ({
                                  key: k,
                                  value: typeof v === 'string' ? v : JSON.stringify(v),
                                }))
                              } catch { /* not JSON */ }
                            }
                            const displayItems = kvItems.length > 0 ? kvItems : jsonItems

                            if (displayItems.length > 0) {
                              return (
                                <div className="space-y-1">
                                  {displayItems.map((item, i) => (
                                    <div key={i} className="flex items-start gap-1.5 text-xs">
                                      <span className="text-muted-foreground font-medium shrink-0 min-w-[60px]">{item.key}</span>
                                      <span className="text-foreground">{item.value}</span>
                                    </div>
                                  ))}
                                </div>
                              )
                            }
                            return (
                              <textarea
                                value={val}
                                onChange={e => updateVolume(idx, 'detail_json', e.target.value)}
                                className={fieldClass}
                                rows={3}
                                placeholder={isZh
                                  ? '> 核心事件：...\n> 主角变化：...\n> 收尾钩子：...'
                                  : '> core_event: ...\n> protagonist: ...\n> ending_hook: ...'}
                              />
                            )
                          })()}
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
