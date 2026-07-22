import { useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'

interface Props {
  retryCount: number
  retryMax: number
  retryWait: number
  onDone?: () => void
}

export default function RetryNotification({ retryCount, retryMax, retryWait, onDone }: Props) {
  const [visible, setVisible] = useState(true)

  useEffect(() => {
    setVisible(true)
    const timer = setTimeout(() => {
      setVisible(false)
      onDone?.()
    }, retryWait * 1000 + 500)
    return () => clearTimeout(timer)
  }, [retryCount, retryWait, onDone])

  if (!visible) return null

  return (
    <div className="absolute top-2 left-1/2 -translate-x-1/2 z-50 animate-in fade-in slide-in-from-top-2 duration-300">
      <div className="flex items-center gap-2 px-4 py-2 rounded-lg bg-amber-500/10 border border-amber-500/30 text-amber-700 dark:text-amber-400 shadow-lg backdrop-blur-sm">
        <RefreshCw className="w-4 h-4 animate-spin" />
        <span className="text-sm font-medium">
          请求受限，{retryWait}秒后重试 {retryMax > 0 ? `(${retryCount}/${retryMax})` : `(第${retryCount}次)`}
        </span>
      </div>
    </div>
  )
}
