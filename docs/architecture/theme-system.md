# 主题系统

> 对应代码：`frontend/src/hooks/useTheme.ts`、`frontend/src/components/settings/ThemeConfigTab.tsx`
> 最后更新：2026-08-04

---

## 一、颜色格式说明

主题支持以下颜色值格式，CSS 变量值写为字符串即可：

| 格式 | 示例 | 适用场景 |
|------|------|---------|
| HEX | `"#ff6600"` | 纯色，不透明 |
| RGB | `"rgb(255,102,0)"` | 纯色，不透明 |
| RGBA | `"rgba(0,0,0,0.06)"` | 半透明（边框、叠加层） |
| HSL | `"hsl(210,100%,45%)"` | **色相调色（推荐）** |
| OKLCH | `"oklch(0.6 0.15 30)"` | 感知均匀色彩空间 |

> **推荐使用 HSL 格式**。HSL 的三个维度（色相 H / 饱和度 S / 明度 L）直接对应人类视觉感知，通过固定色相 H 并调整 S/L 即可派生出一整套协调配色。例如定 H=40°（暖黄）为基底色，全组颜色自动在同一色系内，改 H 值即可快速切换风格。

<!-- 格式说明：HEX 6 位，RGBA 用于透明度，OKLCH 用于 color-mix 计算 -->

---

## 二、变量总表

共 56 个变量，按用途分组。每组内的 `--*-foreground` 变量是该组背景色上使用的文字色。

### 2.1 核心层（18 个）

这组变量控制页面最基础的视觉层级。`--background` 是最底层，`--card`/`--popover` 依次浮起，`--border` 是分隔线。

| 变量 | 用途 | 浅色 | 深色 |
|------|------|------|------|
| `--background` | 页面背景 | `#ffffff` | `#0f172a` |
| `--foreground` | 主文字色 | `#1d1d1f` | `#e2e8f0` |
| `--card` | 卡片背景 | `#ffffff` | `#1e293b` |
| `--card-foreground` | 卡片文字 | `#1d1d1f` | `#e2e8f0` |
| `--popover` | 弹窗/下拉背景 | `#ffffff` | `#1e293b` |
| `--popover-foreground` | 弹窗文字 | `#1d1d1f` | `#e2e8f0` |
| `--primary` | 强调色（按钮/链接/选中） | `#2563eb` | `#60a5fa` |
| `--primary-foreground` | 强调色上的文字 | `#ffffff` | `#0f172a` |
| `--secondary` | 次要背景 | `#f2f3f7` | `#1e293b` |
| `--secondary-foreground` | 次要文字 | `#1d1d1f` | `#e2e8f0` |
| `--muted` | 弱化背景（灰色块） | `#f2f3f7` | `#1e293b` |
| `--muted-foreground` | 弱化文字（辅助信息） | `#86868b` | `#94a3b8` |
| `--accent` | 悬浮/高亮背景 | `#f2f3f7` | `#1e293b` |
| `--accent-foreground` | 悬浮文字 | `#1d1d1f` | `#e2e8f0` |
| `--destructive` | 危险/删除 | `#e74c3c` | `#ef4444` |
| `--destructive-foreground` | 危险上的文字 | `#ffffff` | `#ffffff` |
| `--border` | 边框/分隔线 | `rgba(0,0,0,0.06)` | `rgba(255,255,255,0.08)` |
| `--input` | 输入框背景 | `#f2f3f7` | `#1e293b` |
| `--ring` | 焦点环（键盘导航） | `#2563eb` | `#60a5fa` |

### 2.2 侧边栏层（8 个）

| 变量 | 用途 | 浅色 | 深色 |
|------|------|------|------|
| `--sidebar` | 侧边栏背景 | `#f8f9fa` | `#1a1f2e` |
| `--sidebar-foreground` | 侧边栏文字 | `#1d1d1f` | `#e2e8f0` |
| `--sidebar-primary` | 侧栏选中标记 | `#2563eb` | `#60a5fa` |
| `--sidebar-primary-foreground` | 选中标记上的文字 | `#ffffff` | `#0f172a` |
| `--sidebar-accent` | 侧栏项悬浮背景 | `#e8ecf0` | `#1e293b` |
| `--sidebar-accent-foreground` | 悬浮文字 | `#1d1d1f` | `#e2e8f0` |
| `--sidebar-border` | 侧栏分隔线 | `rgba(0,0,0,0.06)` | `rgba(255,255,255,0.08)` |
| `--sidebar-ring` | 侧栏焦点环 | `#2563eb` | `#60a5fa` |

### 2.3 标签色（6 色 × 2 = 12 个）

每色含背景（`--tag-{color}`）和文字（`--tag-{color}-foreground`）。

| 变量 | 浅色背景 | 浅色文字 | 深色背景 | 深色文字 |
|------|---------|---------|---------|---------|
| `--tag-blue` | `#e8f0fe` | `#1967d2` | `#1a2a30` | `#6a9aaa` |
| `--tag-green` | `#e6f4ea` | `#1e7e34` | `#1a2a1e` | `#5a8a5a` |
| `--tag-amber` | `#fef7e0` | `#b06000` | `#2a2818` | `#9a8a3a` |
| `--tag-rose` | `#fce8e6` | `#c5221f` | `#2a1818` | `#9a5a5a` |
| `--tag-teal` | `#e0f2f1` | `#00796b` | `#182a28` | `#5a8a8a` |
| `--tag-purple` | `#f3e8ff` | `#7c3aed` | `#2a1e30` | `#8a6a9a` |

### 2.4 消息气泡（2 个）

| 变量 | 用途 | 浅色 | 深色 |
|------|------|------|------|
| `--bubble-user` | 用户消息气泡背景 | `#2563eb` | `#60a5fa` |
| `--bubble-user-foreground` | 气泡内文字 | `#ffffff` | `#0f172a` |

### 2.5 操作按钮（4 个）

| 变量 | 用途 | 浅色 | 深色 |
|------|------|------|------|
| `--action-extract` | 提取按钮背景 | `#6b7280` | `#6a7a5a` |
| `--action-extract-foreground` | 提取按钮文字 | `#ffffff` | `#faf4e4` |
| `--action-save` | 保存按钮背景 | `#2563eb` | `#4a7a4a` |
| `--action-save-foreground` | 保存按钮文字 | `#ffffff` | `#faf4e4` |

### 2.6 状态色（7 个）

| 变量 | 用途 | 浅色 | 深色 |
|------|------|------|------|
| `--success` | 成功背景 | `#e6f4ea` | `#1a2a1e` |
| `--success-foreground` | 成功文字/图标 | `#1e7e34` | `#5a8a5a` |
| `--success-border` | 成功边框 | `#a8d8b0` | `#2a4a3a` |
| `--danger-bg` | 危险背景 | `#fce8e6` | `#2a1a18` |
| `--danger-border` | 危险边框 | `#e8a098` | `#4a2a20` |
| `--status-warning` | 警告文字/图标 | `#c49a50` | `#d5b060` |
| `--status-ok` | 正常文字/图标 | `#2e7d32` | `#4a8a5a` |

### 2.7 工具调用色（4 色 × 2 = 8 个）

每色含背景（`--tool-{color}`）和边框（`--tool-{color}-border`）。

| 变量 | 用途 | 浅色背景 | 浅色边框 | 深色背景 | 深色边框 |
|------|------|---------|---------|---------|---------|
| `--tool-blue` | 读操作 | `#e8f0fe` | `#a8c7fa` | `#1a2a38` | `#4a7a9a` |
| `--tool-amber` | 写操作 | `#fef7e0` | `#f9d7a0` | `#2a2818` | `#9a8030` |
| `--tool-green` | 创建操作 | `#e6f4ea` | `#a8d8b0` | `#1a2820` | `#4a7a4a` |
| `--tool-red` | 删除操作 | `#fce8e6` | `#e8a098` | `#2a1a18` | `#9a4a3a` |

### 2.8 贡献图（5 个）

| 变量 | 含义 | 浅色 | 深色 |
|------|------|------|------|
| `--contribution-0` | 无贡献 | `#ebedf0` | `#16241e` |
| `--contribution-1` | 低 | `#9be9a8` | `#1a3a28` |
| `--contribution-2` | 中 | `#40c463` | `#2a5a3a` |
| `--contribution-3` | 高 | `#30a14e` | `#3a7a4a` |
| `--contribution-4` | 最高 | `#216e39` | `#4a9a5a` |

### 2.9 阅读器（2 个）

| 变量 | 用途 | 浅色 | 深色 |
|------|------|------|------|
| `--reader-bg` | 阅读器整体背景 | `#faf8f4` | `#0f1a14` |
| `--reader-paper` | 阅读器纸面 | `#ffffff` | `#1a2a20` |

---

## 三、派生变量

### 3.1 来源关系

以下变量从核心变量自动派生，自定义主题中不需要设置它们：

```
--border           ← 建议从 --foreground 调透明度派生，非硬编码
--sidebar-border   ← 同 --border（分开控制时可独立设）
--sidebar-ring     ← 同 --ring
--narrative-*      ← 从 --primary/--sidebar/--destructive/--success-foreground 等派生
```

### 3.2 Narrative 面板派生公式

| 变量 | 公式 |
|------|------|
| `--narrative-current-bg` | `color-mix(in oklab, var(--primary) 8%, var(--sidebar))` |
| `--narrative-current-border` | `var(--primary)` |
| `--narrative-overdue-bg` | `color-mix(in oklab, var(--destructive) 12%, var(--sidebar))` |
| `--narrative-overdue-text` | `var(--destructive)` |
| `--narrative-resolved-text` | `var(--success-foreground)` |
| `--narrative-resolved-bg` | `color-mix(in oklab, var(--success-foreground) 10%, var(--sidebar))` |
| `--narrative-pending-text` | `var(--tag-blue-foreground)` |
| `--narrative-pending-bg` | `var(--tag-blue)` |
| `--narrative-arc-inactive` | `color-mix(in oklab, var(--muted-foreground) 20%, transparent)` |
| `--narrative-future-card-bg` | `var(--card)` |
| `--narrative-future-card-border` | `var(--border)` |
| `--narrative-hook-type` | `var(--primary)` |
| `--narrative-tab-active` | `var(--primary)` |
| `--narrative-divider` | `var(--border)` |

---

## 四、自定义主题

### 4.1 JSON 格式

```json
{
  "name": "主题名称",
  "type": "dark",
  "colors": {
    "--background": "#ffffff",
    "--foreground": "#1d1d1f",
    "--primary": "#2563eb"
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 显示名称。去重键为 `name__type` |
| `type` | `"light"` 或 `"dark"` | 否，默认 `dark` | 仅标记，不影响 CSS 变量值 |
| `colors` | object | 是 | 键为变量名，值为颜色字符串 |

### 4.2 应用规则

- 同名同类型覆盖：`"墨绿书斋__dark"` 第二次应用覆盖第一次
- 单击列表中的主题即应用，无需确认
- 持久化到 `localStorage("goink_custom_themes")`，重启保留
- 缺失的变量使用内置默认值，不需要覆盖全部

---

## 五、示例主题

### Apple 白（浅色）

```json
{
  "name": "Apple 白",
  "type": "light",
  "colors": {
    "--background": "#ffffff",
    "--foreground": "#1d1d1f",
    "--card": "#ffffff",
    "--card-foreground": "#1d1d1f",
    "--popover": "#ffffff",
    "--popover-foreground": "#1d1d1f",
    "--primary": "#0071e3",
    "--primary-foreground": "#ffffff",
    "--secondary": "#f5f5f7",
    "--secondary-foreground": "#1d1d1f",
    "--muted": "#f5f5f7",
    "--muted-foreground": "#86868b",
    "--accent": "#e8e8ed",
    "--accent-foreground": "#1d1d1f",
    "--destructive": "#ff3b30",
    "--destructive-foreground": "#ffffff",
    "--border": "rgba(0,0,0,0.06)",
    "--input": "#f5f5f7",
    "--ring": "#0071e3",
    "--sidebar": "#fafafa",
    "--sidebar-foreground": "#1d1d1f",
    "--sidebar-primary": "#0071e3",
    "--sidebar-primary-foreground": "#ffffff",
    "--sidebar-accent": "#e8e8ed",
    "--sidebar-accent-foreground": "#1d1d1f",
    "--sidebar-border": "rgba(0,0,0,0.06)",
    "--sidebar-ring": "#0071e3",
    "--tag-blue": "#e8f0fe",
    "--tag-blue-foreground": "#0071e3",
    "--tag-green": "#e8f8ee",
    "--tag-green-foreground": "#34c759",
    "--tag-amber": "#fff7e0",
    "--tag-amber-foreground": "#ff9f0a",
    "--tag-rose": "#ffe8e6",
    "--tag-rose-foreground": "#ff3b30",
    "--tag-teal": "#e0f5f5",
    "--tag-teal-foreground": "#30b0c7",
    "--tag-purple": "#f3e8ff",
    "--tag-purple-foreground": "#af52de",
    "--reader-bg": "#fafafa",
    "--reader-paper": "#ffffff",
    "--bubble-user": "#0071e3",
    "--bubble-user-foreground": "#ffffff",
    "--action-extract": "#86868b",
    "--action-extract-foreground": "#ffffff",
    "--action-save": "#0071e3",
    "--action-save-foreground": "#ffffff",
    "--success": "#e8f8ee",
    "--success-foreground": "#34c759",
    "--success-border": "#b8e8c8",
    "--danger-bg": "#ffe8e6",
    "--danger-border": "#ffb8b0",
    "--status-warning": "#ff9f0a",
    "--status-ok": "#34c759",
    "--tool-blue": "#e8f0fe",
    "--tool-blue-border": "#a8c8fa",
    "--tool-amber": "#fff7e0",
    "--tool-amber-border": "#f9d7a0",
    "--tool-green": "#e8f8ee",
    "--tool-green-border": "#a8e8c0",
    "--tool-red": "#ffe8e6",
    "--tool-red-border": "#ffb8b0",
    "--contribution-0": "#ebedf0",
    "--contribution-1": "#9be9a8",
    "--contribution-2": "#40c463",
    "--contribution-3": "#30a14e",
    "--contribution-4": "#216e39"
  }
}
```

### ClickHouse 暗黑（深色）

```json
{
  "name": "ClickHouse 暗黑",
  "type": "dark",
  "colors": {
    "--background": "#000000",
    "--foreground": "#ffffff",
    "--card": "#141414",
    "--card-foreground": "#ffffff",
    "--popover": "#141414",
    "--popover-foreground": "#ffffff",
    "--primary": "#faff69",
    "--primary-foreground": "#000000",
    "--secondary": "#141414",
    "--secondary-foreground": "#ffffff",
    "--muted": "#141414",
    "--muted-foreground": "#a0a0a0",
    "--accent": "#3a3a3a",
    "--accent-foreground": "#ffffff",
    "--destructive": "#e74c3c",
    "--destructive-foreground": "#ffffff",
    "--border": "rgba(65,65,65,0.8)",
    "--input": "#141414",
    "--ring": "#faff69",
    "--sidebar": "#0a0a0a",
    "--sidebar-foreground": "#ffffff",
    "--sidebar-primary": "#faff69",
    "--sidebar-primary-foreground": "#000000",
    "--sidebar-accent": "#3a3a3a",
    "--sidebar-accent-foreground": "#ffffff",
    "--sidebar-border": "rgba(65,65,65,0.8)",
    "--sidebar-ring": "#faff69",
    "--tag-blue": "#1a2a30",
    "--tag-blue-foreground": "#6a9aaa",
    "--tag-green": "#166534",
    "--tag-green-foreground": "#ffffff",
    "--tag-amber": "#2a2818",
    "--tag-amber-foreground": "#faff69",
    "--tag-rose": "#2a1818",
    "--tag-rose-foreground": "#e74c3c",
    "--tag-teal": "#182a28",
    "--tag-teal-foreground": "#5a8a8a",
    "--tag-purple": "#2a1e30",
    "--tag-purple-foreground": "#8a6a9a",
    "--reader-bg": "#000000",
    "--reader-paper": "#141414",
    "--bubble-user": "#faff69",
    "--bubble-user-foreground": "#000000",
    "--action-extract": "#414141",
    "--action-extract-foreground": "#ffffff",
    "--action-save": "#166534",
    "--action-save-foreground": "#ffffff",
    "--success": "#166534",
    "--success-foreground": "#5a8a5a",
    "--success-border": "#2a4a3a",
    "--danger-bg": "#2a1a18",
    "--danger-border": "#4a2a20",
    "--status-warning": "#faff69",
    "--status-ok": "#5a8a5a",
    "--tool-blue": "#1a2a38",
    "--tool-blue-border": "#4a7a9a",
    "--tool-amber": "#2a2818",
    "--tool-amber-border": "#faff69",
    "--tool-green": "#166534",
    "--tool-green-border": "#2a4a3a",
    "--tool-red": "#2a1a18",
    "--tool-red-border": "#4a2a20",
    "--contribution-0": "#141414",
    "--contribution-1": "#2a4a3a",
    "--contribution-2": "#166534",
    "--contribution-3": "#3a7a4a",
    "--contribution-4": "#5a8a5a"
  }
}
```
---

## 六、自定义主题兜底派生（永不撞色）

自定义主题只覆盖用户填写的变量。**未填的变量由 `index.css` 的 `[data-theme^="custom:"]` 块自动派生**，保证任何配色下文字可读、面板有层次、边框可见：

- **文字自动对比色**：所有 `--*-foreground` 未填时用 `contrast-color(var(--*-bg))`——浏览器自动选黑/白中对比度更高的（WebView2/Chromium 127+ 支持）
- **面板明度阶梯**：`--card/--popover/--secondary/--muted/--accent/--sidebar` 未填时从 `--background` 向对比色方向混色派生（浅色模式自动变浅、深色模式自动变深）
- **边框跟随前景**：`--border` 派生为前景色 18% 透明，任何背景上可见
- **优先级**：用户注入的 style 位于 head 末尾且选择器特异性相同，填写的值覆盖派生；未填的走派生

## 七、一键生成器（设置 → 自定义主题）

输入「主题名 + 模式（浅/深）+ 背景色 + 主色」即可生成全套 71 个变量并应用：

- 派生规则：主色为色相基准，tag/tool 六色系按固定色相偏移（blue+40/green+90/amber+25/rose+155/teal+160/purple+215），保持六色可区分
- 面板色从背景按明度阶梯派生；贡献图用绿色系 5 档
- **所有 bg/fg 对在生成时自动修正到 WCAG AA（≥4.5:1）**，黑/白兜底
- 实现：`frontend/src/lib/themeColors.ts`（`generateTheme`），单测 `themeColors.test.ts`

## 八、对比度校验（粘贴 JSON 时）

粘贴 JSON 应用时，`checkThemeContrast` 会校验 21 对 bg/fg 组合（两个值都可解析的对），不达标（<4.5:1）时显示警告列表。只警告不阻止——大号文字 3:1 也可接受，用户自行判断。支持 hex/rgb 解析，hsl/oklch 格式跳过校验（走 CSS 兜底）。

## 注意事项

- `--border` 建议用 RGBA 透明度而非纯色，确保在不同背景色上融合自然
- 自定义主题不需要覆盖全部变量，缺失的变量使用内置默认值
- 所有 `color-mix()` 百分比已调好，不建议修改
- 同名同类型主题会覆盖，命名注意区分