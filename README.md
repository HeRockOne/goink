<h1 align="center"><img src="assets/logo.svg" width="80" alt="Goink Logo"><br>Goink<br><sub>桌面 AI 写作系统 — Agent 实时决策 × 结构化记忆 × 写完自检</sub></h1>

<p align="center"><strong><a href="README_EN.md">English</a> | 中文</strong></p>

---

> **基于 [sigpanic/goink](https://github.com/sigpanic/goink) v1.1 fork**

---

## 目录

- [与上游 v1.1 的差异](#与上游-v11-的差异)
  - [一、全新创作模块](#一全新创作模块)
  - [二、数据管线架构](#二数据管线架构)
  - [三、阶段门禁](#三阶段门禁)
  - [四、HTTP API](#四http-api23-个端点)
  - [五、模型配置增强](#五模型配置增强)
  - [六、移动端 Web 前端](#六移动端-web-前端)
  - [七、对话界面优化](#七对话界面优化)
  - [八、自定义主题](#八自定义主题)
  - [九、图标替换](#九图标替换)
  - [十、WebDAV](#十webdav)
  - [十一、其他功能](#十一其他功能)
  - [十二、字段扩展](#十二字段扩展)
  - [十三、数据库表扩展](#十三数据库表扩展)
  - [十四、MCP 工具扩展](#十四mcp-工具扩展)
  - [十五、文档](#十五文档)
  - [十六、Skill 体系](#十六skill-体系)
  - [十七、安全](#十七安全)
- [安装](#安装)
- [项目结构](#项目结构)
- [技术栈](#技术栈)
- [License](#license)

---

## 与上游 v1.1 的差异

### 一、全新创作模块（8 个）

| 模块 | 说明 |
|------|------|
| 世界观设定（Lore） | 5 个 MCP 工具 + 前端 UI + arc_id 关联弧线 |
| 物品/法宝（Item） | 5 个 MCP 工具 + 前端 UI + arc_id 关联弧线 |
| 物品流转记录（ItemOccurrence） | 2 个 MCP 工具，追踪物品在各章节的出现和状态变化 |
| 场景管理（Scene） | 4 个 MCP 工具 + 后端 API，arc_node_id 关联弧线节点 |
| 章节元数据（update_chapter_meta） | summary / key_events / characters_in / arc_ids |
| 树状上下文（get_writing_context） | 一次调用获取全量关联数据 + 超期伏笔检测 |
| 写作进度快照（Snapshot） | current_arc_id / active_chars / summary / detailed_state |
| 创作统计（Stats） | 字数 / 弧线进度 / 伏笔回收率 / 角色数 / 地点数 |

### 二、数据管线架构

```
prepare(get_writing_context) → outline(edit outlines/)
→ write(edit chapters/) → review(run_subagent)
→ maintain(update_*/create_* + update_chapter_meta + update_writing_snapshot
           + search_lore + search_items + set_phase)
→ 回到 prepare → 读到 maintain 回写的最新数据
```

- prepare require `get_writing_context` 一次获取全量状态
- maintain require `update_chapter_meta` + `update_writing_snapshot` + `search_lore` + `search_items` 强制回写
- **双层 required**：门禁 require 强制调用工具，jsonschema required 强制填完整字段

### 三、阶段门禁

- 5 阶段校验：prepare → outline → write → review → maintain
- 每阶段有 tools 白名单 + require 必调列表
- maintain 阶段 15 项逐项检查清单（详见 `writing-kernel.md`）
- 门禁配置存数据库，不占 AI 上下文

### 四、HTTP API（23 个端点）

原版无 API。新增全部读端点：

```
GET  /api/novels              小说列表
GET  /api/novels/{id}/chapters  章节列表
GET  /api/chapters/{id}        章节内容
GET  /api/characters           角色
GET  /api/character-relations  角色关系
GET  /api/locations            地点
GET  /api/location-relations   地点关系
GET  /api/lore                 世界观设定
GET  /api/items                物品
GET  /api/item-occurrences     物品流转记录
GET  /api/scenes               场景
GET  /api/timeline             时间线
GET  /api/arcs                 弧线
GET  /api/arc-nodes            弧线节点
GET  /api/reader               读者认知
GET  /api/preferences          偏好
GET  /api/stats                统计
GET  /api/writing-snapshot     写作快照
GET  /api/phase-gate-config    门禁配置
GET  /api/search-memory        语义搜索
GET  /api/writing-context      树状上下文
GET  /api/read                 读取文件
POST /api/chat                 AI 对话（SSE）
```

Bearer Token 认证，详见 [mobile/API.md](mobile/API.md)。

### 五、模型配置增强

| 改动 | 说明 |
|------|------|
| model.dev 取模型数据 | 自动获取模型列表和参数 |
| 思考模式支持 | 深度推理开关（high/max） |
| 自定义模型编辑按钮 | 添加后可点击铅笔图标修改参数 |
| 单轮最大轮次 | 50 → **100** |

### 六、移动端 Web 前端

手机浏览器访问 `https://{局域网IP}:8877/mobile/`。

| 模块 | 功能 |
|------|------|
| 书架 | 小说列表、字数统计、当前书籍标识 |
| 小说详情 | 章节/角色/时间线/弧线/读者/偏好/地点/世界观/物品 九大模块 |
| 全屏阅读器 | 字号行距调节、左右翻页、章节目录、进度记忆 |
| AI 对话 | 流式 SSE、思考过程、会话历史、模型切换、复制按钮 |
| 设置 | 深浅模式、中英语言、Token 管理、模型选择 |

- **离线缓存**：`idb-keyval` + 内存 Map 双缓存，断网秒读
- **Service Worker**：预缓存静态资源，断网页面骨架正常加载
- **双端实时同步**：桌面端与移动端 WebSocket 全双工同步
- **扫码连接**：桌面端 Token 二维码，手机扫码快速连接
- **自动 HTTPS**：启动时自动生成证书
- **移动端主题**：木艺书阁·暖卷沉光主题

### 七、对话界面优化

| 改动 | 说明 |
|------|------|
| 消息泡复制按钮 | 在消息泡边框外（AI 泡右侧/用户泡左侧），不遮挡内容 |
| 一键到达底部按钮 | 快速跳转到最新消息 |
| 消息间距 | AI 消息和用户消息之间增加间距 |
| 门禁拦截提示 | 移到进度条下方，不再遮挡进度条 |

### 八、自定义主题

设置 → 主题 → 粘贴 JSON → 单击即应用，无需确认按钮。

**JSON 格式：**
```json
{
  "name": "墨绿书斋",
  "type": "dark",
  "colors": {
    "--background": "#0f1a14",
    "--foreground": "#d8e8d8",
    "--primary": "#5a9a6a",
    ...
  }
}
```

- `name` — 主题名称
- `type` — `light` 或 `dark`，决定图表配色方案
- `colors` — 全部 CSS 变量键值对，缺任意变量会导致 UI 异常

**去重键**：`name__type`（同名不同深浅可共存）。**支持注释**（`//` 和 `/* */`）。

**颜色变量清单（67 个）：**

| 变量 | 影响区域 |
|------|---------|
| `--background` | 页面最底层背景 |
| `--foreground` | 正文/标题/列表文字 |
| `--card` / `--card-foreground` | 卡片/面板/弹窗 |
| `--popover` / `--popover-foreground` | 浮层/弹窗 |
| `--primary`🔑 / `--primary-foreground` | 按钮/链接/选中态/滑块/开关 |
| `--secondary` / `--secondary-foreground` | 次要面板 |
| `--muted` / `--muted-foreground` | 输入框/代码块/辅助文字 |
| `--accent` / `--accent-foreground` | 悬浮/高亮行 |
| `--destructive` / `--destructive-foreground` | 删除按钮/错误提示 |
| `--border` / `--input` / `--ring` | 边框/聚焦光晕 |
| `--chart-1` ~ `--chart-5` | 图表色序 |
| `--sidebar*`（6 个） | 侧边栏 |
| `--tag-blue` / `--tag-green` / `--tag-amber` / `--tag-rose` / `--tag-teal` / `--tag-purple`🔖（含 `-foreground`） | 标签/徽章色系 |
| `--reader-bg` / `--reader-paper` | 阅读模式背景/纸张 |
| `--bubble-user`💬 / `--bubble-user-foreground` | 用户消息气泡 |
| `--action-extract` / `--action-save` | 操作按钮 |
| `--success`✅ / `--success-foreground` / `--success-border` | 成功提示 |
| `--danger-bg`⚠️ / `--danger-border` | 错误/警告提示 |
| `--status-warning` / `--status-ok` | 状态色 |
| `--tool-blue` / `--tool-amber` / `--tool-green` / `--tool-red`🔧（含 `-border`） | 工具调用块 |
| `--contribution-0` ~ `--contribution-4`📊 | 贡献图色阶 |

**注意事项：**
- 必须提供全部 67 个变量，缺失会导致 UI 破碎
- `type` 字段仅控制图表 light/dark 模式，不是 CSS 模式切换
- JSON 支持 `//` 和 `/* */` 注释
- Diff 编辑器（Monaco）主题色从 CSS 变量读取，切换主题时自动跟随

### 九、图标替换

| 位置 | 用途 | 格式 |
|------|------|------|
| `build/windows/icon.ico` | exe 文件图标 + 窗口标题栏图标 | ICO（多尺寸） |
| `appicon.png` | Wails 构建用的应用图标 | PNG |
| `frontend/public/logo.svg` | 标题栏左上角 Logo | SVG |
| `frontend/public/favicon.svg` | 浏览器标签页图标 | SVG |
| `assets/logo.svg` | Logo 源文件 | SVG |

**替换步骤：**
1. 准备新图标（推荐 SVG 或高清 PNG）
2. 替换对应文件：
   - **exe 图标**：用在线工具将 PNG 转为 ICO，替换 `build/windows/icon.ico`
   - **应用图标**：将 PNG 放到项目根目录，重命名为 `appicon.png`，同时复制到 `build/appicon.png`
   - **标题栏 Logo**：将 SVG 放到 `frontend/public/logo.svg`
   - **Favicon**：将 SVG 放到 `frontend/public/favicon.svg`
3. 运行 `.\build.ps1` 重新构建
4. 若 exe 图标未更新，清除 Windows 图标缓存或重启电脑

### 十、WebDAV

内置 WebDAV 服务器，手机文件管理器直接阅读小说。

### 十一、其他功能

| 功能 | 说明 |
|------|------|
| 章节字数范围调整 | 设置中自定义最少/最多字数 |
| 日志开关 | 设置中启用/禁用文件日志 |
| 数据备份恢复 | 一键备份/恢复全量数据 |
| 自定义软件图标 | 桌面图标、任务栏图标、标题栏图标统一匹配主题风格（图标位置见"图标替换"） |
| 帮助中心 | 52 个工具的中英文描述，含返回结构文档 |
| 系统提示词精简 | ~4700 token → ~2400 token（省 49%） |
| Token 注入统计 | `tokencount` 精确统计系统提示词 + 工具定义注入量（当前约 16.1K token） |
| writing-kernel.md | 15 项 maintain 检查清单 |
| config.json 移除 | 数据目录直接用 exe 位置，无需 config.json |
| 计费面板 | Token 用量按模型累计，缓存命中/未命中分账，价格可配（元/百万 token） |
| 个人中心 Token 趋势图 | 按日期 + 模型聚合的月度消耗总览，SVG 饼图展示缓存占比 |
| 动态叙事面板 | 画布式可拖拽/缩放卡片面板，聚合当前/过去/未来/弧线/伏笔/读者 7 类叙事信息 |
| DOCX 导出 | 纯标准库实现（archive/zip + XML），无外部依赖 |
| Prompt Caching 优化 | 稳定前缀（identity + always + catalog）+ NovelState 动态注入，消息前缀哈希监控缓存稳定性 |
| 输入框引导提示 | 空会话时显示 4 张引导卡片 |
| HTTPS 开关 | 移动端 API 可在设置中关闭 HTTPS 改用 HTTP（局域网调试） |
| 侧边栏宽度拖拽 | SidePanel 可拖拽调整宽度 |

### 十二、字段扩展

| 表 | 上游字段 | 新增字段 |
|----|----------|---------|
| `lore_entries` | reference_type, reference_id | arc_id, reveal_chapter_id, is_public |
| `items` | owner_id, location_id, status | arc_id, first_chapter_id, status_changed_chapter_id, narrative_role, previous_owner_id |
| `scenes` | chapter_id, character_ids, location_id | arc_id, arc_node_id |
| `chapters` | title, summary | key_events, characters_in, arc_ids |
| `writing_snapshots` | last_chapter_id, current_location | current_arc_id, active_chars, summary, detailed_state |

### 十三、数据库表扩展

| 上游 | 本 fork |
|------|---------|
| 22 张表 | **25** 张表（+item_occurrences, scenes, model_usage） |

### 十四、MCP 工具扩展

| 上游 | 本 fork |
|------|---------|
| 33 个工具 | **52** 个工具（+19） |
| 部分工具有 Description | **全部标注返回结构** |
| 部分字段有 required | **依赖链字段全部标注 jsonschema required** |

### 十五、文档

| 文档 | 说明 |
|------|------|
| `docs/README.md` | 文档索引（architecture/design/adr/archive 分层） |
| `docs/architecture/architecture.md` | 完整架构文档（新 AI 接手必读） |
| `docs/architecture/phase-gate.md` | 阶段门禁文档 |
| `docs/architecture/competitor-analysis.md` | 国内百万字级竞品分析 |
| `docs/architecture/narrative-panel.md` | 动态叙事面板设计 |
| `docs/architecture/token-injection.md` | Token 注入构成分析 + tokencount 使用说明 |
| `docs/design/token-optimization-plan.md` | Token 优化方案全集 |
| `docs/adr/0001-prompt-caching.md` | Prompt Caching 决策记录 |
| `docs/archive/` | 审计/测试/调研等一次性过程记录 |
| `mobile/API.md` | HTTP API 文档（27 节） |

### 十六、Skill 体系

三层（内置/用户/小说级）× 三种模式（auto/manual/always）= 9 种策略。

当前 17 个 Skill，新建 `.md` 文件即新 Skill，零代码扩展。

### 十七、安全

- 双层沙箱：正则白名单 + SafePath 杜绝路径穿越
- 文件编辑写入前重读对比，防止覆盖手动修改

---

## 安装

从 [Releases] 下载安装包。

### 运行时依赖

| 依赖 | 说明 |
|------|------|
| WebView2 Runtime | Windows 11 内置；Windows 10 需要安装 |
| LLM API Key | 兼容 OpenAI 格式（DeepSeek、OpenAI、Claude、NVIDIA 等） |

### 从源码构建

```bash
git clone https://github.com
cd goink
sudo apt install libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc  # Linux
make deps && make build  # 或 make dev
```

Windows 一键构建：`.\build.ps1` 或 `build.bat`

---

## 项目结构

```
goink/
├── app/                    # Wails 绑定 + HTTP API（23 端点）
├── tokencount/            # Token 统计工具（精确统计系统提示词 + 工具 JSON 注入）
├── internal/
│   ├── agent/              # Agent loop（MaxTurns 100）
│   ├── agentcfg/           # 系统提示词（2400 token）+ 工具白名单
│   ├── mcp_tools/          # 52 个 MCP 工具
│   ├── llm/                # 多提供商 LLM
│   ├── session/            # 会话 + 消息
│   ├── character/          # 角色 + 有向关系图
│   ├── chapter/            # 章节元数据
│   ├── timeline/           # 伏笔 + 章节计划
│   ├── storyarc/           # 故事弧线 + 节点
│   ├── reader/             # 读者认知
│   ├── location/           # 地点图
│   ├── lore/               # 世界观设定
│   ├── item/               # 物品/法宝
│   ├── itemoccurrence/     # 物品流转记录
│   ├── scene/              # 场景管理
│   ├── writing/            # 写作日志 + 快照
│   ├── rag/                # 向量搜索（ONNX）
│   ├── search/             # 全文搜索
│   ├── skill/              # 技能系统（3 层 × 3 模式）
│   ├── cert/               # 自动 HTTPS 证书
│   ├── webdav/             # WebDAV 服务器
│   └── migrate/            # 25 张表自动迁移
├── mobile/                 # 移动端 Web 前端
├── frontend/               # 桌面端 React 前端
├── docs/                   # 架构/审计/竞品分析文档
├── skills/                 # 17 个内置 Skill
├── build.ps1               # Windows 一键构建
└── build.bat               # Windows 一键构建
```

---

## 技术栈

| 层 | 选型 |
|---|---|
| Agent 引擎 | ReAct 循环（Go，SSE + 52 工具 + 子 Agent，MaxTurns 100） |
| 桌面框架 | Wails v2（Go + WebView） |
| 前端 | React + TypeScript + Tailwind CSS + shadcn/ui |
| 移动端 | HTTP API + 纯原生 JS Web 前端 + idb-keyval 离线缓存 |
| 数据库 | SQLite + GORM（25 张表 + 自动迁移） |
| 向量搜索 | sqlite-vec + ONNX Runtime（BGE 中文模型） |
| 版本控制 | 内置 Git（自动 commit / Diff / Revert） |

---

## License

AGPL-3.0。详见 [LICENSE](LICENSE) 和 [NOTICE](NOTICE)。
