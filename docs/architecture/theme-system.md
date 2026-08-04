# 主题系统（Theme System）

> 日期：2026-08-04
> 对应代码：`frontend/src/hooks/useTheme.ts`、`frontend/src/components/settings/ThemeConfigTab.tsx`

---

## 一、架构概览

Goink 的主题系统基于 **CSS 变量 + `data-theme` 属性** 驱动，不依赖 CSS-in-JS 或预处理器。

```
用户选择主题
    ↓
useTheme.ts（localStorage 持久化）
    ↓
document.documentElement.setAttribute('data-theme', 'light' | 'dark' | 'custom:名称')
    ↓
CSS 选择器 [data-theme="..."] 控制各变量值
    ↓
全局样式使用 var(--xxx) 引用变量
```

### 1.1 主题层级

| 层级 | 来源 | 说明 |
|------|------|------|
| 内置浅色 | `index.css` `:root` | 默认浅色，白底深字 |
| 内置深色 | `index.css` `[data-theme="dark"]` | 深色模式 |
| 自定义主题 | 用户 JSON → `localStorage` → `<style>` 注入 | 用户通过设置面板编辑 |

### 1.2 变量优先级

CSS 变量通过 `color-mix(in oklab, ...)` 派生，形成层级链：

```
--color-primary ──→ --primary ──→ --narrative-current-bg (color-mix)
                                    --narrative-current-border
                                    --narrative-hook-type
```

用户只需设置 `--color-primary`，所有派生变量自动跟随。

---

## 二、完整变量清单

### 2.1 核心颜色（语义层）

设置面板的示例主题 JSON 中定义 `--color-xxx` 形式，CSS 内部引用 `--xxx`（无前缀）。两者值相同，`--color-` 前缀仅用于标识。

| 变量 | 用途 | 浅色默认值 | 深色默认值 |
|------|------|-----------|-----------|
| `--background` | 页面背景 | `#ffffff` | `#0f172a` |
| `--foreground` | 主文字色 | `#1d1d1f` | `#e2e8f0` |
| `--card` | 卡片背景 | `#ffffff` | `#1e293b` |
| `--card-foreground` | 卡片文字 | `#1d1d1f` | `#e2e8f0` |
| `--popover` | 弹窗背景 | `#ffffff` | `#1e293b` |
| `--popover-foreground` | 弹窗文字 | `#1d1d1f` | `#e2e8f0` |
| `--primary` | 强调色 | `#2563eb` | `#60a5fa` |
| `--primary-foreground` | 强调色上的文字 | `#ffffff` | `#0f172a` |
| `--secondary` | 次要背景 | `#f2f3f7` | `#1e293b` |
| `--secondary-foreground` | 次要文字 | `#1d1d1f` | `#e2e8f0` |
| `--muted` | 弱化背景 | `#f2f3f7` | `#1e293b` |
| `--muted-foreground` | 弱化文字 | `#86868b` | `#94a3b8` |
| `--accent` | 强调背景 | `#f2f3f7` | `#1e293b` |
| `--accent-foreground` | 强调文字 | `#1d1d1f` | `#e2e8f0` |
| `--destructive` | 危险/错误 | `#e74c3c` | `#ef4444` |
| `--destructive-foreground` | 危险文字 | `#ffffff` | `#ffffff` |
| `--border` | 边框 | `rgba(0,0,0,0.06)` | `rgba(255,255,255,0.08)` |
| `--input` | 输入框背景 | `#f2f3f7` | `#1e293b` |
| `--ring` | 焦点环 | `#2563eb` | `#60a5fa` |

### 2.2 侧边栏

| 变量 | 用途 | 浅色默认值 | 深色默认值 |
|------|------|-----------|-----------|
| `--sidebar` | 侧边栏背景 | `#f8f9fa` | `#1a1f2e` |
| `--sidebar-foreground` | 侧边栏文字 | `#1d1d1f` | `#e2e8f0` |
| `--sidebar-primary` | 侧边栏强调色 | `#2563eb` | `#60a5fa` |
| `--sidebar-primary-foreground` | 侧边栏强调文字 | `#ffffff` | `#0f172a` |
| `--sidebar-accent` | 侧边栏项悬浮背景 | `#e8ecf0` | `#1e293b` |
| `--sidebar-accent-foreground` | 侧边栏悬浮文字 | `#1d1d1f` | `#e2e8f0` |
| `--sidebar-border` | 侧边栏边框 | `rgba(0,0,0,0.06)` | `rgba(255,255,255,0.08)` |
| `--sidebar-ring` | 侧边栏焦点环 | `#2563eb` | `#60a5fa` |

### 2.3 标签颜色

| 变量 | 用途 | 浅色默认值 | 深色默认值 |
|------|------|-----------|-----------|
| `--tag-blue` | 蓝色标签背景 | `#e8f0fe` | `#1a2a30` |
| `--tag-blue-foreground` | 蓝色标签文字 | `#1967d2` | `#6a9aaa` |
| `--tag-green` | 绿色标签背景 | `#e6f4ea` | `#1a2a1e` |
| `--tag-green-foreground` | 绿色标签文字 | `#1e7e34` | `#5a8a5a` |
| `--tag-amber` | 琥珀标签背景 | `#fef7e0` | `#2a2818` |
| `--tag-amber-foreground` | 琥珀标签文字 | `#b06000` | `#9a8a3a` |
| `--tag-rose` | 玫红标签背景 | `#fce8e6` | `#2a1818` |
| `--tag-rose-foreground` | 玫红标签文字 | `#c5221f` | `#9a5a5a` |
| `--tag-teal` | 青色标签背景 | `#e0f2f1` | `#182a28` |
| `--tag-teal-foreground` | 青色标签文字 | `#00796b` | `#5a8a8a` |
| `--tag-purple` | 紫色标签背景 | `#f3e8ff` | `#2a1e30` |
| `--tag-purple-foreground` | 紫色标签文字 | `#7c3aed` | `#8a6a9a` |

### 2.4 气泡 & 操作

| 变量 | 用途 | 浅色默认值 | 深色默认值 |
|------|------|-----------|-----------|
| `--bubble-user` | 用户消息气泡背景 | `#2563eb` | `#60a5fa` |
| `--bubble-user-foreground` | 用户消息气泡文字 | `#ffffff` | `#0f172a` |
| `--action-extract` | 提取操作按钮 | `#6b7280` | `#6a7a5a` |
| `--action-extract-foreground` | 提取操作文字 | `#ffffff` | `#faf4e4` |
| `--action-save` | 保存操作按钮 | `#2563eb` | `#4a7a4a` |
| `--action-save-foreground` | 保存操作文字 | `#ffffff` | `#faf4e4` |

### 2.5 状态色

| 变量 | 用途 | 浅色默认值 | 深色默认值 |
|------|------|-----------|-----------|
| `--success` | 成功背景 | `#e6f4ea` | `#1a2a1e` |
| `--success-foreground` | 成功文字 | `#1e7e34` | `#5a8a5a` |
| `--success-border` | 成功边框 | `#a8d8b0` | `#2a4a3a` |
| `--danger-bg` | 危险背景 | `#fce8e6` | `#2a1a18` |
| `--danger-border` | 危险边框 | `#e8a098` | `#4a2a20` |
| `--status-warning` | 警告文字/图标 | `#c49a50` | `#d5b060` |
| `--status-ok` | 正常文字/图标 | `#2e7d32` | `#4a8a5a` |

### 2.6 工具调用颜色

| 变量 | 用途 | 浅色默认值 | 深色默认值 |
|------|------|-----------|-----------|
| `--tool-blue` | 读操作背景 | `#e8f0fe` | `#1a2a38` |
| `--tool-blue-border` | 读操作边框 | `#a8c7fa` | `#4a7a9a` |
| `--tool-amber` | 写操作背景 | `#fef7e0` | `#2a2818` |
| `--tool-amber-border` | 写操作边框 | `#f9d7a0` | `#9a8030` |
| `--tool-green` | 创建操作背景 | `#e6f4ea` | `#1a2820` |
| `--tool-green-border` | 创建操作边框 | `#a8d8b0` | `#4a7a4a` |
| `--tool-red` | 删除操作背景 | `#fce8e6` | `#2a1a18` |
| `--tool-red-border` | 删除操作边框 | `#e8a098` | `#9a4a3a` |

### 2.7 贡献图

| 变量 | 用途 | 浅色默认值 | 深色默认值 |
|------|------|-----------|-----------|
| `--contribution-0` | 无贡献 | `#ebedf0` | `#16241e` |
| `--contribution-1` | 低贡献 | `#9be9a8` | `#1a3a28` |
| `--contribution-2` | 中贡献 | `#40c463` | `#2a5a3a` |
| `--contribution-3` | 高贡献 | `#30a14e` | `#3a7a4a` |
| `--contribution-4` | 最高贡献 | `#216e39` | `#4a9a5a` |

### 2.8 阅读器

| 变量 | 用途 | 浅色默认值 | 深色默认值 |
|------|------|-----------|-----------|
| `--reader-bg` | 阅读器背景 | `#faf8f4` | `#0f1a14` |
| `--reader-paper` | 阅读器纸面 | `#ffffff` | `#1a2a20` |

---

## 三、Narrative 面板变量（派生关系）

narrative 面板的 14 个 CSS 变量通过 `color-mix()` 从核心颜色派生，含义如下：

| 变量 | 派生公式 | 来源变量 |
|------|---------|---------|
| `--narrative-current-bg` | `color-mix(in oklab, var(--primary) 8%, var(--sidebar))` | primary, sidebar |
| `--narrative-current-border` | `var(--primary)` | primary |
| `--narrative-overdue-bg` | `color-mix(in oklab, var(--destructive) 12%, var(--sidebar))` | destructive, sidebar |
| `--narrative-overdue-text` | `var(--destructive)` | destructive |
| `--narrative-resolved-text` | `var(--success-foreground)` | success-foreground |
| `--narrative-resolved-bg` | `color-mix(in oklab, var(--success-foreground) 10%, var(--sidebar))` | success-foreground, sidebar |
| `--narrative-pending-text` | `var(--tag-blue-foreground)` | tag-blue-foreground |
| `--narrative-pending-bg` | `var(--tag-blue)` | tag-blue |
| `--narrative-arc-inactive` | `color-mix(in oklab, var(--muted-foreground) 20%, transparent)` | muted-foreground |
| `--narrative-future-card-bg` | `var(--card)` | card |
| `--narrative-future-card-border` | `var(--border)` | border |
| `--narrative-hook-type` | `var(--primary)` | primary |
| `--narrative-tab-active` | `var(--primary)` | primary |
| `--narrative-divider` | `var(--border)` | border |

> 用户只需设置核心颜色变量（如 `--primary`、`--sidebar`），narrative 面板的派生变量自动跟随。

---

## 四、自定义主题

### 4.1 创建步骤

1. 打开设置 → 自定义主题
2. 在 JSON 编辑器中粘贴主题 JSON
3. 单击「应用」即生效并保存
4. 已保存的主题出现在列表中，单击即可切换

### 4.2 JSON 格式

```json
{
  "name": "主题名称",
  "type": "dark",
  "colors": {
    "--background": "#...",
    "--foreground": "#...",
    ...
  }
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 是 | 主题显示名称，去重键为 `name__type` |
| `type` | 否（默认 `dark`） | `light` 或 `dark`，影响 `color-mix()` 的透明度感知 |
| `colors` | 是 | 所有 CSS 变量键值对，不需要全部覆盖，缺失的变量使用内置默认值 |

### 4.3 应用规则

- 同名同类型覆盖：`墨绿书斋__dark` 第二次应用会覆盖第一次
- 单击即应用：列表中的主题单击后立即生效，无需确认
- 持久化：保存在 `localStorage('goink_custom_themes')`，重启后保留

---

## 五、示例主题：Apple 白

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

> 设计说明：Apple 白主题以 `#ffffff` 纯白背景 + `#0071e3` 苹果蓝为基调，`#f5f5f7` 浅灰作为次要背景和输入框底色，`#e8e8ed` 浅灰作为悬浮/边框。整体风格干净、高对比度，与 Apple 设计语言一致。

---

## 六、注意事项

1. **`--narrative-resolved-text`** 已用 `--success-foreground` 派生，自定义主题设置 `--success-foreground` 即可控制
2. 所有 `color-mix()` 百分比已调好，不建议修改百分比值
3. `--border` 使用 `rgba` 而非纯色，确保在不同背景色上融合自然
4. 自定义主题不需要覆盖所有变量，缺失的变量会使用内置默认值