import { useEffect, useState } from 'react'
import { Palette, Trash2, Wand2, Sparkles, Plus, X } from 'lucide-react'
import { useTheme, type CustomThemeData } from '@/hooks/useTheme'
import { checkThemeContrast, generateTheme } from '@/lib/themeColors'
import { EFFECT_PRESETS, presetByName, type EffectLayer, type EffectType } from '@/lib/themeEffects'

const EFFECT_TYPES: { value: EffectType | 'none'; label: string }[] = [
  { value: 'none', label: '无' },
  { value: 'ambient', label: '氛围光' },
  { value: 'particles', label: '粒子' },
  { value: 'streak', label: '流光' },
  { value: 'glow', label: '呼吸光晕' },
]

function validateJSON(text: string): { ok: true; data: CustomThemeData } | { ok: false; error: string } {
  try {
    const cleaned = text.replace(/\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '')
    const parsed = JSON.parse(cleaned)
    if (!parsed.name || typeof parsed.name !== 'string') return { ok: false, error: '缺少 name 字段' }
    if (!parsed.colors || typeof parsed.colors !== 'object') return { ok: false, error: '缺少 colors 对象' }
    if (!parsed.type) parsed.type = 'dark'
    return { ok: true, data: parsed as CustomThemeData }
  } catch (e) {
    return { ok: false, error: 'JSON 格式错误: ' + (e instanceof Error ? e.message : String(e)) }
  }
}

const INITIAL_THEME_JSON = `{
  "name": "墨绿书斋",
  "type": "dark",
  "colors": {
    "--background": "#0f1a14", "--foreground": "#d8e8d8", "--card": "#1a2a20",
    "--card-foreground": "#d8e8d8", "--popover": "#1a2a20", "--popover-foreground": "#d8e8d8",
    "--primary": "#5a9a6a", "--primary-foreground": "#0f1a14", "--secondary": "#1a2820",
    "--secondary-foreground": "#c8d8c8", "--muted": "#1a2820", "--muted-foreground": "#8a9a8a",
    "--accent": "#223028", "--accent-foreground": "#d8e8d8", "--destructive": "#b55a4a",
    "--destructive-foreground": "#faf4e4", "--border": "#223028", "--input": "#1a2820",
    "--ring": "#5a9a6a", "--sidebar": "#16241e", "--sidebar-foreground": "#d8e8d8",
    "--sidebar-primary": "#5a9a6a", "--sidebar-primary-foreground": "#0f1a14",
    "--sidebar-accent": "#1a2a20", "--sidebar-accent-foreground": "#c8d8c8",
    "--sidebar-border": "#223028", "--sidebar-ring": "#5a9a6a",
    "--tag-blue": "#1a2a30", "--tag-blue-foreground": "#6a9aaa",
    "--tag-green": "#1a2a1e", "--tag-green-foreground": "#5a8a5a",
    "--tag-amber": "#2a2818", "--tag-amber-foreground": "#9a8a3a",
    "--tag-rose": "#2a1818", "--tag-rose-foreground": "#9a5a5a",
    "--tag-teal": "#182a28", "--tag-teal-foreground": "#5a8a8a",
    "--tag-purple": "#2a1e30", "--tag-purple-foreground": "#8a6a9a",
    "--reader-bg": "#0f1a14", "--reader-paper": "#1a2a20",
    "--bubble-user": "#5a9a6a", "--bubble-user-foreground": "#0f1a14",
    "--action-extract": "#6a7a5a", "--action-extract-foreground": "#faf4e4",
    "--action-save": "#4a7a4a", "--action-save-foreground": "#faf4e4",
    "--success": "#1a2a1e", "--success-foreground": "#5a8a5a", "--success-border": "#2a4a3a",
    "--danger-bg": "#2a1a18", "--danger-border": "#4a2a20",
    "--status-warning": "#9a8030", "--status-ok": "#4a8a5a",
    "--tool-blue": "#1a2a38", "--tool-blue-border": "#4a7a9a",
    "--tool-amber": "#2a2818", "--tool-amber-border": "#9a8030",
    "--tool-green": "#1a2820", "--tool-green-border": "#4a7a4a",
    "--tool-red": "#2a1a18", "--tool-red-border": "#9a4a3a",
    "--contribution-0": "#16241e", "--contribution-1": "#1a3a28",
    "--contribution-2": "#2a5a3a", "--contribution-3": "#3a7a4a",
    "--contribution-4": "#4a9a5a"
  }
}`

// ── 生成器表单状态（持久化到 localStorage，下次打开记住） ──

interface GenForm {
  name: string
  mode: 'light' | 'dark'
  bg: string
  primary: string
  fg: string
  vibrancy: number
  effectsPreset: string // 'none' | 预设名 | 'custom'
  effectsLayers: EffectLayer[]
}

const GEN_KEY = 'goink_theme_gen'
const GEN_DEFAULT: GenForm = {
  name: '我的主题', mode: 'dark', bg: '#0f1a14', primary: '#5a9a6a', fg: '',
  vibrancy: 1, effectsPreset: 'none', effectsLayers: [],
}

function loadGenForm(): GenForm {
  try {
    const raw = localStorage.getItem(GEN_KEY)
    if (!raw) return GEN_DEFAULT
    return { ...GEN_DEFAULT, ...JSON.parse(raw) }
  } catch {
    return GEN_DEFAULT
  }
}

export default function ThemeConfigTab() {
  const { activeTheme: theme, setTheme, addCustomTheme, deleteCustomTheme, customThemes } = useTheme()
  const [json, setJson] = useState('')
  const [error, setError] = useState('')
  const [warnings, setWarnings] = useState<string[]>([])

  // 生成器表单：每次修改自动持久化
  const [gen, setGen] = useState<GenForm>(loadGenForm)
  useEffect(() => {
    try { localStorage.setItem(GEN_KEY, JSON.stringify(gen)) } catch { /* 忽略存储失败 */ }
  }, [gen])
  const set = <K extends keyof GenForm>(k: K, v: GenForm[K]) => setGen(g => ({ ...g, [k]: v }))

  function isActive(name: string) { return theme === `custom:${name}` }

  function handleApply(text: string) {
    setError('')
    setWarnings([])
    const result = validateJSON(text)
    if (result.ok) {
      // 对比度校验：只警告不阻止（大号文字 3:1 也可接受）
      const bad = checkThemeContrast(result.data.colors)
      if (bad.length > 0) {
        setWarnings(bad.map(p => `${p.bg} × ${p.fg}（${p.ratio}:1）`))
      }
      // 去重键：name + type，同名同类型覆盖
      const key = `${result.data.name}__${result.data.type}`
      const existing = customThemes.find(t => `${t.name}__${t.type}` === key)
      if (existing) deleteCustomTheme(existing.name)
      addCustomTheme(result.data)
      setTheme(`custom:${result.data.name}`)
      setJson('')
    } else {
      setError(result.error)
    }
  }

  function handleDelete(name: string) {
    deleteCustomTheme(name)
  }

  function handleActivate(data: CustomThemeData) {
    addCustomTheme(data)
    setTheme(`custom:${data.name}`)
  }

  function fillInitial() {
    setJson(INITIAL_THEME_JSON)
    setError('')
  }

  // 一键生成：主色 → 全套 71 变量（对比度已保证 AA）
  function handleGenerate() {
    setError('')
    setWarnings([])
    const colors = generateTheme({
      name: gen.name, mode: gen.mode, background: gen.bg, primary: gen.primary,
      foreground: gen.fg || undefined, vibrancy: gen.vibrancy,
    })
    const data: CustomThemeData = {
      name: gen.name, type: gen.mode, colors,
      effects: { layers: gen.effectsLayers.filter(l => l.intensity > 0) },
    }
    const key = `${data.name}__${data.type}`
    const existing = customThemes.find(t => `${t.name}__${t.type}` === key)
    if (existing) deleteCustomTheme(existing.name)
    addCustomTheme(data)
    setTheme(`custom:${data.name}`)
  }

  return (
    <div className="flex flex-col h-full">
      <h3 className="text-sm font-medium mb-2">自定义主题</h3>
      <p className="text-xs text-muted-foreground mb-3">
        选背景色 + 主色一键生成全套主题（自动保证文字对比度 ≥ 4.5:1）；或粘贴包含全部 CSS 变量的主题 JSON。未填的变量会自动派生，不会撞色。生成器设置会自动保存，下次打开记得你上次的配置。
      </p>

      {/* ── 一键生成器 ── */}
      <div className="mb-3 rounded-lg border border-border bg-card p-3">
        <div className="flex items-center gap-1.5 mb-2">
          <Wand2 className="w-3.5 h-3.5 text-primary" />
          <span className="text-xs font-medium text-foreground">一键生成</span>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            主题名
            <input
              value={gen.name}
              onChange={e => set('name', e.target.value)}
              className="rounded border bg-background px-2 py-1 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            模式
            <select
              value={gen.mode}
              onChange={e => set('mode', e.target.value as 'light' | 'dark')}
              className="rounded border bg-background px-2 py-1 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="light">浅色</option>
              <option value="dark">深色</option>
            </select>
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            背景色
            <div className="flex items-center gap-2">
              <input type="color" value={gen.bg} onChange={e => set('bg', e.target.value)}
                className="h-7 w-10 rounded border border-border bg-transparent cursor-pointer" />
              <input value={gen.bg} onChange={e => set('bg', e.target.value)}
                className="flex-1 min-w-0 rounded border bg-background px-2 py-1 text-xs font-mono text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" spellCheck={false} />
            </div>
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            主色
            <div className="flex items-center gap-2">
              <input type="color" value={gen.primary} onChange={e => set('primary', e.target.value)}
                className="h-7 w-10 rounded border border-border bg-transparent cursor-pointer" />
              <input value={gen.primary} onChange={e => set('primary', e.target.value)}
                className="flex-1 min-w-0 rounded border bg-background px-2 py-1 text-xs font-mono text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" spellCheck={false} />
            </div>
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            文字色（可选，留空自动）
            <div className="flex items-center gap-2">
              <input type="color" value={gen.fg || '#000000'} onChange={e => set('fg', e.target.value)}
                className="h-7 w-10 rounded border border-border bg-transparent cursor-pointer" />
              <input value={gen.fg} onChange={e => set('fg', e.target.value)} placeholder="自动"
                className="flex-1 min-w-0 rounded border bg-background px-2 py-1 text-xs font-mono text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" spellCheck={false} />
              {gen.fg && (
                <button onClick={() => set('fg', '')} className="shrink-0 text-[11px] text-muted-foreground hover:text-foreground">清除</button>
              )}
            </div>
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            鲜艳度
            <div className="flex items-center gap-2">
              <input type="range" min={0.5} max={1.5} step={0.05} value={gen.vibrancy}
                onChange={e => set('vibrancy', Number(e.target.value))}
                className="flex-1 accent-[var(--primary)] cursor-pointer" />
              <span className="w-9 shrink-0 text-right text-xs font-mono text-foreground">{gen.vibrancy.toFixed(2)}×</span>
            </div>
          </label>
        </div>

        {/* 特效配置（随主题保存，可组合） */}
        <div className="mt-3 pt-2 border-t border-border">
          <div className="flex items-center gap-1.5 mb-2">
            <Sparkles className="w-3.5 h-3.5 text-primary" />
            <span className="text-xs font-medium text-foreground">特效（随主题保存）</span>
            <span className="text-[10px] text-muted-foreground">颜色自动跟随主色；层可自由组合叠加</span>
          </div>
          <div className="flex items-center gap-2 mb-2">
            <span className="text-[11px] text-muted-foreground shrink-0">预设</span>
            <select
              value={gen.effectsPreset}
              onChange={e => {
                const v = e.target.value
                set('effectsPreset', v)
                if (v === 'none') set('effectsLayers', [])
                else if (v === 'custom') { /* 保持当前层 */ }
                else {
                  const fx = presetByName(v)
                  set('effectsLayers', fx ? fx.layers : [])
                }
              }}
              className="flex-1 min-w-0 rounded border bg-background px-2 py-1 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="none">无特效</option>
              {EFFECT_PRESETS.map(p => <option key={p.name} value={p.name}>{p.name}</option>)}
              <option value="custom">自定义…</option>
            </select>
            <button
              onClick={() => set('effectsLayers', [...gen.effectsLayers, { type: 'ambient', intensity: 0.3 }])}
              className="shrink-0 p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="添加一层"
            >
              <Plus className="w-3.5 h-3.5" />
            </button>
          </div>

          {gen.effectsPreset === 'custom' && (
            <div className="space-y-2">
              {gen.effectsLayers.map((layer, i) => (
                <div key={i} className="rounded border border-border bg-background/60 p-2">
                  <div className="flex items-center gap-2">
                    <select
                      value={layer.type}
                      onChange={e => {
                        const t = e.target.value as EffectType | 'none'
                        const next = [...gen.effectsLayers]
                        if (t === 'none') {
                          next.splice(i, 1)
                        } else {
                          next[i] = { type: t, intensity: layer.intensity, count: t === 'particles' ? layer.count ?? 60 : undefined, speed: layer.speed ?? 1 }
                        }
                        set('effectsLayers', next)
                      }}
                      className="rounded border bg-background px-1.5 py-1 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      {EFFECT_TYPES.map(t => <option key={t.value} value={t.value}>{t.label}</option>)}
                    </select>
                    <span className="text-[11px] text-muted-foreground shrink-0">强度</span>
                    <input type="range" min={0.05} max={1} step={0.05} value={layer.intensity}
                      onChange={e => {
                        const next = [...gen.effectsLayers]
                        next[i] = { ...next[i], intensity: Number(e.target.value) }
                        set('effectsLayers', next)
                      }}
                      className="flex-1 accent-[var(--primary)] cursor-pointer" />
                    <span className="w-8 shrink-0 text-right text-xs font-mono text-foreground">{Math.round(layer.intensity * 100)}%</span>
                    <button
                      onClick={() => { const next = [...gen.effectsLayers]; next.splice(i, 1); set('effectsLayers', next) }}
                      className="shrink-0 p-0.5 rounded text-muted-foreground hover:text-destructive transition-colors" title="删除层"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>
                  {layer.type === 'particles' && (
                    <div className="flex items-center gap-2 mt-1.5">
                      <span className="text-[11px] text-muted-foreground shrink-0">数量（≤150）</span>
                      <input type="range" min={8} max={150} step={2} value={layer.count ?? 60}
                        onChange={e => {
                          const next = [...gen.effectsLayers]
                          next[i] = { ...next[i], count: Number(e.target.value) }
                          set('effectsLayers', next)
                        }}
                        className="flex-1 accent-[var(--primary)] cursor-pointer" />
                      <span className="w-8 shrink-0 text-right text-xs font-mono text-foreground">{layer.count ?? 60}</span>
                    </div>
                  )}
                  {(layer.type === 'particles' || layer.type === 'streak' || layer.type === 'glow') && (
                    <div className="flex items-center gap-2 mt-1.5">
                      <span className="text-[11px] text-muted-foreground shrink-0">速度</span>
                      <input type="range" min={0.1} max={2} step={0.1} value={layer.speed ?? 1}
                        onChange={e => {
                          const next = [...gen.effectsLayers]
                          next[i] = { ...next[i], speed: Number(e.target.value) }
                          set('effectsLayers', next)
                        }}
                        className="flex-1 accent-[var(--primary)] cursor-pointer" />
                      <span className="w-8 shrink-0 text-right text-xs font-mono text-foreground">{(layer.speed ?? 1).toFixed(1)}×</span>
                    </div>
                  )}
                </div>
              ))}
              {gen.effectsLayers.length === 0 && (
                <p className="text-[11px] text-muted-foreground">无特效层，点右侧「+」添加</p>
              )}
            </div>
          )}
        </div>

        <button onClick={handleGenerate} disabled={!gen.name.trim()}
          className="mt-2 w-full px-3 py-1.5 text-xs rounded bg-primary text-primary-foreground disabled:opacity-40 hover:opacity-90 transition-opacity">
          生成并应用
        </button>
      </div>

      {customThemes.length > 0 && (
        <div className="mb-3 space-y-1 max-h-[168px] overflow-y-auto pr-1 shrink-0">
          <div className="text-xs font-medium text-muted-foreground">已保存的主题</div>
          {customThemes.map(ct => (
            <div key={`${ct.name}__${ct.type}`}
              onClick={() => handleActivate(ct)}
              className={`flex items-center justify-between px-3 py-2 rounded-lg border text-sm cursor-pointer ${
                isActive(ct.name) ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted/50'
              }`}
            >
              <div className="flex items-center gap-2">
                <Palette className="w-4 h-4 text-muted-foreground" />
                <span className={isActive(ct.name) ? 'font-medium text-primary' : ''}>{ct.name}</span>
                <span className="text-[10px] text-muted-foreground bg-muted px-1.5 py-0.5 rounded">{ct.type}</span>
              </div>
              <button onClick={e => { e.stopPropagation(); handleDelete(ct.name) }}
                className="p-1 rounded text-muted-foreground hover:text-destructive hover:bg-muted transition-colors" title="删除">
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}

      <div className="flex-1 flex flex-col min-h-0">
        <div className="flex items-center justify-between mb-1">
          <span className="text-xs font-medium text-muted-foreground">主题 JSON</span>
          <button onClick={fillInitial} className="text-xs text-primary hover:underline">
            填入示例主题「墨绿书斋」
          </button>
        </div>
        <textarea
          value={json}
          onChange={e => { setJson(e.target.value); setError(''); setWarnings([]) }}
          placeholder='{"name": "我的主题", "type": "dark", "colors": {"--background": "#...", ...}, "effects": {...}}'
          className="flex-1 min-h-[200px] w-full text-xs font-mono rounded border bg-background p-3 resize-y focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          spellCheck={false}
        />
        {error && <p className="text-xs text-destructive mt-1">{error}</p>}
        {warnings.length > 0 && (
          <div className="mt-1 rounded border border-tag-amber bg-tag-amber px-2 py-1.5">
            <p className="text-[11px] font-medium text-tag-amber-foreground">对比度不足的配色对（建议调整，否则小字费眼）：</p>
            <p className="text-[11px] text-tag-amber-foreground/90">{warnings.join('；')}</p>
          </div>
        )}
        <div className="flex gap-2 mt-2">
          <button onClick={() => handleApply(json)} disabled={!json.trim()}
            className="px-3 py-1.5 text-xs rounded bg-primary text-primary-foreground disabled:opacity-40 hover:opacity-90 transition-opacity">
            应用
          </button>
          <button onClick={() => { setJson(''); setError(''); setWarnings([]) }}
            className="px-3 py-1.5 text-xs rounded border hover:bg-muted transition-colors">清空</button>
        </div>
      </div>
    </div>
  )
}
