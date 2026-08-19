import { describe, it, expect } from 'vitest'
import {
  parseGrowthArc, serializeGrowthArc,
  parseKV, serializeKV,
} from './outlineParse'

describe('parseGrowthArc', () => {
  it('parses standard format', () => {
    const text = `> 第1-8章 废柴期：被欺压的普通弟子，无特殊能力
> 第9-18章 崛起期：获得太初道经，修为暴涨
> 第19-25章 低谷期：被至交背叛，跌入幽渊`
    const stages = parseGrowthArc(text)
    expect(stages).toHaveLength(3)
    expect(stages[0]).toEqual({ chapterStart: 1, chapterEnd: 8, name: '废柴期', description: '被欺压的普通弟子，无特殊能力' })
    expect(stages[1]).toEqual({ chapterStart: 9, chapterEnd: 18, name: '崛起期', description: '获得太初道经，修为暴涨' })
    expect(stages[2]).toEqual({ chapterStart: 19, chapterEnd: 25, name: '低谷期', description: '被至交背叛，跌入幽渊' })
  })

  it('handles Chinese connectors', () => {
    const text = '> 第1到10章 测试：描述'
    expect(parseGrowthArc(text)).toHaveLength(1)
    expect(parseGrowthArc(text)[0].chapterStart).toBe(1)
    expect(parseGrowthArc(text)[0].chapterEnd).toBe(10)
  })

  it('handles tilde connector', () => {
    const text = '> 第5~15章 测试：描述'
    expect(parseGrowthArc(text)).toHaveLength(1)
    expect(parseGrowthArc(text)[0].chapterStart).toBe(5)
    expect(parseGrowthArc(text)[0].chapterEnd).toBe(15)
  })

  it('handles colon variant', () => {
    const text = '> 第1-5章 名称: 描述内容'
    expect(parseGrowthArc(text)).toHaveLength(1)
    expect(parseGrowthArc(text)[0].name).toBe('名称')
    expect(parseGrowthArc(text)[0].description).toBe('描述内容')
  })

  it('returns empty for empty input', () => {
    expect(parseGrowthArc('')).toEqual([])
    expect(parseGrowthArc('just some text')).toEqual([])
  })

  it('ignores non-matching lines', () => {
    const text = `一些自由文本
> 第1-5章 阶段一：描述
另一行文本
> 第6-10章 阶段二：描述`
    expect(parseGrowthArc(text)).toHaveLength(2)
  })
})

describe('serializeGrowthArc', () => {
  it('serializes stages back to text', () => {
    const stages = [
      { chapterStart: 1, chapterEnd: 8, name: '废柴期', description: '被欺压' },
      { chapterStart: 9, chapterEnd: 18, name: '崛起期', description: '修为暴涨' },
    ]
    const result = serializeGrowthArc(stages, '')
    expect(result).toBe('> 第1-8章 废柴期：被欺压\n> 第9-18章 崛起期：修为暴涨')
  })

  it('preserves non-matching lines', () => {
    const stages = [{ chapterStart: 1, chapterEnd: 5, name: '阶段一', description: '描述' }]
    const fallback = '自由文本注释\n> 第1-5章 旧阶段：旧描述\n另一行注释'
    const result = serializeGrowthArc(stages, fallback)
    expect(result).toContain('自由文本注释')
    expect(result).toContain('另一行注释')
    expect(result).toContain('> 第1-5章 阶段一：描述')
    expect(result).not.toContain('旧阶段')
  })

  it('returns fallback when stages empty', () => {
    expect(serializeGrowthArc([], 'fallback')).toBe('fallback')
  })
})

describe('parseKV', () => {
  it('parses standard format', () => {
    const text = `> 主角：林逸
> 反派：陈默
> 根本冲突：被至交背叛`
    const items = parseKV(text)
    expect(items).toHaveLength(3)
    expect(items[0]).toEqual({ key: '主角', value: '林逸' })
    expect(items[1]).toEqual({ key: '反派', value: '陈默' })
    expect(items[2]).toEqual({ key: '根本冲突', value: '被至交背叛' })
  })

  it('handles half-width colon', () => {
    const text = '> Key: Value'
    expect(parseKV(text)).toHaveLength(1)
    expect(parseKV(text)[0]).toEqual({ key: 'Key', value: 'Value' })
  })

  it('returns empty for empty input', () => {
    expect(parseKV('')).toEqual([])
    expect(parseKV('plain text')).toEqual([])
  })
})

describe('serializeKV', () => {
  it('serializes items back to text', () => {
    const items = [
      { key: '主角', value: '林逸' },
      { key: '反派', value: '陈默' },
    ]
    const result = serializeKV(items, '')
    expect(result).toBe('> 主角：林逸\n> 反派：陈默')
  })

  it('preserves non-matching lines', () => {
    const items = [{ key: '主角', value: '林逸' }]
    const fallback = '注释文本\n> 旧键：旧值\n另一行'
    const result = serializeKV(items, fallback)
    expect(result).toContain('注释文本')
    expect(result).toContain('另一行')
    expect(result).toContain('> 主角：林逸')
    expect(result).not.toContain('旧键')
  })

  it('returns fallback when items empty', () => {
    expect(serializeKV([], 'fallback')).toBe('fallback')
  })
})
