import { useState, useCallback, useEffect, useRef } from 'react'
import { Activity, Play, Loader2, TrendingUp } from 'lucide-react'
import { StartCacheSimulation } from '@/lib/wailsjs/go/app/App'
import { EventsOn } from '@/lib/wailsjs/runtime/runtime'

// 模拟结果类型（后端 app.CacheSimResult 通过事件推送，未生成 wails 模型，本地定义）
interface WindowSimMark {
  threshold: number
  reached: boolean
  hit: number
  miss: number
  out: number
  requests: number
  chapter: number
  cost: number
  hit_rate: number
  interval_chapters: number
  interval_cost: number
  interval_per_chapter: number
}

// 阶段打点快照（mixed 模式：开书/短对话/单章轮/批量轮每阶段结束）
interface CacheSimStage {
  stage: string
  chapter: number
  total: number
  requests: number
  cost: number
  hit_rate: number
  interval_cost: number
  interval_chapters: number
  interval_per_chapter: number
}

interface CacheSimResult {
  mode: string
  label: string
  total_hit: number
  total_miss: number
  total_out: number
  hit_rate: number
  cost: number
  marks: WindowSimMark[]
  final_total: number
  final_reqs: number
  final_cost: number
  final_hit_rate: number
  best_interval: string
  best_per_chapter: number
  stages: CacheSimStage[]
}

// 格式化为 M（百万）单位
function fmtM(tokens: number): string {
  const m = tokens / 1_000_000
  if (m >= 100) return `${m.toFixed(0)}M`
  if (m >= 10) return `${m.toFixed(1)}M`
  return `${m.toFixed(2)}M`
}

type Mode = 'single' | 'batch' | 'mixed'

const modeDefs: Record<Mode, { label: string; hint: string }> = {
  single: { label: '单章', hint: '每章完整门禁逐章累积，1M 窗口约 25 章' },
  batch: { label: '批量', hint: '每批 6 章完整批量门禁批次循环，1M 窗口约 109 章' },
  mixed: { label: '混合', hint: '短对话穿插在单章/批量创作之间（真实使用方式）' },
}

// NumInput 数字输入框：允许清空后直接输入（本地字符串 state），blur 时校验回填。
function NumInput(props: {
  value: number
  onChange: (n: number) => void
  min?: number
  max?: number
  label: string
  hint?: string
}) {
  const { value, onChange, min = 0, max = 999, label, hint } = props
  const [text, setText] = useState(String(value))

  // 外部值变化（切换模式/重置）时同步
  useEffect(() => { setText(String(value)) }, [value])

  const commit = () => {
    const n = Number(text)
    if (text === '' || Number.isNaN(n)) {
      setText(String(value))
      return
    }
    const clamped = Math.max(min, Math.min(max, n))
    setText(String(clamped))
    onChange(clamped)
  }

  return (
    <label className="flex flex-col gap-1 text-xs text-muted-foreground">
      <span className="flex items-center gap-1">
        {label}
        {hint && <span className="text-[10px] text-muted-foreground/70">{hint}</span>}
      </span>
      <input
        type="number"
        min={min}
        max={max}
        value={text}
        onChange={e => {
          setText(e.target.value)
          const n = Number(e.target.value)
          if (e.target.value !== '' && !Number.isNaN(n)) {
            onChange(Math.max(min, Math.min(max, n)))
          }
        }}
        onBlur={commit}
        onKeyDown={e => { if (e.key === 'Enter') commit() }}
        className="w-24 px-2 py-1.5 rounded border bg-background text-sm text-foreground"
      />
    </label>
  )
}

// 缓存模拟 Tab：估算一个真实对话窗口写书的 token 消耗与费用。
// 模式 = 窗口内的工作负载：单章逐章 / 批量批次循环 / 混合会话。
// 每个模式输出：该窗口总成本 + 上下文窗口刻度（历史增长到 128K/256K/512K/1024K
// 时的累计成本快照与区间每章成本，找最省区间）。
export default function CacheSimTab() {
  const [mode, setMode] = useState<Mode>('mixed')
  // 输入（按模式使用）
  const [singleChapters, setSingleChapters] = useState(26)
  const [batchChapters, setBatchChapters] = useState(120)
  const [gateRounds, setGateRounds] = useState(3)
  const [qaRounds, setQaRounds] = useState(5)
  const [mixedBatch, setMixedBatch] = useState(5)
  const [batchRounds, setBatchRounds] = useState(3)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<CacheSimResult | null>(null)
  const [error, setError] = useState('')
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    const cleanup = EventsOn('cachesim:done', (data: CacheSimResult) => {
      if (!mountedRef.current) return
      setRunning(false)
      if (data.cost < 0) {
        setError('模拟失败')
        return
      }
      setResult(data)
    })
    return () => { mountedRef.current = false; cleanup() }
  }, [])

  const run = useCallback(async () => {
    setRunning(true)
    setError('')
    try {
      if (mode === 'single') {
        await StartCacheSimulation('single', singleChapters, 0, 0, 1)
      } else if (mode === 'batch') {
        await StartCacheSimulation('batch', 0, 0, batchChapters, 1)
      } else {
        await StartCacheSimulation('mixed', gateRounds, qaRounds, mixedBatch, batchRounds)
      }
    } catch (e) {
      setError(String(e))
      setRunning(false)
    }
  }, [mode, singleChapters, batchChapters, gateRounds, qaRounds, mixedBatch, batchRounds])

  const modeBtn = (m: Mode) => (
    <button
      key={m}
      onClick={() => setMode(m)}
      className={`px-3 py-1.5 rounded border text-sm transition-colors ${mode === m ? 'bg-primary text-primary-foreground' : 'bg-background hover:bg-muted'}`}
    >
      {modeDefs[m].label}
    </button>
  )

  return (
    <div className="flex flex-col h-full gap-4">
      <div className="mb-1 pb-3 border-b">
        <div className="flex items-center gap-2">
          <Activity className="w-4 h-4" />
          <span className="text-sm font-medium">写书成本模拟（含上下文刻度）</span>
        </div>
        <p className="text-xs text-muted-foreground mt-1">
          模拟一个真实对话窗口的历史 Token 消耗与费用。窗口内的工作负载可选：
          单章逐章、批量批次循环、或混合会话（短对话穿插）。必读技能由系统自动注入
          （auto_skill_injection），历史留在服务端缓存；正文长度与思考过程按真实分布生成
          （分阶段 thinking），费用 = 输入命中 × 缓存价 + 输入未命中 × 输入价 + 输出 × 输出价
          （设置页模型价格）。下方同时给出窗口刻度：历史增长到 128K/256K/512K/1024K
          时的累计成本与区间每章成本，找最省区间。
        </p>
      </div>

      {/* 模式选择行 */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs text-muted-foreground mr-1">模式</span>
        {(Object.keys(modeDefs) as Mode[]).map(modeBtn)}
        <span className="text-[10px] text-muted-foreground/70 ml-1">{modeDefs[mode].hint}</span>
      </div>

      {/* 参数行（按模式变化，与开始模拟按钮同一水平线） */}
      <div className="flex items-end gap-3 flex-nowrap overflow-x-auto pb-1">
        {mode === 'single' && (
          <NumInput value={singleChapters} onChange={setSingleChapters} min={1} max={200} label="章数" hint="默认 26 ≈ 1M" />
        )}
        {mode === 'batch' && (
          <NumInput value={batchChapters} onChange={setBatchChapters} min={6} max={400} label="章数" hint="默认 120 ≈ 1M" />
        )}
        {mode === 'mixed' && (
          <>
            <NumInput value={gateRounds} onChange={setGateRounds} min={0} max={20} label="单章轮数" hint="0=不跑" />
            <NumInput value={qaRounds} onChange={setQaRounds} min={0} max={20} label="短对话轮数" hint="穿插创作间" />
            <NumInput value={mixedBatch} onChange={setMixedBatch} min={0} max={20} label="每批章数" hint="每批完整门禁" />
            <NumInput value={batchRounds} onChange={setBatchRounds} min={1} max={20} label="批量轮数" hint="章号顺延" />
          </>
        )}

        <button
          onClick={run}
          disabled={running}
          className="shrink-0 px-4 py-1.5 rounded bg-primary text-primary-foreground text-sm flex items-center gap-1.5 disabled:opacity-50 hover:opacity-90 transition-opacity"
        >
          {running ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
          {running ? '模拟中...' : '开始模拟'}
        </button>
      </div>

      {error && <p className="text-xs text-red-500">{error}</p>}

      {result && (
        <div className="flex-1 min-h-0 overflow-y-auto space-y-4">
          <div className="rounded-lg border p-3">
            <div className="text-sm font-medium mb-2">{result.label}</div>
            <table className="w-full text-xs">
              <thead>
                <tr className="text-muted-foreground">
                  <th className="text-left py-1 font-normal">输入 hit</th>
                  <th className="text-right py-1 font-normal">输入 miss</th>
                  <th className="text-right py-1 font-normal">输出 out</th>
                  <th className="text-right py-1 font-normal">命中率</th>
                  <th className="text-right py-1 font-normal">总成本 ¥</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td className="py-1 tabular-nums">{fmtM(result.total_hit)}</td>
                  <td className="py-1 text-right tabular-nums">{fmtM(result.total_miss)}</td>
                  <td className="py-1 text-right tabular-nums">{fmtM(result.total_out)}</td>
                  <td className="py-1 text-right tabular-nums">{result.hit_rate.toFixed(1)}%</td>
                  <td className="py-1 text-right tabular-nums">{result.cost.toFixed(4)}</td>
                </tr>
              </tbody>
            </table>
          </div>

          {/* ── 混合模式：阶段轮次表（按创作阶段边界打点）── */}
          {result.mode === 'mixed' && result.stages && result.stages.length > 0 && (
            <div className="rounded-lg border p-3">
              <div className="flex items-center gap-2 mb-2">
                <TrendingUp className="w-4 h-4 text-muted-foreground" />
                <span className="text-sm font-medium">阶段轮次成本（每阶段结束快照）</span>
              </div>
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-muted-foreground">
                    <th className="text-left py-1 font-normal">阶段</th>
                    <th className="text-right py-1 font-normal">累计章</th>
                    <th className="text-right py-1 font-normal">历史</th>
                    <th className="text-right py-1 font-normal">累计成本 ¥</th>
                    <th className="text-right py-1 font-normal">区间增量 ¥</th>
                    <th className="text-right py-1 font-normal">区间每章 ¥</th>
                    <th className="text-right py-1 font-normal">请求</th>
                    <th className="text-right py-1 font-normal">命中率</th>
                  </tr>
                </thead>
                <tbody>
                  {result.stages.map((s, i) => (
                    <tr key={i} className="border-t border-border/50">
                      <td className="py-1.5">{s.stage}</td>
                      <td className="py-1.5 text-right tabular-nums">
                        {s.stage === '开书完成' || s.stage.startsWith('短对话') ? '-' : s.chapter}
                      </td>
                      <td className="py-1.5 text-right tabular-nums">{(s.total / 1000).toFixed(0)}K</td>
                      <td className="py-1.5 text-right tabular-nums">{s.cost.toFixed(4)}</td>
                      <td className="py-1.5 text-right tabular-nums">
                        {i > 0 ? `+${s.interval_cost.toFixed(4)}` : '-'}
                      </td>
                      <td className="py-1.5 text-right tabular-nums">
                        {s.interval_per_chapter > 0 ? `¥${s.interval_per_chapter.toFixed(4)}` : '-'}
                      </td>
                      <td className="py-1.5 text-right tabular-nums">{s.requests}</td>
                      <td className="py-1.5 text-right tabular-nums">{s.hit_rate.toFixed(1)}%</td>
                    </tr>
                  ))}
                  <tr className="border-t border-border/50 text-muted-foreground">
                    <td className="py-1.5">终点（共 {result.stages[result.stages.length - 1].chapter} 章）</td>
                    <td className="py-1.5 text-right tabular-nums">{result.stages[result.stages.length - 1].chapter}</td>
                    <td className="py-1.5 text-right tabular-nums">{(result.final_total / 1000).toFixed(0)}K</td>
                    <td className="py-1.5 text-right tabular-nums">{result.final_cost.toFixed(4)}</td>
                    <td className="py-1.5 text-right">-</td>
                    <td className="py-1.5 text-right">-</td>
                    <td className="py-1.5 text-right tabular-nums">{result.final_reqs}</td>
                    <td className="py-1.5 text-right tabular-nums">{result.final_hit_rate.toFixed(1)}%</td>
                  </tr>
                </tbody>
              </table>
              <p className="text-[10px] text-muted-foreground/70 mt-2">
                按创作阶段打点：开书 → 短对话（查/改设定）→ 单章轮 → 批量轮，每阶段结束记录
                累计成本与区间增量（区间每章 = 该阶段增量成本 ÷ 新增章数）。章号连续顺延，
                短对话/开书不产章，区间每章显示 -。
              </p>
            </div>
          )}

          {/* ── 上下文窗口刻度（单窗口成本曲线，single/batch 模式）── */}
          {result.mode !== 'mixed' && (
          <div className="rounded-lg border p-3">
            <div className="flex items-center gap-2 mb-2">
              <TrendingUp className="w-4 h-4 text-muted-foreground" />
              <span className="text-sm font-medium">上下文窗口刻度（单窗口成本曲线）</span>
              {result.best_interval && (
                <span className="text-xs text-primary">
                  最省区间：{result.best_interval}（每章 ¥{result.best_per_chapter.toFixed(4)}）
                </span>
              )}
            </div>
            <table className="w-full text-xs">
              <thead>
                <tr className="text-muted-foreground">
                  <th className="text-left py-1 font-normal">刻度</th>
                  <th className="text-right py-1 font-normal">到达时</th>
                  <th className="text-right py-1 font-normal">累计成本 ¥</th>
                  <th className="text-right py-1 font-normal">累计 miss</th>
                  <th className="text-right py-1 font-normal">累计 hit</th>
                  <th className="text-right py-1 font-normal">请求</th>
                  <th className="text-right py-1 font-normal">命中率</th>
                  <th className="text-right py-1 font-normal">区间每章 ¥</th>
                </tr>
              </thead>
              <tbody>
                {result.marks.map(m => (
                  <tr key={m.threshold} className="border-t border-border/50">
                    <td className="py-1.5 tabular-nums">{m.threshold / 1024}K</td>
                    <td className="py-1.5 text-right tabular-nums">
                      {m.reached ? `第 ${m.chapter} 章` : '未到达'}
                    </td>
                    <td className="py-1.5 text-right tabular-nums">{m.reached ? m.cost.toFixed(4) : '-'}</td>
                    <td className="py-1.5 text-right tabular-nums">{m.reached ? `${(m.miss / 1000).toFixed(0)}K` : '-'}</td>
                    <td className="py-1.5 text-right tabular-nums">{m.reached ? fmtM(m.hit) : '-'}</td>
                    <td className="py-1.5 text-right tabular-nums">{m.reached ? m.requests : '-'}</td>
                    <td className="py-1.5 text-right tabular-nums">{m.reached ? `${m.hit_rate.toFixed(1)}%` : '-'}</td>
                    <td className="py-1.5 text-right tabular-nums">
                      {m.interval_per_chapter > 0 ? `¥${m.interval_per_chapter.toFixed(4)}` : '-'}
                    </td>
                  </tr>
                ))}
                <tr className="border-t border-border/50 text-muted-foreground">
                  <td className="py-1.5">终点（历史 {(result.final_total / 1000).toFixed(0)}K）</td>
                  <td className="py-1.5 text-right">{result.final_reqs} 请求</td>
                  <td className="py-1.5 text-right tabular-nums">{result.final_cost.toFixed(4)}</td>
                  <td className="py-1.5 text-right tabular-nums">-</td>
                  <td className="py-1.5 text-right tabular-nums">-</td>
                  <td className="py-1.5 text-right">-</td>
                  <td className="py-1.5 text-right tabular-nums">{result.final_hit_rate.toFixed(1)}%</td>
                  <td className="py-1.5 text-right">-</td>
                </tr>
              </tbody>
            </table>
            <p className="text-[10px] text-muted-foreground/70 mt-2">
              单窗口内历史增长到 128K/256K/512K/1024K 时的累计成本快照与区间每章成本。
              单章/批量模式按长窗口跑（默认 26/120 章到 1M 窗口）。未到达的刻度说明该模式
              在当前输入下到不了这个上下文规模。
            </p>
          </div>
          )}
        </div>
      )}
    </div>
  )
}
