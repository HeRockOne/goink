import { useState, useEffect, useCallback } from 'react'

const ATTR = 'data-theme'
const FINISH_ATTR = 'data-finish'
const LOWFX_ATTR = 'data-low-fx'

const BUILTIN_THEMES = ['light', 'dark'] as const
export type Theme = (typeof BUILTIN_THEMES)[number]
type ActiveTheme = Theme | `custom:${string}`

export type Finish = 'plain' | 'aura' | 'glass'
const FINISHES: Finish[] = ['plain', 'aura', 'glass']

export interface CustomThemeData {
  name: string
  type: 'light' | 'dark'
  colors: Record<string, string>
  // 质感预设（可选）：主题自带质感，优先于全局质感选择
  finish?: Finish
  // 兼容旧数据：effects 字段（特效模块已移除）读取时忽略
  effects?: unknown
}

const STYLE_ID = 'custom-theme-style'
const STORAGE_KEY = 'goink_custom_themes'
const FINISH_KEY = 'goink_finish'
const LOWFX_KEY = 'goink_low_fx'

function loadFinish(): Finish {
  const v = localStorage.getItem(FINISH_KEY) as Finish | null
  return v && FINISHES.includes(v) ? v : 'plain'
}

function loadLowFx(): boolean {
  return localStorage.getItem(LOWFX_KEY) === '1'
}

function isBuiltin(s: string | null): s is Theme {
  return BUILTIN_THEMES.includes(s as Theme)
}

function isCustom(s: string | null): s is `custom:${string}` {
  return typeof s === 'string' && s.startsWith('custom:')
}

const NEXT: Record<Theme, Theme> = { light: 'dark', dark: 'light' }

function sysTheme(matches: boolean): Theme {
  return matches ? 'dark' : 'light'
}

function loadCustomThemes(): CustomThemeData[] {
  try { return JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]') }
  catch { return [] }
}

function saveCustomThemes(themes: CustomThemeData[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(themes))
}

function injectCustomStyle(t: CustomThemeData) {
  let el = document.getElementById(STYLE_ID) as HTMLStyleElement | null
  if (!el) { el = document.createElement('style'); el.id = STYLE_ID; document.head.appendChild(el) }
  const vars = Object.entries(t.colors).map(([k, v]) => `  ${k}: ${v};`).join('\n')
  el.textContent = `[data-theme="custom:${t.name}"] {\n${vars}\n}`
}

function resolveTheme(): ActiveTheme {
  const stored = localStorage.getItem('theme')
  if (isBuiltin(stored)) return stored
  if (isCustom(stored) && loadCustomThemes().some(t => `custom:${t.name}` === stored)) return stored
  return sysTheme(window.matchMedia('(prefers-color-scheme: dark)').matches)
}

function applyTheme(t: ActiveTheme) {
  document.documentElement.setAttribute(ATTR, t)
  if (isCustom(t)) {
    const found = loadCustomThemes().find(th => th.name === t.slice(7))
    if (found) {
      injectCustomStyle(found)
      // 自定义主题自带质感预设时优先，否则回落到全局质感选择
      if (found.finish) { applyFinish(found.finish); return }
    }
  }
  applyFinish(loadFinish())
}

function applyFinish(f: Finish) {
  document.documentElement.setAttribute(FINISH_ATTR, f)
}

function applyLowFx(on: boolean) {
  document.documentElement.setAttribute(LOWFX_ATTR, on ? '1' : '0')
}

export function useTheme() {
  const [theme, setThemeState] = useState<ActiveTheme>(() => {
    const t = resolveTheme(); applyTheme(t); return t
  })
  const [customThemes, setCustomThemes] = useState<CustomThemeData[]>(() => loadCustomThemes())
  const [finish, setFinishState] = useState<Finish>(() => {
    const f = loadFinish(); applyFinish(f); return f
  })
  const [lowFx, setLowFxState] = useState<boolean>(() => {
    const on = loadLowFx(); applyLowFx(on); return on
  })

  useEffect(() => {
    const obs = new MutationObserver(() => {
      const v = document.documentElement.getAttribute(ATTR)
      if (v && (isBuiltin(v) || isCustom(v))) setThemeState(v as ActiveTheme)
    })
    obs.observe(document.documentElement, { attributes: true, attributeFilter: [ATTR] })
    return () => obs.disconnect()
  }, [])

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => {
      if (localStorage.getItem('theme') === null) { const t = sysTheme(mq.matches); applyTheme(t); setThemeState(t) }
    }
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const themeMode: Theme = isBuiltin(theme) ? theme : (loadCustomThemes().find(t => `custom:${t.name}` === theme)?.type || 'dark')

  const setTheme = useCallback((t: ActiveTheme) => {
    applyTheme(t)
    localStorage.setItem('theme', t)
    setThemeState(t)
  }, [])

  const setFinish = useCallback((f: Finish) => {
    applyFinish(f)
    localStorage.setItem(FINISH_KEY, f)
    setFinishState(f)
  }, [])

  const setLowFx = useCallback((on: boolean) => {
    applyLowFx(on)
    localStorage.setItem(LOWFX_KEY, on ? '1' : '0')
    setLowFxState(on)
  }, [])

  const toggle = useCallback(() => {
    if (isBuiltin(theme)) setTheme(NEXT[theme])
    else setTheme('dark')
  }, [theme, setTheme])

  const addCustomTheme = useCallback((data: CustomThemeData) => {
    const themes = loadCustomThemes()
    const idx = themes.findIndex(t => t.name === data.name)
    if (idx >= 0) themes[idx] = data; else themes.push(data)
    saveCustomThemes(themes)
    setCustomThemes(themes)
    injectCustomStyle(data)
  }, [])

  const deleteCustomTheme = useCallback((name: string) => {
    const themes = loadCustomThemes().filter(t => t.name !== name)
    saveCustomThemes(themes)
    setCustomThemes(themes)
    if (theme === `custom:${name}`) setTheme('light')
  }, [theme, setTheme])

  const getAllCustomThemes = useCallback(() => loadCustomThemes(), [])

  return {
    theme: themeMode,
    activeTheme: theme as ActiveTheme,
    setTheme,
    toggle,
    addCustomTheme,
    deleteCustomTheme,
    customThemes,
    getAllCustomThemes,
    finish,
    setFinish,
    lowFx,
    setLowFx,
  } as const
}
