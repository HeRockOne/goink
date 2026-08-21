// ContextRing — SVG 圆环显示 token 用量 + 成本估算
import { useState, useEffect, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { GetSettings, SaveSettings } from '@/lib/wailsjs/go/app/App'

export interface UsageInfo {
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  prompt_cache_hit_tokens: number
  prompt_cache_miss_tokens: number
  acc_completion_tokens: number
  per_model?: Record<string, { hit: number; miss: number; comp: number }>
  running_tokens: Record<string, number>
  cache_hit_ratio: number
  context_window: number
  usage_ratio: number
  detail_is_estimate?: boolean
  overhead_tokens?: number
  detail: {
    system: number
    user: number
    assistant: number
    tool: number
  }
}

interface PriceConfig {
  priceInput: number
  priceOutput: number
  cachePrice: number
}

const DEFAULT_PRICES: PriceConfig = {
  priceInput: 1.35,
  priceOutput: 8.1,
  cachePrice: 0.27,
}

function computeCosts(usage: UsageInfo, prices: PriceConfig, selectedModel?: string) {
  let hit = usage.prompt_cache_hit_tokens || 0
  let miss = usage.prompt_cache_miss_tokens || 0
  let out = usage.acc_completion_tokens || 0

  // 如果传入了 selectedModel，只显示该模型的消耗
  if (selectedModel) {
    if (usage.per_model && usage.per_model[selectedModel]) {
      const m = usage.per_model[selectedModel]
      hit = m.hit || 0
      miss = m.miss || 0
      out = m.comp || 0
    } else {
      // 该模型尚未在本 session 中使用
      hit = 0
      miss = 0
      out = 0
    }
  }

  const hitCost = hit * prices.cachePrice / 1_000_000
  const missCost = miss * prices.priceInput / 1_000_000
  const outCost = out * prices.priceOutput / 1_000_000
  const totalCost = hitCost + missCost + outCost

  // 按模型拆分
  const modelCosts: Record<string, { hit: number; miss: number; comp: number; hitCost: number; missCost: number; compCost: number; total: number }> = {}
  if (usage.per_model) {
    for (const [modelID, data] of Object.entries(usage.per_model)) {
      const mh = data.hit || 0
      const mm = data.miss || 0
      const mc = data.comp || 0
      modelCosts[modelID] = {
        hit: mh, miss: mm, comp: mc,
        hitCost: mh * prices.cachePrice / 1_000_000,
        missCost: mm * prices.priceInput / 1_000_000,
        compCost: mc * prices.priceOutput / 1_000_000,
        total: mh * prices.cachePrice / 1_000_000 + mm * prices.priceInput / 1_000_000 + mc * prices.priceOutput / 1_000_000,
      }
    }
  }

  return { hitCost, missCost, outCost, totalCost, modelCosts }
}

function ringColor(ratio: number): string {
  if (ratio >= 90) return 'var(--usage-danger)'
  if (ratio >= 80) return 'var(--usage-warn)'
  return 'var(--usage-ok)'
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(0) + 'K'
  return String(n)
}

function formatCost(n: number): string {
  return '¥' + n.toFixed(4)
}

interface Props {
  usage: UsageInfo | null
  selectedModel?: string
  onCompress?: () => void
  isTurnRunning?: boolean
  isCompressing?: boolean
  bar?: boolean // 条状统计条（状态栏模式），默认圆环
}

export default function ContextRing({ usage, selectedModel, onCompress, isTurnRunning, isCompressing, bar }: Props) {
  const { t } = useTranslation()
  const [showPopover, setShowPopover] = useState(false)
  const [threshold, setThreshold] = useState(70)
  const [showPrices, setShowPrices] = useState(true)
  const [showRoles, setShowRoles] = useState(true)
  const [prices, setPrices] = useState<PriceConfig>(DEFAULT_PRICES)
  const thresholdLoaded = useRef(false)
  // 设置保存防抖：价格输入/阈值滑条每次 onChange 只更新本地 state，
  // 合并补丁 600ms 后一次性落库（旧实现每键击全量 SaveSettings 写一次 DB）
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pendingPatchRef = useRef<Partial<{ price_input: number; price_output: number; cache_price: number; compression_threshold: number }>>({})
  const saveSettingsDebounced = useCallback((patch: Partial<{ price_input: number; price_output: number; cache_price: number; compression_threshold: number }>) => {
    Object.assign(pendingPatchRef.current, patch)
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(() => {
      const merged = pendingPatchRef.current
      pendingPatchRef.current = {}
      saveTimerRef.current = null
      if (Object.keys(merged).length > 0) {
        SaveSettings(merged).catch(() => {})
      }
    }, 600)
  }, [])

  useEffect(() => {
    if (thresholdLoaded.current) return
    thresholdLoaded.current = true
    GetSettings().then(s => {
      if (s?.compression_threshold) setThreshold(Math.round(s.compression_threshold * 100))
      if (s?.price_input !== undefined) setPrices({
        priceInput: s.price_input,
        priceOutput: s.price_output,
        cachePrice: s.cache_price || 0,
      })
    }).catch(() => {})
  }, [])

  const DETAIL_LABELS: Record<string, string> = {
    system: t('chat.systemContext'),
    user: t('chat.userInput'),
    assistant: t('chat.aiOutput'),
    tool: t('chat.toolResult'),
  }
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const handleEnter = useCallback(() => {
    if (hideTimerRef.current) {
      clearTimeout(hideTimerRef.current)
      hideTimerRef.current = null
    }
    setShowPopover(true)
  }, [])

  const handleLeave = useCallback(() => {
    hideTimerRef.current = setTimeout(() => setShowPopover(false), 150)
  }, [])

  const hasUsage = usage && usage.context_window && usage.total_tokens
  const ratio = hasUsage ? Math.min(usage.usage_ratio, 100) : 0
  const r = 18
  const circumference = 2 * Math.PI * r
  const offset = circumference - (ratio / 100) * circumference
  const color = hasUsage ? ringColor(ratio) : 'var(--muted-foreground)'
  // 命中率数字颜色：随主题 usage 色系分级（深浅模式自动适配）
  const hitRatio = usage?.cache_hit_ratio ?? 0
  const hitColor = !hasUsage ? 'var(--muted-foreground)' : hitRatio >= 95 ? 'var(--usage-ok)' : hitRatio >= 80 ? 'var(--usage-warn)' : 'var(--usage-danger)'
  const costs = hasUsage ? computeCosts(usage, prices, selectedModel) : null

  return (
    <span
      className={`relative inline-flex items-center justify-center cursor-pointer shrink-0 select-none ${bar ? 'gap-1.5' : ''}`}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
    >
      {bar ? (
        <>
          {hasUsage && usage.cache_hit_ratio > 0 && (
            <span className="text-xs font-semibold tabular-nums" style={{ color: hitColor }}>
              命中率 {usage.cache_hit_ratio.toFixed(2)}%
            </span>
          )}
          <span className="text-xs font-semibold text-muted-foreground shrink-0">上下文</span>
          <span className="w-24 h-2.5 rounded-sm bg-muted border border-border overflow-hidden">
            <span
              className="h-full rounded-sm block transition-all duration-400"
              style={{ width: `${ratio}%`, backgroundColor: color }}
            />
          </span>
          <span className="text-xs font-semibold tabular-nums" style={{ color }}>
            {ratio.toFixed(2)}%
          </span>
        </>
      ) : (
        <>
          <svg width={44} height={44} viewBox="0 0 44 44">
            <circle cx={22} cy={22} r={r} fill="none" stroke="var(--border)" strokeWidth={3} />
            <circle
              cx={22} cy={22} r={r} fill="none"
              stroke={color}
              strokeWidth={3}
              strokeLinecap="round"
              strokeDasharray={circumference}
              strokeDashoffset={offset}
              transform="rotate(-90 22 22)"
              style={{ transition: 'stroke-dashoffset 0.4s ease, stroke 0.4s ease' }}
            />
          </svg>
          <span className="absolute text-[11px] font-semibold tabular-nums pointer-events-none" style={{ color }}>
            {ratio.toFixed(0)}%
          </span>
        </>
      )}

      {showPopover && (
        <div className="absolute bottom-full right-0 mb-2 z-50 flex flex-col gap-2.5 bg-background text-foreground rounded-xl p-3 min-w-[280px] shadow-lg border">
          {/* 标题行 */}
          <div className="flex gap-4 text-[13px] font-semibold">
            <span>{t('chat.contextUsage')}: {ratio.toFixed(1)}%</span>
            {hasUsage && usage.cache_hit_ratio > 0 && (
              <span>{t('chat.cacheHitRate')}: {usage.cache_hit_ratio.toFixed(1)}%</span>
            )}
          </div>

          {/* 进度条 */}
          <div className="h-1.5 rounded-sm bg-muted border border-border overflow-hidden">
            <div
              className="h-full rounded-sm transition-all duration-400"
              style={{ width: `${ratio}%`, backgroundColor: color }}
            />
          </div>

          {/* token 统计 */}
          <div className="text-xs text-muted-foreground">
            {t('chat.used')}: {hasUsage ? formatTokens(usage.total_tokens) : '0'}
            {hasUsage && <>{' · '}{t('chat.totalSize')}: {formatTokens(usage.context_window)}</>}
          </div>

{/* 上下文占比 */}
          {hasUsage && costs && (
            <div className="border-t-2 pt-2">
              <button
                className="flex justify-between items-center text-xs w-full text-left"
                onClick={() => setShowRoles(!showRoles)}
              >
                <span className="text-muted-foreground">上下文占比 {usage.detail_is_estimate ? '(估算)' : ''}</span>
                <span className="text-muted-foreground/60">{showRoles ? '▲' : '▼'}</span>
              </button>
              {showRoles && (
                <div className="flex flex-col gap-1 text-xs mt-1">
                  {Object.entries(DETAIL_LABELS).map(([key, label]) => (
                    <div key={key} className="flex justify-between">
                      <span className="text-muted-foreground">{label}</span>
                      <span className="tabular-nums">{formatTokens((usage.detail as any)[key] || 0)}</span>
                    </div>
                  ))}
                  {usage.overhead_tokens ? (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">格式开销</span>
                      <span className="tabular-nums">{formatTokens(usage.overhead_tokens)}</span>
                    </div>
                  ) : null}
                  <div className="text-[10px] text-muted-foreground/60 mt-0.5">
                     上下文窗口按角色分布，非计费分摊。系统为固定前缀精确值，其余为本地估算。
                   </div>
                </div>
              )}
            </div>
          )}

          {/* 成本估算 */}
          {hasUsage && costs && (
            <div className="border-t-2 pt-2">
              <div className="flex justify-between items-center text-xs mb-1">
                <span className="text-muted-foreground">💰 成本估算</span>
                <span className="font-semibold text-primary">{formatCost(costs.totalCost)}</span>
              </div>

              {/* token 明细 */}
              <div className="flex flex-col gap-1 text-xs">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">缓存读取</span>
                  <span className="tabular-nums">{formatTokens(usage.prompt_cache_hit_tokens || 0)}<span className="text-muted-foreground/60 ml-2">{formatCost(costs.hitCost)}</span></span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">未命中</span>
                  <span className="tabular-nums">{formatTokens(usage.prompt_cache_miss_tokens || 0)}<span className="text-muted-foreground/60 ml-2">{formatCost(costs.missCost)}</span></span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">输出</span>
                  <span className="tabular-nums">{formatTokens(usage.acc_completion_tokens || 0)}<span className="text-muted-foreground/60 ml-2">{formatCost(costs.outCost)}</span></span>
                </div>
              </div>
            </div>
          )}

          {/* 按模型 */}
          {hasUsage && costs && costs.modelCosts && Object.keys(costs.modelCosts).length > 0 && (
            <div className="border-t-2 pt-2">
              <div className="flex flex-col gap-1 text-xs">
                {Object.entries(costs.modelCosts).map(([modelID, mc]) => (
                  <div key={modelID} className="flex justify-between">
                    <span className="text-muted-foreground truncate max-w-[140px]" title={modelID}>{modelID}</span>
                    <span className="tabular-nums text-muted-foreground">
                      {formatTokens(mc.hit + mc.miss + mc.comp)}<span className="text-muted-foreground/60 ml-2">{formatCost(mc.total)}</span>
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* 价格配置 */}
          <div className="border-t-2 pt-2">
            <button
              className="flex justify-between items-center text-xs w-full text-left"
              onClick={() => setShowPrices(!showPrices)}
            >
              <span className="text-muted-foreground">价格配置</span>
              <span className="text-muted-foreground/60">{showPrices ? '▲' : '▼'}</span>
            </button>
            {showPrices && (
              <div className="grid grid-cols-3 gap-2 mt-2">
                <div className="text-center">
                  <div className="text-[10px] text-muted-foreground mb-0.5">输入 ¥/M</div>
                  <input
                    type="number"
                    value={prices.priceInput || ''}
                    onChange={e => {
                      const v = parseFloat(e.target.value) || 0
                      setPrices({ ...prices, priceInput: v })
                      saveSettingsDebounced({ price_input: v })
                    }}
                    placeholder="0"
                    className="w-full h-6 text-center text-xs border rounded bg-background px-1"
                    min={0} step={0.01}
                  />
                </div>
                <div className="text-center">
                  <div className="text-[10px] text-muted-foreground mb-0.5">输出 ¥/M</div>
                  <input
                    type="number"
                    value={prices.priceOutput || ''}
                    onChange={e => {
                      const v = parseFloat(e.target.value) || 0
                      setPrices({ ...prices, priceOutput: v })
                      saveSettingsDebounced({ price_output: v })
                    }}
                    placeholder="0"
                    className="w-full h-6 text-center text-xs border rounded bg-background px-1"
                    min={0} step={0.01}
                  />
                </div>
                <div className="text-center">
                  <div className="text-[10px] text-muted-foreground mb-0.5">缓存 ¥/M</div>
                  <input
                    type="number"
                    value={prices.cachePrice || ''}
                    onChange={e => {
                      const v = parseFloat(e.target.value) || 0
                      setPrices({ ...prices, cachePrice: v })
                      saveSettingsDebounced({ cache_price: v })
                    }}
                    placeholder="0"
                    className="w-full h-6 text-center text-xs border rounded bg-background px-1"
                    min={0} step={0.01}
                  />
                </div>
              </div>
            )}
          </div>

          {/* 压缩按钮 */}
          {onCompress && (
            <button
              className="w-full mt-1 py-1.5 rounded-lg text-xs font-medium border transition-colors
                disabled:opacity-40 disabled:cursor-not-allowed
                hover:bg-tag-amber hover:border-tag-amber-foreground/30 hover:text-tag-amber-foreground"
              disabled={isTurnRunning || isCompressing}
              onClick={(e) => { e.stopPropagation(); onCompress() }}
            >
              {isCompressing ? t('chat.compressing') : t('chat.compressContext')}
            </button>
          )}

          {/* 压缩阈值 */}
          <div className="border-t-2 pt-2 mt-1">
            <div className="flex justify-between items-center text-xs mb-1">
              <span className="text-muted-foreground">压缩阈值</span>
              <span className="tabular-nums font-medium">{threshold}%</span>
            </div>
            <input
              type="range"
              min={50} max={95} step={5}
              value={threshold}
              onChange={e => {
                const v = Number(e.target.value)
                setThreshold(v)
                saveSettingsDebounced({ compression_threshold: v / 100 })
              }}
              className="w-full h-1.5 rounded-full appearance-none cursor-pointer"
              style={{
                background: `linear-gradient(to right, var(--primary) 0%, var(--primary) ${((threshold - 50) / 45) * 100}%, var(--muted) ${((threshold - 50) / 45) * 100}%, var(--muted) 100%)`,
                border: '1px solid var(--border)',
              }}
            />
            <div className="flex justify-between text-[10px] text-muted-foreground mt-0.5">
              <span>50%</span>
              <span>太快</span>
              <span>95%</span>
            </div>
          </div>
        </div>
      )}
    </span>
  )
}
