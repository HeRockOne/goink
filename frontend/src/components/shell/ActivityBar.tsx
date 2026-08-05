import type { LucideIcon } from 'lucide-react'
import { Search, Library, List, Settings, Users, MapPin, GitBranch, History, BookOpen, Sword, GitGraph, Eye, Wrench, BarChart3, Sparkles, ChevronLeft, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useState, useEffect } from 'react'

interface Activity {
  id: string
  icon: LucideIcon
  glyph: string // 单字（v2 太虚风格）
  labelKey: string
  disabled?: boolean
}

// v2 单字图标列：卷/角/地/弧/线/界/物/史/忆/技/计/风
const activities: Activity[] = [
  { id: 'search', icon: Search, glyph: '寻', labelKey: 'shell.search' },
  { id: 'novels', icon: Library, glyph: '架', labelKey: 'shell.bookshelf' },
  { id: 'chapters', icon: List, glyph: '卷', labelKey: 'shell.chapters' },
  { id: 'preferences', icon: Settings, glyph: '好', labelKey: 'shell.preference' },
  { id: 'characters', icon: Users, glyph: '角', labelKey: 'shell.characters' },
  { id: 'locations', icon: MapPin, glyph: '地', labelKey: 'shell.locations' },
  { id: 'storyarcs', icon: GitBranch, glyph: '弧', labelKey: 'shell.arcs' },
  { id: 'timeline', icon: History, glyph: '线', labelKey: 'shell.timeline' },
  { id: 'world', icon: BookOpen, glyph: '界', labelKey: 'shell.world' },
  { id: 'items', icon: Sword, glyph: '物', labelKey: 'shell.items' },
  { id: 'git', icon: GitGraph, glyph: '史', labelKey: 'shell.gitHistory' },
  { id: 'reader', icon: Eye, glyph: '忆', labelKey: 'shell.readerView' },
  { id: 'skills', icon: Wrench, glyph: '技', labelKey: 'shell.skills' },
  { id: 'stats', icon: BarChart3, glyph: '计', labelKey: 'shell.stats' },
  { id: 'style-samples', icon: Sparkles, glyph: '风', labelKey: 'shell.extract' },
]

interface Props {
  activeId: string
  onSelect: (id: string) => void
}

const STORAGE_KEY = 'activity_bar_collapsed'

export default function ActivityBar({ activeId, onSelect }: Props) {
  const { t } = useTranslation()
  const [collapsed, setCollapsed] = useState(() => {
    try { return localStorage.getItem(STORAGE_KEY) !== 'false' } catch { return true }
  })

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, String(collapsed))
  }, [collapsed])

  return (
    <nav className={`activity-bar flex flex-col py-2 border-r bg-sidebar backdrop-blur-md select-none cursor-default transition-all duration-200 ${collapsed ? 'w-[56px] items-center' : 'w-auto min-w-fit px-2'}`}>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center justify-center w-full h-8 text-muted-foreground hover:text-foreground mb-1"
        title={collapsed ? t('common.expand') : t('common.collapse')}
      >
        {collapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
      </button>
      <div className="h-px bg-border mx-2 mb-1" />
      {activities.map((a, i) => {
        const isActive = a.id === activeId
        return (
          <div key={a.id} className={i === 0 || i === 3 ? 'mt-1' : ''}>
            {i === 0 && <div className={`h-px bg-border my-1 ${collapsed ? 'mx-1' : 'mx-2'}`} />}
            {i === 3 && <div className={`h-px bg-border my-1 ${collapsed ? 'mx-1' : 'mx-2'}`} />}
            <button
              disabled={a.disabled}
              onClick={() => onSelect(a.id)}
              title={collapsed ? `${t(a.labelKey)}${a.disabled ? t('shell.comingSoon') : ''}` : undefined}
              className={`activity-item ${isActive && !a.disabled ? 'active' : ''} relative flex items-center rounded-lg transition-all duration-200
                focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring
                ${collapsed
                  ? 'w-10 h-10 justify-center mx-auto'
                  : 'w-full gap-2.5 px-2.5 py-2 text-left whitespace-nowrap'
                }
                ${a.disabled
                  ? 'text-muted-foreground/40 cursor-not-allowed'
                  : isActive
                    ? 'text-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted/60'
                }`}
            >
              {isActive && !a.disabled && (
                <span className={`activity-bar-indicator absolute top-1/2 -translate-y-1/2 w-[3px] h-6 bg-primary rounded-r-full ${collapsed ? 'left-[2px]' : 'left-0'}`} />
              )}
              {collapsed ? (
                <span className="text-[15px] leading-none font-display">{a.glyph}</span>
              ) : (
                <>
                  <a.icon className="shrink-0 w-4 h-4" />
                  <span className="text-xs leading-none">{t(a.labelKey)}</span>
                </>
              )}
            </button>
          </div>
        )
      })}
    </nav>
  )
}
