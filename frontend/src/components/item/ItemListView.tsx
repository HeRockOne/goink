import { useState, useEffect, useCallback } from 'react'
import { Search, Pencil, Trash2, Sword } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useApp } from '@/hooks/useApp'
import type { item } from '@/hooks/useApp'

interface Props { novelId: number }

const ITEM_TYPES = ['法宝', '丹药', '灵药', '功法', '地图', '信物', '武器', '防具', '普通物品']

export default function ItemListView({ novelId }: Props) {
  const app = useApp()
  const { t } = useTranslation()
  const [items, setItems] = useState<item.Item[]>([])
  const [loading, setLoading] = useState(false)
  const [typeFilter, setTypeFilter] = useState('')
  const [search, setSearch] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [editing, setEditing] = useState<item.Item | null>(null)
  const [name, setName] = useState('')
  const [itemType, setItemType] = useState('')
  const [grade, setGrade] = useState('')
  const [desc, setDesc] = useState('')
  const [lore, setLore] = useState('')
  const [ability, setAbility] = useState('')
  const [ownerId, setOwnerId] = useState<number | undefined>(undefined)
  const [itemArcId, setItemArcId] = useState<number | undefined>(undefined)
  const [narrativeRole, setNarrativeRole] = useState('')
  const [detail, setDetail] = useState<item.Item | null>(null)

  const load = useCallback(async () => {
    if (!novelId) return
    setLoading(true)
    try {
      const r = await app.GetItemList(novelId, typeFilter, '', search, 1, 999)
      setItems(r?.items ?? [])
    } catch { /* ignore */ }
    setLoading(false)
  }, [app, novelId, typeFilter, search])

  useEffect(() => { load() }, [load])

  function openCreate() { setEditing(null); setName(''); setItemType(''); setGrade(''); setDesc(''); setLore(''); setAbility(''); setOwnerId(undefined); setItemArcId(undefined); setNarrativeRole(''); setShowForm(true) }
  function openEdit(i: item.Item) { setEditing(i); setName(i.name); setItemType(i.item_type || ''); setGrade(i.grade || ''); setDesc(i.description || ''); setLore(i.lore || ''); setAbility(i.ability || ''); setOwnerId(i.owner_id ?? undefined); setItemArcId(i.arc_id ?? undefined); setNarrativeRole(i.narrative_role || ''); setShowForm(true) }
  async function save() {
    try {
      if (editing) { await app.UpdateItem(editing.id, name, itemType, grade, desc, lore, ability, '', ownerId, itemArcId, narrativeRole) }
      else { await app.CreateItem(novelId, name, itemType, grade, desc, lore, ability, ownerId, itemArcId, narrativeRole) }
      setShowForm(false); load()
    } catch { alert('保存失败') }
  }
  async function remove(id: number) { if (!confirm('确认删除？')) return; await app.DeleteItem(id); load() }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-3 py-2.5 border-b shrink-0">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t('item.items', '物品')} ({items.length})
        </span>
        <button onClick={openCreate} className="text-xs text-primary hover:underline">+ 新建</button>
      </div>

      <div className="px-2 py-1.5 border-b shrink-0 flex items-center gap-1.5">
        <select value={typeFilter} onChange={e => setTypeFilter(e.target.value)} className="h-7 text-xs rounded-md border bg-background px-1.5">
          <option value="">类型</option>
          {ITEM_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
        </select>
        <div className="relative flex-1">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-muted-foreground" />
          <input
            type="text" value={search} onChange={e => setSearch(e.target.value)}
            placeholder={t('item.searchItem', '搜索物品')}
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
            <p className="text-xs text-muted-foreground">{search || typeFilter ? '无匹配结果' : '暂无物品'}</p>
          </div>
        ) : items.map(it => (
          <div key={it.id}
            className="w-full flex items-center gap-1.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors group cursor-pointer"
            onClick={() => setDetail(it)}>
            <Sword className="w-3.5 h-3.5 shrink-0 text-muted-foreground" />
            <div className="flex-1 min-w-0">
              <div className="text-sm truncate">{it.name}{it.grade ? <span className="text-[10px] text-muted-foreground"> ({it.grade})</span> : ''}</div>
              <div className="text-[10px] text-muted-foreground truncate">{it.item_type}{it.description ? ' · ' + it.description.slice(0, 40) : ''}</div>
            </div>
            <span className={`text-[10px] px-1 py-0.5 rounded shrink-0 ${it.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-muted text-muted-foreground'}`}>{it.status}</span>
            <button onClick={e => { e.stopPropagation(); openEdit(it) }} className="shrink-0 p-0.5 rounded text-muted-foreground hover:text-foreground opacity-0 group-hover:opacity-100 transition-opacity">
              <Pencil className="h-3 w-3" />
            </button>
            <button onClick={e => { e.stopPropagation(); remove(it.id) }} className="shrink-0 p-0.5 rounded text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity">
              <Trash2 className="h-3 w-3" />
            </button>
          </div>
        ))}
      </div>

      {showForm && (
        <div className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center" onClick={() => setShowForm(false)}>
          <div className="bg-card rounded-lg shadow-lg p-6 w-[500px] max-w-[90vw] max-h-[85vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <h3 className="text-sm font-medium mb-4">{editing ? '编辑物品' : '新建物品'}</h3>
            <div className="space-y-3">
              <input placeholder="名称" value={name} onChange={e => setName(e.target.value)} className="w-full h-9 text-sm rounded border bg-background px-3" />
              <div className="flex gap-2">
                <select value={itemType} onChange={e => setItemType(e.target.value)} className="flex-1 h-9 text-sm rounded border bg-background px-3">
                  <option value="">选择类型</option>
                  {ITEM_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
                </select>
                <input placeholder="品级" value={grade} onChange={e => setGrade(e.target.value)} className="w-24 h-9 text-sm rounded border bg-background px-3" />
              </div>
              <textarea placeholder="外观/功能描述" rows={2} value={desc} onChange={e => setDesc(e.target.value)} className="w-full text-sm rounded border bg-background p-3 resize-y" />
              <textarea placeholder="来历/传说" rows={3} value={lore} onChange={e => setLore(e.target.value)} className="w-full text-sm rounded border bg-background p-3 resize-y" />
              <textarea placeholder="特殊能力" rows={2} value={ability} onChange={e => setAbility(e.target.value)} className="w-full text-sm rounded border bg-background p-3 resize-y" />
              <div className="flex gap-2">
                <input placeholder="持有者 ID（可选）" type="number" value={ownerId ?? ''} onChange={e => setOwnerId(e.target.value ? Number(e.target.value) : undefined)} className="flex-1 h-9 text-sm rounded border bg-background px-3" />
                <input placeholder="关联弧线 ID（可选）" type="number" value={itemArcId ?? ''} onChange={e => setItemArcId(e.target.value ? Number(e.target.value) : undefined)} className="flex-1 h-9 text-sm rounded border bg-background px-3" />
              </div>
              <select value={narrativeRole} onChange={e => setNarrativeRole(e.target.value)} className="w-full h-9 text-sm rounded border bg-background px-3">
                <option value="">叙事角色（默认）</option>
                <option value="key_prop">关键道具（主线核心）</option>
                <option value="supporting">重要物品</option>
                <option value="normal">普通物品</option>
              </select>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <button onClick={() => setShowForm(false)} className="px-3 py-1.5 text-xs rounded border">{t('common.cancel', '取消')}</button>
              <button onClick={save} disabled={!name} className="px-3 py-1.5 text-xs rounded bg-primary text-primary-foreground disabled:opacity-50">保存</button>
            </div>
          </div>
        </div>
      )}

      {detail && (
        <div className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center" onClick={() => setDetail(null)}>
          <div className="bg-card rounded-lg shadow-lg p-6 w-[500px] max-w-[90vw] max-h-[85vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <h3 className="text-sm font-medium mb-3">{detail.name}</h3>
            <div className="space-y-3 text-sm">
              {detail.item_type && <div>类型: {detail.item_type}</div>}
              {detail.grade && <div>品级: {detail.grade}</div>}
              {detail.status && <div>状态: {detail.status}</div>}
              {detail.owner_id && <div>持有者 ID: {detail.owner_id}</div>}
              {detail.narrative_role && <div>叙事角色: {({ key_prop: '关键道具', supporting: '重要物品', normal: '普通物品' } as Record<string, string>)[detail.narrative_role] || detail.narrative_role}</div>}
              {detail.arc_id && <div>关联弧线 ID: {detail.arc_id}</div>}
              {detail.description && <div className="whitespace-pre-wrap">{detail.description}</div>}
              {detail.lore && <><div className="text-xs font-medium text-muted-foreground mt-2">来历</div><div className="whitespace-pre-wrap">{detail.lore}</div></>}
              {detail.ability && <><div className="text-xs font-medium text-muted-foreground mt-2">能力</div><div className="whitespace-pre-wrap">{detail.ability}</div></>}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
