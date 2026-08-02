/**
 * OutlineParser — 前端正则解析 outlines/*.md
 * 按 ## 标题分割，提取各 section 内容
 * 支持变体：# 标题（会被跳过）、**加粗标题**、### 场景标题等
 */

export interface ParsedOutline {
  chapter: number
  title: string
  tone?: string       // 基调
  wordCount?: string  // 字数估计
  openingStrategy?: string
  scenes: SceneEntry[]
  keyEvents: string[]
  characters: CharacterEntry[]
  foreshadowing: ForeshadowingEntry
  emotionalDesign?: EmotionalDesign
  endingHook?: EndingHook
  goldenFinger?: string
  [key: string]: unknown  // 保留其他未解析字段
}

export interface SceneEntry {
  title: string
  emotion?: string
  narrativePurpose?: string
}

export interface CharacterEntry {
  name: string
  description: string
}

export interface ForeshadowingEntry {
  bury: string[]     // 埋下
  advance: string[]  // 推进
  resolve: string[]  // 回收
}

export interface EmotionalDesign {
  anchor?: string
  accessory?: string
  rhythm?: string
}

export interface EndingHook {
  type: string       // 危机型、怪物睁眼式等
  content: string
}

/**
 * 解析单个大纲文件
 * @param markdown 大纲文件原始内容
 * @param chapterNum 章节号（从文件名提取）
 */
export function parseOutline(markdown: string, chapterNum: number): ParsedOutline {
  const sections = splitByHeaders(markdown)

  const result: ParsedOutline = {
    chapter: chapterNum,
    title: extractTitle(markdown, sections),
    scenes: extractScenes(sections['场景设计'] || sections['场景'] || ''),
    keyEvents: extractListItems(sections['关键事件'] || ''),
    characters: extractCharacters(sections['重点角色'] || ''),
    foreshadowing: extractForeshadowing(sections['伏笔操作'] || sections['伏笔'] || ''),
  }

  // 基调与字数
  const toneSection = sections['基调与字数'] || ''
  if (toneSection) {
    const toneMatch = toneSection.match(/基调[：:]([^字\n]+)/)
    if (toneMatch) result.tone = toneMatch[1].trim()
    const wordMatch = toneSection.match(/(?:预估)?字数[：:]?\s*(\d+[~-]\d+字|\d+字)/)
    if (wordMatch) result.wordCount = wordMatch[1].trim()
  }

  // 开篇策略
  if (sections['开篇策略']) {
    result.openingStrategy = sections['开篇策略'].trim()
  }

  // 情绪设计
  if (sections['情绪设计']) {
    result.emotionalDesign = extractEmotionalDesign(sections['情绪设计'])
  }

  // 章末钩子
  if (sections['章末钩子']) {
    result.endingHook = extractEndingHook(sections['章末钩子'])
  }

  // 金手指状态
  if (sections['金手指状态']) {
    result.goldenFinger = sections['金手指状态'].trim()
  }

  // 保留其他未解析字段
  for (const [key, value] of Object.entries(sections)) {
    if (!['基调与字数', '开篇策略', '场景设计', '关键事件', '重点角色', '伏笔操作', '伏笔', '情绪设计', '章末钩子', '金手指状态', '标题'].includes(key)) {
      if (value && typeof value === 'string' && value.trim()) {
        result[key] = value.trim()
      }
    }
  }

  return result
}

/**
 * 按 ## 标题分割 markdown，返回 { 标题: 内容 }
 * 支持：## 标题、## **加粗标题**、# 标题 等格式
 * 也支持 **字段名**：格式（无 ## 标题的情况）
 */
function splitByHeaders(markdown: string): Record<string, string> {
  const sections: Record<string, string> = {}
  // 先尝试用 ## 标题分割
  const regex = /^#{2,}\s+(.+?)\s*$/gm
  let lastIndex = 0
  let lastTitle = ''
  let match: RegExpExecArray | null
  let hasHeaders = false

  while ((match = regex.exec(markdown)) !== null) {
    hasHeaders = true
    if (lastTitle) {
      sections[lastTitle] = markdown.slice(lastIndex, match.index).trim()
    }
    lastTitle = match[1].replace(/\*\*/g, '').replace(/[`]/g, '').trim()
    lastIndex = regex.lastIndex
  }

  if (lastTitle) {
    sections[lastTitle] = markdown.slice(lastIndex).trim()
  }

  // 如果没有 ## 标题，尝试用 **字段名**：格式分割（005.md 格式）
  if (!hasHeaders) {
    const fieldRegex = /^\*\*([^*]+)\*\*[：:]/gm
    let lastFieldIndex = 0
    let lastFieldName = ''
    while ((match = fieldRegex.exec(markdown)) !== null) {
      if (lastFieldName) {
        sections[lastFieldName] = markdown.slice(lastFieldIndex, match.index).trim()
      }
      lastFieldName = match[1].trim()
      lastFieldIndex = fieldRegex.lastIndex
    }
    if (lastFieldName) {
      sections[lastFieldName] = markdown.slice(lastFieldIndex).trim()
    }
  }

  return sections
}

/**
 * 提取章节标题（从 # 或 ## 标题行）
 * 支持：# 标题、## 标题、## **加粗标题** 等
 */
function extractTitle(markdown: string, sections: Record<string, string>): string {
  // 先找 # 或 ## 标题（有些大纲第一个是 #，有些是 ##）
  const hashMatch = markdown.match(/^[#]+\s+(.+)\s*$/m)
  if (hashMatch) return hashMatch[1].replace(/\*\*/g, '').replace(/[`]/g, '').trim()
  // 或者找 ## 章节标题（去掉"章节标题"等前缀）
  const chapterTitle = sections['章节标题']
  if (chapterTitle) return chapterTitle.replace(/\*\*/g, '').trim()
  // 用第一个 ## 标题作为标题
  const firstSection = Object.values(sections)[0]
  if (firstSection) return Object.keys(sections)[0]
  return ''
}

/**
 * 提取场景列表
 * 支持格式：
 * - 1. **场景名**（情绪功能：描述）  ← 006.md 格式
 * - 1. **镇口观察**（情绪功能：建立氛围...）  ← 005.md 格式
 * - ### 场景一：场景名
 * - **场景名**：描述
 * - 场景名——描述
 */
function extractScenes(text: string): SceneEntry[] {
  if (!text) return []
  const scenes: SceneEntry[] = []
  const lines = text.split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed) continue
    // 有序列表项（1. 2. 3.）- 支持 **加粗** 标题和（情绪功能：描述）格式
    const orderedMatch = trimmed.match(/^\d+[..、)]\s+\*?\*?(.+?)\*?\s*[（(]/)
    if (orderedMatch) {
      const entry: SceneEntry = { title: orderedMatch[1].replace(/\*\*/g, '').trim() }
      // 提取括号内的内容作为描述
      const bracketMatch = trimmed.match(/[（(]([^）)]+)[）)]/)
      if (bracketMatch) {
        const inner = bracketMatch[1]
        // 检查是否是"情绪功能："格式
        const emotionMatch = inner.match(/情绪功能[：:]\s*(.+)/)
        if (emotionMatch) {
          entry.emotion = emotionMatch[1].replace(/^[^-]+-\s*/, '').trim()
          entry.narrativePurpose = inner
        } else {
          entry.emotion = inner
        }
      }
      scenes.push(entry)
      continue
    }
    // ### 场景一：场景名 格式
    const hashMatch = trimmed.match(/^###\s+.+?[：:]\s*(.+)/)
    if (hashMatch) {
      scenes.push({ title: hashMatch[1].replace(/\*\*/g, '').trim() })
      continue
    }
    // **场景名**：描述 格式
    const boldMatch = trimmed.match(/^\*?\*?(.+?)\*?\*?[：:：]\s*(.+)/)
    if (boldMatch) {
      const entry: SceneEntry = { title: boldMatch[1].replace(/\*\*/g, '').trim() }
      const desc = boldMatch[2].trim()
      if (desc) {
        const emotionMatch = desc.match(/[（(]([^）)]+)[）)]/)
        if (emotionMatch) entry.emotion = emotionMatch[1].replace(/^[^-]+-\s*/, '').trim()
      }
      scenes.push(entry)
      continue
    }
  }
  return scenes
}

/**
 * 提取列表项（关键事件、重点角色等）
 * 支持：- item、* item、1. item、1) item、1、item 等格式
 */
function extractListItems(text: string): string[] {
  if (!text) return []
  const items: string[] = []
  const lines = text.split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed) continue
    // 去掉列表前缀（- 、* 、1. 、2. 、1) 、1、 等）
    const cleaned = trimmed.replace(/^[-*→]\s+/, '').replace(/^\d+[..、)]\s+/, '').replace(/^\d+[、，]\s+/, '')
    if (cleaned) items.push(cleaned.replace(/\*\*/g, '').trim())
  }
  return items
}

/**
 * 提取角色列表
 */
function extractCharacters(text: string): CharacterEntry[] {
  if (!text) return []
  const characters: CharacterEntry[] = []
  const lines = text.split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const cleaned = trimmed.replace(/^[-*]\s+/, '').replace(/\*\*/g, '')
    // 格式：**角色名**：描述 或 角色名：描述
    const match = cleaned.match(/^(.+?)[：:]\s*(.+)/)
    if (match) {
      characters.push({ name: match[1].trim(), description: match[2].trim() })
    } else if (cleaned) {
      characters.push({ name: cleaned.trim(), description: '' })
    }
  }
  return characters
}

/**
 * 提取伏笔操作
 * 支持格式：
 * - **回收**：第5章"灰衣人尾随"伏笔，本章回收
 * - **埋下**：平阳关设卡（下一章冲突）
 * - **推进**：悬赏令的传播范围
 * - - 回收：...（带列表前缀）
 */
function extractForeshadowing(text: string): ForeshadowingEntry {
  const result: ForeshadowingEntry = { bury: [], advance: [], resolve: [] }
  if (!text) return result
  const lines = text.split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed) continue
    // 去掉列表前缀（- 、* 、→ 等）
    const cleaned = trimmed.replace(/^[-*→]\s+/, '').replace(/^\*?\*?/, '').replace(/\*?\*?$/, '')
    if (cleaned.startsWith('回收：') || cleaned.startsWith('回收:')) {
      result.resolve.push(cleaned.replace(/^回收[：:]\s*/, '').trim())
    } else if (cleaned.startsWith('埋下：') || cleaned.startsWith('埋下:') || cleaned.startsWith('埋设：') || cleaned.startsWith('埋设:')) {
      result.bury.push(cleaned.replace(/^埋[下设][：:]\s*/, '').trim())
    } else if (cleaned.startsWith('推进：') || cleaned.startsWith('推进:')) {
      result.advance.push(cleaned.replace(/^推进[：:]\s*/, '').trim())
    } else if (cleaned && !cleaned.startsWith('**') && !cleaned.startsWith('-') && !cleaned.startsWith('*')) {
      // 没有前缀的当作推进
      result.advance.push(cleaned)
    }
  }
  return result
}

/**
 * 提取情绪设计
 * 支持格式：
 * - **锚点**：内容
 * - - **锚点**：内容（带列表前缀）
 * - 锚点：内容（无加粗）
 */
function extractEmotionalDesign(text: string): EmotionalDesign {
  const result: EmotionalDesign = {}
  if (!text) return result
  const lines = text.split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed) continue
    // 去掉列表前缀（- 、* 、1. 等）
    const cleaned = trimmed.replace(/^[-*→]\s+/, '').replace(/^\*?\*?/, '').replace(/\*?\*?$/, '')
    if (cleaned.startsWith('锚点：') || cleaned.startsWith('锚点:')) {
      result.anchor = cleaned.replace(/^锚点[：:]\s*/, '').trim()
    } else if (cleaned.startsWith('配件细节：') || cleaned.startsWith('配件：') || cleaned.startsWith('配件细节:')) {
      result.accessory = cleaned.replace(/^配件[细节]?[：:]\s*/, '').trim()
    } else if (cleaned.startsWith('节奏切换：') || cleaned.startsWith('节奏切换:')) {
      result.rhythm = cleaned.replace(/^节奏切换[：:]\s*/, '').trim()
    }
  }
  return result
}

/**
 * 提取章末钩子
 */
function extractEndingHook(text: string): EndingHook {
  if (!text) return { type: '', content: '' }
  // 格式：**类型名**——内容 或 类型——内容
  const match = text.match(/\*?\*?(.+?)\*?\s*[——–-]+\s*(.+)/s)
  if (match) {
    return {
      type: match[1].replace(/\*\*/g, '').trim(),
      content: match[2].replace(/\*\*/g, '').trim().slice(0, 200),
    }
  }
  return { type: '', content: text.replace(/\*\*/g, '').trim().slice(0, 200) }
}

/**
 * 批量解析章纲文件（用于未来章节展示）
 * @param outlines { chapterNum: markdown } 的 map
 */
export function parseOutlines(outlines: Record<number, string>): Record<number, ParsedOutline> {
  const result: Record<number, ParsedOutline> = {}
  for (const [chapterNum, markdown] of Object.entries(outlines)) {
    try {
      result[Number(chapterNum)] = parseOutline(markdown, Number(chapterNum))
    } catch (e) {
      console.warn(`[OutlineParser] Failed to parse chapter ${chapterNum}:`, e)
    }
  }
  return result
}