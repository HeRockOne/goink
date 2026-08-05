// 特效系统核心：类型注册表 + 预设 + 旧配置迁移。
// 层结构允许自由组合（氛围光+粒子+流光同开、同类型多实例），
// 新特效类型 = 在渲染层加分支 + 在此注册，配置端自动可用。

export type EffectType = 'ambient' | 'particles' | 'streak' | 'glow'

export interface EffectLayer {
  type: EffectType
  intensity: number // 0-1 通用强度
  count?: number    // particles：粒子数（上限 MAX_PARTICLES）
  speed?: number    // particles/streak/glow：速度倍率 0.1-2
}

export interface ThemeEffects {
  layers: EffectLayer[]
}

export const EMPTY_EFFECTS: ThemeEffects = { layers: [] }

export const MAX_PARTICLES = 150

// 预设 5 套：名称 → 层配置。颜色全部吃主题变量，任何主题下协调。
export const EFFECT_PRESETS: { name: string; effects: ThemeEffects }[] = [
  {
    name: '静谧氛围',
    effects: { layers: [{ type: 'ambient', intensity: 0.3 }] },
  },
  {
    name: '星夜漫步',
    effects: {
      layers: [
        { type: 'ambient', intensity: 0.35 },
        { type: 'particles', intensity: 0.5, count: 60, speed: 0.8 },
      ],
    },
  },
  {
    name: '极光呼吸',
    effects: {
      layers: [
        { type: 'ambient', intensity: 0.4 },
        { type: 'glow', intensity: 0.3, speed: 1 },
      ],
    },
  },
  {
    name: '流光掠影',
    effects: {
      layers: [
        { type: 'streak', intensity: 0.35, speed: 1 },
        { type: 'ambient', intensity: 0.15 },
      ],
    },
  },
  {
    name: '萤火夏夜',
    effects: {
      layers: [
        { type: 'particles', intensity: 0.6, count: 100, speed: 1.2 },
        { type: 'glow', intensity: 0.2, speed: 1.5 },
      ],
    },
  },
]

export function presetByName(name: string): ThemeEffects | null {
  const p = EFFECT_PRESETS.find(x => x.name === name)
  return p ? p.effects : null
}

// 旧格式（v1：ambient/particles 布尔）迁移为 layers。
export function normalizeEffects(raw: unknown): ThemeEffects {
  if (raw && typeof raw === 'object' && Array.isArray((raw as ThemeEffects).layers)) {
    // 过滤非法层，参数兜底
    const layers = (raw as ThemeEffects).layers
      .filter(l => l && typeof l.type === 'string' && ['ambient', 'particles', 'streak', 'glow'].includes(l.type))
      .map(l => ({
        type: l.type as EffectType,
        intensity: clamp01(typeof l.intensity === 'number' ? l.intensity : 0.3),
        count: l.type === 'particles' ? clampInt(l.count, 8, MAX_PARTICLES, 60) : undefined,
        speed: clampSpeed(l.speed),
      }))
    return { layers }
  }
  const old = raw as { ambient?: boolean; ambientIntensity?: number; particles?: boolean; particleCount?: number; particleSpeed?: number } | null
  if (!old) return EMPTY_EFFECTS
  const layers: EffectLayer[] = []
  if (old.ambient) layers.push({ type: 'ambient', intensity: clamp01(old.ambientIntensity ?? 0.35) })
  if (old.particles) {
    layers.push({
      type: 'particles', intensity: 0.5,
      count: clampInt(old.particleCount, 8, MAX_PARTICLES, 60),
      speed: clampSpeed(old.particleSpeed),
    })
  }
  return { layers }
}

function clamp01(v: number): number {
  return Math.max(0, Math.min(1, v))
}

function clampSpeed(v: unknown): number {
  if (typeof v !== 'number' || !isFinite(v)) return 1
  return Math.max(0.1, Math.min(2, v))
}

function clampInt(v: unknown, min: number, max: number, def: number): number {
  if (typeof v !== 'number' || !isFinite(v)) return def
  return Math.max(min, Math.min(max, Math.round(v)))
}
