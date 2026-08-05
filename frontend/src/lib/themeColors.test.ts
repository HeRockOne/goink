import { describe, expect, it } from 'vitest'
import { checkThemeContrast, contrastRatio, generateTheme } from './themeColors'

const ALL_PAIRS: [string, string][] = [
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

function assertAllPairsAA(colors: Record<string, string>) {
  const bad: string[] = []
  for (const [bgK, fgK] of ALL_PAIRS) {
    const ratio = contrastRatio(colors[bgK], colors[fgK])
    expect(ratio, `${bgK} × ${fgK} 应可解析`).not.toBeNull()
    if (ratio !== null && ratio < 4.5) bad.push(`${bgK}×${fgK}=${ratio.toFixed(2)}:1`)
  }
  expect(bad, `存在对比度不足的对: ${bad.join(', ')}`).toEqual([])
}

describe('themeColors', () => {
  it('深色主题：深棕背景 + 棕主色', () => {
    const c = generateTheme({ name: 't', mode: 'dark', background: '#1a1210', primary: '#c4956a' })
    assertAllPairsAA(c)
  })

  it('深色主题：墨绿背景 + 绿主色', () => {
    const c = generateTheme({ name: 't', mode: 'dark', background: '#0f1a14', primary: '#5a9a6a' })
    assertAllPairsAA(c)
  })

  it('浅色主题：米白背景 + 棕主色', () => {
    const c = generateTheme({ name: 't', mode: 'light', background: '#f5edd6', primary: '#8b5e3c' })
    assertAllPairsAA(c)
  })

  it('浅色主题：白色背景 + 蓝主色', () => {
    const c = generateTheme({ name: 't', mode: 'light', background: '#ffffff', primary: '#2563eb' })
    assertAllPairsAA(c)
  })

  it('极端色：纯黑背景 + 高饱和红主色', () => {
    const c = generateTheme({ name: 't', mode: 'dark', background: '#000000', primary: '#ff0000' })
    assertAllPairsAA(c)
  })

  it('生成完整主题（71 键：核心 + tag/tool 六色 + 状态/阅读/气泡/操作/贡献图）', () => {
    const c = generateTheme({ name: 't', mode: 'dark', background: '#0f1a14', primary: '#5a9a6a' })
    expect(Object.keys(c).length).toBe(71)
  })

  it('校验函数能识别不达标对', () => {
    const bad = checkThemeContrast({ '--background': '#ffffff', '--foreground': '#eeeeee' })
    expect(bad.length).toBe(1)
    expect(bad[0].bg).toBe('--background')
  })

  it('校验函数跳过未填对', () => {
    const bad = checkThemeContrast({ '--background': '#ffffff' })
    expect(bad).toEqual([])
  })

  it('校验函数跳过不可解析格式', () => {
    const bad = checkThemeContrast({ '--background': '#ffffff', '--foreground': 'oklch(0.9 0 0)' })
    expect(bad).toEqual([])
  })
})
