import { useState, useEffect, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight, FileText, Pencil, Plus, Download, Trash2 } from 'lucide-react'
import { toastError } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/useApp'
import type { chapter } from '@/hooks/useApp'
import { EventsOn } from '@/lib/wailsjs/runtime/runtime'

interface Props {
  novelId: number
  target: { path: string; title: string } | null
  onSelectChapter: (ch: chapter.Chapter) => void
  onSelectOutline: (path: string, title: string) => void
  onSelectGoink: () => void
  onSelectBookOutline: () => void
  onExportNovel: () => void
}

const BLOCK_SIZE = 100

// AI 可能把「第N章」写进标题，前端归一化避免与编号重复显示
const stripChapterPrefix = (title: string) => title.replace(/^第\s*\d+\s*章\s*[·:：-]?\s*/, '')

interface OutlineItem {
  chapter_number: number
  file_path: string
  title: string
}

export default function ChapterList({ novelId, target, onSelectChapter, onSelectOutline, onSelectGoink, onSelectBookOutline, onExportNovel }: Props) {
  const { t } = useTranslation()
  const app = useApp()

  const [chapters, setChapters] = useState<chapter.Chapter[]>([])
  const [outlines, setOutlines] = useState<OutlineItem[]>([])
  const [showOutlines, setShowOutlines] = useState(false)
  const [chapterTitle, setChapterTitle] = useState('')
  const [showCreateChapter, setShowCreateChapter] = useState(false)
  const [expandedBlocks, setExpandedBlocks] = useState<Set<number>>(new Set())
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [loadError, setLoadError] = useState('')
  const [createError, setCreateError] = useState('')

  const loadChapters = useCallback(async () => {
    if (!novelId) { setChapters([]); return }
    try {
      const list = await app.GetChapters(novelId)
      setChapters(list ?? [])
      setLoadError('')
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err))
    }
  }, [novelId, app])

  const loadOutlines = useCallback(async () => {
    if (!novelId) { setOutlines([]); return }
    try {
      const list = await app.ListOutlines(novelId)
      setOutlines(list ?? [])
    } catch { /* 大纲列表加载失败不影响主列表 */ }
  }, [novelId, app])

  useEffect(() => { loadChapters(); loadOutlines() }, [loadChapters, loadOutlines])

  // file:changed 时刷新章节列表（字数统计、新章等）
  useEffect(() => {
    const unsub = EventsOn('file:changed', (data: any) => {
      if (data.novel_id !== novelId) return
      if (data.path && (data.path.startsWith('chapters/') || data.path.startsWith('outlines/') || data.path === 'goink.md' || data.path === 'book-outline.md')) {
        loadChapters()
        loadOutlines()
      }
    })
    return () => unsub()
  }, [novelId, loadChapters, loadOutlines])

  // ── 章节分块 ────────────────────────────────────────────

  const chapterBlocks = useMemo(() => {
    const sorted = [...chapters].sort((a, b) => b.chapter_number - a.chapter_number)
    const blocks: { key: number; start: number; end: number; chs: chapter.Chapter[] }[] = []
    for (let i = 0; i < sorted.length; i += BLOCK_SIZE) {
      const slice = sorted.slice(i, Math.min(i + BLOCK_SIZE, sorted.length))
      slice.sort((a, b) => a.chapter_number - b.chapter_number)
      blocks.push({
        key: i / BLOCK_SIZE,
        start: slice[0].chapter_number,
        end: slice[slice.length - 1].chapter_number,
        chs: slice,
      })
    }
    return blocks
  }, [chapters])

  function toggleBlock(key: number) {
    setExpandedBlocks(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  async function handleCreateChapter() {
    if (!chapterTitle.trim()) return
    try {
      await app.CreateChapter({ novel_id: novelId, title: chapterTitle.trim() })
      setChapterTitle('')
      setShowCreateChapter(false)
      setCreateError('')
      loadChapters()
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : String(err))
    }
  }

  function startEdit(ch: chapter.Chapter) {
    setEditingId(ch.id)
    setEditTitle(ch.title)
  }

  async function commitEdit() {
    if (editingId == null) return
    const ch = chapters.find(c => c.id === editingId)
    if (!ch) return
    const newTitle = editTitle.trim()
    if (newTitle && newTitle !== ch.title) {
      try {
        await app.UpdateChapterTitle(novelId, ch.chapter_number, newTitle)
        loadChapters()
      } catch (err) {
        toastError(t('common.saveFailed') + ': ' + (err instanceof Error ? err.message : String(err)))
        console.error(err)
      }
    }
    setEditingId(null)
  }

  function cancelEdit() {
    setEditingId(null)
  }

  async function handleDeleteChapter(ch: chapter.Chapter) {
    const msg = `确认删除第 ${ch.chapter_number} 章「${ch.title}」？\n\n正文文件和章节记录将被永久删除，此操作不可恢复。`
    if (!window.confirm(msg)) return
    try {
      await app.DeleteChapter(novelId, ch.chapter_number)
      loadChapters()
    } catch (err) {
      toastError(t('common.saveFailed') + ': ' + (err instanceof Error ? err.message : String(err)))
    }
  }

  async function handleDeleteOutline(o: OutlineItem) {
    const msg = `确认删除第 ${o.chapter_number} 章大纲？\n\n大纲文件将被永久删除，此操作不可恢复。`
    if (!window.confirm(msg)) return
    try {
      await app.DeleteOutlineFile(novelId, o.chapter_number)
      loadOutlines()
    } catch (err) {
      toastError(t('common.saveFailed') + ': ' + (err instanceof Error ? err.message : String(err)))
    }
  }

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t('sidebar.chaptersCount', { count: chapters.length })}
        </span>
        <div className="flex items-center gap-0.5">
          <button
            onClick={onExportNovel}
            className="w-6 h-6 flex items-center justify-center rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
            title={t('sidebar.export')}
          >
            <Download className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => setShowCreateChapter(true)}
            className="w-6 h-6 flex items-center justify-center rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
          >
            <Plus className="w-4 h-4" />
          </button>
        </div>
      </div>

      {showCreateChapter && (
        <div className="p-3 border-b space-y-2">
          <input
            type="text" value={chapterTitle} autoFocus
            onChange={e => { setChapterTitle(e.target.value); setCreateError('') }}
            onKeyDown={e => e.key === 'Enter' && handleCreateChapter()}
            placeholder={t('sidebar.chapterTitle')}
            className="w-full h-8 rounded-md border bg-background px-2.5 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          {createError && <p className="text-xs text-destructive">{createError}</p>}
          <div className="flex gap-2">
            <Button size="sm" onClick={handleCreateChapter}>{t('sidebar.add')}</Button>
            <Button size="sm" variant="ghost" onClick={() => { setShowCreateChapter(false); setChapterTitle(''); setCreateError('') }}>{t('sidebar.cancel')}</Button>
          </div>
        </div>
      )}

      <button
        onClick={onSelectGoink}
        className={`w-full flex items-center gap-2.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors relative border-b border-border/50 bg-sidebar
          ${target?.path === 'goink.md' ? 'bg-primary/10 font-medium glow-primary' : ''}`}
      >
        {target?.path === 'goink.md' && (
          <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
        )}
        <FileText className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
        <span className="flex-1 text-sm truncate">{t('sidebar.storyStatus')}</span>
      </button>

      <button
        onClick={onSelectBookOutline}
        className={`w-full flex items-center gap-2.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors relative border-b border-border/50 bg-sidebar
          ${target?.path === 'book-outline.md' ? 'bg-primary/10 font-medium glow-primary' : ''}`}
      >
        {target?.path === 'book-outline.md' && (
          <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
        )}
        <FileText className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
        <span className="flex-1 text-sm truncate">{t('sidebar.bookOutline')}</span>
      </button>

      {/* 大纲列表：复用正文列表的交互，读取路径为 outlines/ */}
      <button
        onClick={() => setShowOutlines(!showOutlines)}
        className="w-full flex items-center gap-1.5 px-3 py-1.5 text-left hover:bg-muted/30 transition-colors border-b border-border/50"
      >
        <ChevronRight
          className={`w-3.5 h-3.5 text-muted-foreground shrink-0 transition-transform duration-200 ${showOutlines ? 'rotate-90' : ''}`}
        />
        <span className="text-xs text-muted-foreground">{t('sidebar.outlines', { count: outlines.length })}</span>
        <span className="text-[10px] text-muted-foreground/50 ml-auto">{t('sidebar.chapterCountShort', { count: outlines.length })}</span>
      </button>
      {showOutlines && (
        <div className="border-b border-border/50 max-h-52 overflow-y-auto overscroll-contain">
          {outlines.length === 0 ? (
            <p className="text-xs text-muted-foreground/70 px-3 py-2">{t('sidebar.noOutlines')}</p>
          ) : (
            outlines.map(o => (
              <div key={o.chapter_number} className="group flex items-center w-full">
                <button
                  onClick={() => onSelectOutline(o.file_path, o.title ? `第${o.chapter_number}章 · ${o.title}` : `第${o.chapter_number}章 大纲`)}
                  className={`flex items-center gap-2.5 pl-5 pr-10 py-1.5 text-left hover:bg-muted/50 transition-colors flex-1 min-w-0 relative bg-sidebar
                    ${target?.path === o.file_path ? 'bg-primary/10 font-medium glow-primary' : ''}`}
                >
                  {target?.path === o.file_path && (
                    <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
                  )}
                  <span className="text-xs text-muted-foreground shrink-0 whitespace-nowrap tabular-nums">
                    {t('sidebar.chapterN', { n: o.chapter_number })}
                  </span>
                  <span className="flex-1 text-sm truncate">{stripChapterPrefix(o.title || '') || t('sidebar.outlineFallback')}</span>
                </button>
                <button
                  onClick={e => { e.stopPropagation(); handleDeleteOutline(o) }}
                  className="opacity-0 group-hover:opacity-100 w-6 h-6 flex items-center justify-center rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-all shrink-0 mr-1"
                  title="删除大纲"
                >
                  <Trash2 className="w-3 h-3" />
                </button>
              </div>
            ))
          )}
        </div>
      )}

      <div className="flex-1 min-h-0 overflow-y-auto overscroll-contain">
        {chapters.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <FileText className="w-8 h-8 text-muted-foreground/30 mx-auto mb-2" />
              {loadError ? (
                <>
                  <p className="text-xs text-destructive">{loadError}</p>
                  <button onClick={() => loadChapters()} className="text-xs text-primary underline mt-1">{t('common.retry')}</button>
                </>
              ) : (
                <>
                  <p className="text-xs text-muted-foreground">{t('sidebar.noChapters')}</p>
                  <p className="text-xs text-muted-foreground/60 mt-0.5">{t('sidebar.createFirstChapter')}</p>
                </>
              )}
            </div>
          </div>
        ) : (
          chapterBlocks.map(block => {
            const isExpanded = expandedBlocks.has(block.key)
            const range = block.start === block.end
              ? t('sidebar.chapterN', { n: block.start })
              : t('sidebar.chapterRange', { start: block.start, end: block.end })
            return (
              <div key={block.key}>
                <button
                  onClick={() => toggleBlock(block.key)}
                  className="w-full flex items-center gap-1.5 px-3 py-1.5 text-left hover:bg-muted/30 transition-colors border-b border-border/50 sticky top-0 bg-sidebar z-20"
                >
                  <ChevronRight
                    className={`w-3.5 h-3.5 text-muted-foreground shrink-0 transition-transform duration-200 ${isExpanded ? 'rotate-90' : ''}`}
                  />
                  <span className="text-xs text-muted-foreground">{range}</span>
                  <span className="text-[10px] text-muted-foreground/50 ml-auto">{t('sidebar.chapterCountShort', { count: block.chs.length })}</span>
                </button>
                {isExpanded && (
                  <div>
                    {block.chs.map(ch => (
                      <div
                        key={ch.id}
                        className="group flex items-center w-full relative bg-sidebar"
                      >
                        <button
                          onClick={() => onSelectChapter(ch)}
                          className={`flex items-center gap-2.5 pl-5 pr-10 py-1.5 text-left hover:bg-muted/50 transition-colors flex-1 min-w-0 bg-sidebar
                            ${target?.path === ch.file_path ? 'bg-primary/10 font-medium glow-primary' : ''}`}
                        >
                          {target?.path === ch.file_path && (
                            <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
                          )}
                          <span className="text-xs text-muted-foreground shrink-0 whitespace-nowrap tabular-nums">
                            {t('sidebar.chapterN', { n: ch.chapter_number })}
                          </span>
                          {editingId === ch.id ? (
                            <input
                              value={editTitle}
                              onChange={e => setEditTitle(e.target.value)}
                              onKeyDown={e => {
                                if (e.key === 'Enter') commitEdit()
                                if (e.key === 'Escape') cancelEdit()
                              }}
                              onBlur={commitEdit}
                              autoFocus
                              onClick={e => e.stopPropagation()}
                              className="flex-1 h-6 rounded border bg-background px-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            />
                          ) : (
                            <span className="flex-1 text-sm truncate cursor-text" title="点击编辑" onClick={e => { e.stopPropagation(); startEdit(ch) }}>{stripChapterPrefix(ch.title)}</span>
                          )}
                          {ch.word_count > 0 && editingId !== ch.id && (
                            <span className="text-[10px] text-muted-foreground/60 shrink-0">
                              {t('sidebar.wordCount', { count: ch.word_count })}
                            </span>
                          )}
                        </button>
                        {editingId !== ch.id && (
                          <>
                            <button
                              onClick={e => { e.stopPropagation(); handleDeleteChapter(ch) }}
                              className="absolute right-6 top-1/2 -translate-y-1/2 w-6 h-6 flex items-center justify-center rounded opacity-0 group-hover:opacity-100 hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-all z-10"
                              title="删除章节"
                            >
                              <Trash2 className="w-3 h-3" />
                            </button>
                            <button
                              onClick={e => { e.stopPropagation(); startEdit(ch) }}
                              className="absolute right-0 top-1/2 -translate-y-1/2 w-6 h-6 flex items-center justify-center rounded opacity-0 group-hover:opacity-100 hover:bg-muted text-muted-foreground hover:text-foreground transition-all z-10"
                            >
                              <Pencil className="w-3 h-3" />
                            </button>
                          </>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )
          })
        )}
      </div>
    </>
  )
}
