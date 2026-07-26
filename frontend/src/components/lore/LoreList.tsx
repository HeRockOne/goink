import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useApp } from '@/hooks/useApp'
import type { lore } from '@/hooks/useApp'

interface Props { novelId: number }

export default function LoreList({ novelId }: Props) {
  const app = useApp()
  const { t } = useTranslation()
  const [items, setItems] = useState<lore.LoreEntry[]>([])

  const load = useCallback(async () => {
    if (!novelId) { setItems([]); return }
    try {
      const r = await app.GetLoreList(novelId, '', '', 1, 999)
      setItems(r?.items ?? [])
    } catch { setItems([]) }
  }, [app, novelId])

  useEffect(() => { load() }, [load])

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t('lore.lore', '世界观设定')} ({items.length})
        </span>
      </div>
      <div className="flex-1 overflow-y-auto overscroll-contain">
        {items.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">暂无设定</p>
          </div>
        ) : items.map(item => (
          <div key={item.id}
            className="w-full flex items-center gap-2.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors group">
            <span className="w-5 h-5 rounded-full bg-tag-blue text-tag-blue-foreground text-[10px] font-medium flex items-center justify-center shrink-0">
              {(item.title ?? '').charAt(0) || '?'}
            </span>
            <span className="flex-1 text-sm truncate">{item.title}</span>
          </div>
        ))}
      </div>
    </>
  )
}
