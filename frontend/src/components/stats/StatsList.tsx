import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useApp } from '@/hooks/useApp'

interface Props { novelId: number }

export default function StatsList({ novelId }: Props) {
  const app = useApp()
  const { t } = useTranslation()
  const [data, setData] = useState<{ total_chapters: number; total_words: number; character_count: number; location_count: number } | null>(null)

  const load = useCallback(async () => {
    if (!novelId) return
    try {
      const s = await app.GetNovelStats(novelId)
      setData(s)
    } catch { /* ignore */ }
  }, [app, novelId])

  useEffect(() => { load() }, [load])

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t('stats.stats', '创作统计')}
        </span>
      </div>
      <div className="flex-1 overflow-y-auto overscroll-contain">
        {!data ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">加载中...</p>
          </div>
        ) : (
          <div className="px-3 py-2 space-y-3 text-xs">
            <div><span className="text-muted-foreground">总章节</span><br /><span className="text-sm font-medium">{data.total_chapters}</span></div>
            <div><span className="text-muted-foreground">总字数</span><br /><span className="text-sm font-medium">{fmt(data.total_words)}</span></div>
            <div><span className="text-muted-foreground">角色数</span><br /><span className="text-sm font-medium">{data.character_count}</span></div>
            <div><span className="text-muted-foreground">地点数</span><br /><span className="text-sm font-medium">{data.location_count}</span></div>
          </div>
        )}
      </div>
    </>
  )
}

function fmt(n: number): string {
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return String(n)
}
