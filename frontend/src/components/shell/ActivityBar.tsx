import type { LucideIcon } from 'lucide-react'
import { Search, Library, List, Heart, Users, MapPin, GitBranch, History, BookOpen, Sword, GitGraph, Eye, Wrench, BarChart3, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

// v2 太虚风格：固定图标列（无折叠/展开）
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
  { id: 'preferences', icon: Heart, labelKey: 'shell.preference' },
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

export default function ActivityBar({ activeId, onSelect }: Props) {
  const { t } = useTranslation()

  return (
    <nav className="activity-bar flex flex-col py-2 border-r bg-sidebar select-none cursor-default w-[56px] items-center overflow-y-auto overflow-hidden">
      <div className="h-px bg-border mx-2 mb-1" />
      {activities.map((a, i) => {
        const isActive = a.id === activeId
        const Icon = a.icon
        return (
          <div key={a.id} className={i === 0 || i === 3 ? 'mt-1' : ''}>
            {i === 0 && <div className="h-px bg-border my-1 mx-1" />}
            {i === 3 && <div className="h-px bg-border my-1 mx-1" />}
            <button
              disabled={a.disabled}
              onClick={() => onSelect(a.id)}
              title={`${t(a.labelKey)}${a.disabled ? t('shell.comingSoon') : ''}`}
              className={`activity-item ${isActive && !a.disabled ? 'active' : ''} relative flex items-center rounded-lg transition-all duration-200
                focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring
                w-10 h-10 justify-center mx-auto
                ${a.disabled
                  ? 'text-muted-foreground/40 cursor-not-allowed'
                  : isActive
                    ? 'text-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted/60'
                }`}
            >
              {isActive && !a.disabled && (
                <span className="activity-bar-indicator absolute top-1/2 -translate-y-1/2 w-[3px] h-6 bg-primary rounded-r-full left-[2px]" />
              )}
              <Icon className="w-5 h-5" strokeWidth={1.75} />
            </button>
          </div>
        )
      })}
    </nav>
  )
}
