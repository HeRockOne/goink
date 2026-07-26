import { useState, useEffect, useCallback } from 'react'
import { FileText, BookOpen, Users, MapPin, GitBranch, Eye, MessageSquare } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useApp } from '@/hooks/useApp'
import type { stats } from '@/hooks/useApp'

interface Props { novelId: number }

export default function StatsView({ novelId }: Props) {
  const app = useApp()
  const { t } = useTranslation()
  const [data, setData] = useState<stats.NovelStats | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    if (!novelId) return
    setLoading(true)
    try {
      const s = await app.GetNovelStats(novelId)
      setData(s)
    } catch { /* ignore */ }
    setLoading(false)
  }, [app, novelId])

  useEffect(() => { load() }, [load])

  const arcRate = data && data.arc_count > 0 ? Math.round(data.arc_completed / data.arc_count * 100) : 0
  const fRate = data && data.foreshadowing_total > 0 ? Math.round(data.foreshadowing_resolved / data.foreshadowing_total * 100) : 0

  const rows = [
    { icon: <FileText className="w-4 h-4" />, label: '总章节', value: String(data?.total_chapters ?? 0) },
    { icon: <BookOpen className="w-4 h-4" />, label: '总字数', value: fmt(data?.total_words ?? 0) },
    { icon: <MessageSquare className="w-4 h-4" />, label: '均章字数', value: fmt(data?.avg_chapter_words ?? 0) },
    { icon: <GitBranch className="w-4 h-4" />, label: '弧线进度', value: `${data?.arc_completed ?? 0}/${data?.arc_count ?? 0} (${arcRate}%)` },
    { icon: <Eye className="w-4 h-4" />, label: '伏笔回收', value: `${data?.foreshadowing_resolved ?? 0}/${data?.foreshadowing_total ?? 0} (${fRate}%)` },
    { icon: <Users className="w-4 h-4" />, label: '角色数', value: String(data?.character_count ?? 0) },
    { icon: <MapPin className="w-4 h-4" />, label: '地点数', value: String(data?.location_count ?? 0) },
  ]

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-3 py-2.5 border-b shrink-0">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t('stats.stats', '创作统计')}
        </span>
      </div>

      <div className="flex-1 overflow-y-auto overscroll-contain">
        {loading ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">{t('common.loading', '加载中')}</p>
          </div>
        ) : !data ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">暂无数据</p>
          </div>
        ) : (
          <>
            {rows.map((row, i) => (
              <div key={i} className="w-full flex items-center gap-1.5 px-3 py-1.5 hover:bg-muted/50 transition-colors">
                <span className="w-3.5 h-3.5 shrink-0 text-muted-foreground flex items-center justify-center">{row.icon}</span>
                <span className="flex-1 text-sm truncate text-foreground">{row.label}</span>
                <span className="text-xs text-muted-foreground">{row.value}</span>
              </div>
            ))}
            {data.latest_chapter_num > 0 && (
              <div className="px-3 pt-2 mt-2 border-t">
                <p className="text-xs text-muted-foreground">
                  最新章节：第 {data.latest_chapter_num} 章 {data.latest_chapter_title}
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

function fmt(n: number): string {
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return String(n)
}
