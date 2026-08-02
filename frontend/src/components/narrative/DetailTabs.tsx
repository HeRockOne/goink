import { useState, useEffect, memo, useRef } from 'react'
import { useApp } from '@/hooks/useApp'
import { GetSceneListByNovel } from '@/lib/wailsjs/go/app/App'
import { EventsOn } from '@/lib/wailsjs/runtime/runtime'

type TabId = 'characters' | 'locations' | 'items' | 'lore' | 'scenes'

const TABS: Array<{ id: TabId; label: string; icon: string }> = [
  { id: 'characters', label: '角色', icon: '👤' },
  { id: 'locations', label: '地点', icon: '📍' },
  { id: 'items', label: '物品', icon: '📦' },
  { id: 'lore', label: '世界观', icon: '🌍' },
  { id: 'scenes', label: '场景', icon: '🎬' },
]

interface Props {
  novelId: number
  chapterNum: number
}

export default memo(function DetailTabs({ novelId, chapterNum }: Props) {
  const app = useApp()
  const [activeTab, setActiveTab] = useState<TabId>('characters')
  const [data, setData] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [chMap, setChMap] = useState<Record<number, number>>({})

  useEffect(() => {
    if (!novelId) return
    let cancelled = false
    setLoading(true)
    setData(null)

    const load = async () => {
      try {
        let result: any = null
        switch (activeTab) {
          case 'characters': result = await app.GetCharacters(novelId); break
          case 'locations': result = await app.GetLocations(novelId); break
          case 'items': result = await app.GetItemList(novelId, '', '', '', 1, 20); break
          case 'lore': result = await app.GetLoreList(novelId, '', '', 1, 20); break
          case 'scenes': {
            result = await GetSceneListByNovel(novelId)
            // 加载章节映射 chapter_id → chapter_number
            try {
              const chapters = await app.GetChapters(novelId)
              if (Array.isArray(chapters)) {
                const map: Record<number, number> = {}
                for (const ch of chapters) { map[ch.id] = ch.chapter_number }
                if (!cancelled) setChMap(map)
              }
            } catch { /* */ }
            break
          }
        }
        if (!cancelled && result) setData(result)
      } catch (e) {
        console.warn(`[DetailTabs] load ${activeTab} failed:`, e)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [activeTab, novelId, chapterNum, app])

  // 事件监听：对话完成或文件变更时自动刷新当前 Tab
  const refreshRef = useRef(0)
  useEffect(() => {
    if (!novelId) return
    const refresh = () => {
      const id = ++refreshRef.current
      setTimeout(() => {
        if (id !== refreshRef.current) return // 防抖，只执行最后一次
        setLoading(true)
        const load = async () => {
          try {
            let result: any = null
            switch (activeTab) {
              case 'characters': result = await app.GetCharacters(novelId); break
              case 'locations': result = await app.GetLocations(novelId); break
              case 'items': result = await app.GetItemList(novelId, '', '', '', 1, 20); break
              case 'lore': result = await app.GetLoreList(novelId, '', '', 1, 20); break
              case 'scenes': {
                result = await GetSceneListByNovel(novelId)
                try {
                  const chapters = await app.GetChapters(novelId)
                  if (Array.isArray(chapters)) {
                    const map: Record<number, number> = {}
                    for (const ch of chapters) { map[ch.id] = ch.chapter_number }
                    setChMap(map)
                  }
                } catch { /* */ }
                break
              }
            }
            if (result) setData(result)
          } catch { /* */ }
          setLoading(false)
        }
        load()
      }, 300)
    }
    const subs = [
      EventsOn('chat:api_done', refresh),
      EventsOn('chat:session_created', refresh),
    ]
    return () => { subs.forEach(s => s?.()) }
  }, [novelId, activeTab, app])

  return (
    <div className="detail-tabs">
      <div className="detail-tabs-nav">
        {TABS.map(tab => (
          <button key={tab.id} className={`detail-tab-btn${activeTab === tab.id ? ' active' : ''}`} onClick={() => setActiveTab(tab.id)}>
            {tab.icon} {tab.label}
          </button>
        ))}
      </div>
      <div className="detail-tabs-content">
        {loading ? <div className="detail-empty">加载中…</div> : <TabContent tab={activeTab} data={data} chMap={chMap} />}
      </div>
    </div>
  )
})

function TabContent({ tab, data, chMap }: { tab: TabId; data: any; chMap?: Record<number, number> }) {
  if (!data) return <div className="detail-empty">暂无数据</div>

  switch (tab) {
    case 'characters': {
      const items = Array.isArray(data) ? data : []
      return items.length ? (
        <div>{items.slice(0, 10).map((c: any) => (
          <div key={c.id} className="detail-list-item"><span>👤</span><div><div className="detail-list-name">{c.name}</div>{c.location?.name && <div className="detail-list-desc">📍 {c.location.name}</div>}</div></div>
        ))}</div>
      ) : <EmptyState />
    }
    case 'locations': {
      const items = Array.isArray(data) ? data : []
      return items.length ? (
        <div>{items.slice(0, 10).map((l: any) => (
          <div key={l.id} className="detail-list-item"><span>📍</span><div><div className="detail-list-name">{l.name}</div>{l.location_type && <div className="detail-list-desc">{l.location_type}</div>}</div></div>
        ))}</div>
      ) : <EmptyState />
    }
    case 'items': {
      const items = data?.items ?? (Array.isArray(data) ? data : [])
      return items.length ? (
        <div>{items.slice(0, 10).map((it: any) => (
          <div key={it.id} className="detail-list-item"><span>📦</span><div><div className="detail-list-name">{it.name}</div>{it.narrative_role && <div className="detail-list-desc">{it.narrative_role}</div>}</div></div>
        ))}</div>
      ) : <EmptyState />
    }
    case 'lore': {
      const items = data?.items ?? (Array.isArray(data) ? data : [])
      return items.length ? (
        <div>{items.slice(0, 8).map((l: any) => (
          <div key={l.id} className="detail-list-item"><span>🌍</span><div><div className="detail-list-name">{l.title}</div>{l.category && <div className="detail-list-desc">{l.category}</div>}</div></div>
        ))}</div>
      ) : <EmptyState />
    }
    case 'scenes': {
      const items = Array.isArray(data) ? data : []
      // 按 chapter_id 分组
      const byChapter: Record<number, any[]> = {}
      for (const s of items) {
        const cid = s.chapter_id || 0
        if (!byChapter[cid]) byChapter[cid] = []
        byChapter[cid].push(s)
      }
      const chapterIds = Object.keys(byChapter).map(Number).sort((a, b) => b - a)
      return chapterIds.length ? (
        <div>{chapterIds.map(cid => (
          <SceneChapterGroup key={cid} chapterNum={chMap?.[cid] || cid} scenes={byChapter[cid]} />
        ))}</div>
      ) : <EmptyState />
    }
    default: return <EmptyState />
  }
}

function EmptyState() {
  return <div className="detail-empty">暂无数据</div>
}

function SceneChapterGroup({ chapterNum, scenes }: { chapterNum: number; scenes: any[] }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="detail-list-group">
      <div className="detail-list-group-header" onClick={() => setOpen(!open)}>
        <span className={`detail-list-arrow ${open ? 'open' : ''}`}>▸</span>
        <span className="detail-list-group-title">第 {chapterNum} 章</span>
        <span className="detail-list-group-count">{scenes.length} 场景</span>
      </div>
      {open && scenes.map((s: any) => (
        <div key={s.id} className="detail-list-item" style={{ paddingLeft: '1.5rem' }}>
          <span>🎬</span>
          <div>
            <div className="detail-list-name">{s.title}</div>
            {s.summary && <div className="detail-list-desc">{s.summary}</div>}
          </div>
        </div>
      ))}
    </div>
  )
}