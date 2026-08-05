import { useState } from 'react'

// v2 顶部装饰标语：⚔ 万剑归宗 · 剑气极盛，双击可自定义（localStorage 持久化）
const SLOGAN_KEY = 'header_slogan'
const DEFAULT_SLOGAN = '⚔ 万剑归宗 · 剑气极盛'

function loadSlogan(): string {
  try { return localStorage.getItem(SLOGAN_KEY) || DEFAULT_SLOGAN } catch { return DEFAULT_SLOGAN }
}

export default function HeaderSlogan() {
  const [slogan, setSlogan] = useState(loadSlogan)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')

  if (editing) {
    return (
      <input
        autoFocus
        value={draft}
        onChange={e => setDraft(e.target.value)}
        onBlur={() => { setEditing(false) }}
        onKeyDown={e => {
          if (e.key === 'Enter') {
            const v = draft.trim()
            if (v) { setSlogan(v); try { localStorage.setItem(SLOGAN_KEY, v) } catch { /* */ } }
            setEditing(false)
          }
          if (e.key === 'Escape') setEditing(false)
        }}
        onClick={e => e.stopPropagation()}
        className="header-slogan mr-3 w-44 bg-transparent outline-none border-b border-dashed border-current"
        title="回车保存，Esc 取消"
      />
    )
  }

  return (
    <span
      className="header-slogan mr-3 cursor-text"
      title="双击自定义标语"
      onDoubleClick={() => { setDraft(slogan); setEditing(true) }}
    >
      {slogan}
    </span>
  )
}
