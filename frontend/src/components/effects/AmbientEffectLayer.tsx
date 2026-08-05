import { useEffect, useRef } from 'react'
import { useTheme } from '@/hooks/useTheme'

// 主题特效层：读取当前主题的 effects 配置渲染氛围光与粒子层。
// 设计约束：
// - 颜色吃主题 CSS 变量（--primary/--accent），换主题自动联动
// - 只做 transform/opacity 动画（GPU 合成）；粒子 Canvas 独立图层 + DPR 降采样
// - pointer-events-none 不挡交互；prefers-reduced-motion 下全部静止
// - 窗口失焦（visibilitychange）暂停粒子，避免后台空转烧 GPU

const MAX_PARTICLES = 150
const DPR_CAP = 1.5

export default function AmbientEffectLayer() {
  const { activeTheme, activeEffects, customThemes } = useTheme()
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const eff = activeEffects

  // ── 氛围光（CSS 光斑，强度 0 时隐藏） ──
  const ambientOn = eff.ambient && eff.ambientIntensity > 0

  // ── 粒子层（Canvas 2D + rAF） ──
  useEffect(() => {
    if (!eff.particles) return
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const dpr = Math.min(window.devicePixelRatio || 1, DPR_CAP)
    const count = Math.max(8, Math.min(MAX_PARTICLES, Math.round(eff.particleCount)))
    const speed = eff.particleSpeed
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

    // 主题色：从计算样式读 --primary（换主题时 effect 依赖 customThemes 触发重读）
    const readColors = () => {
      const cs = getComputedStyle(document.documentElement)
      return { primary: cs.getPropertyValue('--primary').trim() || '#888888', accent: cs.getPropertyValue('--accent').trim() || '#888888' }
    }

    const init = () => {
      resize()
      parts = Array.from({ length: count }, () => ({
        x: Math.random() * w,
        y: Math.random() * h,
        r: 0.6 + Math.random() * 1.8,
        vx: (Math.random() - 0.5) * 0.25 * speed,
        vy: (Math.random() - 0.5) * 0.25 * speed,
        a: 0.15 + Math.random() * 0.5,
      }))
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
        ctx.globalAlpha = p.a * 0.5
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
    // customThemes 变化（新增/删除主题）时重读主题色；eff 参数变化时重建
  }, [eff.particles, eff.particleCount, eff.particleSpeed, eff.ambientIntensity, activeTheme, customThemes])

  const showParticles = eff.particles

  return (
    <div className="pointer-events-none fixed inset-0 z-[5] overflow-hidden" aria-hidden="true">
      {ambientOn && (
        <>
          <div
            className="ambient-blob ambient-blob-1"
            style={{ opacity: eff.ambientIntensity * 0.5 }}
          />
          <div
            className="ambient-blob ambient-blob-2"
            style={{ opacity: eff.ambientIntensity * 0.4 }}
          />
          <div
            className="ambient-blob ambient-blob-3"
            style={{ opacity: eff.ambientIntensity * 0.3 }}
          />
        </>
      )}
      {showParticles && (
        <canvas ref={canvasRef} className="fixed inset-0" />
      )}
    </div>
  )
}
