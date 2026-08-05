import { useEffect, useMemo, useRef } from 'react'
import { useTheme } from '@/hooks/useTheme'
import { MAX_PARTICLES } from '@/lib/themeEffects'

// 主题特效层：遍历当前主题 effects.layers 渲染各类型特效。
// 类型注册表：ambient（氛围光）/ particles（Canvas 粒子）/ streak（流光）/ glow（呼吸光晕）。
// 设计约束：
// - 颜色吃主题 CSS 变量（--primary/--accent），换主题自动联动
// - 只做 transform/opacity 动画（GPU 合成）；多粒子层合并进同一 Canvas（总粒子上限）
// - pointer-events-none 不挡交互；prefers-reduced-motion 下全部静止
// - 窗口失焦（visibilitychange）暂停粒子，避免后台空转烧 GPU

const DPR_CAP = 1.5
const TOTAL_PARTICLE_CAP = 200 // 所有粒子层的合并上限（性能红线）

export default function AmbientEffectLayer() {
  const { activeTheme, activeEffects, customThemes } = useTheme()
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const layers = activeEffects.layers

  // 稳定引用：避免每次 render 生成新数组导致粒子 effect 反复重建
  const particleLayers = useMemo(() => layers.filter(l => l.type === 'particles'), [layers])
  const showParticles = particleLayers.length > 0

  // ── 粒子层（多层合并到同一 Canvas，总粒子数封顶） ──
  useEffect(() => {
    if (particleLayers.length === 0) return
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const dpr = Math.min(window.devicePixelRatio || 1, DPR_CAP)
    // 合并所有粒子层的粒子：数量按层求和但封顶，速度/透明度取层参数
    const total = Math.min(TOTAL_PARTICLE_CAP, particleLayers.reduce((s, l) => s + (l.count ?? 60), 0))
    const layersCfg = particleLayers
    let w = 0, h = 0
    let raf = 0
    let running = true

    type P = { x: number; y: number; r: number; vx: number; vy: number; a: number }
    let parts: P[] = []

    const resize = () => {
      w = window.innerWidth
      h = window.innerHeight
      canvas.width = Math.round(w * dpr)
      canvas.height = Math.round(h * dpr)
      canvas.style.width = w + 'px'
      canvas.style.height = h + 'px'
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    }

    const readColors = () => {
      const cs = getComputedStyle(document.documentElement)
      return { primary: cs.getPropertyValue('--primary').trim() || '#888888', accent: cs.getPropertyValue('--accent').trim() || '#888888' }
    }

    // 按层权重分配粒子，让每一层的 count/speed 生效
    const init = () => {
      resize()
      parts = []
      for (const l of layersCfg) {
        const n = Math.max(4, Math.min(MAX_PARTICLES, l.count ?? 60))
        const speed = Math.max(0.1, Math.min(2, l.speed ?? 1))
        for (let i = 0; i < n && parts.length < total; i++) {
          parts.push({
            x: Math.random() * w,
            y: Math.random() * h,
            r: 0.6 + Math.random() * 1.8,
            vx: (Math.random() - 0.5) * 0.25 * speed,
            vy: (Math.random() - 0.5) * 0.25 * speed,
            a: (0.15 + Math.random() * 0.5) * Math.max(0, Math.min(1, l.intensity)) * 1.4,
          })
        }
      }
    }

    let colors = readColors()
    const frame = () => {
      if (!running) return
      ctx.clearRect(0, 0, w, h)
      for (const p of parts) {
        p.x += p.vx
        p.y += p.vy
        if (p.x < -4) p.x = w + 4
        if (p.x > w + 4) p.x = -4
        if (p.y < -4) p.y = h + 4
        if (p.y > h + 4) p.y = -4
        ctx.beginPath()
        ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2)
        ctx.fillStyle = Math.random() > 0.75 ? colors.accent : colors.primary
        ctx.globalAlpha = p.a
        ctx.fill()
      }
      ctx.globalAlpha = 1
      raf = requestAnimationFrame(frame)
    }

    const onVis = () => {
      if (document.hidden) {
        running = false
        cancelAnimationFrame(raf)
      } else if (running === false) {
        running = true
        colors = readColors()
        raf = requestAnimationFrame(frame)
      }
    }

    init()
    raf = requestAnimationFrame(frame)
    window.addEventListener('resize', resize)
    document.addEventListener('visibilitychange', onVis)
    return () => {
      running = false
      cancelAnimationFrame(raf)
      window.removeEventListener('resize', resize)
      document.removeEventListener('visibilitychange', onVis)
    }
  }, [showParticles, activeTheme, customThemes, particleLayers])

  // ── 各类型渲染 ──
  const ambientLayers = layers.filter(l => l.type === 'ambient' && l.intensity > 0)
  const glowLayers = layers.filter(l => l.type === 'glow' && l.intensity > 0)
  const streakLayers = layers.filter(l => l.type === 'streak' && l.intensity > 0)

  return (
    <div className="pointer-events-none fixed inset-0 z-[5] overflow-hidden" aria-hidden="true">
      {/* 氛围光：每层一组光斑（最多 2 层，性能保护） */}
      {ambientLayers.slice(0, 2).map((l, i) => (
        <div key={`ambient-${i}`} style={{ opacity: l.intensity * 0.5 }}>
          <div className="ambient-blob ambient-blob-1" />
          <div className="ambient-blob ambient-blob-2" />
          <div className="ambient-blob ambient-blob-3" />
        </div>
      ))}
      {/* 呼吸光晕：每层一个（最多 2 层） */}
      {glowLayers.slice(0, 2).map((l, i) => (
        <div
          key={`glow-${i}`}
          className="fx-glow"
          style={{ opacity: l.intensity * 0.5, animationDuration: `${Math.max(4, 10 / (l.speed ?? 1))}s` }}
        />
      ))}
      {/* 流光：每层一组光带（最多 2 层） */}
      {streakLayers.slice(0, 2).map((l, i) => (
        <div
          key={`streak-${i}`}
          className="fx-streak"
          style={{ opacity: l.intensity * 0.6, animationDuration: `${Math.max(8, 22 / (l.speed ?? 1))}s` }}
        />
      ))}
      {showParticles && <canvas ref={canvasRef} className="fixed inset-0" />}
    </div>
  )
}
