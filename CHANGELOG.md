# Changelog

本 fork 基于 [sigpanic/goink](https://github.com/sigpanic/goink) v1.1，完整差异见 [README.md](README.md#与上游-v11-的差异)。

---

## [Unreleased]

### 新增

- **动态叙事面板**（NarrativeTimeline）：画布式可拖拽/缩放卡片面板，聚合写作上下文
  - 当前/过去/未来/弧线/伏笔/读者/详细设定 7 张卡片，自由布局，持久化存储
  - 四边四角拖拽缩放，自动吸附其他卡片边缘
  - 卡片标题可双击重命名，布局存 localStorage
  - 实时刷新：监听 `file:changed` / `chat:api_done` / `chat:session_created` 事件，300ms 防抖
  - 面板宽度持久化 localStorage
- **GetWritingContext Wails 绑定**：一次 IPC 调用返回完整叙事上下文
  - `writing_context.go` 聚合 8 个数据源（章节/角色/弧线/伏笔/读者/快照/统计/大纲）
  - characters 返回 desc（角色描述）+ items（持有物品名）
  - active_arcs 返回 nodes（弧线节点详情：title/description/status/target_chapter/actual_chapter）
  - reader 返回 entries（近 2 章读者视角条目详情）
- **计费面板重构**：兼容 OpenAI 标准格式 + DeepSeek 格式缓存字段
  - 优先读取 `prompt_tokens_details.cached_tokens`，fallback `prompt_cache_hit_tokens`
  - 新增 `acc_completion_tokens` session 级累计输出 token
  - 按模型独立累计（`per_model`），面板切换模型时显示对应消耗
  - 新增 `model_usage` 持久化表
  - 每消息存储 API 精确 usage 到 `ExtraMetadata.usage`
- **个人中心 Token 趋势图**：按日期 + 模型聚合的月度消耗总览
  - 日期选择器（开始/结束日期），模型下拉筛选
  - SVG 饼图展示缓存命中/未命中/输出占比
- **DOCX 导出**：纯标准库实现（`archive/zip` + XML），无外部依赖
- **输入框引导提示**：空会话时显示 4 张引导卡片
- **MCP 关键字段 jsonschema required**：核心创建/更新工具的关键业务字段加入 `jsonschema:"required"` 双层约束
- **update_timeline_entry 状态校验**：`status=resolved` 时强制要求 `resolved_chapter_id`
- **主题系统扩展**：59 变量全量覆盖（+8 个 `--narrative-*` 叙事变量）
  - `normalizeTheme()` 自动补全缺失变量
  - ThemeConfigTab 自动补全逻辑，示例主题「墨绿书斋」
- **MemoryAgent 检索效率**：向 memoryAgentTools 添加 `get_writing_context`，一次获取 8 个数据源全景
- **系统提示词引导**：mainAgentSystem1 prepare 阶段描述引导使用 `get_writing_context`；memoryAgentSystem1 工作流程增加"全景概览"步骤

### 修复

- **审稿 Agent 安全边界**：从 reviewAgentTools 移除 `edit`，审稿 Agent 不再能直接修改文件
- **叙事面板空数据崩溃**：`NarrativeTimeline` 在 `ctx` 为 null 时因 `ctx?.characters.filter()` 调用在 undefined 上抛出 TypeError，修复为 `(ctx?.characters ?? []).filter()`
- **叙事面板非空断言隐患**：`ctx!.recent_chapters` 替换为 `ctx?.recent_chapters`，消除依赖 `&&` 短路的安全假象
- **叙事面板无关伏笔显示**：`pendingByChapter` 分组时跳过 `target_chapter <= 0` 的条目
- **recent_chapters 锚点错位**：`GetWritingContext` 和 `get_writing_context` 工具的 `recent_chapters` 改为以当前章节为锚点，新增 `chapter.Store.GetRecentBefore()`

### 优化

- **首装引导**：HelpDialog 自动弹出快速入门标签页
- **分角色/价格配置**：面板默认展开，不再折叠
- **性能**：ChatPanel SSE 流式输出 100ms 批量防抖 + `flushSync` 同步 DOM
- **性能**：ThinkingBlock `React.memo` 减少重渲染
- **SidePanel**：可拖拽调整宽度
- **Prompt Caching 优化**：消息顺序重构，保持前缀稳定以利用 DeepSeek 缓存
  - `writeSystemMessages` 只写入稳定前缀（identity + always + catalog）
  - NovelState 动态注入到 user message 之后，不破坏缓存前缀
  - 工具按名称排序，全量工具发送，确保每轮前缀一致
  - 前缀哈希检测（`computePrefixHash`），缓存失效时日志警告
- **ContextRing detail 修正**：分角色 token 数改为直接显示 `runningTokens` 原始值，移除误导性百分比

---

## Fork 以来的完整变更

以下是从上游 v1.1 fork 后累积的全部变更，按功能模块组织。

### 全新创作模块（8 个）

| 模块 | 工具/文件 | 说明 |
|------|----------|------|
| 世界观设定（Lore） | `lore_tools.go`, `lore/` store + types, 前端 UI | 5 个 MCP 工具，arc_id 关联弧线 |
| 物品/法宝（Item） | `item_tools.go`, `item/` store + types, 前端 UI | 5 个 MCP 工具，arc_id 关联弧线 |
| 物品流转记录（ItemOccurrence） | `item_occurrence_tools.go`, `itemoccurrence/` | 2 个 MCP 工具，追踪物品在各章节的出现和状态变化 |
| 场景管理（Scene） | `scene_tools.go`, `scene/` store + types | 4 个 MCP 工具，arc_node_id 关联弧线节点 |
| 章节元数据（ChapterMeta） | `chapter_meta_tools.go` | update_chapter_meta 工具，summary / key_events / characters_in / arc_ids |
| 树状上下文（WritingContext） | `writing_context_tools.go`, `app/writing_context.go` | 一次调用获取全量关联数据 + 超期伏笔检测 |
| 写作进度快照（Snapshot） | `snapshot_tools.go`, `writing/` snapshot store | current_arc_id / active_chars / summary / detailed_state |
| 创作统计（Stats） | `stats_tools.go`, `stats/` store | 字数 / 弧线进度 / 伏笔回收率 / 角色数 / 地点数 |

### 阶段门禁系统

- **5 阶段校验**：prepare → outline → write → review → maintain
- 每阶段有 tools 白名单 + require 必调列表
- maintain 阶段 15 项逐项检查清单
- 门禁配置存数据库，不占 AI 上下文
- 支持 single（单章）和 batch（批量）两种模式
- `set_phase` 特殊工具硬编码在 agent 循环中，不走 registry
- edit 工具路径按阶段限制（outline 只能编辑 outlines/）

### HTTP API + 移动端

- **23 个 REST 端点**：`api_server.go`，覆盖所有读操作
- **SSE 对话流**：`POST /api/chat`，移动端通过 EventSource 接收
- **移动端 Web 前端**：`mobile/` 目录，纯原生 JS
  - 书架/详情/阅读器/对话/设置 完整功能
  - IndexedDB 离线缓存，Service Worker 离线可用
  - WebSocket 双端同步（`ws/` Hub）
- **HTTPS 自签名证书**：`cert/cert.go`，自动生成
- **API 认证**：Bearer Token，设置页获取

### 动态叙事面板

- 7 张卡片：当前/过去/未来/弧线/伏笔/读者/详细设定
- 画布式布局：拖拽、缩放、吸附、重命名、显隐
- 布局持久化：localStorage
- 实时刷新：监听文件变更和对话事件，300ms 防抖
- 数据来源：`GetWritingContext` 一次 IPC 调用聚合 8 个数据源

### 模型配置增强

- 自动获取模型列表（`DiscoverModels`）
- 思考模式支持（`ReasoningEffort`：high / max）
- 单轮最大轮次：100（上游 50）
- LLM 自动重试：429 限流和可重试错误指数退避，最长 60 秒等待
- 可配置压缩阈值（默认 0.7）
- 网络搜索：Exa API（上游 DeepSeek 搜索）

### 计费面板

- 兼容 OpenAI 标准格式 + DeepSeek 格式缓存字段
- 按模型独立累计（`per_model`）
- Token 趋势图（日期 + 模型聚合，SVG 饼图）
- 缓存命中率实测 89-93%

### WebDAV

- 内置 WebDAV 服务器，可配置端口/用户/密码
- 对话结束后自动导出 TXT 到 outputs 目录
- 手机文件管理器直接阅读

### 自定义主题

- 67 个 CSS 变量全量覆盖
- 亮色/暗色双模式
- JSON 粘贴即应用
- 示例主题「墨绿书斋」
- `normalizeTheme()` 自动补全缺失变量

### 安全增强

- **双层沙箱**：正则白名单 + SafePath 杜绝路径穿越
- **文件编辑**：写入前重读对比，防止覆盖手动修改
- **API 认证**：Bearer Token，自动生成
- **审稿 Agent 限权**：reviewAgentTools 移除 edit，审稿不能改数据
- **操作日志**：`operation_log` 表记录所有 DB 变更操作

### MCP 工具扩展（33 → 57）

新增工具分类：

| 分类 | 工具数 | 说明 |
|------|--------|------|
| Lore | 5 | 世界观 CRUD + 搜索 |
| Item | 5 | 物品 CRUD + 搜索 |
| ItemOccurrence | 2 | 物品流转记录 |
| Scene | 4 | 场景 CRUD |
| Stats | 1 | 创作统计 |
| Snapshot | 2 | 写作快照读写 |
| PhaseGate | 2 | 门禁配置读写 |
| WritingContext | 1 | 树状上下文 |
| ChapterMeta | 1 | 章节元数据更新 |
| WebSearch/Fetch | 2 | 网络搜索和网页抓取 |
| Subagent | 1 | 子 Agent 调度 |
| Delete | 1 | 通用删除 |

### Skill 体系扩展（12 → 41）

| 分类 | 数量 | 说明 |
|------|------|------|
| 核心系统（core） | 5 | init-phase, writing-kernel, communication-standard |
| 写作技术（tech） | 20+ | show-dont-tell, chapter-hook, dialogue, pacing, 等 |
| 类型专精（type） | 8 | 玄幻修仙, 都市武侠, 科幻, 悬疑, 等 |
| 子技能（sub） | 8 | review-standards, anti-ai-grade, 等 |

### 数据库表扩展（17 → 25）

新增表：

| 表 | 用途 |
|----|------|
| lore_entries | 世界观设定 |
| items | 物品/法宝 |
| item_occurrences | 物品流转记录 |
| scenes | 场景 |
| story_arcs / arc_nodes | 叙事弧线 + 节点 |
| chapter_arcs | 章节-弧线关联 |
| timeline_entries | 时间线事件/伏笔 |
| reader_perspectives | 读者认知 |
| writing_logs / writing_snapshots | 写作日志 + 快照 |
| model_usage | 模型级 Token 累计 |
| operation_logs | 操作审计日志 |
| app_config | 运行时配置 |
| phase_gate_configs | 阶段门禁配置 |
| refresh_queue | 向量索引刷新队列 |

### 前端优化

- 对话界面：复制按钮、一键到底、门禁提示挪位
- SSE 流式输出 100ms 批量防抖 + `flushSync` 同步 DOM
- ThinkingBlock `React.memo` 减少重渲染
- SidePanel 可拖拽调整宽度
- 首装引导：HelpDialog 自动弹出快速入门标签页
- 分角色/价格配置面板默认展开
- 主题系统：亮色/暗色 + 自定义主题
- 书架：封面展示、新建/导入/删除/编辑
- 错误处理：TitleBoundary 兜底，toast 替代整屏错误

### 文档

- 分层文档体系：`architecture/` / `design/` / `adr/` / `archive/`
- 架构文档、阶段门禁设计、竞品分析、叙事面板设计
- ADR-0001：Prompt Caching 消息前缀稳定化决策
- Token 注入分析、缓存命中机制、Provider 状态
- 审计报告：数据完整性、MCP 工具依赖链、Schema Required