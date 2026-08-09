import { useState, useCallback, useEffect, useRef } from 'react'
import { Activity, Play, Loader2 } from 'lucide-react'
import { StartCacheSimulation } from '@/lib/wailsjs/go/app/App'
import { EventsOn } from '@/lib/wailsjs/runtime/runtime'

// 模拟结果类型（后端 app.CacheSimResult 通过事件推送，未生成 wails 模型，本地定义）
interface CacheSimScenario {
  name: string
  now_hit: number
  now_miss: number
  legacy_hit: number
  legacy_miss: number
  now_hit_rate: number
  legacy_hit_rate: number
  miss_save_pct: number
  now_cost: number
  legacy_cost: number
}

interface CacheSimResult {
  scenarios: CacheSimScenario[]
  total_now_hit: number
  total_now_miss: number
  total_legacy_hit: number
  total_legacy_miss: number
  now_cost: number
  legacy_cost: number
  now_hit_rate: number
  legacy_hit_rate: number
  miss_save_pct: number
}

// 格式化为 M（百万）单位
function fmtM(tokens: number): string {
  const m = tokens / 1_000_000
  if (m >= 100) return `${m.toFixed(0)}M`
  if (m >= 10) return `${m.toFixed(1)}M`
  return `${m.toFixed(2)}M`
}

// 缓存模拟 Tab：估算一个真实对话窗口写书的 token 消耗与费用——
// 短对话（查设定/改设定）与单章/批量创作交替发生在同一条历史里。
// 对比「历史随对话保留（当前版本）」与「每轮重发（旧版本）」的差距。
export default function CacheSimTab() {
  const [singleRounds, setSingleRounds] = useState(5)
  const [batchChapters, setBatchChapters] = useState(5)
  const [qaRounds, setQaRounds] = useState(3)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<CacheSimResult | null>(null)
  const [error, setError] = useState('')
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    const cleanup = EventsOn('cachesim:done', (data: CacheSimResult) => {
      if (!mountedRef.current) return
      setRunning(false)
      if (data.now_cost < 0) {
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
      await StartCacheSimulation(singleRounds, qaRounds, batchChapters)
    } catch (e) {
      setError(String(e))
      setRunning(false)
    }
  }, [singleRounds, qaRounds, batchChapters])

  const input = (value: number, setValue: (n: number) => void, min = 0, max = 20, label: string, hint?: string) => (
    <label className="flex flex-col gap-1 text-xs text-muted-foreground">
      {label}
      <input
        type="number"
        min={min}
        max={max}
        value={value}
        onChange={e => setValue(Math.max(min, Math.min(max, Number(e.target.value) || min)))}
        className="w-28 px-2 py-1.5 rounded border bg-background text-sm text-foreground"
      />
      {hint && <span className="text-[10px] text-muted-foreground/70">{hint}</span>}
    </label>
  )

  const renderScenario = (s: CacheSimScenario) => {
    return (
      <div key={s.name} className="rounded-lg border p-3">
        <div className="text-sm font-medium mb-2">{s.name}</div>
        <table className="w-full text-xs">
          <tbody>
            <tr>
              <td className="py-1">总 token（输入）</td>
              <td className="text-right tabular-nums">{fmtM(s.now_hit + s.now_miss)}</td>
            </tr>
            <tr>
              <td className="py-1">缓存命中率</td>
              <td className="text-right tabular-nums">{s.now_hit_rate.toFixed(1)}%</td>
            </tr>
            <tr>
              <td className="py-1">估算费用（¥）</td>
              <td className="text-right tabular-nums">{s.now_cost.toFixed(4)}</td>
            </tr>
          </tbody>
        </table>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full gap-4">
      <div className="mb-1 pb-3 border-b">
        <div className="flex items-center gap-2">
          <Activity className="w-4 h-4" />
          <span className="text-sm font-medium">写书成本模拟</span>
        </div>
        <p className="text-xs text-muted-foreground mt-1">
          模拟一个真实对话窗口的历史 Token 消耗与费用。短对话与单章/批量创作
          交替发生在同一条对话历史里。必读技能由系统自动注入（auto_skill_injection），
          历史留在服务端缓存，费用按设置中的模型价格估算。
        </p>
      </div>

      <div className="flex items-end gap-4">
        {input(singleRounds, setSingleRounds, 0, 20, '单章流程轮数', '0 = 不跑')}
        {input(batchChapters, setBatchChapters, 0, 20, '批量创作章数', '0 = 不跑')}
        {input(qaRounds, setQaRounds, 0, 20, '短对话穿插轮数', '穿插在创作轮之间，0 = 不穿插')}
        <button
          onClick={run}
          disabled={running}
          className="px-4 py-1.5 rounded bg-primary text-primary-foreground text-sm flex items-center gap-1.5 disabled:opacity-50 hover:opacity-90 transition-opacity"
        >
          {running ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
          {running ? '模拟中...' : '开始模拟'}
        </button>
      </div>

      {error && <p className="text-xs text-red-500">{error}</p>}

      {result && (
        <div className="flex-1 min-h-0 overflow-y-auto space-y-4">
          {result.scenarios.map(renderScenario)}

          <div className="rounded-lg border p-3 bg-muted/30">
            <div className="text-sm font-medium mb-2">合计</div>
            <table className="w-full text-xs">
              <tbody>
                <tr>
                  <td className="py-1">总输入 token</td>
                  <td className="text-right tabular-nums">{fmtM(result.total_now_hit + result.total_now_miss)}</td>
                </tr>
                <tr>
                  <td className="py-1">总命中率</td>
                  <td className="text-right tabular-nums">{result.now_hit_rate.toFixed(1)}%</td>
                </tr>
                <tr>
                  <td className="py-1">估算费用（¥）</td>
                  <td className="text-right tabular-nums">{result.now_cost.toFixed(4)}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
