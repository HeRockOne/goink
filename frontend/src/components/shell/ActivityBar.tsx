import type { LucideIcon } from 'lucide-react'
import { Search, Library, List, Settings, Users, MapPin, GitBranch, History, BookOpen, Sword, GitGraph, Eye, Wrench, BarChart3, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

// v2 太虚风格：固定单字图标列（无折叠/展开）
interface Activity {
  id: string
  icon: LucideIcon
  glyph: string
  labelKey: string
  disabled?: boolean
}

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

export default function ActivityBar({ activeId, onSelect }: Props) {
  const { t } = useTranslation()

  return (
    <nav className="activity-bar flex flex-col py-2 border-r bg-sidebar backdrop-blur-md select-none cursor-default w-[56px] items-center">
      <div className="h-px bg-border mx-2 mb-1" />
      {activities.map((a, i) => {
        const isActive = a.id === activeId
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
              <span className="text-[15px] leading-none font-display">{a.glyph}</span>
            </button>
          </div>
        )
      })}
    </nav>
  )
}
