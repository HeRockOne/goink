import { useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronUp } from 'lucide-react'

interface Option {
  value: string
  label: string
}

interface OptionGroup {
  label: string
  icon?: React.ReactNode
  options: Option[]
}

interface FooterAction {
  label: string
  onClick: () => void
}

interface Props {
  value: string
  options?: Option[]
  groups?: OptionGroup[]
  onChange: (value: string) => void
  onOpen?: () => void
  className?: string
  minWidth?: string
  placeholder?: string
  footerAction?: FooterAction
  dropUp?: boolean // true=向上弹出(默认), false=向下弹出
}

export default function PopSelect({ value, options, groups, onChange, onOpen, className = '', minWidth = '130px', placeholder, footerAction, dropUp = true }: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const handleToggle = () => {
    if (!open && onOpen) onOpen()
    setOpen(!open)
  }

  useEffect(() => {
    if (!open) return
    const handleClick = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  // 找到选中项的 label（兼容扁平和分组）
  const selectedLabel = (() => {
    if (options) {
      return options.find(o => o.value === value)?.label
    }
    if (groups) {
      for (const g of groups) {
        const found = g.options.find(o => o.value === value)
        if (found) return found.label
      }
    }
    return undefined
  })()

  const isGrouped = !!groups && groups.length > 0

  return (
    <div ref={containerRef} className={`relative min-w-0 ${className}`}>
      <button
        onClick={handleToggle}
        style={{ minWidth }}
        className="h-[30px] w-full rounded-lg border bg-background px-2.5 text-xs text-muted-foreground flex items-center justify-between gap-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
      >
        <span className="truncate">{selectedLabel || placeholder || t('chat.noModelAvailable')}</span>
        <ChevronUp className={`w-3 h-3 shrink-0 transition-transform ${open === dropUp ? 'rotate-180' : ''}`} />
      </button>

      {open && (
        <div className={`absolute left-0 w-full max-h-[320px] overflow-y-auto rounded-lg border bg-background shadow-lg z-50 ${dropUp ? 'bottom-full mb-1' : 'top-full mt-1'}`}>
          {isGrouped ? (
            groups!.map((group, gi) => (
              <div key={gi}>
                {gi > 0 && <div className="border-t border-border/50 mx-2 my-0.5" />}
                <div className="flex items-center gap-1.5 px-2.5 pt-2 pb-1 text-[10px] font-bold uppercase tracking-wider text-muted-foreground/70 select-none">
                  {group.icon}
                  {group.label}
                </div>
                {group.options.map(opt => (
                  <button
                    key={opt.value}
                    onClick={() => {
                      onChange(opt.value)
                      setOpen(false)
                    }}
                    className={`w-full text-left px-4 py-1.5 text-xs hover:bg-muted transition-colors ${
                      opt.value === value ? 'bg-primary/20 text-primary font-medium' : 'text-muted-foreground'
                    }`}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            ))
          ) : (
            (options || []).map(opt => (
              <button
                key={opt.value}
                onClick={() => {
                  onChange(opt.value)
                  setOpen(false)
                }}
                className={`w-full text-left px-2.5 py-1.5 text-xs hover:bg-muted transition-colors ${
                  opt.value === value ? 'bg-primary/20 text-primary font-medium' : 'text-muted-foreground'
                }`}
              >
                {opt.label}
              </button>
            ))
          )}
          {footerAction && (
            <>
              <div className="border-t my-0.5" />
              <button
                onClick={() => {
                  footerAction.onClick()
                  setOpen(false)
                }}
                className="w-full text-left px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-muted transition-colors"
              >
                ⚙ {footerAction.label}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}
