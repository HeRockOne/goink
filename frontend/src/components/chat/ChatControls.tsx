import type { llm } from '@/hooks/useApp'
import { useTranslation } from 'react-i18next'
import { Brain } from 'lucide-react'
import PopSelect from './PopSelect'

interface Props {
  models: llm.AvailableModel[]
  selectedKey: string
  onSelectModel: (key: string) => void
  onRefreshModels?: () => void
  reasoningEffort: string
  onSelectEffort: (effort: string) => void
  thinkingEnabled: boolean
  approvalMode: 'manual' | 'auto'
  onToggleApproval: () => void
  onConfigModel: () => void
}

export default function ChatControls({
  models,
  selectedKey,
  onSelectModel,
  onRefreshModels,
  reasoningEffort,
  onSelectEffort,
  thinkingEnabled,
  approvalMode,
  onToggleApproval,
  onConfigModel,
}: Props) {
  const { t } = useTranslation()
  const selected = models.find(m => m.Key === selectedKey)
  const supportsThinking = selected?.SupportsThinking ?? false

  const modelOptions = models.map(m => ({ value: m.Key, label: m.ProviderName ? `${m.ProviderName} / ${m.ModelName}` : m.ModelName }))
  const levels = selected?.ReasoningLevels?.length
    ? selected.ReasoningLevels
    : supportsThinking ? ['low', 'high', 'max'] : []
  // 深度文本统一显示首字母大写（None/Low/Medium/High/Max），中英文一致不翻译
  const cap = (s: string) => s ? s[0].toUpperCase() + s.slice(1) : s

  return (
    <div className="flex items-center gap-1.5 px-4 py-2 text-xs shrink-0 select-none">
      <PopSelect
        value={selectedKey}
        options={modelOptions}
        onChange={onSelectModel}
        onOpen={onRefreshModels}
        footerAction={{ label: t('chat.configureModel'), onClick: onConfigModel }}
      />

      {supportsThinking && (
        <div className="relative shrink-0">
          <Brain className="w-3.5 h-3.5 absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none" />
          <PopSelect
            value={thinkingEnabled ? (reasoningEffort || levels[0] || 'high') : ''}
            options={[
              { value: '', label: t('chat.thinkingOff') },
              ...levels.map(level => ({
                value: level,
                label: `${t('chat.thinking')} · ${cap(level)}`,
              })),
            ]}
            onChange={onSelectEffort}
            minWidth="120px"
            className="[&>button]:pl-7"
          />
        </div>
      )}

      <div className="flex-1" />

      <button
        onClick={onToggleApproval}
        className={`h-[30px] rounded-lg border px-2.5 text-xs transition-colors shrink-0 ${
          approvalMode === 'auto'
            ? 'bg-primary/10 text-primary border-primary/30'
            : 'text-muted-foreground'
        }`}
      >
        {t('chat.auto')}
      </button>
    </div>
  )
}
