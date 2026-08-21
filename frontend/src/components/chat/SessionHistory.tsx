import { useEffect, useRef, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageSquare, Loader2, History, Trash2, CheckSquare, Square, CheckCheck, ArrowUpDown, Download } from 'lucide-react'
import { toast } from 'sonner'
import type { app } from '@/hooks/useApp'
import { useApp } from '@/hooks/useApp'

interface Props {
  open: boolean
  novelId: number
  activeSessionId?: string | null
  onClose: () => void
  onSelectSession: (sessionId: string) => void
  onDeleted?: (deletedIds: string[]) => void
}

export default function SessionHistory({ open, novelId, activeSessionId, onClose, onSelectSession, onDeleted }: Props) {
  const { t } = useTranslation()
  const app = useApp()
  const [mounted, setMounted] = useState(false)
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!open) return
    const timer = setInterval(() => setNow(Date.now()), 60_000)
    return () => clearInterval(timer)
  }, [open])

  function timeAgo(iso: string): string {
    const diff = now - new Date(iso).getTime()
    const min = Math.floor(diff / 60000)
    if (min < 1) return t('chat.justNow')
    if (min < 60) return t('chat.minutesAgo', { count: min })
    const hour = Math.floor(min / 60)
    if (hour < 24) return t('chat.hoursAgo', { count: hour })
    const day = Math.floor(hour / 24)
    if (day < 30) return t('chat.daysAgo', { count: day })
    return t('chat.monthsAgo', { count: Math.floor(day / 30) })
  }
  const [visible, setVisible] = useState(false)
  const [sessions, setSessions] = useState<app.SessionMeta[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [isLoading, setIsLoading] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [search, setSearch] = useState('')
  const [submittedSearch, setSubmittedSearch] = useState('')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [isDeleting, setIsDeleting] = useState(false)
  const listRef = useRef<HTMLDivElement>(null)
  const loadingRef = useRef(false)
  const searchRef = useRef('')

  const loadPageRef = useRef<(p: number) => void>(null as any)

  useEffect(() => {
    loadPageRef.current = async (p: number) => {
      if (loadingRef.current) return
      loadingRef.current = true
      setIsLoading(true)
      try {
        const result = await app.GetSessions({ novel_id: novelId, page: p, size: 20, search: searchRef.current })
        if (result?.items) {
          setSessions(prev => p === 1 ? result.items : [...prev, ...result.items])
          setTotal(result.total)
          setHasMore(result.page < result.total_pages)
          if (p === 1) setSelectedIds(new Set())
        }
      } catch {
        // ignore
      } finally {
        setIsLoading(false)
        loadingRef.current = false
      }
    }
  }, [app, novelId])

  useEffect(() => {
    if (open) {
      setMounted(true)
      requestAnimationFrame(() => setVisible(true))
    } else {
      setVisible(false)
      const timer = setTimeout(() => setMounted(false), 200)
      return () => clearTimeout(timer)
    }
  }, [open])

  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchRef.current !== search) {
        searchRef.current = search
        setSubmittedSearch(search)
        setSessions([])
        setPage(1)
        setHasMore(true)
        loadPageRef.current?.(1)
      }
    }, 300)
    return () => clearTimeout(timer)
  }, [search])

  useEffect(() => {
    if (!open) return
    setSearch('')
    setSubmittedSearch('')
    searchRef.current = ''
    setSessions([])
    setPage(1)
    setHasMore(true)
    setSelectedIds(new Set())
    loadPageRef.current?.(1)
  }, [open, novelId])

  const handleScroll = useCallback(() => {
    if (!listRef.current || !hasMore || isLoading) return
    const { scrollTop, scrollHeight, clientHeight } = listRef.current
    if (scrollHeight - scrollTop - clientHeight < 80) {
      const next = page + 1
      setPage(next)
      loadPageRef.current?.(next)
    }
  }, [hasMore, isLoading, page])

  const toggleSelect = useCallback((sid: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(sid)) next.delete(sid)
      else next.add(sid)
      return next
    })
  }, [])

  const selectAll = useCallback(() => {
    setSelectedIds(new Set(sessions.map(s => s.session_id)))
  }, [sessions])

  const invertSelect = useCallback(() => {
    setSelectedIds(prev => {
      const next = new Set<string>()
      sessions.forEach(s => {
        if (!prev.has(s.session_id)) next.add(s.session_id)
      })
      return next
    })
  }, [sessions])

  const deleteSelected = useCallback(async () => {
    const count = selectedIds.size
    if (count === 0) return
    const msg = count === 1 ? t('chat.confirmDeleteSession') : t('chat.confirmDeleteSessions', '确定要删除选中的 ' + count + ' 个会话吗？此操作不可恢复。')
    if (!window.confirm(msg)) return
    setIsDeleting(true)
    const ids = [...selectedIds]
    for (const sid of ids) {
      try { await app.DeleteSession(sid) } catch { /* ignore */ }
    }
    setIsDeleting(false)
    setSelectedIds(new Set())
    loadPageRef.current?.(1)
    // 回传被删会话 ID：父级据此判断当前活跃会话是否被删，避免残留脏 session_id
    onDeleted?.(ids)
  }, [selectedIds, app, t, onDeleted])

  // 导出单个会话为 Markdown
  const [exportingId, setExportingId] = useState<string | null>(null)
  const handleExport = useCallback(async (sid: string) => {
    setExportingId(sid)
    try {
      const path = await app.ExportSession(sid)
      if (path) {
        toast.success(t('chat.exported', '会话已导出：{path}').replace('{path}', path), { duration: 4000 })
      }
    } catch (e) {
      toast.error(t('chat.exportFailed', '导出失败') + (e instanceof Error ? `：${e.message}` : ''))
    } finally {
      setExportingId(null)
    }
  }, [app, t])

  if (!mounted) return null

  const hasSelection = selectedIds.size > 0

  return (
    <div className="absolute inset-0 pointer-events-none">
      <div className="absolute inset-0 z-30 pointer-events-auto" onClick={onClose} />
      <div className={`absolute right-3 left-3 z-40 flex flex-col bg-background/95 border rounded-xl shadow-lg pointer-events-auto transition-all duration-200 ease-out overflow-hidden ${visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-2'}`}
        style={{ height: '40%', top: '4px' }}>
      <div className="flex items-center justify-between px-4 py-2 border-b shrink-0">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <History className="w-4 h-4 text-muted-foreground" />
            <span className="text-xs font-medium">{t('chat.historySessions')}</span>
          </div>
          {total > 0 && (
            <span className="text-[10px] text-muted-foreground">{t('chat.totalSessions', { count: total })}</span>
          )}
        </div>
        <button onClick={onClose} className="text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer px-1">
          ✕
        </button>
      </div>

      <div className="px-4 py-2 shrink-0 border-b border-border/30">
        <div className="flex items-center gap-2">
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder={t('chat.searchSessions')}
            className="flex-1 h-7 rounded-md border bg-muted/30 px-2.5 text-xs"
          />
          {/* 批量操作按钮 */}
          <button
            onClick={selectAll}
            className="flex items-center gap-1 text-[10px] text-muted-foreground hover:text-foreground transition-colors cursor-pointer whitespace-nowrap"
            title={t('selectAll')}
          >
            <CheckCheck className="w-3 h-3" /> {t('selectAll')}
          </button>
          <button
            onClick={invertSelect}
            className="flex items-center gap-1 text-[10px] text-muted-foreground hover:text-foreground transition-colors cursor-pointer whitespace-nowrap"
            title={t('chat.invertSelect', '反选')}
          >
            <ArrowUpDown className="w-3 h-3" /> {t('chat.invertSelect', '反选')}
          </button>
          {hasSelection && (
            <button
              onClick={deleteSelected}
              disabled={isDeleting}
              className="flex items-center gap-1 text-[10px] text-destructive hover:text-destructive/80 transition-colors cursor-pointer disabled:opacity-40 whitespace-nowrap"
            >
              {isDeleting
                ? <Loader2 className="w-3 h-3 animate-spin" />
                : <Trash2 className="w-3 h-3" />
              }
              {t('chat.delete', '删除')}({selectedIds.size})
            </button>
          )}
        </div>
      </div>

      {/* 会话列表 */}
      <div
        ref={listRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto overscroll-contain px-3 pb-2"
      >
        {sessions.length === 0 && isLoading ? (
          <div className="flex items-center justify-center h-full">
            <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
          </div>
        ) : sessions.length === 0 && submittedSearch ? (
          <div className="flex items-center justify-center h-full">
            <span className="text-xs text-muted-foreground">{t('chat.noMatchingSessions')}</span>
          </div>
        ) : (
          <div className="space-y-0.5">
            {sessions.map(s => {
              const isSelected = selectedIds.has(s.session_id)
              return (
              <div key={s.session_id} className="group flex items-center">
                {/* 复选框 */}
                <button
                  onClick={() => toggleSelect(s.session_id)}
                  className="shrink-0 p-1.5 mr-1 rounded hover:bg-muted/50 transition-colors cursor-pointer"
                >
                  {isSelected
                    ? <CheckSquare className="w-4 h-4 text-primary" />
                    : <Square className="w-4 h-4 text-muted-foreground/40 group-hover:text-muted-foreground" />
                  }
                </button>
                <button
                  onClick={() => { onSelectSession(s.session_id); onClose() }}
                  className={`w-full flex items-center gap-2.5 px-2.5 py-2.5 rounded-lg text-left transition-colors cursor-pointer select-none ${s.session_id === activeSessionId ? 'bg-muted/60' : 'hover:bg-muted/50'}`}
                >
                  <MessageSquare className={`w-4 h-4 shrink-0 ${s.session_id === activeSessionId ? 'text-primary' : 'text-muted-foreground'}`} />
                  <div className="min-w-0 flex-1">
                    <div className={`text-xs truncate ${isSelected ? 'text-primary' : s.session_id === activeSessionId ? 'text-primary font-medium' : ''}`}>{s.title || t('chat.newChat')}</div>
                    <div className="text-[10px] text-muted-foreground mt-0.5">{timeAgo(s.updated_at)}</div>
                  </div>
                </button>
                {/* 导出会话为 Markdown */}
                <button
                  onClick={e => { e.stopPropagation(); handleExport(s.session_id) }}
                  disabled={exportingId === s.session_id}
                  className="shrink-0 p-1.5 mr-1 rounded text-muted-foreground/40 hover:text-foreground hover:bg-muted/50 transition-colors cursor-pointer disabled:opacity-40"
                  title={t('chat.exportSession', '导出会话（Markdown）')}
                >
                  {exportingId === s.session_id
                    ? <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    : <Download className="w-3.5 h-3.5" />
                  }
                </button>
              </div>
            )})}
            {isLoading && (
              <div className="flex justify-center py-3">
                <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
              </div>
            )}
            {!hasMore && sessions.length > 0 && (
              <div className="text-center text-[10px] text-muted-foreground py-2">{t('chat.allSessionsShown')}</div>
            )}
          </div>
        )}
      </div>
    </div>
    </div>
  )
}
