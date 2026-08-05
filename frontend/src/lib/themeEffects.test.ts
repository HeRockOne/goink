import { describe, expect, it } from 'vitest'
import { EFFECT_PRESETS, MAX_PARTICLES, normalizeEffects, presetByName } from './themeEffects'

describe('themeEffects', () => {
  it('预设 6 套', () => {
    expect(EFFECT_PRESETS.length).toBe(6)
    for (const p of EFFECT_PRESETS) {
      expect(p.name.length).toBeGreaterThan(0)
      expect(p.effects.layers.length).toBeGreaterThan(0)
    }
  })

  it('预设可按名获取', () => {
    const e = presetByName('星夜漫步')
    expect(e).not.toBeNull()
    expect(e!.layers.map(l => l.type)).toEqual(['ambient', 'particles'])
    expect(presetByName('不存在')).toBeNull()
  })

  it('旧格式迁移：ambient/particles 布尔 → layers', () => {
    const e = normalizeEffects({ ambient: true, ambientIntensity: 0.4, particles: true, particleCount: 80, particleSpeed: 1.5 })
    expect(e.layers).toEqual([
      { type: 'ambient', intensity: 0.4 },
      { type: 'particles', intensity: 0.5, count: 80, speed: 1.5 },
    ])
  })

  it('旧格式全关 → 空层', () => {
    expect(normalizeEffects({ ambient: false, particles: false })).toEqual({ layers: [] })
    expect(normalizeEffects(null)).toEqual({ layers: [] })
    expect(normalizeEffects(undefined)).toEqual({ layers: [] })
  })

  it('新格式透传并过滤非法层', () => {
    const e = normalizeEffects({
      layers: [
        { type: 'ambient', intensity: 0.9 },
        { type: 'nebula', intensity: 0.5 },
        { type: 'particles', intensity: 1.2, count: 9999, speed: 99 },
        { type: null, intensity: 0.5 },
      ],
    })
    expect(e.layers.map(l => l.type)).toEqual(['ambient', 'particles'])
    expect(e.layers[0].intensity).toBe(0.9)
    // 参数钳制
    expect(e.layers[1].intensity).toBe(1)
    expect(e.layers[1].count).toBe(MAX_PARTICLES)
    expect(e.layers[1].speed).toBe(2)
  })
})
