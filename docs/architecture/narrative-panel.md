# 动态叙事面板 — 技术文档

> 版本：1.0
> 创建日期：2026-07-27
> 对应代码：`frontend/src/components/narrative/` + `app/writing_context.go`

---

## 一、设计理念

### 1.1 为什么需要这个面板

在 AI 辅助写作过程中，作者需要同时关注多个维度：

- **当前场景**：故事发生的地点、在场角色、关键物品
- **叙事连续性**：已完成的章节写了什么、关键事件是什么
- **未来规划**：接下来几章的大纲、情绪基调、章末钩子
- **结构追踪**：弧线节点推进到什么位置、哪些伏笔待回收
- **读者认知**：读者知道了什么、还有哪些悬念

传统方式需要作者在不同面板之间来回切换，**每次切换都是一次思维打断**。动态叙事面板将这些信息**聚合到一张画布上**，让作者在写作时一目了然。

### 1.2 核心原则

1. **写前预览**：当前卡展示"即将写入正文的设定"，不是"已经写完的总结"
2. **一次加载**：通过 `GetWritingContext` Wails 绑定一次 IPC 调用拿全部聚合数据
3. **画布自由布局**：卡片可任意拖拽、缩放、添加/删除、重命名，布局持久化到 localStorage
4. **实时刷新**：监听 `file:changed`、`chat:api_done`、`chat:session_created` 事件，300ms 防抖自动刷新

---

## 二、数据架构

### 2.1 数据流

```
Go 后端 Store/DB
    │
    ▼
GetWritingContext(novelID, chapterNum)  ← Wails 绑定
    │
    ▼
WritingContext JSON  ← 一次 IPC 调用
    │
    ▼
前端 NarrativeTimeline 组件
    │
    ▼
7 张画布卡片（当前/过去/未来/弧线/伏笔/读者/详细设定）
```

### 2.2 WritingContext 完整字段

**根字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `chapter` | `WritingChapter` | 当前章节信息 |
| `recent_chapters` | `[]WritingChapterBrief` | 最近 5 章摘要 |
| `characters` | `[]WritingCharacterBrief` | 全量角色列表 |
| `active_arcs` | `[]WritingArcBrief` | 活跃弧线（status=active） |
| `timeline` | `WritingTimeline` | 伏笔分类（pending/resolved/overdue） |
| `reader` | `WritingReader` | 读者视角计数+条目详情 |
| `writing_snapshot` | `*WritingSnapshotBrief` | 当前写作快照 |
| `scenes` | `[]WritingSceneBrief` | 当前章场景列表 |
| `volume` | `*WritingVolume` | 当前卷纲（arc_type=volume 且 status=active） |
| `volume_entities` | `*WritingVolumeEntities` | 卷范围内实体 ID 聚合（省 token） |

**WritingChapter：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `id` | int64 | `chapters` 表 | 章节记录 ID |
| `num` | int | `chapter_number` | 章节号 |
| `title` | string | `title` | 章节标题 |
| `word_count` | int | `word_count` | 该章字数 |

**WritingChapterBrief：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `num` | int | `chapter_number` | 章节号 |
| `title` | string | `title` | 章节标题 |
| `summary` | string | `summary` | 剧情概要（AI 生成的摘要） |
| `key_events` | string | `key_events` | 关键事件 JSON 数组 |
| `word_cnt` | int | `word_count` | 该章字数 |

**WritingCharacterBrief：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `id` | int64 | `characters` 表 | 角色 ID |
| `name` | string | `name` | 角色名 |
| `status` | string | `status` | alive/dead/missing/unknown（dead=已死亡，不得让其出场） |
| `desc` | string | `description` | 角色完整描述 |
| `location` | `*WritingLocationBrief` | `location_id` → `locations` 表 | 角色所在地（名称） |
| `item_count` | int64 | `items` 表 count | 持有物品数 |
| `items` | `[]string` | `items` 表 where owner_id | 持有物品名列表 |

**WritingArcBrief：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `id` | int64 | `story_arcs` 表 | 弧线 ID |
| `name` | string | `name` | 弧线名称 |
| `type_zh` | string | `arc_type` 映射 | 弧线类型中文 |
| `nodes_done` | int64 | `arc_nodes` count(completed) | 已完成节点数 |
| `nodes_total` | int64 | `arc_nodes` count | 总节点数 |
| `nodes` | `[]WritingArcNodeBrief` | `arc_nodes` where story_arc_id | 节点详情列表 |

**WritingArcNodeBrief：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `id` | int64 | `arc_nodes` 表 | 节点 ID |
| `title` | string | `title` | 节点标题 |
| `description` | string | `description` | 节点描述 |
| `status` | string | `status` | completed/pending |
| `target_chapter` | int | `target_chapter` | 目标章节 |
| `actual_chapter` | int | `actual_chapter` | 实际完成章节 |

**WritingTimelineEntry：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `id` | int64 | `timeline_entries` 表 | 条目 ID |
| `title` | string | `title` | 伏笔标题 |
| `status` | string | `status` | pending/resolved |
| `target_chapter` | int | `target_chapter` | 应回收的目标章节 |
| `importance` | int | `importance` | 重要度 1-5 |
| `resolved_chapter` | int64 | `resolved_chapter_id` | 实际回收章节 ID |
| `overdue_by` | int | 计算值 | 已超期章数（=current - target） |

**WritingReader：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `known` | int | `reader_perspectives` count(type=known) | 读者已知信息数 |
| `suspense` | int | count(type=suspense,未揭示) | 活跃悬念数 |
| `misconception` | int | count(type=misconception,未揭示) | 未纠正的误解数 |
| `entries` | `[]WritingReaderEntry` | `reader_perspectives` select(type/content/planted_chapter/revealed_chapter) where planted_chapter >= current-1 | 最近2章的读者视角条目 |

**WritingReaderEntry：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `id` | int64 | `reader_perspectives` 表 | 条目 ID |
| `type` | string | `type` | known/suspense/misconception |
| `content` | string | `content` | 条目内容 |
| `planted_chapter` | int | `planted_chapter` | 种下章节 |
| `revealed_chapter` | int | `revealed_chapter` | 揭示章节（0=未揭示） |

**WritingSnapshotBrief：**

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `last_chapter_num` | int | `writing_snapshots` 表 | 最后完成的章节号 |
| `current_arc_id` | *int64 | `current_arc_id` | 当前进行中的弧线 ID |
| `current_location` | string | `current_location` | 当前地点名称 |
| `active_chars` | string | `active_chars` | 当前出场角色 ID JSON 数组 |

---

## 三、卡片数据映射与筛选规则

### 3.1 当前卡片（cardId: current）

**作用**：写前预览——即将写入正文的设定。

| 展示项 | 字段 | 筛选规则 |
|--------|------|----------|
| 📍 地点 | `writing_snapshot.current_location` | 直接展示 |
| 📝 内容摘要 | `recent_chapters[num=current].summary` | 匹配 `chapter.num` |
| 👤 出场角色 | `characters` 过滤 | `active_chars` JSON 数组中的 ID 匹配；若无则全展示 |
| 📦 物品名 | `characters[].items[]` | 每个角色持有物品名列表 |
| 📍 角色所在地 | `characters[].location.name` | 每个角色的当前所在地 |
| ⏳ 近期待收 | `timeline.pending` 过滤 | `target_chapter == current+1 \|\| target_chapter == current+2` |
| 📝 字数 | `chapter.word_count` | 当前章节字数 |

### 3.2 过去卡片（cardId: past）

**作用**：历史回顾——已完成的章节写了什么。

| 展示项 | 字段 | 筛选规则 |
|--------|------|----------|
| 每章标题 | `recent_chapters[].title` | 倒序取最近 3 章（有 summary 的） |
| 📖 剧情概要 | `recent_chapters[].summary` | 直接展示 |
| 📌 关键事件 | `recent_chapters[].key_events` | JSON 数组 parse；过滤长度 >5 的条目；清理 `埋：` `推：` 等前缀；取前 3 条 |
| 字数 | `recent_chapters[].word_cnt` | >0 时显示 |

**筛选**：`recent_chapters.filter(c => c.summary && c.num <= currentCh.num).slice(0, 3)`

### 3.3 未来卡片（cardId: future）

**作用**：后续走向——接下来章节的大纲。

**数据来源**：`get_writing_context` 返回的 `outline_chapters[]` 字段（当前章 -1 ～ +2，共 4 章）。使用 `react-markdown` 直接渲染原始大纲 Markdown，不再依赖 `OutlineParser` 语义解析。

| 展示项 | 来源字段 | 说明 |
|--------|----------|------|
| 章节标题 | `outline_chapters[].title` | 从大纲 `## 章节标题` 提取 |
| 内容 | `outline_chapters[].content` | 原始 Markdown，react-markdown 渲染 |
| 章节号 | `outline_chapters[].num` | 用于排序和筛选 |

**排序**：按章节号降序（最新章在前）

### 3.4 弧线卡片（cardId: arcs）

**作用**：整体进度——弧线推进状态。

| 展示项 | 字段 | 说明 |
|--------|------|------|
| 弧线名 | `active_arcs[].name` | |
| 类型 | `active_arcs[].type_zh` | 主线/支线/角色弧 |
| 进度 | `nodes_done / nodes_total` | 已完成/总节点 |
| 进度条 | 百分比进度条 | `>=75%` 绿色，`>=40%` 黄色，`<40%` 灰色 |
| 节点详情 | `active_arcs[].nodes[]` | 每个节点的 title/description/status/target_chapter |

**筛选**：`story_arcs WHERE status = 'active'`

### 3.5 伏笔卡片（cardId: foreshadow）

**作用**：伏笔追踪——待回收/超期/已回收的伏笔。

| 展示项 | 字段 | 筛选规则 |
|--------|------|----------|
| ⚠️ 超期 | `timeline.overdue[]` | `status != resolved && target_chapter < current` |
| ⏳ 待回收 | `timeline.pending` 按 target_chapter 分组 | `status = pending` |
| ✅ 已回收 | `timeline.resolved` | `status = resolved`（取前 5 条） |
| 重要度 | `importance` | 5→★★★★★ 必收，4→★★★★ 重要，3→★★★ 一般 |
| 超期章数 | `overdue_by = current - target_chapter` | 计算值 |

### 3.6 读者视角卡片（cardId: reader）

**作用**：读者认知——读者知道了什么、还有什么悬念。

| 展示项 | 字段 | 筛选规则 |
|--------|------|----------|
| 👁 已知 | `reader.known` | count(type=known) |
| ❓ 悬念 | `reader.suspense` | count(type=suspense, revealed=0) |
| ❌ 误知 | `reader.misconception` | count(type=misconception, revealed=0) |
| 条目详情 | `reader.entries[]` | `planted_chapter >= current-1`，取前 6 条 |
| 条目类型 | `type` | known→👁 已知，suspense→❓ 悬念，misconception→❌ 误知 |

### 3.7 详细设定卡片（cardId: detailtabs）

**作用**：补充参考——角色/地点/物品/世界观/场景/弧线/伏笔/读者 Tab。

数据通过 `DetailTabs` 组件懒加载各自 Wails 绑定（`GetCharacters` / `GetLocations` / `GetItemList` / `GetLoreList` / `GetSceneList` / `GetStoryArcs` / `GetTimelineEntries` / `GetReaderPerspectives`）。

---

## 四、前端技术实现

### 4.1 画布引擎

- 卡片使用 `position: absolute` 定位在画布容器内
- 拖动：监听 header `onMouseDown`，更新 `CardPos.x/y`
- 缩放：4 边 + 4 角 resize 手柄（`edge-n/s/w/e` + `corner-nw/ne/sw/se`），更新 `CardPos.w/h`
- 吸附：8px 阈值吸附到其他卡片边缘或画布边缘（`snapTo()` 函数）
- 布局持久化：`localStorage.getItem('narrative_canvas_layout')`
- 卡片标题重命名：双击标题进入 input 编辑，`localStorage.getItem('narrative_card_labels')`

### 4.2 实时刷新

监听 Wails 事件，300ms 防抖：

```typescript
EventsOn('file:changed', (data) => refresh(data?.novel_id))
EventsOn('chat:api_done', () => refresh())
EventsOn('chat:session_created', () => refresh())
```

### 4.3 工具栏

- 鼠标移入画布右上角 60px 区域时显示 + 按钮
- 10 秒无操作自动隐藏
- 点击 + 弹出下拉列表，切换任意卡片显示/隐藏
- 下拉列表 z-index: 999，不被卡片遮挡

### 4.4 性能

- `React.memo` 包裹整个组件
- `useCallback` 包裹所有事件处理函数
- 事件监听在 `useEffect` cleanup 中解绑
- 章纲内容在 `loadOutlines` hook 中缓存（避免重复请求）

---

## 五、实时数据获取与刷新

### 5.1 初次加载

当 `novelId > 0` 时触发 `loadContext(activeChapterNum || 0)`。
若 `activeChapterNum` 为 0（新书或无章节选中），首次加载 snapshot 后使用 `last_chapter_num` 重新加载。

### 5.2 切换章节

`handleSelectChapter` 设置 `activeChapterNum` → `useEffect` 触发重新加载。

### 5.3 后端事件驱动刷新

| 事件 | 触发时机 | 刷新范围 |
|------|----------|----------|
| `file:changed` | AI 写入/编辑任意文件 | 全量刷新，300ms 防抖 |
| `chat:api_done` | AI 对话一次完整回复结束 | 全量刷新，300ms 防抖 |
| `chat:session_created` | 新建对话 session | 全量刷新，300ms 防抖 |

---

## 六、章纲文件格式

章纲文件位于 `outlines/NNN.md`（如 `outlines/001.md`），由 AI 大纲阶段生成。

### 6.1 支持的 section 标题

| 标题 | 说明 | 是否必选 |
|------|------|---------|
| `## 章节标题` | 章节标题 | 可选 |
| `## 基调与字数` | 情绪基调、字数 | 可选 |
| `## 开篇策略` | 开局方式 | 可选 |
| `## 场景设计` | 场景列表 | 可选 |
| `## 关键事件` | 核心事件 | 可选 |
| `## 重点角色` | 出场角色 | 可选 |
| `## 伏笔操作` | 伏笔埋/推/收 | 可选 |
| `## 情绪设计` | 情绪节奏 | 可选 |
| `## 章末钩子` | 结尾悬念 | 可选 |
| `## 金手指状态` | 金手指进展 | 可选 |

### 6.2 渲染方式

未来卡片（Section 3.3）使用 `react-markdown` 直接渲染原始 Markdown，不再依赖 `OutlineParser` 语义解析。`OutlineParser` 保留用于其他需要结构化字段的场景。

---

## 七、CSS 变量（主题系统）

面板颜色通过 CSS 变量控制，由自定义主题系统统一管理（`useTheme.ts`）。

| 变量 | 用途 | 默认值（light） |
|------|------|----------------|
| `--narrative-current-bg` | 当前章节卡片头背景 | `color-mix(in oklab, var(--primary) 8%, var(--sidebar))` |
| `--narrative-current-border` | 当前章节卡片边框 | `var(--primary)` |
| `--narrative-overdue-bg` | 超期伏笔背景 | `color-mix(in oklab, var(--destructive) 12%, var(--sidebar))` |
| `--narrative-overdue-text` | 超期伏笔文字 | `var(--destructive)` |
| `--narrative-resolved-text` | 已回收伏笔文字 | `#3a7a3a` |
| `--narrative-resolved-bg` | 已回收伏笔背景 | `color-mix(in oklab, #3a7a3a 10%, var(--sidebar))` |
| `--narrative-pending-text` | 待回收伏笔文字 | `var(--tag-blue-foreground)` |
| `--narrative-pending-bg` | 待回收伏笔背景 | `var(--tag-blue)` |
| `--narrative-arc-inactive` | 弧线进度条背景 | `color-mix(in oklab, var(--muted-foreground) 20%, transparent)` |
| `--narrative-future-card-bg` | 未来章纲卡片背景 | `var(--card)` |
| `--narrative-future-card-border` | 未来章纲卡片边框 | `var(--border)` |
| `--narrative-hook-type` | 章末钩子类型文字 | `var(--primary)` |
| `--narrative-tab-active` | Tab 激活下划线 | `var(--primary)` |
| `--narrative-divider` | 面板分割线 | `var(--border)` |

---

## 八、布局与 UI

### 8.1 面板结构

```
┌──────────────────────────────────────────────┐
│  📖 动态叙事                                   │  ← 标题栏
├──────────────────────────────────────────────┤
│  [+ 按钮]（右上角 hover 自动显示/隐藏）          │
│                                              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │  当前     │ │  过去    │ │  未来     │      │  ← 画布卡片
│  │  📍 地点  │ │  第5章... │ │  第6章... │      │     （可拖拽/缩放）
│  │  👤 角色  │ │  第4章... │ │  第7章... │      │
│  └──────────┘ └──────────┘ └──────────┘      │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │  弧线     │ │  伏笔    │ │  读者     │      │
│  │  进度条   │ │  ⚠️超期   │ │  👁已知    │      │
│  │  节点详情  │ │  ⏳待收   │ │  ❓悬念    │      │
│  └──────────┘ └──────────┘ └──────────┘      │
│  ┌────────────────────────────────────┐       │
│  │  详细设定（Tab）                      │       │
│  └────────────────────────────────────┘       │
└──────────────────────────────────────────────┘
```

### 8.2 面板加载方式

固定定位 overlay，`z-index: 50`，右边界 = 对话面板左边框。不改变原有三栏（ActivityBar + SidePanel + ContentPanel + ChatPanel）布局。

### 8.3 面板尺寸

最小宽度 240px，可通过右边框拖拽调整（带吸附，自动对齐对话面板左边界）。宽度存 `localStorage('narrative_panel_width')`。

---

## 九、局限与后续优化

1. **章纲解析覆盖率 ~85%**：部分变体（如 `**加粗标题**`、嵌套列表）可能解析不完整
2. **虚拟滚动**：200k 以上上下文 -> 数百条消息 -> 可引入 react-window 优化 ChatPanel 渲染
3. **卡片可扩展**：可增加自定义卡片类型，或引入第三方数据源
4. **离线缓存**：writing-context 数据可做 Service Worker 缓存，加速首屏加载
