## [Unreleased]

### 新增

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
- **动态叙事面板**（NarrativeTimeline）：画布式可拖拽/缩放卡片面板，聚合写作上下文
  - 当前/过去/未来/弧线/伏笔/读者/详细设定 7 张卡片，自由布局，持久化存储
  - 四边四角拖拽缩放，自动吸附其他卡片边缘
  - 卡片标题可双击重命名，布局存 localStorage
  - 实时刷新：监听 `file:changed` / `chat:api_done` / `chat:session_created` 事件，300ms 防抖
  - 标题栏显示 "📖 动态叙事已展开" 指示
  - 面板宽度持久化 localStorage
- **GetWritingContext Wails 绑定**：一次 IPC 调用返回完整叙事上下文
  - `writing_context.go` 聚合 8 个数据源（章节/角色/弧线/伏笔/读者/快照/统计/大纲）
  - characters 返回 desc（角色描述）+ items（持有物品名）
  - active_arcs 返回 nodes（弧线节点详情：title/description/status/target_chapter/actual_chapter）
  - reader 返回 entries（近 2 章读者视角条目详情）
- **主题系统扩展**：59 变量全量覆盖（+8 个 `--narrative-*` 叙事变量）
  - `normalizeTheme()` 自动补全缺失变量
  - ThemeConfigTab 自动补全逻辑，示例主题「墨绿书斋」

### 修复

- **stream.go 恢复原始代码**：移除本地未提交的 `hasFinish` 检查修改
- **per_model 双倍累加**：`m["hit"] += hitTokens` 重复写入导致数值翻倍
- **UpsertModelUsage 传值错误**：传入累计值而非增量导致 DB 数据重复
- **面板 fallback 逻辑**：`selectedModel` 无数据时不回退全局合计

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
- **ContextRing detail 修正**：分角色 token 数改为直接显示 `runningTokens` 原始值
  - 移除误导性百分比（`runningTokens` 是累积值，与当前请求 `apiTotal` 混算比例无意义）
  - detail 现在显示上下文窗口中各 role 的真实 token 数