# Scene 设计文档

## 概述

当前最小写作单元是"章"（Chapter）。但一章内可能包含多个场景/幕的切换：

```
第 35 章（3000 字）
  ├── 场景 1：青云宗演武场（500 字）
  │   └── 主角 vs 大师兄，展示新学的剑法
  ├── 场景 2：宗门议事厅（1200 字）
  │   └── 长老会讨论秘境开启，主角被点名
  └── 场景 3：主角居所（1300 字）
      └── 主角与师弟对话，揭示天玄剑的异常
```

当前没有 Scene 层，Agent 写一章就是一个 `write_chapter_content` 调用。问题：

- 一章内多个场景切换，Agent 需要自行在内容里标注（"---"分割），但这些标注不可查询
- 无法按场景维度统计（"所有发生在演武场的场景"）
- 无法精确引用场景位置（"在第 35 章的演武场场景中"）

## 方案：Scene 作为 Chapter 的子表

```go
type Scene struct {
    ID              int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
    NovelID         int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
    ChapterID       int64     `gorm:"column:chapter_id;not null;index"  json:"chapter_id"`
    SceneNumber     int       `gorm:"column:scene_number;not null"      json:"scene_number"` // 场景在章内的序号，从 1 开始
    Title           string    `gorm:"column:title"                      json:"title"`        // 场景标题（可选）
    LocationID      *int64    `gorm:"column:location_id;index"          json:"location_id"`  // 场景发生的地点 ID（软引用）
    CharacterIDs    string    `gorm:"column:character_ids"              json:"character_ids"` // JSON 数组，出场角色 ID 列表
    WordCount       int       `gorm:"column:word_count;default:0"       json:"word_count"`
    Summary         string    `gorm:"column:summary"                    json:"summary"`      // 场景摘要（AI 写，50-100 字）
    CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
    UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}
```

### 设计决策

| 问题 | 决定 |
|------|------|
| Scene 自己存 content 吗 | **不存**。正文内容在 Chapter 中。Scene 是描述性元数据，不是内容容器 |
| Scene 由谁维护 | **Agent 在写完一章后调用 `create_scene` 建立场景索引**，而不是实时跟踪。不要求每写一段就更新 |
| 可以不建场景吗 | **可以**。Scene 完全可选，不写场景的章节照常工作 |
| 场景在编辑时变化 | Agent 可以 update_scene 重新标注（比如把一个大场景拆成两个）|

## MCP 工具（4 个）

| 工具 | 功能 |
|------|------|
| `get_scenes` | 获取章节的场景列表（按 scene_number 排序） |
| `create_scene` | 为某章创建一个场景条目 |
| `update_scene` | 更新场景信息 |
| `delete_scene` | 删除场景条目 |

### get_scenes 的用途

Agent 在写新章节前可查询：
```
"主角第 3 章在演武场，第 15 章也在演武场——查所有发生在演武场的场景"
→ get_scenes(location_id=演武场) → 返回两个场景的引用
```

虽然场景正文不可查，但场景摘要和关联角色/地点构成了一个轻量的索引层。

## 前端展示

- 章节详情页内以折叠组展示场景列表
- 每个场景显示：标题 + 所在地点 + 出场角色 + 摘要
- 不直接在 reader 中渲染场景标记（保持阅读流畅性）

## 不做功能

- **自动场景分割**：不通过 AI 分析章节内容来自动拆分场景。完全由 Agent/用户手动维护
- **场景间转场效果**：属于写作风格，不在系统层约束
- **场景时间戳**（"第 35 章场景 1 发生在午时"）：用场景摘要自然语言表达即可

## 实现路线

1. `internal/scene/types.go` + `internal/scene/store.go`
2. `internal/mcp_tools/scene_tools.go` — 4 个工具注册
3. 迁移：新增 scenes 表
4. 前端展示场景列表
