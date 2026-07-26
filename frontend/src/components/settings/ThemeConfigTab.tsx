import { useState } from 'react'
import { Palette, Trash2 } from 'lucide-react'
import { useTheme, type CustomThemeData } from '@/hooks/useTheme'

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

  function isActive(name: string) { return theme === `custom:${name}` }

  function handleApply(text: string) {
    setError('')
    const result = validateJSON(text)
    if (result.ok) {
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

  return (
    <div className="flex flex-col h-full">
      <h3 className="text-sm font-medium mb-2">自定义主题</h3>
      <p className="text-xs text-muted-foreground mb-3">
        粘贴包含全部 CSS 变量的完整主题 JSON。单击主题名即应用。
      </p>

      {customThemes.length > 0 && (
        <div className="mb-3 space-y-1">
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
          onChange={e => { setJson(e.target.value); setError('') }}
          placeholder='{"name": "我的主题", "type": "dark", "colors": {"--background": "#...", ...}}'
          className="flex-1 min-h-[200px] w-full text-xs font-mono rounded border bg-background p-3 resize-y focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          spellCheck={false}
        />
        {error && <p className="text-xs text-destructive mt-1">{error}</p>}
        <div className="flex gap-2 mt-2">
          <button onClick={() => handleApply(json)} disabled={!json.trim()}
            className="px-3 py-1.5 text-xs rounded bg-primary text-primary-foreground disabled:opacity-40 hover:opacity-90 transition-opacity">
            应用
          </button>
          <button onClick={() => { setJson(''); setError('') }}
            className="px-3 py-1.5 text-xs rounded border hover:bg-muted transition-colors">清空</button>
        </div>
      </div>
    </div>
  )
}
