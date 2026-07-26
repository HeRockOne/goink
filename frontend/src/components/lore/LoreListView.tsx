import { useState, useEffect, useCallback } from 'react'
import { Search, Pencil, Trash2, BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useApp } from '@/hooks/useApp'
import type { lore } from '@/hooks/useApp'

interface Props { novelId: number }

const CATEGORIES = ['力量体系', '社会构成', '历史事件', '核心冲突', '天道法则', '文化习俗', '种族', '地理概述']

export default function LoreListView({ novelId }: Props) {
  const app = useApp()
  const { t } = useTranslation()
  const [items, setItems] = useState<lore.LoreEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [category, setCategory] = useState('')
  const [search, setSearch] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [editing, setEditing] = useState<lore.LoreEntry | null>(null)
  const [title, setTitle] = useState('')
  const [cat, setCat] = useState('')
  const [content, setContent] = useState('')
  const [summary, setSummary] = useState('')
  const [arcId, setArcId] = useState<number | undefined>(undefined)
  const [revealChapterId, setRevealChapterId] = useState<number | undefined>(undefined)
  const [isPublic, setIsPublic] = useState(true)
  const [detail, setDetail] = useState<lore.LoreEntry | null>(null)

  const load = useCallback(async () => {
    if (!novelId) return
    setLoading(true)
    try {
      const r = await app.GetLoreList(novelId, category, search, 1, 999)
      setItems(r?.items ?? [])
    } catch { /* ignore */ }
    setLoading(false)
  }, [app, novelId, category, search])

  useEffect(() => { load() }, [load])

  function openCreate() { setEditing(null); setTitle(''); setCat(''); setContent(''); setSummary(''); setArcId(undefined); setRevealChapterId(undefined); setIsPublic(true); setShowForm(true) }
  function openEdit(i: lore.LoreEntry) { setEditing(i); setTitle(i.title); setCat(i.category); setContent(i.content); setSummary(i.summary); setArcId(i.arc_id ?? undefined); setRevealChapterId(i.reveal_chapter_id ?? undefined); setIsPublic(i.is_public ?? true); setShowForm(true) }
  async function save() {
    try {
      if (editing) { await app.UpdateLore(editing.id, { title, category: cat, content, summary, arc_id: arcId ?? undefined, reveal_chapter_id: revealChapterId ?? undefined, is_public: isPublic }) }
      else { await app.CreateLore(novelId, title, cat, content, summary, arcId ?? undefined, revealChapterId ?? undefined, isPublic) }
      setShowForm(false); load()
    } catch { alert('保存失败') }
  }
  async function remove(id: number) { if (!confirm('确认删除？')) return; await app.DeleteLore(id); load() }

  const grouped: Record<string, lore.LoreEntry[]> = {}
  items.forEach(i => { if (!grouped[i.category]) grouped[i.category] = []; grouped[i.category].push(i) })
  const allCats = [...new Set([...CATEGORIES, ...items.map(i => i.category)])]

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-3 py-2.5 border-b shrink-0">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t('lore.lore', '世界观设定')} ({items.length})
        </span>
        <button onClick={openCreate} className="text-xs text-primary hover:underline">+ 新建</button>
      </div>

      <div className="px-2 py-1.5 border-b shrink-0 flex items-center gap-1.5">
        <select value={category} onChange={e => setCategory(e.target.value)} className="h-7 text-xs rounded-md border bg-background px-1.5">
          <option value="">分类</option>
          {allCats.map(c => <option key={c} value={c}>{c}</option>)}
        </select>
        <div className="relative flex-1">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-muted-foreground" />
          <input
            type="text" value={search} onChange={e => setSearch(e.target.value)}
            placeholder={t('lore.searchLore', '搜索设定')}
            className="w-full h-7 rounded-md border bg-background pl-7 pr-2 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto overscroll-contain">
        {loading ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">{t('common.loading', '加载中')}</p>
          </div>
        ) : items.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">{search || category ? '无匹配结果' : '暂无设定'}</p>
          </div>
        ) : Object.entries(grouped).map(([cg, entries]) => (
          <div key={cg}>
            <div className="text-[10px] font-medium text-muted-foreground/70 px-3 py-1.5 bg-muted/30 sticky top-0 z-10">{cg}</div>
            {entries.map(item => (
              <div key={item.id}
                className="w-full flex items-center gap-1.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors group cursor-pointer"
                onClick={() => setDetail(item)}>
                <BookOpen className="w-3.5 h-3.5 shrink-0 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <div className="text-sm truncate">{item.title}</div>
                  <div className="text-[10px] text-muted-foreground truncate">{item.summary || item.content?.slice(0, 50)}</div>
                </div>
                <button onClick={e => { e.stopPropagation(); openEdit(item) }} className="shrink-0 p-0.5 rounded text-muted-foreground hover:text-foreground opacity-0 group-hover:opacity-100 transition-opacity">
                  <Pencil className="h-3 w-3" />
                </button>
                <button onClick={e => { e.stopPropagation(); remove(item.id) }} className="shrink-0 p-0.5 rounded text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity">
                  <Trash2 className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        ))}
      </div>

      {/* Form Dialog */}
      {showForm && (
        <div className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center" onClick={() => setShowForm(false)}>
          <div className="bg-card rounded-lg shadow-lg p-6 w-[500px] max-w-[90vw] max-h-[85vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <h3 className="text-sm font-medium mb-4">{editing ? '编辑设定' : '新建设定'}</h3>
            <div className="space-y-3">
              <input placeholder="标题" value={title} onChange={e => setTitle(e.target.value)} className="w-full h-9 text-sm rounded border bg-background px-3" />
              <select value={cat} onChange={e => setCat(e.target.value)} className="w-full h-9 text-sm rounded border bg-background px-3">
                <option value="">选择分类</option>
                {allCats.map(c => <option key={c} value={c}>{c}</option>)}
              </select>
              <input placeholder="一句话摘要（可选）" value={summary} onChange={e => setSummary(e.target.value)} className="w-full h-9 text-sm rounded border bg-background px-3" />
              <textarea placeholder="正文内容（Markdown）" rows={8} value={content} onChange={e => setContent(e.target.value)} className="w-full text-sm rounded border bg-background p-3 resize-y" />
              <div className="flex gap-2">
                <input placeholder="关联弧线 ID（可选）" type="number" value={arcId ?? ''} onChange={e => setArcId(e.target.value ? Number(e.target.value) : undefined)} className="flex-1 h-9 text-sm rounded border bg-background px-3" />
                <input placeholder="揭示章节 ID（可选）" type="number" value={revealChapterId ?? ''} onChange={e => setRevealChapterId(e.target.value ? Number(e.target.value) : undefined)} className="flex-1 h-9 text-sm rounded border bg-background px-3" />
              </div>
              <label className="flex items-center gap-2 text-xs">
                <input type="checkbox" checked={isPublic} onChange={e => setIsPublic(e.target.checked)} className="rounded" />
                公开设定（取消勾选即隐藏，仅角色知晓）
              </label>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <button onClick={() => setShowForm(false)} className="px-3 py-1.5 text-xs rounded border">{t('common.cancel', '取消')}</button>
              <button onClick={save} disabled={!title || !cat} className="px-3 py-1.5 text-xs rounded bg-primary text-primary-foreground disabled:opacity-50">保存</button>
            </div>
          </div>
        </div>
      )}

      {/* Detail Dialog */}
      {detail && (
        <div className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center" onClick={() => setDetail(null)}>
          <div className="bg-card rounded-lg shadow-lg p-6 w-[500px] max-w-[90vw] max-h-[85vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <h3 className="text-sm font-medium mb-2">{detail.title}</h3>
            <div className="space-y-2 text-sm">
              <div className="text-xs text-muted-foreground">分类: {detail.category}</div>
              {detail.arc_id && <div className="text-xs text-muted-foreground">关联弧线 ID: {detail.arc_id}</div>}
              {detail.reveal_chapter_id && <div className="text-xs text-muted-foreground">揭示章节: 第 {detail.reveal_chapter_id} 章</div>}
              {detail.is_public === false && <div className="text-xs text-destructive">隐藏设定（读者未知）</div>}
              {detail.summary && <div className="text-sm text-muted-foreground mb-2">{detail.summary}</div>}
            </div>
            <div className="text-sm whitespace-pre-wrap leading-relaxed mt-3">{detail.content}</div>
          </div>
        </div>
      )}
    </div>
  )
}
