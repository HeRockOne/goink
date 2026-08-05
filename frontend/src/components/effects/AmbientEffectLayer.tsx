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
  const swordLayers = useMemo(() => layers.filter(l => l.type === 'sword' && l.intensity > 0), [layers])
  const showParticles = particleLayers.length > 0 || swordLayers.length > 0

  // ── 统一 Canvas 循环：粒子层（合并，总数封顶）+ 剑气层（横贯/低掠/弧光） ──
  useEffect(() => {
    if (particleLayers.length === 0 && swordLayers.length === 0) return
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

    type P = { x: number; y: number; r: number; vx: number; vy: number; a: number; isLine: boolean }
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
      return {
        primary: cs.getPropertyValue('--particle-color').trim() || 'rgba(161,196,214,0.5)',
        glow: cs.getPropertyValue('--particle-glow').trim() || 'rgba(161,196,214,0.16)',
        line: cs.getPropertyValue('--particle-line').trim() || 'rgba(161,196,214,0.18)',
      }
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
            // 上升漂移：垂直速度偏向负（向上升），水平微漂移
            vx: (Math.random() - 0.5) * 0.18 * speed,
            vy: -((0.1 + Math.random() * 0.25) * speed),
            a: (0.15 + Math.random() * 0.5) * Math.max(0, Math.min(1, l.intensity)) * 1.4,
            isLine: Math.random() > 0.55, // 剑形线段形态
          })
        }
      }
    }

    let colors = readColors()
    const frame = () => {
      if (!running) return
      ctx.clearRect(0, 0, w, h)
      const now = performance.now()
      for (const p of parts) {
        p.x += p.vx
        p.y += p.vy
        if (p.y < -20) { p.y = h + 20; p.x = Math.random() * w }
        if (p.x < -20) p.x = w + 20
        if (p.x > w + 20) p.x = -20
        // 发光：先画大半径低透明度光晕，再画实心（比 shadowBlur 便宜）
        ctx.beginPath()
        ctx.arc(p.x, p.y, p.r * 3, 0, Math.PI * 2)
        ctx.fillStyle = colors.glow
        ctx.globalAlpha = p.a * 0.5
        ctx.fill()
        ctx.beginPath()
        ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2)
        ctx.fillStyle = colors.primary
        ctx.globalAlpha = p.a
        ctx.fill()
        // 剑形线段：沿速度方向画的短线
        if (p.isLine) {
          const ang = Math.atan2(p.vy, p.vx)
          const len = p.r * 4
          ctx.strokeStyle = colors.line
          ctx.lineWidth = 0.7
          ctx.globalAlpha = p.a * 0.8
          ctx.beginPath()
          ctx.moveTo(p.x - Math.cos(ang) * len, p.y - Math.sin(ang) * len)
          ctx.lineTo(p.x + Math.cos(ang) * len, p.y + Math.sin(ang) * len)
          ctx.stroke()
        }
      }
      ctx.globalAlpha = 1
      // 粒子间连接线：距离 < 120 连线，透明度随距离衰减
      const LINE_DIST = 120
      for (let i = 0; i < parts.length; i++) {
        for (let j = i + 1; j < parts.length; j++) {
          const dx = parts[i].x - parts[j].x
          const dy = parts[i].y - parts[j].y
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist < LINE_DIST) {
            ctx.globalAlpha = (1 - dist / LINE_DIST) * 0.06
            ctx.strokeStyle = colors.line
            ctx.lineWidth = 0.4
            ctx.beginPath()
            ctx.moveTo(parts[i].x, parts[i].y)
            ctx.lineTo(parts[j].x, parts[j].y)
            ctx.stroke()
          }
        }
      }
      ctx.globalAlpha = 1
      // ── 剑气层：横贯长空 + 低空掠过 + 挥剑弧光（周期触发，克制） ──
      if (swordLayers.length > 0) {
        const sInten = Math.max(0.1, Math.min(1, swordLayers[0].intensity))
        drawSwords(ctx, now, w, h, sInten, colors)
      }
      raf = requestAnimationFrame(frame)
    }

    // 剑气绘制：二次贝塞尔轨迹 + 渐变拖尾 + 白亮剑尖
    const bezier = (t: number, p0: number, c: number, p1: number) =>
      (1 - t) * (1 - t) * p0 + 2 * (1 - t) * t * c + t * t * p1

    const drawBlade = (t: number, p0x: number, p0y: number, cx: number, cy: number, p1x: number, p1y: number, width: number, colors: ReturnType<typeof readColors>, alpha: number) => {
      const x = bezier(t, p0x, cx, p1x)
      const y = bezier(t, p0y, cy, p1y)
      // 切线方向
      const tx = 2 * (1 - t) * (cx - p0x) + 2 * t * (p1x - cx)
      const ty = 2 * (1 - t) * (cy - p0y) + 2 * t * (p1y - cy)
      const len = Math.sqrt(tx * tx + ty * ty) || 1
      const ux = tx / len, uy = ty / len
      const tail = width * 7
      // 拖尾光带（从尾到尖渐变）
      const g = ctx.createLinearGradient(x - ux * tail, y - uy * tail, x + ux * tail, y + uy * tail)
      g.addColorStop(0, 'rgba(0,0,0,0)')
      g.addColorStop(0.55, colors.glow)
      g.addColorStop(0.92, colors.line)
      g.addColorStop(1, '#ffffff')
      ctx.globalAlpha = alpha
      ctx.strokeStyle = g
      ctx.lineWidth = 3
      ctx.lineCap = 'round'
      ctx.beginPath()
      ctx.moveTo(x - ux * tail, y - uy * tail)
      ctx.lineTo(x + ux * tail * 0.9, y + uy * tail * 0.9)
      ctx.stroke()
      // 剑尖光点 + 光晕
      ctx.beginPath()
      ctx.arc(x, y, 7, 0, Math.PI * 2)
      ctx.fillStyle = colors.glow
      ctx.globalAlpha = alpha * 0.6
      ctx.fill()
      ctx.beginPath()
      ctx.arc(x, y, 2.6, 0, Math.PI * 2)
      ctx.fillStyle = '#ffffff'
      ctx.globalAlpha = alpha
      ctx.fill()
      ctx.globalAlpha = 1
    }

    const drawSwords = (ctx: CanvasRenderingContext2D, now: number, w: number, h: number, inten: number, colors: ReturnType<typeof readColors>) => {
      const cyc = (period: number) => (now % (period * 1000)) / (period * 1000)
      const windowed = (t: number, w0: number, w1: number): number | null => {
        if (t < w0 || t > w1) return null
        return (t - w0) / (w1 - w0)
      }
      const alpha = 0.9 * inten
      // 剑气一：横贯长空（8s 周期，1.4s 划过）
      const t1 = windowed(cyc(8), 0.14, 0.31)
      if (t1 !== null) drawBlade(t1, -120, h * 0.2, w * 0.42, h * 0.1, w + 120, h * 0.32, 44, colors, alpha)
      // 剑气二：低空掠过（11s 周期，2.2s 划过）
      const t2 = windowed(cyc(11), 0.6, 0.82)
      if (t2 !== null) drawBlade(t2, -120, h * 0.78, w * 0.5, h * 0.72, w + 120, h * 0.74, 34, colors, alpha * 0.75)
      // 剑气三：挥剑弧光（3.6s 周期，弧线划出残影）
      const t3 = windowed(cyc(3.6), 0, 0.35)
      if (t3 !== null) {
        const ax = bezier(t3, w * 0.3, w * 0.26, w * 0.36)
        const ay = bezier(t3, h * 0.26, h * 0.56, h * 0.7)
        ctx.globalAlpha = alpha * 0.8
        ctx.strokeStyle = colors.line
        ctx.lineWidth = 2
        ctx.lineCap = 'round'
        ctx.beginPath()
        for (let i = 0; i <= t3; i += 0.02) {
          const x = bezier(i, w * 0.3, w * 0.26, w * 0.36)
          const y = bezier(i, h * 0.26, h * 0.56, h * 0.7)
          if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y)
        }
        ctx.stroke()
        ctx.beginPath()
        ctx.arc(ax, ay, 3, 0, Math.PI * 2)
        ctx.fillStyle = '#e8f4ff'
        ctx.globalAlpha = alpha
        ctx.fill()
        ctx.globalAlpha = 1
      }
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
  }, [showParticles, activeTheme, customThemes, particleLayers, swordLayers])

  // ── 各类型渲染 ──
  const ambientLayers = layers.filter(l => l.type === 'ambient' && l.intensity > 0)
  const glowLayers = layers.filter(l => l.type === 'glow' && l.intensity > 0)
  const streakLayers = layers.filter(l => l.type === 'streak' && l.intensity > 0)

  return (
    <div className="pointer-events-none fixed inset-0 z-[10] overflow-hidden" aria-hidden="true">
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
