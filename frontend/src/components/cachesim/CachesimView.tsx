import { useCallback, useEffect, useRef, useState } from 'react'
import { Play, Loader2, Plus, RotateCcw, X, Gauge, ChevronDown, ChevronUp, BookOpen, Package, MessagesSquare, TrendingUp } from 'lucide-react'
import { StartCacheSimScenarios, GetSettings, SetSimHitRateAdjust } from '@/lib/wailsjs/go/app/App'
import { EventsOn } from '@/lib/wailsjs/runtime/runtime'
import type { config } from '@/lib/wailsjs/go/models'
import CacheSimDeepDive from './CacheSimDeepDive'

// 模拟结果类型（后端 app.CacheSimResult 通过事件推送，本地定义）
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
  stages: CacheSimStage[]
  compresses: number
  miss_by_cat: Record<string, number>
}

interface SimScenario {
  name: string
  gate_rounds: number
  short_qa_rounds: number
  batch_chapters: number
  batch_rounds: number
}

// ── 三档预设（普通用户视角：写法 → 参数自动映射）──
// 三档共同语义：同一个会话窗口内连续完成（写下一章不新开对话），
// 上下文一路累积、缓存延续。每章成本 = 该窗口总花费 ÷ 窗口内章数。
interface Preset {
  key: string
  title: string
  desc: string
  icon: React.ReactNode
  scenario: SimScenario
}

const PRESETS: Preset[] = [
  { key: 'single', title: '逐章精写', desc: '一个会话窗口里连续写 9 章，每章都走完整流程（大纲→正文→审校），上下文一路累积不换窗口', icon: <BookOpen className="w-4 h-4" />, scenario: { name: '逐章精写', gate_rounds: 9, short_qa_rounds: 0, batch_chapters: 0, batch_rounds: 1 } },
  { key: 'batch', title: '批量连写', desc: '一个会话窗口里每批写 6 章（先一起出大纲、再逐章写正文），连写 3 批共 18 章，上下文一路累积不换窗口', icon: <Package className="w-4 h-4" />, scenario: { name: '批量连写', gate_rounds: 0, short_qa_rounds: 0, batch_chapters: 18, batch_rounds: 1 } },
  { key: 'mixed', title: '边写边聊', desc: '一个会话窗口里：写 3 章精修 → 聊 2 轮设定 → 再批量写 3 章，贴近日常边写边调整', icon: <MessagesSquare className="w-4 h-4" />, scenario: { name: '边写边聊', gate_rounds: 3, short_qa_rounds: 2, batch_chapters: 3, batch_rounds: 1 } },
]

// ── 高级详情：自定义场景（保留原能力）──
const DEFAULT_SCENARIOS: SimScenario[] = [
  { name: '单章 1 章', gate_rounds: 1, short_qa_rounds: 0, batch_chapters: 0, batch_rounds: 1 },
  { name: '单章 3 章', gate_rounds: 3, short_qa_rounds: 0, batch_chapters: 0, batch_rounds: 1 },
  { name: '单章 5 章', gate_rounds: 5, short_qa_rounds: 0, batch_chapters: 0, batch_rounds: 1 },
  { name: '批量 20 章', gate_rounds: 0, short_qa_rounds: 0, batch_chapters: 20, batch_rounds: 1 },
  { name: '批量 40 章', gate_rounds: 0, short_qa_rounds: 0, batch_chapters: 40, batch_rounds: 1 },
  { name: '批量 60 章', gate_rounds: 0, short_qa_rounds: 0, batch_chapters: 60, batch_rounds: 1 },
  { name: '混合 3+2+3', gate_rounds: 3, short_qa_rounds: 2, batch_chapters: 3, batch_rounds: 1 },
  { name: '混合 5+5+5', gate_rounds: 5, short_qa_rounds: 5, batch_chapters: 5, batch_rounds: 1 },
]

const MISS_CATS: { key: string; label: string }[] = [
  { key: 'thinking', label: 'thinking' },
  { key: 'skill_inject', label: '技能注入' },
  { key: 'update', label: '工具结果' },
  { key: 'query', label: '查询' },
  { key: 'fixed', label: '固定/NS' },
  { key: 'body', label: '正文' },
  { key: 'outline', label: '大纲' },
  { key: 'other', label: '其他' },
]

function fmtM(tokens: number): string {
  const m = tokens / 1_000_000
  if (m >= 100) return `${m.toFixed(0)}M`
  if (m >= 10) return `${m.toFixed(1)}M`
  return `${m.toFixed(2)}M`
}

function chaptersOf(sc: SimScenario): number {
  if (sc.batch_chapters > 0 && sc.gate_rounds === 0) return Math.max(1, sc.batch_chapters)
  if (sc.gate_rounds > 0 && sc.batch_chapters === 0) return Math.max(1, sc.gate_rounds)
  return Math.max(1, sc.gate_rounds + sc.batch_chapters * sc.batch_rounds)
}

type Tab = 'compare' | 'miss' | 'deep'

const TABS: { id: Tab; label: string }[] = [
  { id: 'compare', label: '场景对比' },
  { id: 'miss', label: 'miss 构成' },
  { id: 'deep', label: '单场景深挖' },
]

export default function CachesimView() {
  // ── 第一屏状态 ──
  const [selected, setSelected] = useState('batch')
  const [monthlyChapters, setMonthlyChapters] = useState(30)
  const [presetResults, setPresetResults] = useState<CacheSimResult[]>([])
  const [presetRunning, setPresetRunning] = useState(true)
  // ── 高级详情状态 ──
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [scenarios, setScenarios] = useState<SimScenario[]>(DEFAULT_SCENARIOS)
  const [windowK, setWindowK] = useState(0)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [results, setResults] = useState<CacheSimResult[]>([])
  const [tab, setTab] = useState<Tab>('compare')
  const [prices, setPrices] = useState({ input: 1, output: 2, cache: 0.02 })
  const [hitAdjust, setHitAdjust] = useState(0.95)
  const [hitAdjustSaved, setHitAdjustSaved] = useState(true)
  const mountedRef = useRef(true)

  // 进面板自动跑三档预设
  useEffect(() => {
    mountedRef.current = true
    const c1 = EventsOn('cachesim:batch-done', (data: CacheSimResult[]) => {
      if (!mountedRef.current) return
      setPresetRunning(false)
      setRunning(false)
      const ok = data.filter(d => d.cost >= 0)
      if (ok.length === 0) {
        setError('模拟失败')
        return
      }
      setPresetResults(ok)
      setResults(ok)
    })
    StartCacheSimScenarios(PRESETS.map(p => ({ ...p.scenario, context_window: windowK * 1000 })))
    return () => { mountedRef.current = false; c1() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    GetSettings().then((s: config.AppSettings) => {
      setPrices({
        input: s.price_input > 0 ? s.price_input : 1,
        output: s.price_output > 0 ? s.price_output : 2,
        cache: s.cache_price > 0 ? s.cache_price : 0.02,
      })
      if (s.sim_hit_rate_adjust > 0 && s.sim_hit_rate_adjust <= 1) {
        setHitAdjust(s.sim_hit_rate_adjust)
      }
    }).catch(() => {})
  }, [])

  // ── 第一屏结论 ──
  const presetIndex = PRESETS.findIndex(p => p.key === selected)
  const current = presetResults[presetIndex]
  const perChapter = current ? current.cost / chaptersOf(PRESETS[presetIndex].scenario) : 0
  const monthlyCost = perChapter * monthlyChapters

  // 建议句：最省档 + 相对最贵档节省
  let suggestion = ''
  if (presetResults.length === 3) {
    const costs = presetResults.map((r, i) => r.cost / chaptersOf(PRESETS[i].scenario))
    const best = Math.min(...costs)
    const worst = Math.max(...costs)
    const bestIdx = costs.indexOf(best)
    if (bestIdx >= 0 && worst > 0) {
      const save = Math.round((1 - best / worst) * 100)
      suggestion = `建议用「${PRESETS[bestIdx].title}」：每章约 ¥${best.toFixed(3)}，比「${PRESETS[costs.indexOf(worst)].title}」省约 ${save}%`
    }
  }

  const runAll = useCallback(async () => {
    setRunning(true)
    setError('')
    setResults([])
    const req = scenarios.map(sc => ({ ...sc, context_window: windowK * 1000 }))
    try {
      await StartCacheSimScenarios(req)
    } catch (e) {
      setError(String(e))
      setRunning(false)
    }
  }, [scenarios, windowK])

  const updateScenario = (i: number, patch: Partial<SimScenario>) => {
    setScenarios(prev => prev.map((s, idx) => (idx === i ? { ...s, ...patch } : s)))
  }

  let cheapestIdx = -1
  if (results.length > 0) {
    let best = Infinity
    results.forEach((r, i) => {
      const sc = scenarios[i] ?? { gate_rounds: 1, short_qa_rounds: 0, batch_chapters: 0, batch_rounds: 1 }
      const perCh = r.cost / chaptersOf(sc)
      if (perCh < best) {
        best = perCh
        cheapestIdx = i
      }
    })
  }

  const numInput = (value: number, onChange: (n: number) => void, max = 200) => (
    <input
      type="number"
      min={0}
      max={max}
      value={value}
      onChange={e => {
        const n = Number(e.target.value)
        if (e.target.value !== '' && !Number.isNaN(n)) onChange(Math.max(0, Math.min(max, n)))
      }}
      className="w-14 px-1 py-1 rounded border bg-background text-xs text-center text-foreground"
    />
  )

  return (
    <main className="flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
      <div className="max-w-5xl mx-auto px-8 py-6 space-y-5">
        {/* 标题 */}
        <div className="flex items-center gap-2">
          <Gauge className="w-5 h-5 text-primary" />
          <h1 className="text-lg font-semibold text-foreground">写书成本估算</h1>
          <span className="text-xs text-muted-foreground">按你的写法和当前模型价格，估算写书的费用</span>
        </div>

        {/* ── 第一屏：我的写作成本 ── */}
        {!advancedOpen && (
          <>
            {/* 三档写法选择 */}
            <div className="grid grid-cols-3 gap-3">
              {PRESETS.map(p => {
                const active = selected === p.key
                const res = presetResults[PRESETS.findIndex(x => x.key === p.key)]
                const perCh = res ? res.cost / chaptersOf(p.scenario) : 0
                return (
                  <button
                    key={p.key}
                    onClick={() => setSelected(p.key)}
                    disabled={presetRunning}
                    className={`rounded-lg border p-4 text-left transition-all ${active
                      ? 'border-primary ring-2 ring-primary/20 bg-primary/5'
                      : 'border-border bg-card hover:border-muted-foreground/40'}`}
                  >
                    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                      {p.icon}
                      {p.title}
                    </div>
                    <p className="text-xs text-muted-foreground mt-1">{p.desc}</p>
                    <p className="text-xs tabular-nums mt-2">
                      {presetRunning
                        ? <span className="text-muted-foreground/60">估算中...</span>
                        : perCh > 0
                          ? <span className={active ? 'text-primary font-medium' : 'text-muted-foreground'}>每章约 ¥{perCh.toFixed(3)}</span>
                          : <span className="text-muted-foreground/60">暂无数据</span>}
                    </p>
                  </button>
                )
              })}
            </div>

            {/* 柱状图：三档每章成本对比 */}
            <div className="rounded-lg border bg-card p-4">
              <div className="text-sm font-medium text-foreground mb-3">三种写法每章成本对比</div>
              {presetRunning ? (
                <div className="flex items-center justify-center py-8 text-xs text-muted-foreground gap-2">
                  <Loader2 className="w-4 h-4 animate-spin" /> 正在估算（约半分钟）...
                </div>
              ) : (
                <div className="space-y-2.5">
                  {PRESETS.map((p, i) => {
                    const res = presetResults[i]
                    const perCh = res ? res.cost / chaptersOf(p.scenario) : 0
                    const maxCh = Math.max(...PRESETS.map((_, j) => presetResults[j] ? presetResults[j].cost / chaptersOf(PRESETS[j].scenario) : 0), 0.001)
                    const active = selected === p.key
                    return (
                      <button
                        key={p.key}
                        onClick={() => setSelected(p.key)}
                        className="w-full flex items-center gap-3 group"
                      >
                        <span className={`w-16 shrink-0 text-xs text-right ${active ? 'text-foreground font-medium' : 'text-muted-foreground'}`}>{p.title}</span>
                        <span className="flex-1 h-6 rounded bg-muted overflow-hidden">
                          <span
                            className={`h-full flex items-center justify-end pr-2 text-[11px] tabular-nums transition-all ${active ? 'bg-primary/80 text-primary-foreground' : 'bg-primary/25 text-muted-foreground'}`}
                            style={{ width: `${Math.max(6, (perCh / maxCh) * 100)}%` }}
                          >
                            ¥{perCh.toFixed(3)}
                          </span>
                        </span>
                      </button>
                    )
                  })}
                </div>
              )}
            </div>

            {/* 大数字卡：写一章 / 写一个月 */}
            <div className="grid grid-cols-2 gap-3">
              <div className="rounded-lg border bg-card p-5">
                <p className="text-xs text-muted-foreground">按「{PRESETS[presetIndex].title}」写一章约</p>
                <p className="text-3xl font-semibold text-foreground tabular-nums mt-1">
                  {perChapter > 0 ? `¥${perChapter.toFixed(3)}` : '—'}
                </p>
              </div>
              <div className="rounded-lg border bg-card p-5">
                <p className="text-xs text-muted-foreground flex items-center gap-2">
                  每月写
                  <input
                    type="number"
                    min={1}
                    max={300}
                    value={monthlyChapters}
                    onChange={e => {
                      const n = Number(e.target.value)
                      if (e.target.value !== '' && !Number.isNaN(n)) setMonthlyChapters(Math.max(1, Math.min(300, n)))
                    }}
                    className="w-16 px-1.5 py-0.5 rounded border bg-background text-xs text-center text-foreground"
                  />
                  章约
                </p>
                <p className="text-3xl font-semibold text-foreground tabular-nums mt-1">
                  {perChapter > 0 ? `¥${monthlyCost.toFixed(2)}` : '—'}
                </p>
              </div>
            </div>

            {suggestion && (
              <div className="rounded-lg border border-primary/30 bg-primary/5 px-4 py-3 flex items-center gap-2">
                <TrendingUp className="w-4 h-4 text-primary shrink-0" />
                <p className="text-sm text-foreground">{suggestion}</p>
              </div>
            )}

            <p className="text-[10px] text-muted-foreground/70">
              三档写法都是「同一个会话窗口里连续完成」：写下一章不新开对话，AI 记得之前写的内容，缓存延续更省钱。
              每章成本 = 该窗口总花费 ÷ 窗口内产出章数。估算基于当前模型价格（缓存 ¥{prices.cache.toFixed(2)}/M · 输入 ¥{prices.input.toFixed(2)}/M · 输出 ¥{prices.output.toFixed(2)}/M）与真实写作流程模拟。
              {hitAdjust < 1 && (
                <span> 命中率已按真机校准（×{hitAdjust.toFixed(2)}，可在高级详情调整）。</span>
              )}
              想对比更多写法、看缓存命中明细，展开下方高级详情。
            </p>
          </>
        )}

        {/* ── 高级详情（折叠） ── */}
        <div className="rounded-lg border bg-card">
          <button
            onClick={() => setAdvancedOpen(prev => !prev)}
            className="w-full flex items-center gap-2 px-4 py-3 text-sm text-foreground hover:bg-muted/50 transition-colors"
          >
            <Gauge className="w-4 h-4 text-muted-foreground" />
            <span className="font-medium">高级详情（场景对比 / miss 构成 / 窗口刻度 / 单场景深挖）</span>
            <div className="flex-1" />
            {advancedOpen ? <ChevronUp className="w-4 h-4 text-muted-foreground" /> : <ChevronDown className="w-4 h-4 text-muted-foreground" />}
          </button>

          {advancedOpen && (
            <div className="border-t border-border p-4 space-y-5">
              {/* 参数行 */}
              <div className="flex items-center gap-4">
                <label className="flex items-center gap-2 text-xs text-muted-foreground shrink-0">
                  模拟窗口 K
                  <input
                    type="number"
                    min={0}
                    max={2000}
                    value={windowK}
                    onChange={e => {
                      const n = Number(e.target.value)
                      if (e.target.value !== '' && !Number.isNaN(n)) setWindowK(Math.max(0, Math.min(2000, n)))
                    }}
                    className="w-20 px-2 py-1 rounded border bg-background text-sm text-foreground"
                  />
                  <span className="text-[10px] text-muted-foreground/70">0=按选中模型</span>
                </label>
                <label className="flex items-center gap-2 text-xs text-muted-foreground shrink-0" title="真机命中率（89-93%）低于模拟器（96-97%），系数把模拟命中率乘此值：0.95=按 95% 折算，1=不校准">
                  命中率校准 ×
                  <input
                    type="number"
                    min={0.5}
                    max={1}
                    step={0.01}
                    value={hitAdjust}
                    onChange={e => {
                      const n = Number(e.target.value)
                      if (e.target.value !== '' && !Number.isNaN(n)) {
                        setHitAdjust(Math.max(0.5, Math.min(1, n)))
                        setHitAdjustSaved(false)
                      }
                    }}
                    onBlur={() => {
                      if (hitAdjustSaved) return
                      SetSimHitRateAdjust(hitAdjust).then(() => setHitAdjustSaved(true)).catch(() => {})
                    }}
                    className="w-16 px-2 py-1 rounded border bg-background text-sm text-foreground"
                  />
                  <span className="text-[10px] text-muted-foreground/70">
                    {hitAdjustSaved ? '已保存' : '修改后自动保存'}
                  </span>
                </label>
                <div className="flex-1" />
                <button
                  onClick={runAll}
                  disabled={running || scenarios.length === 0}
                  className="shrink-0 px-4 py-1.5 rounded bg-primary text-primary-foreground text-sm flex items-center gap-1.5 disabled:opacity-50 hover:opacity-90 transition-opacity"
                >
                  {running ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                  {running ? '模拟中...' : '跑全部场景'}
                </button>
              </div>

              {/* 场景编辑器 */}
              <div className="flex items-center gap-2 mb-1">
                <span className="text-sm font-medium text-foreground">场景集（可自定义）</span>
                <span className="text-[10px] text-muted-foreground/70">单章章数 g · 短对话轮 q · 批量章 b · 批量批数 r</span>
                <div className="flex-1" />
                <button
                  onClick={() => setScenarios(prev => [...prev, { name: `新场景 ${prev.length + 1}`, gate_rounds: 3, short_qa_rounds: 0, batch_chapters: 0, batch_rounds: 1 }])}
                  className="flex items-center gap-1 px-2 py-1 rounded border text-xs text-muted-foreground hover:bg-muted transition-colors"
                >
                  <Plus className="w-3.5 h-3.5" /> 添加场景
                </button>
                <button
                  onClick={() => { setScenarios(DEFAULT_SCENARIOS) }}
                  className="flex items-center gap-1 px-2 py-1 rounded border text-xs text-muted-foreground hover:bg-muted transition-colors"
                >
                  <RotateCcw className="w-3.5 h-3.5" /> 恢复默认
                </button>
              </div>
              <div className="space-y-1.5">
                {scenarios.map((sc, i) => (
                  <div key={i} className="flex items-center gap-3">
                    <input
                      value={sc.name}
                      onChange={e => updateScenario(i, { name: e.target.value })}
                      className="w-36 px-2 py-1 rounded border bg-background text-xs text-foreground"
                    />
                    <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                      单章{numInput(sc.gate_rounds, n => updateScenario(i, { gate_rounds: n }))}
                    </span>
                    <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                      短对话{numInput(sc.short_qa_rounds, n => updateScenario(i, { short_qa_rounds: n }))}
                    </span>
                    <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                      批量章{numInput(sc.batch_chapters, n => updateScenario(i, { batch_chapters: n }))}
                    </span>
                    <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                      批量批{numInput(sc.batch_rounds, n => updateScenario(i, { batch_rounds: n }))}
                    </span>
                    <button
                      onClick={() => setScenarios(prev => prev.filter((_, idx) => idx !== i))}
                      className="p-1 rounded text-muted-foreground hover:text-destructive transition-colors"
                      title="删除场景"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
              </div>

              {error && <p className="text-xs text-red-500">{error}</p>}

              {/* Tab 栏 */}
              <div className="flex items-center gap-1 border-b border-border">
                {TABS.map(t => (
                  <button
                    key={t.id}
                    onClick={() => setTab(t.id)}
                    className={`px-4 py-2 text-sm rounded-t border-b-2 -mb-px transition-colors ${tab === t.id
                      ? 'border-primary text-foreground font-medium'
                      : 'border-transparent text-muted-foreground hover:text-foreground'}`}
                  >
                    {t.label}
                  </button>
                ))}
              </div>

              {/* Tab 1 场景对比 */}
              {tab === 'compare' && (
                <div className="rounded-lg border overflow-hidden">
                  {results.length === 0 ? (
                    <p className="text-xs text-muted-foreground text-center py-8">
                      {running ? '正在模拟...' : '点击"跑全部场景"生成对比表'}
                    </p>
                  ) : (
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="text-muted-foreground border-b border-border">
                          <th className="text-left py-2 pl-4 font-normal">场景</th>
                          <th className="text-right py-2 font-normal">输入 hit</th>
                          <th className="text-right py-2 font-normal">输入 miss</th>
                          <th className="text-right py-2 font-normal">输出 out</th>
                          <th className="text-right py-2 font-normal">命中率</th>
                          <th className="text-right py-2 font-normal">总成本 ¥</th>
                          <th className="text-right py-2 pr-4 font-normal">每章 ¥</th>
                        </tr>
                      </thead>
                      <tbody>
                        {results.map((r, i) => {
                          const sc = scenarios[i] ?? { gate_rounds: 1, short_qa_rounds: 0, batch_chapters: 0, batch_rounds: 1 }
                          const perCh = r.cost / chaptersOf(sc)
                          const isCheapest = i === cheapestIdx
                          return (
                            <tr key={i} className={`border-b border-border/50 ${isCheapest ? 'bg-amber-50/60' : ''}`}>
                              <td className="py-2 pl-4">
                                <span className="text-foreground">{r.label}</span>
                                {isCheapest && (
                                  <span className="ml-2 px-1.5 py-0.5 rounded bg-amber-100 text-amber-700 text-[10px]">最省</span>
                                )}
                              </td>
                              <td className="py-2 text-right tabular-nums">{fmtM(r.total_hit)}</td>
                              <td className="py-2 text-right tabular-nums">{fmtM(r.total_miss)}</td>
                              <td className="py-2 text-right tabular-nums">{fmtM(r.total_out)}</td>
                              <td className="py-2 text-right tabular-nums">{r.hit_rate.toFixed(1)}%</td>
                              <td className="py-2 text-right tabular-nums">{r.cost.toFixed(4)}</td>
                              <td className="py-2 pr-4 text-right tabular-nums font-medium">{perCh.toFixed(4)}</td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  )}
                  <p className="text-[10px] text-muted-foreground/70 px-4 py-2 border-t border-border">
                    每章成本 = 总成本 ÷ 产出章数（单章=轮数，批量=章数，混合=单章轮 + 批量章×批量轮）。最省行高亮。
                  </p>
                </div>
              )}

              {/* Tab 2 miss 构成 */}
              {tab === 'miss' && (
                <div className="rounded-lg border overflow-hidden">
                  {results.length === 0 ? (
                    <p className="text-xs text-muted-foreground text-center py-8">点击"跑全部场景"生成 miss 构成表</p>
                  ) : (
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="text-muted-foreground border-b border-border">
                          <th className="text-left py-2 pl-4 font-normal">场景</th>
                          <th className="text-right py-2 font-normal">miss 总计</th>
                          {MISS_CATS.map(c => (
                            <th key={c.key} className="text-right py-2 font-normal">{c.label}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {results.map((r, i) => (
                          <tr key={i} className="border-b border-border/50">
                            <td className="py-2 pl-4 text-foreground">{r.label}</td>
                            <td className="py-2 text-right tabular-nums">{fmtM(r.total_miss)}</td>
                            {MISS_CATS.map(c => (
                              <td key={c.key} className="py-2 text-right tabular-nums">
                                {r.miss_by_cat && r.miss_by_cat[c.key] > 0 ? fmtM(r.miss_by_cat[c.key]) : '-'}
                              </td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                  <p className="text-[10px] text-muted-foreground/70 px-4 py-2 border-t border-border">
                    miss 按消息来源分类（now 协议）：thinking=思考链，技能注入=必读技能全文，工具结果=写正文/大纲的工具参数，
                    查询=get_*/search_*，固定/NS=系统提示与小说设定，正文/大纲=写作正文与大纲消息。
                  </p>
                </div>
              )}

              {/* Tab 3 单场景深挖 */}
              {tab === 'deep' && <CacheSimDeepDive />}
            </div>
          )}
        </div>
      </div>
    </main>
  )
}

