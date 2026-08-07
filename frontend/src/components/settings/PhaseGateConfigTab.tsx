import { useState, useEffect, useCallback } from 'react'
import { Shield } from 'lucide-react'
import { SaveSettings, GetSettings, SetPhaseGateEnabled } from '@/lib/wailsjs/go/app/App'
import { useTranslation } from 'react-i18next'

export default function PhaseGateConfigTab() {
  const { t } = useTranslation()
  const [config, setConfig] = useState('')
  const [phaseGateEnabled, setPhaseGateEnabled] = useState(true)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')

  useEffect(() => {
    GetSettings().then(s => {
      setConfig(s?.phase_gate_config || '')
      if (s?.phase_gate_enabled !== undefined && s?.phase_gate_enabled !== null) {
        setPhaseGateEnabled(s.phase_gate_enabled as boolean)
      }
    }).catch(() => {})
  }, [])

  async function handleSave() {
    setSaving(true)
    setMsg('')
    try {
      await SaveSettings({ phase_gate_config: config })
      setMsg('已保存')
    } catch (e) {
      setMsg('保存失败: ' + String(e))
    }
    setSaving(false)
  }

  const handlePhaseGateToggle = useCallback(async () => {
    const newValue = !phaseGateEnabled
    setPhaseGateEnabled(newValue)
    try {
      await SetPhaseGateEnabled(newValue)
    } catch {
      setPhaseGateEnabled(!newValue)
    }
  }, [phaseGateEnabled])

  return (
    <div className="flex flex-col h-full">
      <div className="mb-4 pb-4 border-b">
        <div className="flex items-center gap-2">
          <Shield className="w-4 h-4" />
          <span className="text-sm font-medium">{t('settings.phaseGate')}</span>
          <button
            onClick={handlePhaseGateToggle}
            aria-pressed={phaseGateEnabled}
            className={`relative inline-flex h-5 w-9 items-center rounded-full border transition-colors shadow-inner ${
              phaseGateEnabled
                ? 'bg-primary border-primary shadow-[0_0_6px_var(--glow)]'
                : 'bg-muted-foreground/25 border-border'
            }`}
          >
            <span
              className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow-[0_1px_3px_rgba(0,0,0,0.35)] transition-transform ${
                phaseGateEnabled ? 'translate-x-[18px]' : 'translate-x-[2px]'
              }`}
            />
          </button>
        </div>
        <p className="text-xs text-muted-foreground mt-1">{t('settings.phaseGateDesc')}</p>
      </div>

      <p className="text-xs text-muted-foreground mb-2">
        此配置存储在数据库，仅门禁代码读取，不占用 AI 上下文 token。
        AI 可通过 <code className="text-xs bg-muted px-1 rounded">update_phase_gate_config</code> 工具编辑。
      </p>
      <div className="flex-1 flex flex-col min-h-0">
        <textarea
          value={config}
          onChange={e => { setConfig(e.target.value); setMsg('') }}
          className="flex-1 min-h-[280px] w-full text-xs font-mono rounded border bg-background p-3 resize-y focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          spellCheck={false}
          placeholder='<!-- phase-gate-config
mode: single
phase: prepare
tools: get_chapter_list, read, ...
require: ...
next: outline
-->'
        />
        {msg && <p className="text-xs text-muted-foreground mt-1">{msg}</p>}
        <div className="flex gap-2 mt-2">
          <button onClick={handleSave} disabled={saving}
            className="px-3 py-1.5 text-xs rounded bg-primary text-primary-foreground disabled:opacity-40 hover:opacity-90 transition-opacity">
            {saving ? '保存中...' : '保存配置'}
          </button>
        </div>
      </div>
    </div>
  )
}
