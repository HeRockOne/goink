// 主题颜色工具：WCAG 对比度计算 + 一键生成全套主题变量。
// 生成策略：以用户主色为色相基准，固定色相偏移派生 tag/tool 六色系，
// 面板色从背景按明度阶梯派生，所有 bg/fg 对自动修正到 AA（≥4.5:1）。

export interface ThemeGenInput {
  name: string
  mode: 'light' | 'dark'
  background: string // hex
  primary: string // hex
  foreground?: string // 可选：自定义文字色，留空自动对比色；与背景对比度不足时自动回退
  vibrancy?: number // 鲜艳度 0.5-1.5，默认 1.0，缩放派生色的饱和度
}

interface HSL { h: number; s: number; l: number }

export function hexToRgb(hex: string): { r: number; g: number; b: number } | null {
  const m = hex.trim().match(/^#?([0-9a-f]{6})$/i)
  if (!m) return null
  const n = parseInt(m[1], 16)
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 }
}

function rgbToHex(r: number, g: number, b: number): string {
  const c = (v: number) => Math.round(Math.max(0, Math.min(255, v))).toString(16).padStart(2, '0')
  return `#${c(r)}${c(g)}${c(b)}`
}

function rgbToHsl(r: number, g: number, b: number): HSL {
  r /= 255; g /= 255; b /= 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b)
  let h = 0, s = 0
  const l = (max + min) / 2
  if (max !== min) {
    const d = max - min
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
    switch (max) {
      case r: h = (g - b) / d + (g < b ? 6 : 0); break
      case g: h = (b - r) / d + 2; break
      default: h = (r - g) / d + 4
    }
    h *= 60
  }
  return { h, s: s * 100, l: l * 100 }
}

function hslToRgb(h: number, s: number, l: number): { r: number; g: number; b: number } {
  h = ((h % 360) + 360) % 360
  s = Math.max(0, Math.min(100, s)) / 100
  l = Math.max(0, Math.min(100, l)) / 100
  const c = (1 - Math.abs(2 * l - 1)) * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = l - c / 2
  let r = 0, g = 0, b = 0
  if (h < 60) { r = c; g = x }
  else if (h < 120) { r = x; g = c }
  else if (h < 180) { g = c; b = x }
  else if (h < 240) { g = x; b = c }
  else if (h < 300) { r = x; b = c }
  else { r = c; b = x }
  return { r: (r + m) * 255, g: (g + m) * 255, b: (b + m) * 255 }
}

function hslHex(h: number, s: number, l: number): string {
  const { r, g, b } = hslToRgb(h, s, l)
  return rgbToHex(r, g, b)
}

// WCAG 2.x 相对亮度
function luminance(rgb: { r: number; g: number; b: number }): number {
  const f = (v: number) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * f(rgb.r) + 0.7152 * f(rgb.g) + 0.0722 * f(rgb.b)
}

// 支持 hex / rgb() 解析，其他格式（hsl/oklch）返回 null 跳过校验
export function parseColor(input: string): { r: number; g: number; b: number } | null {
  const t = input.trim()
  const hex = hexToRgb(t)
  if (hex) return hex
  const m = t.match(/^rgb\(\s*(\d+)[,\s]+(\d+)[,\s]+(\d+)\s*\)$/)
  if (m) return { r: +m[1], g: +m[2], b: +m[3] }
  return null
}

export function contrastRatio(a: string, b: string): number | null {
  const ra = parseColor(a), rb = parseColor(b)
  if (!ra || !rb) return null
  const la = luminance(ra), lb = luminance(rb)
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la]
  return (hi + 0.05) / (lo + 0.05)
}

// 与背景对比度达标的文字色：优先同色相深/浅色，黑/白兜底
function readableText(bg: string, mode: 'light' | 'dark'): string {
  const rgb = parseColor(bg)
  if (!rgb) return mode === 'light' ? '#000000' : '#ffffff'
  const hsl = rgbToHsl(rgb.r, rgb.g, rgb.b)
  // 浅色模式：用深色文字（同色相）；深色模式：用浅色文字（同色相）
  let l = mode === 'light' ? 18 : 88
  let s = Math.max(6, Math.min(30, hsl.s))
  for (let i = 0; i < 8; i++) {
    const c = hslHex(hsl.h, s, l)
    const ratio = contrastRatio(c, bg)
    if (ratio !== null && ratio >= 4.5) return c
    l = mode === 'light' ? Math.max(4, l - 4) : Math.min(96, l + 4)
  }
  // 兜底：纯黑/白
  const black = contrastRatio('#000000', bg) ?? 0
  const white = contrastRatio('#ffffff', bg) ?? 0
  return black >= white ? '#000000' : '#ffffff'
}

// 面板色：背景向对比方向（浅色模式提亮 / 深色模式加深）走 step 步
function surfaceFrom(bg: string, mode: 'light' | 'dark', step: number): string {
  const rgb = parseColor(bg)
  if (!rgb) return bg
  const hsl = rgbToHsl(rgb.r, rgb.g, rgb.b)
  const l = mode === 'light' ? Math.min(97, hsl.l + step) : Math.max(6, hsl.l - step)
  return hslHex(hsl.h, Math.max(4, Math.min(12, hsl.s)), l)
}

// 边框：前景色的低透明度版本。透明度按模式区分，确保元素边界清晰
function borderFrom(fg: string, dark: boolean): string {
  const rgb = parseColor(fg)
  if (!rgb) return dark ? 'rgba(255,255,255,0.35)' : 'rgba(0,0,0,0.35)'
  return `rgba(${rgb.r},${rgb.g},${rgb.b},${dark ? 0.4 : 0.32})`
}

// tag/tool 六色系：固定绝对色相（与主色无关），保证颜色名实相符、六色互相可区分
const HUE_OFFSETS: Record<string, number> = {
  blue: 215, green: 145, amber: 42, rose: 348, teal: 175, purple: 272,
}

// 对比度不足的 bg/fg 对（只校验两个值都可解析的对）
export function checkThemeContrast(colors: Record<string, string>): { bg: string; fg: string; ratio: number }[] {
  const pairs: [string, string][] = [
    ['--background', '--foreground'],
    ['--card', '--card-foreground'],
    ['--popover', '--popover-foreground'],
    ['--primary', '--primary-foreground'],
    ['--secondary', '--secondary-foreground'],
    ['--muted', '--muted-foreground'],
    ['--accent', '--accent-foreground'],
    ['--destructive', '--destructive-foreground'],
    ['--sidebar', '--sidebar-foreground'],
    ['--sidebar-primary', '--sidebar-primary-foreground'],
    ['--sidebar-accent', '--sidebar-accent-foreground'],
    ['--bubble-user', '--bubble-user-foreground'],
    ['--action-extract', '--action-extract-foreground'],
    ['--action-save', '--action-save-foreground'],
    ['--tag-blue', '--tag-blue-foreground'],
    ['--tag-green', '--tag-green-foreground'],
    ['--tag-amber', '--tag-amber-foreground'],
    ['--tag-rose', '--tag-rose-foreground'],
    ['--tag-teal', '--tag-teal-foreground'],
    ['--tag-purple', '--tag-purple-foreground'],
    ['--success', '--success-foreground'],
  ]
  const bad: { bg: string; fg: string; ratio: number }[] = []
  for (const [bgK, fgK] of pairs) {
    const bgV = colors[bgK], fgV = colors[fgK]
    if (!bgV || !fgV) continue
    const ratio = contrastRatio(bgV, fgV)
    if (ratio === null) continue
    if (ratio < 4.5) bad.push({ bg: bgK, fg: fgK, ratio: Math.round(ratio * 100) / 100 })
  }
  return bad
}

// 一键生成全套主题变量（71 键）。所有 bg/fg 对在生成时修正到 AA。
export function generateTheme(input: ThemeGenInput): Record<string, string> {
  const { mode, background, primary, foreground, vibrancy = 1 } = input
  const dark = mode === 'dark'
  const vib = Math.max(0.3, Math.min(2, vibrancy))
  // 饱和度缩放（鲜艳度）：面板色保持低饱和，彩色系按比例缩放
  const sat = (s: number) => Math.max(0, Math.min(100, s * vib))

  const bg = background
  const modeStr: 'light' | 'dark' = dark ? 'dark' : 'light'
  // 文字色：用户指定且对比度达标则使用，否则自动对比色
  let fg = readableText(bg, modeStr)
  if (foreground && foreground.trim() !== '') {
    const ratio = contrastRatio(foreground, bg)
    if (ratio !== null && ratio >= 4.5) fg = foreground.trim()
  }
  const card = surfaceFrom(bg, modeStr, dark ? 4 : 3)
  const popover = surfaceFrom(bg, modeStr, dark ? 7 : 6)
  const secondary = surfaceFrom(bg, modeStr, dark ? 5 : 4)
  const muted = surfaceFrom(bg, modeStr, dark ? 5 : 4)
  const accent = surfaceFrom(bg, modeStr, dark ? 9 : 8)
  const sidebar = surfaceFrom(bg, modeStr, dark ? 2 : 2)
  const border = borderFrom(fg, dark)

  // 语义色：固定色相（成功绿/警告琥珀/危险红），不随主色漂移
  const primaryFg = readableText(primary, modeStr)
  const successH = 145
  const warnH = 42
  const dangerH = 4
  const success = hslHex(successH, sat(dark ? 30 : 45), dark ? 18 : 93)
  const successFg = hslHex(successH, sat(dark ? 45 : 60), dark ? 75 : 27)
  const statusWarning = hslHex(warnH, sat(dark ? 55 : 70), dark ? 68 : 38)
  const statusOk = hslHex(successH, sat(dark ? 45 : 60), dark ? 70 : 34)
  const destructive = hslHex(dangerH, sat(dark ? 40 : 55), dark ? 40 : 55)
  const dangerBg = hslHex(dangerH, sat(dark ? 30 : 30), dark ? 12 : 94)
  const dangerBorder = hslHex(dangerH, sat(dark ? 35 : 45), dark ? 28 : 82)

  // tag/tool 六色系：主色相 + 固定偏移，背景淡/深 + 文字对比
  const tags: Record<string, string> = {}
  const tagNames = ['blue', 'green', 'amber', 'rose', 'teal', 'purple']
  for (const name of tagNames) {
    const h = HUE_OFFSETS[name]
    const tagBg = hslHex(h, sat(dark ? 35 : 30), dark ? 18 : 93)
    const tagFg = readableText(tagBg, modeStr)
    tags[`--tag-${name}`] = tagBg
    tags[`--tag-${name}-foreground`] = tagFg
    const toolBg = hslHex(h, sat(dark ? 30 : 25), dark ? 17 : 94)
    const toolBorder = hslHex(h, sat(dark ? 45 : 55), dark ? 52 : 52)
    tags[`--tool-${name}`] = toolBg
    tags[`--tool-${name}-border`] = toolBorder
  }

  // 贡献图：绿色系 5 档
  const contribH = successH
  const contrib: Record<string, string> = {}
  const contribL = dark ? [16, 26, 38, 52, 66] : [74, 62, 50, 38, 28]
  contribL.forEach((l, i) => {
    contrib[`--contribution-${i}`] = hslHex(contribH, sat(dark ? 40 : 50), l)
  })

  // 质感层：背景渐变 + 发光跟随主色（与 index.css 的 custom 兜底派生一致）
  const pr = hexToRgb(primary)
  const p = (a: number) => pr ? `rgba(${pr.r},${pr.g},${pr.b},${a})` : primary
  const bgGrad = `radial-gradient(ellipse at 30% 20%, ${p(0.1)} 0%, transparent 60%), radial-gradient(ellipse at 70% 80%, ${p(0.07)} 0%, transparent 55%), linear-gradient(180deg, ${bg} 0%, ${bg} 100%)`

  return {
    '--background': bg,
    '--foreground': fg,
    '--card': card,
    '--card-foreground': readableText(card, modeStr),
    '--popover': popover,
    '--popover-foreground': readableText(popover, modeStr),
    '--primary': primary,
    '--primary-foreground': primaryFg,
    '--secondary': secondary,
    '--secondary-foreground': readableText(secondary, modeStr),
    '--muted': muted,
    '--muted-foreground': readableText(muted, modeStr),
    '--accent': accent,
    '--accent-foreground': readableText(accent, modeStr),
    '--destructive': destructive,
    '--destructive-foreground': readableText(destructive, modeStr),
    '--border': border,
    '--input': secondary,
    '--ring': primary,
    '--sidebar': sidebar,
    '--sidebar-foreground': readableText(sidebar, modeStr),
    '--sidebar-primary': primary,
    '--sidebar-primary-foreground': primaryFg,
    '--sidebar-accent': accent,
    '--sidebar-accent-foreground': readableText(accent, modeStr),
    '--sidebar-border': borderFrom(fg, dark),
    '--sidebar-ring': primary,
    ...tags,
    '--success': success,
    '--success-foreground': successFg,
    '--success-border': hslHex(successH, dark ? 40 : 45, dark ? 30 : 80),
    '--danger-bg': dangerBg,
    '--danger-border': dangerBorder,
    '--status-warning': statusWarning,
    '--status-ok': statusOk,
    '--reader-bg': bg,
    '--reader-paper': card,
    '--bubble-user': primary,
    '--bubble-user-foreground': primaryFg,
    '--action-extract': surfaceFrom(bg, modeStr, dark ? 12 : 10),
    '--action-extract-foreground': readableText(surfaceFrom(bg, modeStr, dark ? 12 : 10), modeStr),
    '--action-save': success,
    '--action-save-foreground': successFg,
    '--bg-layer-grad': bgGrad,
    '--glow': p(dark ? 0.3 : 0.35),
    '--glow-strong': p(dark ? 0.55 : 0.6),
    ...contrib,
  }
}
