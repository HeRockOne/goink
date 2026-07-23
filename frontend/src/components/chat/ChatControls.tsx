import type { llm } from '@/hooks/useApp'
import { useTranslation } from 'react-i18next'
import { Brain } from 'lucide-react'
import PopSelect from './PopSelect'
import ContextRing from './ContextRing'
import type { UsageInfo } from './ContextRing'

interface Props {
  models: llm.AvailableModel[]
  selectedKey: string
  onSelectModel: (key: string) => void
  onRefreshModels?: () => void
  reasoningEffort: string
  onSelectEffort: (effort: string) => void
  thinkingEnabled: boolean
  onToggleThinking: () => void
  approvalMode: 'manual' | 'auto'
  onToggleApproval: () => void
  onConfigModel: () => void
  usage: UsageInfo | null
  onCompress?: () => void
  isTurnRunning?: boolean
  isCompressing?: boolean
}

export default function ChatControls({
  models,
  selectedKey,
  onSelectModel,
  onRefreshModels,
  reasoningEffort,
  onSelectEffort,
  thinkingEnabled,
  onToggleThinking,
  approvalMode,
  onToggleApproval,
  onConfigModel,
  usage,
  onCompress,
  isTurnRunning,
  isCompressing,
}: Props) {
  const { t } = useTranslation()
  const selected = models.find(m => m.Key === selectedKey)
  const supportsThinking = selected?.SupportsThinking ?? false

  const modelOptions = models.map(m => ({ value: m.Key, label: m.ProviderName ? `${m.ProviderName} / ${m.ModelName}` : m.ModelName }))
  const levels = selected?.ReasoningLevels?.length
    ? selected.ReasoningLevels
    : supportsThinking ? ['low', 'high', 'max'] : []
  const reasoningOptions = supportsThinking
    ? [
        { value: '', label: t('chat.thinkingOff') },
        ...levels.map(level => ({
          value: level,
          label: level === 'low' ? t('chat.lowReasoning') : level === 'high' ? t('chat.highReasoning') : level === 'max' ? t('chat.maxReasoning') : level,
        })),
      ]
    : []

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
        <>
          <button
            onClick={onToggleThinking}
            className={`h-[30px] rounded-lg border px-2 flex items-center gap-1 transition-colors shrink-0 ${
              thinkingEnabled
                ? 'bg-primary/10 text-primary border-primary/30'
                : 'text-muted-foreground'
            }`}
            title={thinkingEnabled ? t('chat.thinkingEnabled') : t('chat.thinkingDisabled')}
          >
            <Brain className="w-3.5 h-3.5" />
            <span>{t('chat.thinking')}</span>
          </button>
          {thinkingEnabled && (
            <PopSelect
              value={reasoningEffort}
              options={reasoningOptions}
              onChange={onSelectEffort}
              minWidth="80px"
            />
          )}
        </>
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

      <ContextRing usage={usage} onCompress={onCompress} isTurnRunning={isTurnRunning} isCompressing={isCompressing} />
    </div>
  )
}
