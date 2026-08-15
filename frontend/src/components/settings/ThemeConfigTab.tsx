import { useEffect, useState } from 'react'
import { Palette, Trash2, Wand2, Layers, Gauge } from 'lucide-react'
import { useTheme, type CustomThemeData, type Finish } from '@/hooks/useTheme'
import { checkThemeContrast, generateTheme } from '@/lib/themeColors'

const FINISH_OPTIONS: { value: Finish; label: string; desc: string }[] = [
  { value: 'plain', label: '纯净', desc: '实色面板 + 中性阴影，最省性能' },
  { value: 'aura', label: '氛围光', desc: '多光源渐变 + 主色调投影 + 辉光 + 噪点' },
  { value: 'glass', label: '玻璃', desc: '面板半透明毛玻璃 + 细描边（局部 blur）' },
  { value: 'paper', label: '暖纸', desc: '纸纹纹理 + 柔和阴影 + 弱辉光' },
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
  "name": "太虚·夜",
  "type": "dark",
  "colors": {
    "--background": "#0a0e17", "--foreground": "#e8eef2", "--card": "rgba(15,21,31,0.72)",
    "--card-foreground": "#e8eef2", "--popover": "rgba(17,27,43,0.95)", "--popover-foreground": "#e8eef2",
    "--primary": "#a1c4d6", "--primary-foreground": "#0a0e17", "--secondary": "rgba(17,27,43,0.6)",
    "--secondary-foreground": "#d8e4ee", "--muted": "rgba(17,27,43,0.55)", "--muted-foreground": "#8aa8c0",
    "--accent": "rgba(161,196,214,0.1)", "--accent-foreground": "#e8eef2", "--destructive": "#d45a6a",
    "--destructive-foreground": "#f0f4f8", "--border": "rgba(161,196,214,0.16)", "--input": "rgba(17,27,43,0.6)",
    "--ring": "#a1c4d6", "--sidebar": "rgba(11,15,24,0.78)", "--sidebar-foreground": "#e8eef2",
    "--sidebar-primary": "#a1c4d6", "--sidebar-primary-foreground": "#0a0e17",
    "--sidebar-accent": "rgba(161,196,214,0.1)", "--sidebar-accent-foreground": "#d8e4ee",
    "--sidebar-border": "rgba(161,196,214,0.12)", "--sidebar-ring": "#a1c4d6",
    "--tag-blue": "#14242f", "--tag-blue-foreground": "#7ab0d5",
    "--tag-green": "#142a20", "--tag-green-foreground": "#6ac09a",
    "--tag-amber": "#2a2414", "--tag-amber-foreground": "#d0b060",
    "--tag-rose": "#2a1418", "--tag-rose-foreground": "#d08090",
    "--tag-teal": "#142a28", "--tag-teal-foreground": "#6ab8b0",
    "--tag-purple": "#241c30", "--tag-purple-foreground": "#a88ac8",
    "--reader-bg": "#0a0e17", "--reader-paper": "#121a28",
    "--bubble-user": "#a1c4d6", "--bubble-user-foreground": "#0a0e17",
    "--action-extract": "#4a6a80", "--action-extract-foreground": "#e8eef2",
    "--action-save": "#5a9a7a", "--action-save-foreground": "#0a0e17",
    "--success": "#12251a", "--success-foreground": "#6ac09a", "--success-border": "#244030",
    "--danger-bg": "#281416", "--danger-border": "#4a2024",
    "--status-warning": "#d0b060", "--status-ok": "#6ac09a",
    "--tool-blue": "#13222e", "--tool-blue-border": "#5a8ab5",
    "--tool-amber": "#282010", "--tool-amber-border": "#b59040",
    "--tool-green": "#12231a", "--tool-green-border": "#5a9a6a",
    "--tool-red": "#281416", "--tool-red-border": "#b55050",
    "--contribution-0": "#10161f", "--contribution-1": "#1a3a4a",
    "--contribution-2": "#2a5a70", "--contribution-3": "#3a7a90", "--contribution-4": "#4a9ab0"
  },
  "effects": {
    "layers": [
      { "type": "particles", "intensity": 0.4, "count": 70, "speed": 0.8 },
      { "type": "ambient", "intensity": 0.3 }
    ]
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
}

const GEN_KEY = 'goink_theme_gen'
const GEN_DEFAULT: GenForm = {
  name: '我的主题', mode: 'dark', bg: '#0a0e17', primary: '#a1c4d6', fg: '',
  vibrancy: 1,
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
  const { activeTheme: theme, setTheme, addCustomTheme, deleteCustomTheme, customThemes, finish, setFinish, lowFx, setLowFx } = useTheme()
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

      {/* ── 质感预设 ── */}
      <div className="mb-3 rounded-lg border border-border bg-card p-3">
        <div className="flex items-center gap-1.5 mb-2">
          <Layers className="w-3.5 h-3.5 text-primary" />
          <span className="text-xs font-medium text-foreground">质感</span>
          <span className="text-[11px] text-muted-foreground">（光影/材质/纹理，独立于配色，可任意组合）</span>
        </div>
        <div className="grid grid-cols-2 gap-1.5">
          {FINISH_OPTIONS.map(opt => (
            <button
              key={opt.value}
              onClick={() => setFinish(opt.value)}
              className={`text-left px-2.5 py-2 rounded-lg border transition-colors ${
                finish === opt.value ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted/50'
              }`}
            >
              <div className={`text-xs font-medium ${finish === opt.value ? 'text-primary' : 'text-foreground'}`}>{opt.label}</div>
              <div className="text-[10px] text-muted-foreground leading-snug mt-0.5">{opt.desc}</div>
            </button>
          ))}
        </div>
        <label className="mt-2 flex items-center gap-1.5 text-[11px] text-muted-foreground cursor-pointer select-none">
          <input type="checkbox" checked={lowFx} onChange={e => setLowFx(e.target.checked)}
            className="accent-[var(--primary)] cursor-pointer" />
          <Gauge className="w-3 h-3" />
          低性能模式（关闭毛玻璃与纹理，阴影降档；低配机器或大屏叙事面板建议开启）
        </label>
      </div>

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
            填入示例主题「太虚·夜」
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
