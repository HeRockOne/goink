import type { LucideIcon } from 'lucide-react'
import { Library, List, Search, Settings, Users, MapPin, GitBranch, History, GitGraph, Eye, Wrench, Sparkles, BookOpen, Sword, BarChart3, ChevronLeft, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useState, useEffect } from 'react'

interface Activity {
  id: string
  icon: LucideIcon
  labelKey: string
  disabled?: boolean
}

const activities: Activity[] = [
  { id: 'search', icon: Search, labelKey: 'shell.search' },
  { id: 'novels', icon: Library, labelKey: 'shell.bookshelf' },
  { id: 'chapters', icon: List, labelKey: 'shell.chapters' },
  { id: 'preferences', icon: Settings, labelKey: 'shell.preference' },
  { id: 'characters', icon: Users, labelKey: 'shell.characters' },
  { id: 'locations', icon: MapPin, labelKey: 'shell.locations' },
  { id: 'storyarcs', icon: GitBranch, labelKey: 'shell.arcs' },
  { id: 'timeline', icon: History, labelKey: 'shell.timeline' },
  { id: 'world', icon: BookOpen, labelKey: 'shell.world' },
  { id: 'items', icon: Sword, labelKey: 'shell.items' },
  { id: 'git', icon: GitGraph, labelKey: 'shell.gitHistory' },
  { id: 'reader', icon: Eye, labelKey: 'shell.readerView' },
  { id: 'skills', icon: Wrench, labelKey: 'shell.skills' },
  { id: 'stats', icon: BarChart3, labelKey: 'shell.stats' },
  { id: 'style-samples', icon: Sparkles, labelKey: 'shell.extract' },
]

interface Props {
  activeId: string
  onSelect: (id: string) => void
}

const STORAGE_KEY = 'activity_bar_collapsed'

export default function ActivityBar({ activeId, onSelect }: Props) {
  const { t } = useTranslation()
  const [collapsed, setCollapsed] = useState(() => {
    try { return localStorage.getItem(STORAGE_KEY) === 'true' } catch { return false }
  })

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, String(collapsed))
  }, [collapsed])

  return (
    <nav className={`flex flex-col py-2 border-r bg-sidebar select-none cursor-default transition-all duration-200 ${collapsed ? 'w-12 items-center' : 'w-auto min-w-fit px-2'}`}>
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
          <div key={a.id}>
            {i === 0 && <div className={`h-px bg-border my-1 ${collapsed ? 'mx-1' : 'mx-2'}`} />}
            {i === 3 && <div className={`h-px bg-border my-1 ${collapsed ? 'mx-1' : 'mx-2'}`} />}
            <button
              disabled={a.disabled}
              onClick={() => onSelect(a.id)}
              title={collapsed ? `${t(a.labelKey)}${a.disabled ? t('shell.comingSoon') : ''}` : undefined}
              className={`relative flex items-center rounded-lg transition-all duration-200
                focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring
                ${collapsed
                  ? 'w-10 h-10 justify-center mx-auto'
                  : 'w-full gap-2.5 px-2.5 py-2 text-left whitespace-nowrap'
                }
                ${a.disabled
                  ? 'text-muted-foreground/40 cursor-not-allowed'
                  : isActive
                    ? 'text-foreground bg-muted'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted/60'
                }`}
            >
              {isActive && !a.disabled && (
                <span className={`absolute top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full ${collapsed ? 'left-0' : 'left-0'}`} />
              )}
              <a.icon className={`shrink-0 ${collapsed ? 'w-5 h-5' : 'w-4 h-4'}`} />
              {!collapsed && <span className="text-xs leading-none">{t(a.labelKey)}</span>}
            </button>
          </div>
        )
      })}
    </nav>
  )
}
