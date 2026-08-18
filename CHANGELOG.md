# Changelog

本 fork 基于 [sigpanic/goink](https://github.com/sigpanic/goink) v1.1，完整差异见 [README.md](README.md#与上游-v11-的差异)。

---

## [Unreleased]

### 新增

- **移动端 i18n 全量补全**：80+ 中英双语 key，覆盖 token 弹窗、删除/新建小说、扫描二维码、设置页等
- **移动端 token 弹窗重构**：累积值显示（cache_hit/cache_miss/acc_completion），标题改为"当前会话分类统计"/"今日统计"，可滚动+分割线
- **移动端成本显示移到书名栏**：书名左+成本右同行，点击弹出详情
- **移动端思考模式动态化**：读取模型 reasoning_levels，不再硬编码
- **后端 reasoning_levels**：mobile 模型列表 API 返回 reasoning_levels 字段
- **门禁结果门控**：check_story_consistency 返回 [ERROR] 时禁止 set_phase 推进
- **check_story_consistency 新增 pacing_gap**：连续多章无高密度场景检测，支持题材（xuanhuan/suspense/romance/urban）
- **check_story_consistency 新增 promise_fulfillment**：卷纲承诺大爽点到期未兑现检测
- **check_story_consistency 新增 init_consistency**：开书一致性校验（file_db_sync/type_pacing/pref_conflict）
- **总纲数据库化**：outlines + outline_beats 表，替代 book-outline.md
- **总纲 MCP 工具**：get_outline/update_outline/create_outline_beat/update_outline_beat/delete_outline_beat
- **key_events 标签格式规范**：[冲突][对话][伏笔][转折][日常][探索][情感][战斗][成长]

### 修复

- **移动端 token 数据口径**：统一使用累积值（prompt_cache_hit_tokens/prompt_cache_miss_tokens/acc_completion_tokens），与桌面端一致
- **build.ps1 修复**：SelectString 过滤问题 + 构建产物验证
- **返回格式标准化**：emoji → [ERROR]/[WARNING]

### 新增

- **卷纲体系**（长篇宏观一致性）：`story_arc` 加 `arc_type=volume` 枚举、`detail_json` 卷纲数据、`start_chapter`/`end_chapter` 卷边界
  - `create_story_arc` 强制校验：`arc_type=volume` 必须填起止章，否则报错
  - 前端弧线面板：卷类型筛选 + 起止章表单输入 + 前端校验
  - 系统提示词 + skill 引导：长篇必须建卷，卷必填起止章
- **get_writing_context 卷级聚合**：返回 `volume_entities`（本卷章节范围内的角色/物品/设定/伏笔 ID 列表）
  - 查询时从现有表派生（scene、item_occurrence、lore、timeline），零缓存表，零同步负担
  - 只返回 ID+名字，省 token；按卷边界过滤，不依赖 AI 填 entities 字段
- **get_writing_context 规划场景**：返回 `chapter_id IS NULL` 的规划场景（关联当前卷弧线）
- **get_writing_context recent_chapters 结构化**：暴露 `characters_in`（角色 ID）和 `arc_ids`（弧线 ID），数据已存在之前未返回
- **场景规划**：`scene.chapter_id` 改为 `*int64` nullable，AI 可在写前创建规划场景，写完后 `update_scene` 回填 chapter_id
- **反查工具 `get_entity_appearances`**：回溯"实体 X 出现在哪些章节"
  - 角色：通过 scene.character_ids 反查
  - 物品：通过 item_occurrence 流转史
  - 设定：通过 reveal_chapter_id
  - 伏笔：通过 timeline 埋/收/目标章
- **程序化一致性检查工具 `check_story_consistency`**：review agent 自动调用，SQL 实证返回三类问题
  - `foreshadow_overdue`：超期未回收伏笔（硬错误）
  - `character_vanished`：近 30 章未出场的角色（断档警告）
  - `item_conflict`：已销毁/丢失物品在后续章节出现（硬错误）
- **系统提示词强制消费**：prepare 阶段描述要求必须检查 `volume_entities`；review agent 流程集成 `check_story_consistency` + `get_entity_appearances`
- **get_writing_context 性能优化**：`characters` 按卷过滤（只返回当前卷角色，省 80% token），弧线节点 `LIMIT 50`，伏笔查询 `Size 20→100`
- **角色状态追踪**：`character` 表加 `status` 字段（`alive`/`dead`/`missing`/`unknown`），create/update 工具支持
- **跨卷摘要引导**：每卷结束时用 `update_story_arc` 写入 `detail_json.volume_summary`，系统提示词 + skill + 工具描述三层引导
- **GetMessagesForAPI LIMIT 1000**：安全网，防止意外全量加载
- **POST /api/novels 创建端点**：接受 `{title, genre, description}`，返回 `{novel}`；移动端书架页加"新建"按钮 + 对话框（类型下拉选择，与桌面一致）
- **移动端 Apple 浅色主题**：纯白背景 `#ffffff`，Apple 蓝 `#0071e3` 强调色，修复 `--accent` 未定义导致按钮不可见问题
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

### 新增

- **字体列表中文名**：GetSystemFonts 手动解析字体 name 表，优先 Windows zh-CN 家族名，支持 .ttc 集合——黑体/仿宋/楷体/宋体/微软雅黑/等线等正确显示中文名（此前 TTC 解析失败回退文件名、中文名缺失）
- **全局字体持久化**：DB 优先 + localStorage 兜底双写（SaveSettingsInput.DisplayFont 指针化，支持清空回默认）
- **新 skill main-tech-chapter-title-design**：章节标题设计方法论（7 类型 + 平台适配 + 番茄头条推荐标题 + AI 生成红线），已登记 kernel outline 阶段
- **skill 命名铁律**：identity.go 规定新 skill 文件名必须等于 frontmatter name，禁止加前缀/复制改名
- **全 miss 告警日志**：hit=0 且 miss>1 万时 WARN（区分代码问题与厂商缓存失效）
- **cacheprobe 完整创作流程模拟**：init 开书 + 5 章 × ~86 工具调用 + 子 agent 内部请求序列 + legacy 真实 NS 协议
- **思考模式合并**：开关 + 深度选择合并为单下拉（关 / low / high / max）
- **prompt_cache_key 路由粘性**：所有请求携带 prompt_cache_key=sessionID（对齐 opencode PR #22569），相同前缀路由同一后端节点，消除偶发全 miss

### 修复

- **审稿 Agent 安全边界**：从 reviewAgentTools 移除 `edit`，审稿 Agent 不再能直接修改文件
- **叙事面板空数据崩溃**：`NarrativeTimeline` 在 `ctx` 为 null 时因 `ctx?.characters.filter()` 调用在 undefined 上抛出 TypeError，修复为 `(ctx?.characters ?? []).filter()`
- **叙事面板非空断言隐患**：`ctx!.recent_chapters` 替换为 `ctx?.recent_chapters`，消除依赖 `&&` 短路的安全假象
- **叙事面板无关伏笔显示**：`pendingByChapter` 分组时跳过 `target_chapter <= 0` 的条目
- **recent_chapters 锚点错位**：`GetWritingContext` 和 `get_writing_context` 工具的 `recent_chapters` 改为以当前章节为锚点，新增 `chapter.Store.GetRecentBefore()`

### 修复

- **缓存协议五轮迭代**（完整前缀匹配 + 路由粘性）：
  - NS 落库进消息历史、永不清理（K=3 清理破坏前缀；请求尾临时拼导致完整匹配失效实测 89%）
  - 子 agent fork 完整主历史（重复 read 的 4-10K/轮 miss 归零；首轮命中主会话缓存条目）
  - prompt_cache_key 路由粘性（小米 MiMo 直连偶发全 miss 15.5 万/次）
  - usage_ratio 改本地估算（单调不回跳）；流式 usage 仅请求结束统一处理；压缩请求缓存对齐（全量工具）
  - 命中率统计全量口径（主/子统一累计，UpdateMessageUsage 按 agent_type 分写）
- **ONNX 按需加载重载失败**：onnxruntime 环境进程级单例，二次 InitializeEnvironment 报 already initialized 导致卸载后向量刷新全挂——环境改 sync.Once，卸载只销毁 session 释放模型内存
- **门禁字数校验跨章泄漏**：word_count_ok 布尔未绑定章节，上一章达标放行本章——进入 write 阶段重置，回归测试锁定
- **main-core-main-core-writing-kernel 双前缀**：恢复正确文件名/frontmatter，删除错误副本
- **append 污染**：请求尾拼 NS 时直接 append 原地写 opts.Messages 底层数组（Go slice 别名坑）

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

### 优化

- **Tab 激活标题发光**：三层 text-shadow 加强
- **ContextRing**：轨道加边框（深浅模式可见）、压缩阈值滑块进度填充、usage 色按主题区分（去荧光绿）
- **个人中心 Token 趋势**：单位改 M Token
- **主题生成器**：语义色固定色相（成功/警告/危险/六 tag 名实相符），边框透明度提升
- **工具调用详情**：执行中不再显示“处理中”徽章；set_phase（门禁阶段推进）不再显示
- **写审分离保持**：子 agent 身份/NS/指令在尾部，不污染主会话固定前缀

### 缓存模拟（2026-08-11 重构）

- **模式驱动重构**：`StartCacheSimulation(mode, gateRounds, shortQARounds, batchChapters, batchRounds)`——single（每章完整门禁逐章累积）/ batch（每批 6 章批次循环）/ mixed（混合会话，批量可多轮循环、章号顺延不重叠）；`cacheprobe.RunWindowMode` 单一入口；`cacheprobe.Run` 三方对照恢复原样
- **上下文刻度**：single/batch 长窗口输出 128K/256K/512K/1024K 累计成本快照 + 区间每章成本 + 最省区间；修复批末审稿回改 chapters/001.md 导致刻度章号反序（取最大章）、未到达刻度 Threshold=0 显示"0K"
- **混合模式阶段轮次表**：按创作阶段打点（开书/短对话/单章轮/批量轮每阶段结束快照 + 区间增量 + 每章成本）替代上下文刻度——混合窗口大小由输入决定，刻度到不了大档位且反映不出工作负载结构
- **性能 9 倍**：缓存 key 改轻量 msgFingerprint（不 marshal）+ step 增量路径（前缀连续时只处理新增消息），结果零变化（60s+→6.9s）
- **UI**：模式选择行 + 参数行分离、输入框可清空重输、按钮与输入框同一水平线

### 技能注入（2026-08-11）

- **可见性判定**：去重判定从"是否注入过"记录改为全文比对（history+cur 中 role=system 且 content 相同）——压缩清理历史后自动重新注入全文，杜绝误判跳过
- **短提醒注入**：首次进入阶段注入全文（学习内容常驻历史）；再次进入同阶段注入短提醒（`BuildSkillsReminder`：技能名 + description 要点，~300 字符 vs 全文 ~8K，紧跟请求尾部注意力最强位置）——解决 Lost in the Middle（全文可见 ≠ 被注意），业界对照 Anthropic skills #591 / hermes-agent / autogen
- 模拟验证：miss 降 13.8%（506,703→436,595），命中率 97.4% 不变

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