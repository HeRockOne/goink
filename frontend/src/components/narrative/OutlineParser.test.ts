import { describe, it, expect } from 'vitest'
import { parseOutline } from './OutlineParser'

describe('OutlineParser 容错增强', () => {
  it('支持 # 单井号章节标题 + ## 段落（kernel 标准格式）', () => {
    const md = `# 第3章 破镜

## 基调与字数
基调：紧张
预估字数：3000字

## 关键事件
- 主角突破金丹
- 仇敌登场

## 章末钩子
悬念型——玉佩发出异光`
    const o = parseOutline(md, 3)
    expect(o.title).toBe('破镜') // 剥离"第3章"前缀
    expect(o.tone).toBe('紧张')
    expect(o.wordCount).toBe('3000字')
    expect(o.keyEvents).toEqual(['主角突破金丹', '仇敌登场'])
    expect(o.endingHook?.type).toBe('悬念型')
    expect(o.endingHook?.content).toContain('玉佩发出异光')
  })

  it('支持 **字段名**：格式（无 ## 标题的 005 风格）', () => {
    const md = `**章节标题**：迷雾

**关键事件**：
1. **入秘境**：秦烈踏入禁地
2. **遇仇敌**：灰袍人现身`
    const o = parseOutline(md, 5)
    expect(o.title).toBe('迷雾')
    expect(o.keyEvents).toContain('入秘境：秦烈踏入禁地')
  })

  it('支持 【标题】 和模糊匹配（仅带括号后缀时命中）', () => {
    const md = `## 关键事件（本章）
- 事件B

## 情绪设计
锚点：压抑`
    const o = parseOutline(md, 7)
    // 无标准"关键事件"，模糊匹配"关键事件（本章）"
    expect(o.keyEvents).toContain('事件B')
    expect(o.emotionalDesign?.anchor).toBe('压抑')
  })

  it('支持 **事件**：细节 列表项与 **回收**：伏笔前缀', () => {
    const md = `## 关键事件
- **破镜**：石门轰然洞开

## 伏笔操作
- **回收**：玉佩来历伏笔
- **埋下**：石门后的身影`
    const o = parseOutline(md, 9)
    expect(o.keyEvents).toEqual(['破镜：石门轰然洞开'])
    expect(o.foreshadowing.resolve).toEqual(['玉佩来历伏笔'])
    expect(o.foreshadowing.bury).toEqual(['石门后的身影'])
  })

  it('章节标题行不误判为 section', () => {
    const md = `# 第 10 章 · 风起

## 场景设计
1. **山巅**（情绪功能：肃杀）
2. **洞府**（情绪功能：隐秘）`
    const o = parseOutline(md, 10)
    expect(o.title).toBe('风起')
    expect(o.scenes.length).toBe(2)
    expect(o.scenes[0].title).toBe('山巅')
  })
})
