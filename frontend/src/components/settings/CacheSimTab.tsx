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

// 缓存模拟 Tab：手动触发模拟（后台异步，完成后事件推送），对比 NS 落库 vs 不落库的缓存收益。
export default function CacheSimTab() {
  const [gateRounds, setGateRounds] = useState(5)
  const [shortQARounds, setShortQARounds] = useState(3)
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
      await StartCacheSimulation(gateRounds, shortQARounds)
    } catch (e) {
      setError(String(e))
      setRunning(false)
    }
  }, [gateRounds, shortQARounds])

  return (
    <div className="flex flex-col h-full gap-4">
      <div className="mb-1 pb-3 border-b">
        <div className="flex items-center gap-2">
          <Activity className="w-4 h-4" />
          <span className="text-sm font-medium">缓存命中模拟</span>
        </div>
        <p className="text-xs text-muted-foreground mt-1">
          模拟完整门禁创作流程的缓存命中，对比 NS 落库协议（修复后）vs 不落库（修复前）。
          成本按设置中的模型价格估算（输入/输出/缓存命中单价）。
          耗时约 1-6 分钟（tiktoken 精确计数），请耐心等待。
        </p>
      </div>

      <div className="flex items-end gap-4">
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          门禁创作轮数
          <input
            type="number"
            min={1}
            max={20}
            value={gateRounds}
            onChange={e => setGateRounds(Math.max(1, Math.min(20, Number(e.target.value) || 1)))}
            className="w-24 px-2 py-1.5 rounded border bg-background text-sm text-foreground"
          />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          短对话穿插轮数（0 = 不穿插）
          <input
            type="number"
            min={0}
            max={20}
            value={shortQARounds}
            onChange={e => setShortQARounds(Math.max(0, Math.min(20, Number(e.target.value) || 0)))}
            className="w-28 px-2 py-1.5 rounded border bg-background text-sm text-foreground"
          />
        </label>
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
          {result.scenarios.map((s: CacheSimScenario) => (
            <div key={s.name} className="rounded-lg border p-3">
              <div className="text-sm font-medium mb-2">{s.name}</div>
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-muted-foreground">
                    <th className="text-left py-1">指标</th>
                    <th className="text-right py-1">修复前（NS 不落库）</th>
                    <th className="text-right py-1">修复后（NS 落库）</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td className="py-1">累计 hit</td>
                    <td className="text-right tabular-nums">{s.legacy_hit.toLocaleString()}</td>
                    <td className="text-right tabular-nums">{s.now_hit.toLocaleString()}</td>
                  </tr>
                  <tr>
                    <td className="py-1">累计 miss</td>
                    <td className="text-right tabular-nums">{s.legacy_miss.toLocaleString()}</td>
                    <td className="text-right tabular-nums">{s.now_miss.toLocaleString()}</td>
                  </tr>
                  <tr>
                    <td className="py-1">命中率</td>
                    <td className="text-right tabular-nums">{s.legacy_hit_rate.toFixed(1)}%</td>
                    <td className="text-right tabular-nums">{s.now_hit_rate.toFixed(1)}%</td>
                  </tr>
                  <tr className="border-t">
                    <td className="py-1">miss 降幅</td>
                    <td className="text-right tabular-nums" colSpan={2}>
                      <span className={s.miss_save_pct > 0 ? 'text-success-foreground' : 'text-status-warning'}>
                        {s.miss_save_pct.toFixed(1)}%
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          ))}

          <div className="rounded-lg border p-3 bg-muted/30">
            <div className="text-sm font-medium mb-2">汇总与成本估算（按设置价格）</div>
            <table className="w-full text-xs">
              <tbody>
                <tr>
                  <td className="py-1">总命中率</td>
                  <td className="text-right tabular-nums">{result.legacy_hit_rate.toFixed(1)}% → {result.now_hit_rate.toFixed(1)}%</td>
                </tr>
                <tr>
                  <td className="py-1">总 miss 降幅</td>
                  <td className="text-right tabular-nums">
                    <span className={result.miss_save_pct > 0 ? 'text-success-foreground' : 'text-status-warning'}>
                      {result.miss_save_pct.toFixed(1)}%
                    </span>
                  </td>
                </tr>
                <tr>
                  <td className="py-1">估算成本（¥）</td>
                  <td className="text-right tabular-nums">
                    {result.legacy_cost.toFixed(4)} → {result.now_cost.toFixed(4)}
                  </td>
                </tr>
                <tr>
                  <td className="py-1">成本节约</td>
                  <td className="text-right tabular-nums text-success-foreground">
                    ¥{(result.legacy_cost - result.now_cost).toFixed(4)}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
