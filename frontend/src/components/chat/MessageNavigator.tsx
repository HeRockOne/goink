import { memo } from 'react'
import type { Turn } from './types'

interface Props {
  turns: Turn[]
  onJump: (turnId: string) => void
}

// MessageNavigator 右侧消息导航条：每个含用户消息的 turn 一个锚点圆点，
// 悬停预览内容摘要，点击平滑滚动定位。轮次多时才渲染（由父组件控制）。
export default memo(function MessageNavigator({ turns, onJump }: Props) {
  const anchors = turns.filter(t => t.userMessage && !t.compressionOnly)
  if (anchors.length < 4) return null

  return (
    <div
      className="absolute right-0.5 top-1/2 -translate-y-1/2 z-20 flex flex-col items-center gap-1.5 py-2 max-h-[70vh] overflow-y-auto scrollbar-none"
      data-navigator
    >
      {anchors.map(turn => (
        <button
          key={turn.id}
          onClick={() => onJump(turn.id)}
          title={turn.userMessage.length > 48 ? turn.userMessage.slice(0, 48) + '…' : turn.userMessage}
          className="group/nav relative flex items-center justify-center w-3 h-3 shrink-0 cursor-pointer"
        >
          <span className="w-1.5 h-1.5 rounded-full bg-muted-foreground/25 transition-all group-hover/nav:bg-primary group-hover/nav:scale-150" />
        </button>
      ))}
    </div>
  )
})
