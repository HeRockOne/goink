// ── 总纲字段文本格式解析/序列化工具函数 ──
// 统一使用 `>` 行约定：
//   growth_arc:  > 第N-M章 阶段名：描述
//   core_conflict / ending_direction:  > 键：值

export interface GrowthStage {
  chapterStart: number
  chapterEnd: number
  name: string
  description: string
}

export interface KVItem {
  key: string
  value: string
}

// ── growth_arc ──

const STAGE_LINE_RE = /^>\s*第(\d+)[\-~到至](\d+)章\s*(.+?)(?:：|:)\s*(.*)/

export function parseGrowthArc(text: string): GrowthStage[] {
  if (!text) return []
  const stages: GrowthStage[] = []
  for (const line of text.split('\n')) {
    const m = line.match(STAGE_LINE_RE)
    if (m) {
      stages.push({
        chapterStart: parseInt(m[1]),
        chapterEnd: parseInt(m[2]),
        name: m[3].trim(),
        description: m[4].trim(),
      })
    }
  }
  return stages
}

export function serializeGrowthArc(stages: GrowthStage[], fallback: string): string {
  if (stages.length === 0) return fallback
  const otherLines = fallback ? fallback.split('\n').filter(l => !l.match(/^>\s*第\d+/)) : []
  const stageLines = stages.map(s =>
    `> 第${s.chapterStart}-${s.chapterEnd}章 ${s.name}：${s.description}`
  )
  return [...otherLines, ...stageLines].join('\n')
}

// ── 通用 > 键：值 格式 ──

const KV_LINE_RE = /^>\s*(.+?)(?:：|:)\s*(.*)/

export function parseKV(text: string): KVItem[] {
  if (!text) return []
  const items: KVItem[] = []
  for (const line of text.split('\n')) {
    const m = line.match(KV_LINE_RE)
    if (m) items.push({ key: m[1].trim(), value: m[2].trim() })
  }
  return items
}

export function serializeKV(items: KVItem[], fallback: string): string {
  if (items.length === 0) return fallback
  const otherLines = fallback ? fallback.split('\n').filter(l => !l.match(/^>\s*.+(?:：|:)/)) : []
  const kvLines = items.map(i => `> ${i.key}：${i.value}`)
  return [...otherLines, ...kvLines].join('\n')
}
