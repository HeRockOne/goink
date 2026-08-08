import { useState, useEffect, useCallback } from 'react'
import { Shield, ChevronDown, RotateCcw, CheckCircle2, AlertTriangle, XCircle } from 'lucide-react'
import { SaveSettings, GetSettings, SetPhaseGateEnabled, RestoreDefaultPhaseGateConfig, ValidatePhaseGateConfig } from '@/lib/wailsjs/go/app/App'
import type { agent } from '@/lib/wailsjs/go/models'
import { useTranslation } from 'react-i18next'

export default function PhaseGateConfigTab() {
  const { t } = useTranslation()
  const [config, setConfig] = useState('')
  const [phaseGateEnabled, setPhaseGateEnabled] = useState(true)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')
  const [issues, setIssues] = useState<agent.ValidationIssue[] | null>(null)
  const [validating, setValidating] = useState(false)

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

  async function handleRestoreDefault() {
    setMsg('')
    try {
      await RestoreDefaultPhaseGateConfig()
      const s = await GetSettings()
      setConfig(s?.phase_gate_config || '')
      setIssues(null)
      setMsg('已恢复出厂默认配置')
    } catch (e) {
      setMsg('恢复失败: ' + String(e))
    }
  }

  async function handleValidate() {
    setValidating(true)
    setMsg('')
    try {
      const result = await ValidatePhaseGateConfig(config)
      setIssues(result ?? [])
    } catch (e) {
      setMsg('校验失败: ' + String(e))
    }
    setValidating(false)
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
        <p className="text-xs text-muted-foreground mt-1">
          门禁按「准备 → 大纲 → 正文 → 审读 → 维护」强制 AI 依序创作，出厂配置已按创作流程设计好，普通用户无需修改。
        </p>
      </div>

      {/* 高级配置（默认收起，避免普通用户面对配置代码） */}
      <button
        onClick={() => setShowAdvanced(!showAdvanced)}
        className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors self-start mb-2 cursor-pointer"
      >
        <ChevronDown className={`w-3.5 h-3.5 transition-transform ${showAdvanced ? 'rotate-180' : ''}`} />
        高级配置（阶段白名单 / 必读技能 / 流转规则）
      </button>

      {showAdvanced && (
        <div className="flex-1 flex flex-col min-h-0">
          <p className="text-xs text-muted-foreground mb-2">
            此配置存储在数据库，仅门禁代码读取，不占用 AI 上下文 token。
            AI 可通过 <code className="text-xs bg-muted px-1 rounded">update_phase_gate_config</code> 工具编辑。
            字段说明与设计指南见 <code className="text-xs bg-muted px-1 rounded">docs/architecture/phase-gate.md</code>「配置设计指南」。
          </p>
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

          {/* 校验结果 */}
          {issues && issues.length > 0 && (
            <div className="mt-2 space-y-1 max-h-40 overflow-y-auto">
              {issues.map((it, i) => (
                <div key={i} className={`flex items-start gap-1.5 text-[11px] leading-tight rounded px-2 py-1 ${
                  it.level === 'error' ? 'bg-destructive/10 text-destructive' : 'bg-status-warning/10 text-status-warning'
                }`}>
                  {it.level === 'error'
                    ? <XCircle className="w-3 h-3 shrink-0 mt-0.5" />
                    : <AlertTriangle className="w-3 h-3 shrink-0 mt-0.5" />}
                  <span>
                    <span className="font-medium">{it.mode}·{it.phase}:</span> {it.message}
                  </span>
                </div>
              ))}
            </div>
          )}
          {issues && issues.length === 0 && (
            <p className="flex items-center gap-1.5 text-xs text-success-foreground mt-2">
              <CheckCircle2 className="w-3.5 h-3.5" />
              配置有效，无问题
            </p>
          )}

          <div className="flex gap-2 mt-2">
            <button onClick={handleSave} disabled={saving}
              className="px-3 py-1.5 text-xs rounded bg-primary text-primary-foreground disabled:opacity-40 hover:opacity-90 transition-opacity">
              {saving ? '保存中...' : '保存配置'}
            </button>
            <button onClick={handleValidate} disabled={validating}
              className="px-3 py-1.5 text-xs rounded border border-border hover:bg-muted transition-colors">
              {validating ? '校验中...' : '校验配置'}
            </button>
            <button onClick={handleRestoreDefault}
              className="px-3 py-1.5 text-xs rounded border border-border hover:bg-muted transition-colors flex items-center gap-1">
              <RotateCcw className="w-3 h-3" />
              恢复出厂默认
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
