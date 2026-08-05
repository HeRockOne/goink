import { useState } from 'react'
import { Palette, Trash2, Wand2 } from 'lucide-react'
import { useTheme, type CustomThemeData } from '@/hooks/useTheme'
import { checkThemeContrast, generateTheme } from '@/lib/themeColors'

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

export default function ThemeConfigTab() {
  const { activeTheme: theme, setTheme, addCustomTheme, deleteCustomTheme, customThemes } = useTheme()
  const [json, setJson] = useState('')
  const [error, setError] = useState('')
  const [warnings, setWarnings] = useState<string[]>([])

  // 生成器表单
  const [genName, setGenName] = useState('我的主题')
  const [genMode, setGenMode] = useState<'light' | 'dark'>('dark')
  const [genBg, setGenBg] = useState('#0f1a14')
  const [genPrimary, setGenPrimary] = useState('#5a9a6a')
  const [genFg, setGenFg] = useState('')
  const [genVibrancy, setGenVibrancy] = useState(1)

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
      name: genName, mode: genMode, background: genBg, primary: genPrimary,
      foreground: genFg || undefined, vibrancy: genVibrancy,
    })
    const data: CustomThemeData = { name: genName, type: genMode, colors }
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
        选背景色 + 主色一键生成全套主题（自动保证文字对比度 ≥ 4.5:1）；或粘贴包含全部 CSS 变量的主题 JSON。未填的变量会自动派生，不会撞色。
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
              value={genName}
              onChange={e => setGenName(e.target.value)}
              className="rounded border bg-background px-2 py-1 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            模式
            <select
              value={genMode}
              onChange={e => setGenMode(e.target.value as 'light' | 'dark')}
              className="rounded border bg-background px-2 py-1 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="light">浅色</option>
              <option value="dark">深色</option>
            </select>
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            背景色
            <div className="flex items-center gap-2">
              <input type="color" value={genBg} onChange={e => setGenBg(e.target.value)}
                className="h-7 w-10 rounded border border-border bg-transparent cursor-pointer" />
              <input value={genBg} onChange={e => setGenBg(e.target.value)}
                className="flex-1 min-w-0 rounded border bg-background px-2 py-1 text-xs font-mono text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" spellCheck={false} />
            </div>
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            主色
            <div className="flex items-center gap-2">
              <input type="color" value={genPrimary} onChange={e => setGenPrimary(e.target.value)}
                className="h-7 w-10 rounded border border-border bg-transparent cursor-pointer" />
              <input value={genPrimary} onChange={e => setGenPrimary(e.target.value)}
                className="flex-1 min-w-0 rounded border bg-background px-2 py-1 text-xs font-mono text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" spellCheck={false} />
            </div>
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            文字色（可选，留空自动）
            <div className="flex items-center gap-2">
              <input type="color" value={genFg || '#000000'} onChange={e => setGenFg(e.target.value)}
                className="h-7 w-10 rounded border border-border bg-transparent cursor-pointer" />
              <input value={genFg} onChange={e => setGenFg(e.target.value)} placeholder="自动"
                className="flex-1 min-w-0 rounded border bg-background px-2 py-1 text-xs font-mono text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" spellCheck={false} />
              {genFg && (
                <button onClick={() => setGenFg('')} className="shrink-0 text-[11px] text-muted-foreground hover:text-foreground">清除</button>
              )}
            </div>
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
            鲜艳度
            <div className="flex items-center gap-2">
              <input type="range" min={0.5} max={1.5} step={0.05} value={genVibrancy}
                onChange={e => setGenVibrancy(Number(e.target.value))}
                className="flex-1 accent-[var(--primary)] cursor-pointer" />
              <span className="w-9 shrink-0 text-right text-xs font-mono text-foreground">{genVibrancy.toFixed(2)}×</span>
            </div>
          </label>
        </div>
        <button onClick={handleGenerate} disabled={!genName.trim()}
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
          placeholder='{"name": "我的主题", "type": "dark", "colors": {"--background": "#...", ...}}'
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
