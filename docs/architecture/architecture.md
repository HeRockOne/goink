# Goink 架构文档

> 最后更新：2026-07-26
> 用途：新 AI 接手时第一份阅读材料，避免重复审计

## 1. 项目概览

Goink 是一个桌面 AI 写作系统，Wails（Go + React）构建。核心能力是用 MCP 工具驱动 AI 按阶段（prepare→outline→write→review→maintain）写百万字级长篇小说，通过数据库结构化存储维护跨章节一致性。

**技术栈**：Go 1.26 + Wails v2.13 + React + SQLite（CGO）+ ONNX（向量检索）

## 2. 目录结构

```
goink-master/
├── app/                    # Wails 绑定 + HTTP API 服务器
│   ├── handler.go          # Wails 绑定函数
│   ├── api_server.go       # HTTP REST API（23 个读端点 + chat）
│   ├── dialog.go           # 系统对话框
│   └── tray.go             # 系统托盘
├── internal/
│   ├── agent/              # Agent loop：LLM 调用 → 工具执行 → 阶段门禁 → 压缩
│   ├── agentcfg/           # 系统提示词、工具白名单、Skill 目录
│   ├── approval/           # 文件编辑/删除审批服务
│   ├── chapter/            # 章节元数据 Store
│   ├── character/          # 角色 + 角色关系 Store（append-only 关系图）
│   ├── config/             # 全局配置（AppSettings 单行 SQLite）
│   ├── export/             # 导出（TXT/Markdown/EPUB）
│   ├── git/                # 每本小说的 Git 仓库管理
│   ├── import/             # 导入（TXT/EPUB + LLM 辅助）
│   ├── item/               # 物品/法宝 Store
│   ├── itemoccurrence/     # 物品章节出现记录 Store
│   ├── llm/                # LLM 客户端（多供应商、流式、Token 计数、Web 搜索）
│   ├── location/           # 地点 + 空间关系 Store
│   ├── lore/               # 世界观设定 Store
│   ├── mcp_tools/          # 所有 MCP 工具定义（57 个）
│   ├── migrate/            # 数据库自动迁移（25 张表）
│   ├── novel/              # 小说索引 + 创作偏好 Store
│   ├── rag/                # RAG 向量检索（ONNX Embedder + sqlite-vec）
│   ├── reader/             # 读者认知 Store（已知/悬念/误知）
│   ├── scene/              # 章节内场景 Store
│   ├── search/             # 统一搜索服务（实体+正文+RAG 三路合并）
│   ├── session/            # 对话会话 + 消息 Store
│   ├── skill/              # 技能系统（内置/用户/小说三层）
│   ├── stats/              # 统计聚合
│   ├── storage/            # SQLite 连接池 + 分页 + PATCH 工具
│   ├── storyarc/           # 叙事弧线 + 节点 Store
│   ├── timeline/           # 伏笔 + 章节计划 Store
│   ├── web/                # 网页抓取
│   ├── writing/            # 写作日志 + 写作进度快照 Store
│   └── ws/                 # WebSocket Hub（桌面-移动端同步）
├── frontend/               # 桌面端 React 前端
├── mobile/                 # 移动端 Web 前端
├── skills/                 # 内置 Skill
├── docs/                   # 文档
└── build/                  # 构建输出
```

## 3. 数据库表（25 张）

### 核心数据表

| 表名 | 模型 | 用途 |
|------|------|------|
| `novels` | Novel | 小说索引 |
| `chapters` | Chapter | 章节元数据（num/title/summary/key_events/characters_in/arc_ids/word_count） |
| `chapter_arcs` | ChapterArc | 章节-弧线关联 |
| `characters` | Character | 角色（name/description/personality/abilities/location_id） |
| `character_relations` | CharacterRelation | 角色关系（append-only，is_current 标记当前状态） |
| `locations` | Location | 地点（name/type/description/parent_location_id） |
| `location_relations` | LocationRelation | 地点连通关系（无向边） |
| `items` | Item | 物品/法宝（name/arc_id/owner_id/narrative_role/status） |
| `item_occurrences` | ItemOccurrence | 物品章节出现记录（item_id/chapter_id/action） |
| `scenes` | Scene | 章节内场景（title/summary/location_id/character_ids/arc_node_id） |
| `lore_entries` | LoreEntry | 世界观设定（title/category/content/arc_id/reveal_chapter_id） |
| `story_arcs` | StoryArc | 叙事弧线（name/arc_type/status） |
| `arc_nodes` | ArcNode | 弧线节点（title/target_chapter/actual_chapter/status） |
| `time_entries` | TimelineEntry | 伏笔/用户指令（category/title/target_chapter/importance/status） |
| `reader_perspectives` | ReaderPerspective | 读者认知（type=known/suspense/misconception/content/planted_chapter） |
| `preference_items` | PreferenceItem | 创作偏好（category/content/is_global） |
| `writing_snapshots` | WritingSnapshot | 写作进度快照（last_chapter_num/current_arc_id/current_location/active_chars/summary） |
| `writing_log` | WritingLog | 每章字数变化日志 |
| `style_samples` | Sample | 风格素材库 |

### 系统表

| 表名 | 模型 | 用途 |
|------|------|------|
| `app_config` | AppSettings | 全局配置（28 字段） |
| `sessions` | Session | 对话会话 |
| `messages` | Message | 对话消息（append-only，版本化） |
| `turn_commits` | TurnCommit | Git commit 映射（用于回退） |
| `operation_log` | OperationLogRecord | 数据操作日志 |
| `model_usage` | ModelUsage | 按模型的 token 消耗累计（session_id + model_id 唯一） |

### 关键外键关系

```
chapters.novel_id → novels.id
characters.novel_id → novels.id
characters.location_id → locations.id
character_relations.novel_id → novels.id
items.novel_id → novels.id
items.owner_id → characters.id
items.arc_id → story_arcs.id
item_occurrences.item_id → items.id
item_occurrences.chapter_id → chapters.id
scenes.chapter_id → chapters.id
scenes.location_id → locations.id
lore_entries.novel_id → novels.id
lore_entries.arc_id → story_arcs.id
story_arcs.novel_id → novels.id
arc_nodes.story_arc_id → story_arcs.id
time_entries.novel_id → novels.id
reader_perspectives.novel_id → novels.id
writing_snapshots.novel_id → novels.id (primaryKey)
```

## 4. MCP 工具清单（57 个）

### 按模块分组

#### 小说管理（4 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_chapter_list` | GET | chapter_id(R) |
| `get_stats` | GET | - |
| `get_writing_context` | GET | current_chapter(R) |
| `update_chapter_meta` | UPDATE | chapter_id(R), summary(R), key_events(R), characters_in(R), arc_ids(R) |

#### 角色（5 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_characters` | GET | - |
| `get_character_relations` | GET | - |
| `create_character` | CREATE | name(R), description(R) |
| `update_character` | UPDATE | character_id(R) |
| `update_character_relationship` | UPDATE | relation_describe(R) |

#### 地点（5 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_locations` | GET | - |
| `create_location` | CREATE | name(R) |
| `update_location` | UPDATE | location_id(R) |
| `create_location_relation` | CREATE | location_a_id(R), location_b_id(R), relation_type(R) |
| `update_location_relation` | UPDATE | relation_id(R) |

#### 物品（7 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_items` | GET | - |
| `search_items` | GET | - |
| `create_item` | CREATE | name(R), arc_id(R), owner_id(R), narrative_role(R) |
| `update_item` | UPDATE | item_id(R) |
| `delete_item` | DELETE | item_id(R) |
| `get_item_occurrences` | GET | item_id(R) |
| `create_item_occurrence` | CREATE | item_id(R), chapter_id(R), action(R) |

#### 场景（4 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_scenes` | GET | chapter_id(R) |
| `create_scene` | CREATE | chapter_id(R), scene_number(R), title(R), summary(R), location_id(R), character_ids(R) |
| `update_scene` | UPDATE | scene_id(R) |
| `delete_scene` | DELETE | scene_id(R) |

#### 世界观（5 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_lore` | GET | - |
| `search_lore` | GET | - |
| `create_lore` | CREATE | title(R), category(R), content(R), arc_id(R), reveal_chapter_id(R) |
| `update_lore` | UPDATE | lore_id(R) |
| `delete_lore` | DELETE | lore_id(R) |

#### 弧线（5 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_story_arcs` | GET | - |
| `create_story_arc` | CREATE | name(R), arc_type(R) |
| `update_story_arc` | UPDATE | arc_id(R) |
| `create_arc_node` | CREATE | story_arc_id(R), title(R), target_chapter(R) |
| `update_arc_node` | UPDATE | node_id(R) |

#### 时间线（4 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_timeline` | GET | - |
| `create_timeline_entry` | CREATE | title(R), category(R), target_chapter(R), importance(R) |
| `update_timeline_entry` | UPDATE | entry_id(R) |
| `update_chapter_plan` | UPDATE | scope(R), content(R) |

#### 读者认知（3 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_reader_perspective` | GET | - |
| `create_reader_perspective_entry` | CREATE | type(R), content(R), planted_chapter(R) |
| `update_reader_perspective_entry` | UPDATE | entry_id(R) |

#### 偏好（3 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_preferences` | GET | - |
| `create_preference` | CREATE | category(R), content(R) |
| `update_preference` | UPDATE | preference_id(R) |

#### 写作快照（2 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `get_writing_snapshot` | GET | - |
| `update_writing_snapshot` | UPDATE | summary(R) |

#### 阶段门禁（3 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `set_phase` | ACTION | phase(R) |
| `get_phase_gate_config` | GET | - |
| `update_phase_gate_config` | UPDATE | config(R) |

#### 文件操作（2 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `read` | GET | path(R) |
| `edit` | WRITE | path(R), change_type(R) |

#### 搜索/辅助（6 个）
| 工具 | 类型 | required 字段 |
|------|------|--------------|
| `search_story_memory` | GET | query(R) |
| `web_search` | GET | query(R) |
| `web_fetch` | GET | url(R) |
| `run_subagent` | ACTION | agent_type(R) |
| `delete_record` | DELETE | table(R), record_id(R) |

## 5. get_writing_context 返回结构

```
chapter:           {num, title, word_count}
recent_chapters[]: {num, title, summary, key_events, word_cnt}
scenes[]:          {title, summary, word_count, location:{name,type}, arc_node:{title,arc_name}}
characters[]:      {name, location:{name}, items:[{name,role}], item_count}
active_arcs[]:     {name, type_zh, nodes_done, nodes_total, related_lore[], related_items[]}
timeline.pending[]: {title, category, target_chapter, importance}
timeline.resolved[]: {title, resolved_chapter}
timeline.overdue[]:  {title, target_chapter, importance, overdue_by}
reader:            {known, suspense, misconception}
writing_snapshot:  {last_chapter_num, current_arc_id, current_location, active_chars}
stats:             {total_chapters, min_words, max_words}
```

## 6. 阶段门禁

### 数据管线

```
prepare(get_writing_context) → outline(edit outlines/)
→ write(edit chapters/) → review(run_subagent)
→ maintain(update_*/create_* + update_chapter_meta + update_writing_snapshot + search_lore + search_items + set_phase)
→ 回到 prepare → 读到 maintain 回写的最新数据
```

### 门禁配置

每阶段有 `tools`（允许列表）和 `require`（必须调用）：
- prepare require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
- outline require: edit
- write require: edit, get_chapter_list
- review require: run_subagent
- maintain require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective

完整配置见 `docs/mcp-tools-audit.md`。

## 7. HTTP API（23 个读端点）

| 端点 | 对应工具 |
|------|---------|
| `GET /api/novels` | - |
| `GET /api/novels/{id}/chapters` | get_chapter_list |
| `GET /api/chapters/{id}` | read |
| `GET /api/characters` | get_characters |
| `GET /api/character-relations` | get_character_relations |
| `GET /api/locations` | get_locations |
| `GET /api/location-relations` | get_locations(network) |
| `GET /api/lore` | get_lore / search_lore |
| `GET /api/items` | get_items / search_items |
| `GET /api/item-occurrences` | get_item_occurrences |
| `GET /api/scenes` | get_scenes |
| `GET /api/timeline` | get_timeline |
| `GET /api/arcs` | get_story_arcs |
| `GET /api/arc-nodes` | get_story_arcs(nodes) |
| `GET /api/reader` | get_reader_perspective |
| `GET /api/preferences` | get_preferences |
| `GET /api/stats` | get_stats |
| `GET /api/writing-snapshot` | get_writing_snapshot |
| `GET /api/phase-gate-config` | get_phase_gate_config |
| `GET /api/search-memory` | search_story_memory |
| `GET /api/writing-context` | get_writing_context |
| `GET /api/read` | read |
| `POST /api/chat` | - |

认证：Bearer Token（`app_config.apitoken`）

## 8. Skill 体系

### 三层 Skill

| 层 | 路径 | 权限 |
|----|------|------|
| 内置 | `/builtin/skills/*.md` | 只读 |
| 用户级 | `~/.goink/skills/*.md` | 读写 |
| 小说级 | `{novel_dir}/skills/*.md` | 读写 |

### 当前 Skill 列表

**2 个 always（用户级 `~/.goink/skills/`，可调整）：**
| Skill | mode | 阶段 |
|-------|------|------|
| writing-kernel | always | 核心调度（每对话自动注入） |
| ai-communication-standard | always | 通信规范（每对话自动注入） |

**41 个内置（`internal/skill/builtin/`，打包进 exe）：**

| 阶段 | Skill |
|------|-------|
| init（开书） | init-phase, genre-templates, book-outline, character-design, world-building-system |
| prepare（准备） | common-sense-logic, genre-templates, book-outline, brainstorm-composer（按需） |
| outline（大纲） | book-outline, chapter-opening, chapter-hook-enhanced, maliang-method, dialogue-subtext, emotional-arc, opening-chapter |
| write（正文） | show-dont-tell, info-density, pov-purity, anti-ai-writing, shuangdian-pacing, climax-scene, foreshadow-cycle, pacing-control, scene-beats, emotion-injection, word-count-calibration |
| write后（自审） | revision-pass, anti-ai-grade |
| review（审稿） | review-standards（16 项判定） |
| maintain（维护） | anti-repetition, foreshadow-cycle |
| 完结 | book-completion |
| manual（`/` 触发） | collect, memory, next, review |

> 完整阶段技能表见 `skills/writing-kernel.md`。新增 skill 放用户级 `~/.goink/skills/`，并在 writing-kernel 登记。

## 9. LLM 集成

### Provider 架构

```
llm.Provider{Name, ChatURL, APIKey, Models[], BuildRequest, BuildHeaders, ParseError}
```

支持 OpenAI/Anthropic/DeepSeek/Gemini 等兼容 API。通过 `agent.Agent` loop 驱动：
1. 构建请求 → 流式调用
2. 解析 tool_calls → 执行工具 → 注入结果
3. 阶段门禁检查 → 是否允许切换阶段
4. 上下文压缩（token 超阈值时自动触发）

### 关键参数

- `CompressionThreshold`: 上下文压缩触发阈值（default 0.7）
- `MaxTurns`: 单次对话最大工具调用轮数
- `PhaseGateEnabled`: 阶段门禁开关

## 10. 构建

```bash
make deps      # 下载运行时依赖
make build     # 生产构建
make dev       # 开发模式
```

Windows 一键构建：`.\build.ps1` 或 `build.bat`

### CGO 依赖

- `go-sqlite3`: SQLite3 驱动
- `onnxruntime_go`: ONNX Runtime（向量嵌入）
- `sqlite-vec-go-bindings`: SQLite 向量扩展
- `wails/v2`: 桌面框架

ONNX 运行时搜索链：`<exe_dir>/runtime/` → `~/Goink/runtime/` → 系统 PATH

## 11. 前端架构

### 桌面端（React + Vite + TypeScript）

```
frontend/src/
├── App.tsx                     # 根组件，路由入口
├── views/
│   ├── InitView.tsx            # 首次启动引导
│   └── WorkspaceView.tsx       # 主工作区
├── components/
│   ├── chat/                   # 对话核心（24 个组件）
│   │   ├── ChatPanel.tsx       # 主面板（消息流 + 工具调用渲染）
│   │   ├── MessageBubble.tsx   # 消息气泡（复制按钮在边框外）
│   │   ├── PhaseGateBar.tsx    # 阶段门禁进度条 + 错误提示
│   │   ├── SubagentCard.tsx    # 子代理审稿报告卡片
│   │   ├── ThinkingBlock.tsx   # AI thinking 展开/折叠
│   │   ├── ToolCallCard.tsx    # 工具调用展示
│   │   ├── WebSearchCard.tsx   # 搜索结果卡片
│   │   ├── WebFetchCard.tsx    # 网页抓取结果卡片
│   │   ├── SlashMenu.tsx       # / 快捷指令菜单
│   │   ├── CompressionBlock.tsx # 上下文压缩提示
│   │   └── RecentSessions.tsx  # 历史会话列表
│   ├── settings/               # 设置面板（11 个组件）
│   │   ├── SettingsDialog.tsx  # 设置主弹窗
│   │   ├── ModelConfigTab.tsx  # 模型配置
│   │   ├── ThemeConfigTab.tsx  # 自定义主题（JSON 编辑 + 单击应用）
│   │   ├── PhaseGateConfigTab.tsx # 阶段门禁配置编辑器
│   │   └── GeneralConfigTab.tsx   # 通用设置
│   ├── character/              # 角色 CRUD 面板
│   ├── location/               # 地点 CRUD + 关系网络
│   ├── item/                   # 物品 CRUD + 出现记录
│   ├── lore/                   # 世界观设定 CRUD
│   ├── storyarc/               # 弧线 + 节点管理
│   ├── timeline/               # 时间线 + 章节计划
│   ├── reader/                 # 读者认知管理
│   ├── preference/             # 偏好管理
│   ├── stats/                  # 统计面板
│   ├── help/                   # 帮助中心（51 工具中英文描述）
│   ├── shell/                  # Shell 布局（侧边栏 + 主区）
│   └── ui/                     # 通用 UI 组件
├── hooks/                      # 自定义 Hook
├── lib/wailsjs/                # Wails JS 绑定（自动生成，勿手动修改）
├── i18n/locales/               # 国际化（zh-CN.json / en.json）
└── assets/                     # 静态资源
```

**技术栈**：React 18 + TypeScript + Vite + Tailwind CSS + Lucide Icons
**构建**：`npm run build` → 输出到 `frontend/dist/`（Wails 自动打包进 exe）

### 移动端（原生 HTML/CSS/JS，无框架）

```
mobile/
├── index.html          # 主页面（SPA，模板 id="tpl-chat-msg" 等）
├── app.js              # 核心逻辑（对话 + 工具调用渲染 + IndexedDB 缓存）
├── style.css           # 样式（CSS 变量主题 + 太虚剑宗/木艺书阁主题）
├── sw.js               # Service Worker（离线缓存静态资源）
├── idb-keyval.min.js   # IndexedDB 封装（离线数据持久化）
├── marked.min.js       # Markdown 渲染
├── jsQR.js             # QR 码扫描（扫码连接桌面端）
├── wspulse.mjs         # WebSocket 同步模块
├── manifest.json       # PWA 配置
└── API.md              # HTTP API 文档（27 节，23 个端点）
```

**部署**：桌面端 `webdav` 包提供只读 HTTP 服务，移动端通过 `https://桌面IP:端口/mobile/` 访问
**离线**：Service Worker 缓存静态资源，idb-keyval 持久化对话数据
**同步**：WebSocket Hub 实现桌面-移动端实时双向同步
**认证**：HTTPS 自签名证书 + Bearer Token（`app_config.apitoken`）

### 主题系统

CSS 变量驱动。自定义主题 JSON 格式：
```json
{
  "name": "主题名",
  "type": "dark/light",
  "colors": { "--background": "#...", "--foreground": "#...", ... }
}
```

去重键：`name__type`（同名不同深浅可共存）。单击即应用，无需确认按钮。

## 12. Token 计费架构

### 12.1 数据流

```
LLM API 响应（SSE 流）
  → stream.go parseSSE 提取 usage
  → agent.go EventUsage
  → tokens.go updateUsage
      ├─ 按模型累加 accHit/accMiss/accCompletion（session 级）
      ├─ 按模型累加 per_model（模型级明细）
      ├─ 写入 message.ExtraMetadata.usage（审计）
      ├─ 写入 model_usage 表（持久化查询）
      ├─ 更新 session.Usage JSON（前端面板）
      └─ wails 推送前端
```

### 12.2 缓存字段优先级

1. OpenAI 标准：`prompt_tokens_details.cached_tokens`（`miss = prompt_tokens - cached`）
2. DeepSeek：`prompt_cache_hit_tokens` + `prompt_cache_miss_tokens`

### 12.3 按模型累计

`session.Usage.per_model` 结构：
```json
{
  "step-3.7-flash": { "hit": 130496, "miss": 7225, "comp": 1765 },
  "deepseek-v4-pro":  { "hit": 5000,  "miss": 200,  "comp": 100  }
}
```

`model_usage` 表：`session_id + model_id` 唯一，支持跨 session 聚合（个人中心趋势图）。

### 12.4 价格来源

面板从 `app_config` 表读取用户配置的 `price_input` / `price_output` / `cache_price`，不在代码中硬编码。

### 12.5 导出格式

| 格式 | 实现 | 依赖 |
|------|------|------|
| TXT | `export/txt.go` | 无 |
| Markdown | `export/markdown.go` | 无 |
| EPUB | `export/epub.go` | 无 |
| **DOCX** | `export/docx.go` | 纯标准库（`archive/zip` + XML） |

## 13. 已知技术债

| 项目 | 描述 |
|------|------|
| `style_samples` 表 | 完整 DB 表但无 MCP 工具读写 |
| `preference_items.status` 字段 | 默认 'active'，无工具可设为 'superseded' |
| `novels.genre/description` | 创建后无更新工具 |
| `timeline.entry_type` | 默认 'foreshadowing'，不可创建 'chronicle' 条目 |
| `writing_snapshots.detailed_state` | 有字段但无工具读取 |
| `items.source` / `lore.source` | 默认 'ai'，无工具读取 |
