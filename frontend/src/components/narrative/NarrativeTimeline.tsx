import { useState, useCallback, useEffect, useRef, useMemo, memo } from 'react'
import ReactMarkdown from 'react-markdown'
import { EventsOn } from '@/lib/wailsjs/runtime/runtime'
import { useApp } from '@/hooks/useApp'
import { useOutlineCache } from './useOutlineCache'
import DetailTabs from './DetailTabs'
import type { app } from '@/lib/wailsjs/go/models'

interface Props {
  activeChapterNum: number
  novelId: number
  width: number
  onWidthChange: (w: number) => void
  chatPanelWidth: number
}

interface CardPos { id: string; x: number; y: number; w: number; h: number; z?: number }

const ALL_CARDS = ['current', 'past', 'future', 'arcs', 'foreshadow', 'reader', 'detailtabs'] as const
type CardId = typeof ALL_CARDS[number]

const CARD_LABELS: Record<CardId, string> = {
  current: '当前', past: '过去', future: '未来', arcs: '弧线', foreshadow: '伏笔', reader: '读者', detailtabs: '详细设定',
}

function loadLabels(): Record<string, string> {
  try { const s = localStorage.getItem('narrative_card_labels'); return s ? JSON.parse(s) : {} } catch { return {} }
}
function saveLabels(l: Record<string, string>) { localStorage.setItem('narrative_card_labels', JSON.stringify(l)) }

const LAYOUT_KEY = 'narrative_canvas_layout'
const DEFAULT_LAYOUT: CardPos[] = [
  { id: 'current', x: 8, y: 8, w: 260, h: 280, z: 1 },
  { id: 'past', x: 278, y: 8, w: 260, h: 280, z: 2 },
  { id: 'future', x: 548, y: 8, w: 210, h: 280, z: 3 },
  { id: 'arcs', x: 8, y: 298, w: 200, h: 140, z: 4 },
  { id: 'foreshadow', x: 218, y: 298, w: 340, h: 140, z: 5 },
  { id: 'reader', x: 568, y: 298, w: 190, h: 140, z: 6 },
  { id: 'detailtabs', x: 8, y: 448, w: 750, h: 130, z: 7 },
]

const SNAP = 8
const MIN_W = 120
const MIN_H = 60
const IMP: Record<number, string> = { 5: '★★★★★ 必收', 4: '★★★★ 重要', 3: '★★★ 一般', 2: '★★', 1: '★' }

function loadLayout(): CardPos[] {
  try { const s = localStorage.getItem(LAYOUT_KEY); return s ? JSON.parse(s) : DEFAULT_LAYOUT } catch { return DEFAULT_LAYOUT }
}
function saveLayout(l: CardPos[]) { localStorage.setItem(LAYOUT_KEY, JSON.stringify(l)) }

function snapTo(v: number, targets: number[], t = SNAP): number {
  for (const x of targets) if (Math.abs(v - x) < t) return x
  return v
}

export default memo(function NarrativeTimeline({ activeChapterNum, novelId, width, onWidthChange, chatPanelWidth }: Props) {
  const app = useApp()
  const { loadOutlines, invalidateCache } = useOutlineCache()
  const [ctx, setCtx] = useState<app.WritingContext | null>(null)
const [rawOutlines, setRawOutlines] = useState<Record<number, string>>({})
  const [layout, setLayout] = useState<CardPos[]>(loadLayout)
  const [showAddMenu, setShowAddMenu] = useState(false)
  const [showToolbar, setShowToolbar] = useState(false)
  const [labels, setLabels] = useState<Record<string, string>>(loadLabels)
  const [renaming, setRenaming] = useState<string | null>(null)
  const [chapters, setChapters] = useState<Array<{ chapter_number: number; title: string; word_count: number }>>([])
  // 当前卡视图：auto=跟随后端 preview 字段（待写章时自动预览）；手动点按钮可锁定到另一种
  const [currentMode, setCurrentMode] = useState<'auto' | 'preview' | 'review'>('auto')
  const renameRef = useRef<HTMLInputElement>(null)
  const toolbarTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const maxZRef = useRef(10)

  const updateCard = useCallback((id: string, patch: Partial<CardPos>) => {
    setLayout(prev => { const next = prev.map(c => c.id === id ? { ...c, ...patch } : c); saveLayout(next); return next })
  }, [])

  const bringToFront = useCallback((id: string) => {
    maxZRef.current++
    updateCard(id, { z: maxZRef.current })
  }, [updateCard])

  const onContentMouseMove = useCallback((e: React.MouseEvent) => {
    const r = e.currentTarget.getBoundingClientRect()
    if (e.clientX - r.right > -60 && e.clientY - r.top < 60) {
      setShowToolbar(true)
      if (toolbarTimer.current) clearTimeout(toolbarTimer.current)
      toolbarTimer.current = setTimeout(() => setShowToolbar(false), 10000)
    }
  }, [])

  const startDrag = useCallback((id: string, e: React.MouseEvent) => {
    bringToFront(id)
    const card = layout.find(c => c.id === id)
    if (!card) return
    const sx = e.clientX, sy = e.clientY, ox = card.x, oy = card.y
    const edges = [0]
    for (const c of layout) if (c.id !== id) edges.push(c.x, c.x + c.w, c.y, c.y + c.h)
    const mv = (ev: MouseEvent) => {
      let nx = Math.max(0, ox + ev.clientX - sx)
      let ny = Math.max(0, oy + ev.clientY - sy)
      nx = snapTo(snapTo(nx, edges), edges.map(l => l - card.w))
      ny = snapTo(snapTo(ny, edges), edges.map(l => l - card.h))
      updateCard(id, { x: nx, y: ny })
    }
    const up = () => { document.removeEventListener('mousemove', mv); document.removeEventListener('mouseup', up) }
    document.addEventListener('mousemove', mv)
    document.addEventListener('mouseup', up)
  }, [layout, updateCard, bringToFront])

  const startResize = useCallback((id: string, edge: string, e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    bringToFront(id)
    const card = layout.find(c => c.id === id)
    if (!card) return
    const sx = e.clientX, sy = e.clientY
    const edges = [0]
    for (const c of layout) if (c.id !== id) edges.push(c.x, c.x + c.w, c.y, c.y + c.h)
    const mv = (ev: MouseEvent) => {
      const dx = ev.clientX - sx, dy = ev.clientY - sy
      let { x, y, w, h } = card
      if (edge.includes('e')) { w = snapTo(Math.max(MIN_W, card.w + dx), edges.map(l => l - card.x)) }
      if (edge.includes('w')) { const nw = Math.max(MIN_W, card.w - dx); x = card.x + card.w - nw; w = snapTo(nw, edges) }
      if (edge.includes('s')) { h = snapTo(Math.max(MIN_H, card.h + dy), edges.map(l => l - card.y)) }
      if (edge.includes('n')) { const nh = Math.max(MIN_H, card.h - dy); y = card.y + card.h - nh; h = snapTo(nh, edges) }
      updateCard(id, { x: Math.max(0, x), y: Math.max(0, y), w: Math.max(MIN_W, w), h: Math.max(MIN_H, h) })
    }
    const up = () => { document.removeEventListener('mousemove', mv); document.removeEventListener('mouseup', up) }
    document.addEventListener('mousemove', mv)
    document.addEventListener('mouseup', up)
  }, [layout, updateCard, bringToFront])

  const removeCard = useCallback((id: string) => {
    setLayout(prev => { const next = prev.filter(c => c.id !== id); saveLayout(next); return next })
  }, [])

  const addCard = useCallback((id: CardId) => {
    setLayout(prev => {
      if (prev.find(c => c.id === id)) return prev
      maxZRef.current++
      const next = [...prev, { id, x: 30 + Math.random() * 60, y: 30 + Math.random() * 60, w: 240, h: 180, z: maxZRef.current }]
      saveLayout(next)
      return next
    })
    setShowAddMenu(false)
  }, [])

  const getLabel = useCallback((id: string) => labels[id] || CARD_LABELS[id as CardId] || id, [labels])

  const startRename = useCallback((id: string) => {
    setRenaming(id)
    setTimeout(() => renameRef.current?.focus(), 50)
  }, [])

  const finishRename = useCallback((id: string, val: string) => {
    if (val.trim()) {
      const next = { ...labels, [id]: val.trim() }
      setLabels(next)
      saveLabels(next)
    }
    setRenaming(null)
  }, [labels])

  const latestRef = useRef({ novelId, activeChapterNum })
  latestRef.current = { novelId, activeChapterNum }
  const snapshotRef = useRef<number>(0)
  if ((ctx?.writing_snapshot as any)?.last_chapter_num) snapshotRef.current = (ctx!.writing_snapshot as any).last_chapter_num

  const loadContext = useCallback(async (ch: number) => {
    if (!novelId) return
    try {
      const w = await app.GetWritingContext(novelId, ch)
      setCtx(w)
      const chapters = [-1, 0, 1].map(i => ch + i).filter(n => n > 0)
      const results = await Promise.all(chapters.map(n => loadOutlines(novelId, n)))
      const raw: Record<number, string> = {}
      results.forEach((content, i) => {
        const cn = chapters[i]
        if (content) raw[cn] = content
      })
      setRawOutlines(raw)
      // 章节状态：已写（有正文）/ 待写（仅大纲或空）
      try {
        const chs = await app.GetChapters(novelId)
        setChapters((chs ?? []).map((c: any) => ({ chapter_number: c.chapter_number, title: c.title, word_count: c.word_count })))
      } catch { /* 章节状态加载失败不影响主面板 */ }
    } catch (e) { console.error('[NarrativeTimeline]', e) }
  }, [novelId, app, loadOutlines])

  useEffect(() => { if (novelId > 0) loadContext(activeChapterNum || 0) }, [activeChapterNum, novelId, loadContext])

  useEffect(() => {
    if (ctx?.writing_snapshot && activeChapterNum === 0 && (ctx.writing_snapshot as any).last_chapter_num > 0 && ctx.chapter.num === 0) {
      loadContext((ctx.writing_snapshot as any).last_chapter_num)
    }
  }, [ctx, activeChapterNum, loadContext])

  useEffect(() => {
    if (!novelId) return
    let timer: ReturnType<typeof setTimeout> | null = null
    const refresh = (nid?: number) => {
      if (nid && nid !== novelId) return
      if (timer) clearTimeout(timer)
      timer = setTimeout(async () => {
        timer = null
        invalidateCache()
        // 优先用最新章节号拉上下文：写完新章后 current/past 卡片必须显示最新章，
        // 否则会一直停留在用户选中的旧章锚点（recent 以锚点为基准往前取）
        let ch = latestRef.current.activeChapterNum || 0
        try {
          const chs = await app.GetChapters(novelId)
          const latest = Math.max(0, ...(chs ?? []).map((c: any) => c.chapter_number ?? 0))
          if (latest > 0) ch = Math.max(ch, latest)
        } catch { /* 保持原值 */ }
        loadContext(ch)
      }, 300)
    }
    const subs = [
      EventsOn('file:changed', (d: any) => refresh(d?.novel_id)),
      EventsOn('chat:api_done', () => refresh()),
      EventsOn('chat:session_created', () => refresh()),
    ]
    return () => { if (timer) clearTimeout(timer); subs.forEach(s => s?.()) }
  }, [novelId, loadContext, invalidateCache, app])

  const snap = ctx?.writing_snapshot as any
  const effectiveChapter = snap?.last_chapter_num && activeChapterNum <= snap.last_chapter_num ? snap.last_chapter_num : activeChapterNum
  const currentChapter = ctx?.chapter?.num ? ctx.chapter : { num: effectiveChapter, title: '', word_count: 0 }

  // 当前章出场角色：优先 maintain 强制写入的 characters_in（事实层），
  // 回退到快照 active_chars（状态层）。两者都空则显示全部角色。
  const currentChapterBrief = (ctx?.recent_chapters ?? []).find((c: any) => c.num === currentChapter.num)
  const charsInIds = new Set<number>()
  try {
    if (currentChapterBrief?.characters_in) {
      JSON.parse(currentChapterBrief.characters_in).forEach((id: number) => charsInIds.add(Number(id)))
    }
  } catch { /* */ }
  const snapshotCharIds = new Set<number>()
  try {
    if ((ctx?.writing_snapshot as any)?.active_chars) {
      JSON.parse((ctx!.writing_snapshot as any).active_chars).forEach((id: number) => snapshotCharIds.add(Number(id)))
    }
  } catch { /* */ }
  const activeCharIds = charsInIds.size > 0 ? charsInIds : snapshotCharIds
  const activeChars = (ctx?.characters ?? []).filter((c: any) => activeCharIds.size === 0 || activeCharIds.has(c.id))
  const pendingByChapter: Record<number, any[]> = {}
  const untimedPending: any[] = []
  for (const p of ctx?.timeline.pending ?? []) {
    const k = (p as any).target_chapter || 0
    if (k <= 0) { untimedPending.push(p); continue } // 未定时伏笔单独分组显示
    if (!pendingByChapter[k]) pendingByChapter[k] = []
    pendingByChapter[k].push(p)
  }
  // 过去卡只显示已完成的章（num < current），当前章归"当前卡"，避免三卡视觉重复
  const pastChapters = (ctx?.recent_chapters ?? []).filter(c => c.summary && c.num < currentChapter.num).slice(0, 3)

  // summary 由 maintain 生成时常带 "第N章," 前缀（标题已单独显示），显示层去掉避免重复
  const cleanSummary = (s: string) => (s || '').replace(/^第\s*\d+\s*章[,，:：]\s*/, '')
  const hasCard = (id: string) => layout.some(c => c.id === id)

  // 章节状态维护：已写（word_count>0）/ 待写（有章记录但无正文）/ 未开始（无记录）
  const chapterStatus = useMemo(() => {
    const maxNum = Math.max(currentChapter.num, ...chapters.map(c => c.chapter_number))
    const written = new Map<number, number>() // chapter_number -> word_count
    for (const c of chapters) written.set(c.chapter_number, c.word_count)
    const list: Array<{ num: number; status: 'done' | 'drafting' | 'todo'; wordCount: number }> = []
    for (let n = 1; n <= Math.max(maxNum, 1); n++) {
      const wc = written.get(n)
      if (wc !== undefined && wc > 0) list.push({ num: n, status: 'done', wordCount: wc })
      else if (wc !== undefined) list.push({ num: n, status: 'drafting', wordCount: 0 })
      else list.push({ num: n, status: 'todo', wordCount: 0 })
    }
    return list
  }, [chapters, currentChapter.num])

  return (
    <div className="narrative-panel" style={{ width, minWidth: 240 }}>
      <div className="narrative-resize-handle" onMouseDown={e => {
        e.preventDefault(); e.stopPropagation()
        const sx = e.clientX, sw = width
        const snapTarget = innerWidth - chatPanelWidth
        const mv = (ev: MouseEvent) => {
          const w = Math.max(240, sw + (ev.clientX - sx))
          onWidthChange(Math.abs(w - snapTarget) < SNAP ? snapTarget : w)
        }
        const up = () => { document.removeEventListener('mousemove', mv); document.removeEventListener('mouseup', up); document.body.style.cursor = ''; document.body.style.userSelect = '' }
        document.addEventListener('mousemove', mv); document.addEventListener('mouseup', up)
        document.body.style.cursor = 'col-resize'; document.body.style.userSelect = 'none'
      }} />
      <div className="narrative-content" onMouseMove={onContentMouseMove}>
        {showToolbar && <div className="canvas-toolbar"><button onClick={() => setShowAddMenu(!showAddMenu)} className="canvas-btn-add" title="添加/隐藏卡片">{showAddMenu ? '✕' : '+'}</button></div>}
        {showAddMenu && <div className="canvas-add-dropdown">{ALL_CARDS.map(id => { const exists = hasCard(id); return <div key={id} onClick={() => { if (exists) removeCard(id); else addCard(id) }} className={`canvas-add-item${exists ? ' checked' : ''}`}><span className="canvas-add-check">{exists ? '✓' : ''}</span>{CARD_LABELS[id]}</div> })}</div>}

        {layout.map(card => (
          <div key={card.id} className="narrative-card" data-card={card.id} style={{ left: card.x, top: card.y, width: card.w, height: card.h, zIndex: card.z ?? 1 }}>
            <div className="resize-edge edge-n" onMouseDown={e => startResize(card.id, 'n', e)} />
            <div className="resize-edge edge-s" onMouseDown={e => startResize(card.id, 's', e)} />
            <div className="resize-edge edge-w" onMouseDown={e => startResize(card.id, 'w', e)} />
            <div className="resize-edge edge-e" onMouseDown={e => startResize(card.id, 'e', e)} />
            <div className="resize-corner corner-nw" onMouseDown={e => startResize(card.id, 'nw', e)} />
            <div className="resize-corner corner-ne" onMouseDown={e => startResize(card.id, 'ne', e)} />
            <div className="resize-corner corner-sw" onMouseDown={e => startResize(card.id, 'sw', e)} />
            <div className="resize-corner corner-se" onMouseDown={e => startResize(card.id, 'se', e)} />
            <div className="narrative-card-header" onMouseDown={e => startDrag(card.id, e)} onDoubleClick={() => startRename(card.id)}>
              <span className="narrative-card-arrow">▸</span>
              {renaming === card.id ? (
                <input ref={renameRef} className="card-rename-input" defaultValue={getLabel(card.id)} onBlur={e => finishRename(card.id, e.target.value)} onKeyDown={e => { if (e.key === 'Enter') finishRename(card.id, (e.target as HTMLInputElement).value); if (e.key === 'Escape') setRenaming(null) }} onClick={e => e.stopPropagation()} autoFocus />
              ) : (
                <span className="narrative-card-title" title="双击重命名">{getLabel(card.id)}</span>
              )}
              <button className="narrative-card-delete" onClick={e => { e.stopPropagation(); removeCard(card.id) }}>✕</button>
            </div>
            <div className="narrative-card-body">
              {card.id === 'current' && <>
                {(() => {
                  const preview = (ctx as any)?.preview
                  const showPreview = currentMode === 'preview' || (currentMode === 'auto' && !!preview)
                  // 切换按钮：点击锁定到另一种模式（标题行右侧小链接）
                  const toggleBtn = (
                    <button
                      onClick={() => setCurrentMode(showPreview ? 'review' : 'preview')}
                      className="shrink-0 text-[10px] text-muted-foreground/70 hover:text-primary transition-colors cursor-pointer ml-2"
                      title="点击切换当前卡视图（写前预览 / 已完成章回顾）"
                    >
                      {showPreview ? '上章回顾 ▸' : '写前预览 ▸'}
                    </button>
                  )
                  if (showPreview) {
                    // ── 写前预览模式：待写章的设定摘要（状态层 + 规划层）──
                    const dueForeshadow: any[] = preview.due_foreshadow ?? []
                    const overdueForeshadow: any[] = preview.overdue_foreshadow ?? []
                    const dueNodes: any[] = preview.due_arc_nodes ?? []
                    const charNames = (preview.prev_chars ?? []).map((id: number) => (ctx?.characters as any[]).find((c: any) => c.id === id)?.name).filter(Boolean)
                    return (
                      <>
                        <div className="card-sec">
                          <div className="card-current-title card-current-title-row">
                            第{preview.chapter_num}章（待写）
                            {toggleBtn}
                          </div>
                        </div>
                        {(dueForeshadow.length > 0 || dueNodes.length > 0) && (
                          <div className="card-sec"><div className="card-sec-title">🎯 本章目标</div>
                            {dueForeshadow.map((e: any) => <div key={e.id} className="card-item"><span className="card-item-name">{e.title}</span><span className="card-item-tag" title={IMP[e.importance]}>伏笔·到期</span></div>)}
                            {dueNodes.map((n: any) => <div key={n.id} className="card-item"><span className="card-item-name">{n.title}</span><span className="card-item-tag">弧线节点</span>{n.description && <div className="card-item-desc">{n.description}</div>}</div>)}
                          </div>
                        )}
                        {overdueForeshadow.length > 0 && (
                          <div className="card-sec"><div className="card-sec-title">⚠️ 超期伏笔</div>
                            {overdueForeshadow.map((e: any) => <div key={e.id} className="card-item card-item-overdue"><span className="card-item-name">{e.title}</span><span className="card-item-tag">超{e.overdue_by}章</span></div>)}
                          </div>
                        )}
                        <div className="card-sec"><div className="card-sec-title">📍 状态延续</div>
                          {preview.prev_location && <div className="card-item">地点：{preview.prev_location}</div>}
                          {charNames.length > 0 && <div className="card-item">👤 延续角色：{charNames.join('、')}</div>}
                          {(preview.prev_items ?? []).length > 0 && <div className="card-item">📦 在途物品：{preview.prev_items.join('、')}</div>}
                          {preview.recent_suspense > 0 && <div className="card-item">❓ 遗留悬念：{preview.recent_suspense} 条</div>}
                        </div>
                        {preview.has_outline && (
                          <div className="card-sec"><div className="card-sec-title">📝 大纲</div><div className="card-val">outlines/{String(preview.chapter_num).padStart(3, '0')}.md 已生成，可在右侧编辑器查看</div></div>
                        )}
                      </>
                    )
                  }
                  // ── 回顾模式：已完成章的状态（现状）──
                  return (
                    <>
                      {currentChapter.num > 0 && (
                        <div className="card-sec">
                          <div className="card-current-title card-current-title-row">
                            第{currentChapter.num}章{currentChapter.title ? ` ${currentChapter.title}` : ''}
                            {preview && toggleBtn}
                          </div>
                        </div>
                      )}
                      {snap?.current_location && <div className="card-sec"><div className="card-sec-title">📍 地点</div><div className="card-val">{snap.current_location}</div></div>}
                      {currentChapter.num > 0 && <div className="card-sec"><div className="card-sec-title">📝 字数</div><div className="card-val">{currentChapter.word_count || 0} 字</div></div>}
                      {(ctx?.recent_chapters as any[] | undefined)?.find((c: any) => c.num === currentChapter.num)?.summary && <div className="card-sec"><div className="card-sec-title">📝 内容摘要</div><div className="card-val">{cleanSummary((ctx?.recent_chapters as any[] | undefined)?.find((c: any) => c.num === currentChapter.num)?.summary)}</div></div>}
                      {activeChars.length > 0 && <div className="card-sec"><div className="card-sec-title">👤 本章出场 ({activeChars.length})</div>{activeChars.map((c: any) => <div key={c.id} className="card-item"><div className="card-item-name">{c.name}</div>{c.desc && <div className="card-item-desc">{c.desc}</div>}{Array.isArray(c.items) && c.items.length > 0 && <div className="card-item-tags" style={{ marginTop: 3 }}><span className="card-item-tag card-item-tag-hold">持有 {c.items.join('、')}</span></div>}</div>)}</div>}
                      {((ctx as any)?.item_occurrences ?? []).length > 0 && <div className="card-sec"><div className="card-sec-title">📦 本章物品流转</div>{((ctx as any).item_occurrences as any[]).map((o: any, i: number) => <div key={i} className="card-item"><span className="card-item-name">{o.item_name || `#${o.item_id}`}</span><span className="card-item-tag">{o.action}</span>{o.description && <div className="card-item-desc">{o.description}</div>}</div>)}</div>}
                    </>
                  )
                })()}
              </>}
              {card.id === 'past' && <>
                <div className="card-sec"><div className="card-sec-title">📑 章节进度</div><div className="card-item-tags" style={{ flexWrap: 'wrap' }}>{chapterStatus.slice(-12).map(s => <span key={s.num} className={`card-item-tag ch-status ch-status-${s.status}`} title={s.status === 'done' ? `第${s.num}章 已写 ${s.wordCount} 字` : s.status === 'drafting' ? `第${s.num}章 写作中` : `第${s.num}章 未开始`}>{s.num}{s.status === 'done' ? '✓' : s.status === 'drafting' ? '✍' : ''}</span>)}</div></div>
                {pastChapters.map(ch => {
                let events: string[] = []
                try { events = JSON.parse(ch.key_events || '[]') } catch { /* */ }
                events = events.filter(e => e?.length > 5).map(e => e.replace(/^[埋推种揭铺设]*[：:]\s*/, '')).slice(0, 3)
                return <div key={ch.num} className="card-item"><div className="card-item-name">第{ch.num}章「{ch.title}」{ch.word_cnt > 0 && <span className="card-item-tag">{ch.word_cnt}字</span>}</div>{ch.summary && <><div className="card-sec-title" style={{ marginTop: 4 }}>📖 剧情概要</div><div className="card-item-desc">{cleanSummary(ch.summary)}</div></>}{events.length > 0 && <><div className="card-sec-title" style={{ marginTop: 4 }}>📌 关键事件</div>{events.map((e, i) => <div key={i} className="card-item-events">• {e}</div>)}</>}</div>
              })}
              </>}
              {card.id === 'future' && <>
                {Object.keys(rawOutlines).map(Number).filter(n => n >= currentChapter.num).sort((a, b) => b - a).map(n => {
                  const raw = rawOutlines[n]
                  const fileTitle = raw?.split('\n')[0]?.replace(/^#\s+/, '')?.trim() || ''
                  // 大纲首行已含"第N章"前缀时不再重复加
                  const title = fileTitle.startsWith(`第${n}章`) ? fileTitle : (fileTitle ? `第${n}章 · ${fileTitle}` : `第${n}章`)
                  // markdown 内容剔除首行标题，避免与卡片标题重复渲染
                  const body = raw?.split('\n').slice(1).join('\n') ?? ''
                  return <div key={n} className="card-item" style={{ marginBottom: 8 }}>
                    <div className="card-item-name" style={{ marginBottom: 4, fontSize: '0.85rem' }}>{title}</div>
                    <div className="outline-markdown" style={{ fontSize: '0.78rem', lineHeight: 1.6, color: 'var(--muted-foreground)' }}>
                      <ReactMarkdown>{body}</ReactMarkdown>
                    </div>
                  </div>
                })}
                {Object.keys(rawOutlines).length === 0 && <div className="card-item" style={{ color: 'var(--muted-foreground)' }}>暂无章纲</div>}
              </>}
              {card.id === 'arcs' && (ctx?.active_arcs ?? []).map((a: any) => { const p = a.nodes_total > 0 ? Math.round(a.nodes_done / a.nodes_total * 100) : 0; return <div key={a.id}><div className="card-item"><span className="card-item-name">{a.name}</span><span className="card-item-tag">{a.type_zh}</span><span className="card-item-tag">{a.nodes_done}/{a.nodes_total}</span><div className="arc-progress-bar-container"><div className={`arc-progress-bar ${p >= 75 ? 'high' : p >= 40 ? 'medium' : 'low'}`} style={{ width: `${p}%` }} /></div></div>{(a.nodes || []).map((n: any) => { const done = n.status === 'completed'; const overdue = !done && (n.target_chapter > 0 && n.target_chapter <= currentChapter.num); const isCurrent = overdue && !(a.nodes || []).some((nn: any) => nn.status !== 'completed' && nn.target_chapter > n.target_chapter && nn.target_chapter <= currentChapter.num); return <div key={n.id} className="card-item" style={{ marginLeft: '0.5rem', borderLeft: isCurrent ? '3px solid var(--primary)' : '2px solid var(--primary)', padding: '0.25rem 0.4rem', background: isCurrent ? 'color-mix(in oklab, var(--primary) 8%, transparent)' : undefined }}><div className="card-item-name">{n.title}{isCurrent && ' ← 当前'}</div>{n.description && <div className="card-item-desc">{n.description}</div>}<div className="card-item-tags"><span className="card-item-tag">{done ? '✅' : '⏳'}</span>{n.target_chapter > 0 && <span className="card-item-tag">目标第{n.target_chapter}章</span>}{n.actual_chapter > 0 && <span className="card-item-tag">实际第{n.actual_chapter}章</span>}</div></div>})}</div> })}
              {card.id === 'foreshadow' && <>
                {(ctx?.timeline.overdue ?? []).map((e: any) => <div key={e.id} className="card-item card-item-overdue"><div className="card-item-name">⚠️ 第{e.target_chapter}章 · {e.title}（已超{e.overdue_by}章）</div></div>)}
                {Object.entries(pendingByChapter).sort(([a], [b]) => +a - +b).map(([c, es]) => <div key={c} className="card-sec"><div className="card-sec-title">⏳ 第{c}章 · 待回收 {es.length} 条</div>{es.map((e: any) => <div key={e.id} className="card-item"><span className="card-item-name">{e.title}</span><span className="card-item-tag">{IMP[e.importance] || `${e.importance}★`}</span></div>)}</div>)}
                {untimedPending.length > 0 && <div className="card-sec"><div className="card-sec-title">⏳ 未定时 · 待回收 {untimedPending.length} 条</div>{untimedPending.map((e: any) => <div key={e.id} className="card-item"><span className="card-item-name">{e.title}</span><span className="card-item-tag">{IMP[e.importance] || `${e.importance}★`}</span></div>)}</div>}
                {(ctx?.timeline.resolved ?? []).length > 0 && <div className="card-sec"><div className="card-sec-title">✅ 已回收</div>{(ctx?.timeline.resolved ?? []).slice(0, 5).map((e: any) => <div key={e.id} className="card-item card-item-resolved"><span>✅ {e.title}</span></div>)}</div>}
              </>}
              {card.id === 'reader' && ctx?.reader && <>
                <div className="card-sec"><div className="card-sec-title">📊 读者认知状态</div><div className="card-item"><span className="card-item-name">👁 已知 {(ctx as any).reader.known}</span><span className="card-item-name">❓ 悬念 {(ctx as any).reader.suspense}</span><span className="card-item-name">❌ 误知 {(ctx as any).reader.misconception}</span></div></div>
                {(ctx as any).reader.entries?.length > 0 && <div className="card-sec"><div className="card-sec-title">📋 详情</div>{(ctx as any).reader.entries.slice(0, 6).map((e: any) => { const m: Record<string, string> = { known: '👁 已知', suspense: '❓ 悬念', misconception: '❌ 误知' }; const icon = m[e.type] || e.type; return <div key={e.id} className="card-item"><span className="card-item-tag">{icon}</span><div className="card-item-content"><div className="card-item-desc">{e.content}</div><div className="card-item-tags"><span className="card-item-tag">第{e.planted_chapter}章种下</span>{e.revealed_chapter > 0 ? <span className="card-item-tag">第{e.revealed_chapter}章揭示</span> : <span className="card-item-tag">未揭示</span>}</div></div></div> })}</div>}
              </>}
              {card.id === 'detailtabs' && <DetailTabs novelId={novelId} chapterNum={activeChapterNum} />}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
})
