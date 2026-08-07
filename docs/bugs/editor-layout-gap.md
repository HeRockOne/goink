# Bug: 编辑器面板底部与状态栏之间的布局空隙（最终结论）

## 现象

- 24px 字号在无边框窗口下，编辑器面板底部与全局状态栏之间出现垂直空白间隙（约 28-240px）
- 字号越大间隙越明显；18px 以下通常无间隙
- 间隙大小随窗口高度变化，规律为 `rectTop = vh - 998`
- 该 bug 是**间歇性**的，非稳定复现，取决于 GPU 合成器/窗口历史状态
- 实测中同一构建产物有时正常，有时触发

## 诊断结论

### 根因（已确认）

**WebView2 合成器在无边框窗口 + 125% DPI 下的坐标偏移 bug。** 不是 CSS 布局问题，不是编辑器问题，不是 flex 布局问题。

证据：
- `offsetTop=66` 正确（布局坐标），但 `getBoundingClientRect().top=-174`（渲染坐标错位），差 240px
- height 与 top/bottom 自相矛盾（716 vs 368）
- 所有祖先链 CSS clean（无 transform/margin/backdrop-filter/position 偏移）
- 移除 backdrop-filter、translateZ、items-stretch、GPU 合成、升级 go-webview2 均无效
- 编辑器从 CM6 换到 textarea 后空隙不变
- 有边框窗口（`Frameless: false`）下一切正常 → 问题锁定在无边框窗口的 WebView2 bounds 处理

### 性质

该 bug 是 **非确定性（intermittent）** 的，与 GPU 合成器状态、窗口 resize 历史、系统 DPI 耦合。同一构建产物在不同窗口状态下可能触发或不触发。因此任何针对 CSS 的"修复"都可能只是偶然未触发，而非真正修复。

## 尝试过的修复（均无效或不可靠）

1. backdrop-filter 移除（怀疑合成层偏移）
2. `align-items: stretch` 显式
3. `transform: translateZ(0)` 强制 GPU 合成层
4. `--disable-gpu-compositing`（WebView2 环境变量）
5. `go-webview2` 升级 v1.0.22→v1.0.23
6. `Frameless: false`（有边框有效，但无边框无效）
7. 编辑器替换：CM6 → textarea
8. 布局引擎：flex → grid（grid 在有边框有效，在无边框下 1 秒后 collapse）

## 最终状态

- 保持无边框窗口 + flex 布局（当前状态）
- 已恢复 `:root { font-size: var(--font-size) }`（全局字号缩放）
- 编辑器沿用 CM6（恢复备份前的原始状态）
- ActivityBar 加 `overflow-y-auto` 防止字号放大时被裁切
- 该 bug 在 18px 以下通常不触发，日常使用 15-17px 安全

## 如果未来复现

- 升级 Wails v3（WebView2 集成重写，修复了 DPI/无边框的 bounds 问题）
- 或者接受有边框窗口 + grid 布局（确认无空隙）
- 或者升级 WebView2 Runtime 等未来版本修复