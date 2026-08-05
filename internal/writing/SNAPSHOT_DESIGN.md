# Writing Snapshot 设计文档

## 概述

Writing Snapshot 是"当前写作进度快照"——Agent 在开始写新章节前需要知道的上下文摘要：故事进展到哪了、最新一章发生了什么、当前聚焦哪些角色和地点、下一章计划写什么。

目前 Agent 通过 `get_writing_context` 获取结构化上下文（DB 聚合）。goink.md 已收敛为章节指纹账本（仅 append 追加指纹），不再承载故事状态。问题：
- 纯文本 → Agent 每次都要全文重读，token 成本高
- AI 自维护 → 格式不一，有时过期，有时被覆盖
- 不可查询 → 无法回答"最近 5 章有哪些角色出场"

Writing Snapshot 用结构化方式解决这个问题，**替代 goink.md 的状态存储职责**——快速摘要，精确查询。

## 表结构

```go
type WritingSnapshot struct {
    ID              int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
    NovelID         int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
    LastChapterID   int64     `gorm:"column:last_chapter_id"            json:"last_chapter_id"`   // 最新已完成的章节 ID
    LastChapterNum  int       `gorm:"column:last_chapter_num"           json:"last_chapter_num"`  // 最新章节号（冗余，方便排序）
    CurrentArcID    *int64    `gorm:"column:current_arc_id"             json:"current_arc_id"`    // 正在进行的弧线 ID
    CurrentLocation string    `gorm:"column:current_location"           json:"current_location"`  // 当前故事焦点地点名称（非 ID，AI 自由书写）
    ActiveChars     string    `gorm:"column:active_chars"               json:"active_chars"`      // JSON 数组，当前活跃角色 ID 列表
    PendingThreads  string    `gorm:"column:pending_threads"            json:"pending_threads"`   // JSON 自由文本，待处理的剧情线索
    Summary         string    `gorm:"column:summary"                    json:"summary"`           // 一句话当前状态
    DetailedState   string    `gorm:"column:detailed_state"             json:"detailed_state"`    // Markdown 复杂状态描述（AI 自由书写，类似 goink.md）
    CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
    UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}
```

### 字段说明

| 字段 | 用途 | 谁来维护 |
|------|------|---------|
| `LastChapterID/Num` | Agent 知道"我从哪开始写" | Agent 写完一章后自动 `update_snapshot` |
| `CurrentArcID` | 当前的弧线，Agent 写新章节时以此为准 | Agent 主动切换，或从弧线系统自动推进 |
| `CurrentLocation` | 故事现在的地理焦点 | Agent 每章更新 |
| `ActiveChars` | 当前活跃角色 ID 列表，用于查询"最近谁出场了" | Agent 每章更新 |
| `PendingThreads` | 待处理的剧情线索（自由文本），Agent 写下一章前参考 | Agent 自维护 |
| `Summary` | 一句话状态："主角在青云宗试炼中，刚通过第一关" | Agent 每章更新 |
| `DetailedState` | 自由 Markdown，AI 想写什么写什么，goink.md 的替代 | Agent 自维护 |

## 与 goink.md 的关系

goink.md 已收敛为**章节指纹账本**（仅 append 追加指纹），不再承载故事状态。Writing Snapshot 接管状态存储职责：

```
Writing Snapshot（结构化）:
  last_chapter: 35
  current_arc: "宗门试炼篇"
  active_chars: [主角, 长老, 副帮主]
  pending_threads: "副帮主可疑行为"
  summary: "主角获得天玄剑，长老将察觉剑的异常"

goink.md（章节指纹账本，append only）:
  ## Chapter 35
  - fingerprint: a1b2c3d4
  - summary: 主角在试炼中获得了天玄剑
  - characters: 主角, 长老, 副帮主
  - threads: 副帮主可疑行为
```

Agent 写下一章前快速读取 Snapshot（几个字段，< 200 token），goink.md 仅用于跨对话指纹验证。

## MCP 工具

| 工具 | 功能 |
|------|------|
| `get_writing_snapshot` | 获取当前写作快照（无参数，自动取当前小说） |
| `update_writing_snapshot` | 更新快照（PATCH，传入要修改的字段） |

仅 2 个工具。Snapshot 由 Agent 主动维护，系统不自动生成。

## 不做功能

- **自动 diff 检测**：不自动比对正文变化来推断进度（不可靠）。完全由 Agent 主动汇报
- **多版本历史**：Snapshot 是当前状态，只保留最新一条，不追踪历史。历史记录通过 Git commit 完成

## 实现路线

1. `internal/writing/snapshot_types.go` + `internal/writing/snapshot_store.go`
2. `internal/mcp_tools/writing_tools.go` — get/update snapshot
3. 迁移：新增 writing_snapshots 表
