# Lore 设计文档

## 概述

Lore 管理小说的世界观设定集——力量体系、社会构成、历史事件、核心冲突等横跨整本书、不绑定某个具体地点或角色的知识。由一张表组成：`lore_entries`。

与 Location（地理空间）和 Character（角色）互补：Location 回答"哪里"，Character 回答"谁"，Lore 回答**"这个世界的规则是什么"**。

```

世界观（Lore）         地理（Location）           角色（Character）
┌──────────────┐     ┌──────────────┐         ┌──────────────┐
│ 力量体系        │     │ 苍澜大陆        │         │ 主角          │
│ 社会构成        │     │ ├─ 东荒        │         │ ├─ 金丹期     │← Lore 引用
│ 天道法则        │     │ │  ├─ 青云山脉   │──────→ │ ├─ 太虚宗弟子  │← Lore 引用
│ 三万年前大战     │     │ │  │  └─ 太虚宗  │         │ └─ 师从清虚真人 │← Character 关系
│ 正魔之争        │     │ └─ 西海        │         └──────────────┘
└──────────────┘     └──────────────┘
```

## 表结构

```go
type LoreEntry struct {
    ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
    NovelID     int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
    Title       string    `gorm:"column:title;not null;index"       json:"title"`       // 标题，如"金丹期""天玄大陆人族联盟""正魔之战的起源"
    Category    string    `gorm:"column:category;not null;index"    json:"category"`    // 分类：力量体系/社会构成/历史事件/核心冲突/天道法则/文化习俗/种族/其他
    Content     string    `gorm:"column:content;not null"           json:"content"`     // markdown 正文，AI 自由书写
    Summary     string    `gorm:"column:summary"                    json:"summary"`     // 一句话摘要，列表展示和快速检索用
    ReferenceID *int64    `gorm:"column:reference_id;index"         json:"reference_id"`// 关联实体ID（如关联到 location_id/character_id），可选
    ReferenceType string  `gorm:"column:reference_type"             json:"reference_type"`// "location" / "character" / ""，配合 reference_id 使用
    Tags        string    `gorm:"column:tags"                       json:"tags"`        // JSON 自由标签，如 ["修仙文明","人族"]
    Version     int       `gorm:"column:version;default:1"          json:"version"`     // 版本号，每次更新 +1，支持追溯
    CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
    UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}
```

### 字段设计说明

| 字段 | 理由 |
|------|------|
| `Category` | 自由文本，与 `location_type` 设计一致。不设枚举，AI 可自行扩展"魔法体系""家族谱系"等分类 |
| `Content` | Markdown 正文，支持结构化书写（列表、表格、引用），AI 在对话中可轻松读写 |
| `Summary` | 一句话摘要，用于列表查询时快速筛选，避免每次都拉全文 |
| `ReferenceID + ReferenceType` | 可选关联。比如"青云山脉的灵气浓度"可以关联到 location=青云山脉，"魔尊重楼"关联到 character=重楼 |
| `Version` | 每次更新 +1，用于跟踪设定变更历史，Agent 可判断设定是否稳定 |

### 不会引入的字段

| 字段 | 不引入的理由 |
|------|-------------|
| `detail_json` | Content 是 markdown 自由文本，足够表达结构化内容，不需要 JSON 扩展槽 |
| `is_active` / `status` | 设定没有"废弃"状态，过时设定也是世界历史的一部分，通过 version 追踪 |
| `parent_id` | Lore 不构成树形结构，各条目是并列的知识点 |
| `related_entries` | 通过 tags 和搜索表达关联，不设硬 FK |

## 与同类模块的对比

| | Character | Location | Lore |
|---|---|---|---|
| 实体 | 角色 | 地点 | 设定/知识 |
| 关系 | 人际图（有向） | 包含树 + 空间图（无向） | 无图，平铺条目 + 标签关联 |
| 演进 | append-only，is_current 标记 | UPSERT 覆盖，不追踪历史 | version 递增，保留旧版本 |
| 引用方式 | character_id | location_id | reference_id(reference_type)，可选 |
| 核心用途 | "谁在哪，什么性格" | "周围有什么" | "这个世界的规则是什么" |

## MCP 工具（5 个）

| 工具 | 功能 | 参数 |
|------|------|------|
| `get_lore` | list（分类/搜索过滤）/ detail（全文） | category, search, lore_id |
| `create_lore` | 新建设定条目 | title, category, content, summary, reference_id, reference_type, tags |
| `update_lore` | 更新条目（PATCH），版本号自动递增 | lore_id + 可选字段 |
| `delete_lore` | 删除条目 | lore_id |
| `search_lore` | 全文搜索所有设定条目 | query, category |

### get_lore 两种模式

- **list**：返回条目列表（id, title, category, summary, tags），支持 category 和 search 过滤
- **detail**：返回单条完整信息（所有字段 + 版本号）

### create_lore 与 update_lore

参考 create_location / update_location 相同的 CRUD 模式，独立不合并。

### search_lore

专设全文搜索工具而非复用 get_lore，原因：
- LLM 查设定时往往是模糊的（"那个金丹期的设定在哪"），全文搜索比分类过滤更直接
- 区别于 get_lore 的精确查询，search_lore 用 SQL LIKE 或全文索引做模糊匹配
- 在 Agent 循环中，search_lore 放在只读工具白名单中，不消耗写额度

## Category 分类建议

以下分类是初始推荐值，AI 可自由扩展：

| 分类 | 内容示例 |
|------|---------|
| 力量体系 | 境界划分（筑基/金丹/元婴）、灵力修炼方式、突破条件 |
| 社会构成 | 宗门等级、国家制度、阶层划分、货币体系 |
| 历史事件 | 创世传说、万年大战、某个朝代的兴衰 |
| 核心冲突 | 正魔对立、天道崩坏、资源争夺的主线矛盾 |
| 天道法则 | 世界运行的底层规则（如"灵力会自然衰减""突破必遇天劫"） |
| 文化习俗 | 节日、礼仪、禁忌、信仰 |
| 种族/物种 | 人族/妖族/魔族的生理特征、社会地位、天赋能力 |
| 地理概述 | 大陆格局的整体描述（与 Location 互补：Location 是具体节点，此条目是概览） |

## 与 Location 的协作

设定条目可以关联到 Location，但二者职责分开：

```
Location: "青云山脉" → type="山脉", desc="灵气充沛，主峰高三千丈"
Lore: "青云山脉的灵气" → category="地理概述", 
      content="青云山脉地下有一条灵脉，每千年喷发一次灵力潮汐...",
      reference_id=青云山脉.id, reference_type="location"
```

Agent 查 `get_locations(detail, location_id=X)` 时，可以在返回值中附带关联的 Lore 条目数量或引用，引导 LLM 进一步查询。

## 与前端的交互

- **列表页**：分类折叠展示（同 location detail 页的 tab 风格），每项显示 title + category + summary
- **详情弹窗**：复用现有 detailSheet，标题 + markdown 正文 + 关联实体链接 + 版本号
- **搜索**：统一搜索入口，可跨 Lore / Character / Location 搜索

## 版本追踪

每次 update_lore 时 version 自动 +1。旧版本保留在数据库（保留最近 5 版）：

```
version=1: "金丹期可活500年"
version=2: "金丹期可活800年（修订：大衍天诀可延寿）"
```

Agent 在更新时可看到当前版本号，判断设定是刚刚创建还是已经修订多次。前端版本信息不直接展示，只在设定详情页标注"已修订 N 次"。

## 没有引入的功能及理由

| 功能 | 不引入的理由 |
|------|-------------|
| 条目间引用图 | 创作初期条目少（50 条以内），不需要图结构。后续可通过 search 和 tags 替代 |
| 自动化冲突检测 | 两句话矛盾（"金丹期活500年"vs"金丹期活800年"）需要语义理解，现阶段做不到可靠检测，做假阳性反而让 AI 困惑 |
| 导入/导出设定集 | 属于后续迭代，非核心功能 |

## 实现路线

### Phase 1 — 基础 CRUD
1. `internal/lore/store.go` — GORM CRUD + list/detail/search
2. `internal/lore/types.go` — LoreEntry 模型
3. `internal/lore/mcp_tools.go` — 5 个 MCP 工具
4. `internal/mcp_tools/lore_tools.go` — 注册到 Registry
5. `internal/migrate/` — AutoMigrate 新增表

### Phase 2 — 关联能力
6. `get_locations` 返回值中附带关联的 lore 条目摘要
7. `get_character` 返回值中附带关联的 lore 条目摘要

### Phase 3 — 搜索 & 前端
8. 全文搜索接入
9. 移动端前端设定集 tab
10. 桌面端浏览/编辑界面
