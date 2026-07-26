# Timeline Chronicle 设计文档

## 概述

当前 `timeline_entry` 表设计为"伏笔/待办"——每一条都有 `status`（pending/resolved）、`target_chapter`、`importance`。它不是"客观事件记录"。

但长篇创作需要两种时间线：

| 类型 | 例子 | 当前支持 |
|------|------|---------|
| **伏笔**（Foreshadowing） | "主角在第 10 章获得的神秘玉佩在第 50 章揭示来历" | ✅ TimelineEntry，status=pending |
| **编年史**（Chronicle） | "三万年前仙魔大战，魔尊被封印""灵历 1000 年，天玄宗建立" | ❌ 没有适合的记录方式 |

把编年史硬塞进 timeline_entry 的问题：
- status 必填，但"三万年前的大战"不待解决
- target_chapter 必填，但历史事件没有目标章节
- importance 必填，但历史事件的权重与伏笔不同

## 方案：扩展 TimelineEntry，不新增表

在 `timeline_entry` 中加一个 `entry_type` 字段区分两种模式，避免多一张表的管理成本：

```go
// 新增字段
entry_type  string  // "foreshadowing"（伏笔，默认）| "chronicle"（编年史）
```

### 伏笔模式（默认）

现有行为不变：
- `status = pending/resolved`
- `target_chapter` 必填
- `importance` 起作用
- 查询可过滤 `entry_type = foreshadowing`

### 编年史模式

- `status` 设为空或 `"occurred"`
- `target_chapter` 设为 0（不适用）
- `importance` 设为 0（不适用）
- 查询可过滤 `entry_type = chronicle`
- 新增可选字段 `chronology_date`——事件在设定中的时间坐标，如"灵历 1000 年""三万年前"

### 可选新增字段

```go
ChronologyDate  string  `gorm:"column:chronology_date"`  // 编年史时间坐标，自由文本
```

不单设 `end_date`、`era` 等——复杂度过高。编年史在内容上用 Markdown 表达。

### MCP 工具变化

`create_timeline_entry` 新增参数：
```go
entry_type      string  // "foreshadowing"（默认）| "chronicle"
chronology_date string  // 编年史专用
```

`get_timeline` 新增过滤：
```go
entry_type      string  // 空=全部，"foreshadowing"=仅伏笔，"chronicle"=仅编年史
```

### 前端展示

- 编年史事件以时间线方式展示（chronology_date 排序）
- 伏笔以当前的折叠分组方式展示（按 status 分组）
- 可在同一页面/组件中切换视图

## 不做功能

- **多维时间线**（多条编年史分枝同时推进）。当前用 tag 或 content 区分即可
- **事件间因果图**。太复杂，编年史是平铺的

## 实现路线

1. 数据库 migration：新增 `entry_type` + `chronology_date` 字段
2. MCP 工具参数扩展：create/get 接口增加 entry_type 过滤
3. 前端展示区分两种模式
