import { clsx, type ClassValue } from "clsx"
import { toast } from 'sonner'
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * 把 LLM 自由格式的 JSON 数组字段规整为字符串数组。
 * LLM 可能把 abilities/tags 写成对象数组（如 [{name, level, description}]），
 * 直接渲染会触发 React #31。对象元素取 name ?? description 兜底。
 */
export function parseStringArray(json: string): string[] {
  try {
    const v = JSON.parse(json)
    if (!Array.isArray(v)) return []
    return v
      .map((a: unknown) => {
        if (typeof a === 'string') return a.trim()
        if (a && typeof a === 'object') {
          const o = a as Record<string, unknown>
          const s = typeof o.name === 'string' ? o.name : typeof o.description === 'string' ? o.description : ''
          return s.trim()
        }
        return String(a ?? '').trim()
      })
      .filter(Boolean)
  } catch {
    return []
  }
}

/** 显示错误 toast，带复制按钮，方便用户报告具体错误信息。 */
export function toastError(msg: string) {
  return toast.error(msg, {
    action: {
      label: '复制',
      onClick: () => navigator.clipboard.writeText(msg),
    },
    actionButtonStyle: {
      backgroundColor: 'var(--primary)',
      color: 'var(--primary-foreground)',
      border: 'none',
      padding: '2px 10px',
      borderRadius: '4px',
      fontSize: '12px',
    },
  })
}
