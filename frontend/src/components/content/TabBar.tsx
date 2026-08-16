import { X, FileDiff } from 'lucide-react'

interface Props {
  tabs: { id: string; type: string; title: string }[]
  activeTabId: string | null
  onSelect: (id: string) => void
  onClose: (id: string) => void
}

export default function TabBar({ tabs, activeTabId, onSelect, onClose }: Props) {
  if (tabs.length === 0) return null

  return (
    <div className="flex items-center bg-[var(--editor-statusbar)] border-b shrink-0 overflow-x-auto">
      {tabs.map(tab => {
        const isDiff = tab.type === 'diff'
        const active = tab.id === activeTabId
        return (
          <div
            key={tab.id}
            className={`group flex items-center gap-1 px-3 py-1.5 text-xs cursor-pointer border-r shrink-0 transition-colors select-none ${
              active
                ? 'bg-background text-foreground border-t-2 border-t-blue-500 -mt-[1px] tab-title-active'
                : 'text-muted-foreground hover:bg-muted/50'
            } ${isDiff ? 'italic text-tag-amber-foreground/80' : ''}`}
            onClick={() => onSelect(tab.id)}
          >
            {isDiff && <FileDiff className="w-3 h-3 shrink-0 text-tag-amber-foreground/70" />}
            <span className="truncate max-w-[160px]">{tab.title}</span>
            <button
              className="ml-0.5 p-0.5 rounded opacity-0 group-hover:opacity-100 hover:bg-muted transition-opacity cursor-pointer"
              onClick={e => { e.stopPropagation(); onClose(tab.id) }}
            >
              <X className="w-3 h-3" />
            </button>
          </div>
        )
      })}
    </div>
  )
}
