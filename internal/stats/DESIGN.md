# Stats Dashboard 设计文档

## 概述

当前 Goink 只有字数统计（`writing_log` + `chapter.word_count`），缺少全局维度的聚合视图。对于长篇创作（尤其 100 万字+），作者需要知道：

- 主角最近 10 章出现了吗？出场率多少？
- 某个地点已经多少章没出现了？
- 伏笔回收率怎么样？还有多少没回收？
- 每天写多少字？连续写作天数？

这些数据**都已经存在于数据库中**，只是没有聚合查询和展示。

## 方案：新增 `internal/stats` 包，纯查询，无新表

所有数据从现有表聚合，不新增任何 DB 表。

```go
type NovelStats struct {
    // 进度概览
    TotalChapters      int   `json:"total_chapters"`
    TotalWords         int   `json:"total_words"`
    AvgChapterWords    int   `json:"avg_chapter_words"`
    LatestChapterNum   int   `json:"latest_chapter_num"`
    LatestChapterTitle string `json:"latest_chapter_title"`
    DaysSinceLastWrite int   `json:"days_since_last_write"`
    
    // 弧线
    ArcCount      int `json:"arc_count"`
    ArcCompleted  int `json:"arc_completed"`
    ArcActive     int `json:"arc_active"`
    
    // 伏笔
    ForeshadowingTotal    int `json:"foreshadowing_total"`
    ForeshadowingResolved int `json:"foreshadowing_resolved"`
    ForeshadowingPending  int `json:"foreshadowing_pending"`
    
    // 角色
    CharacterCount int `json:"character_count"`
    
    // 地点
    LocationCount   int `json:"location_count"`
    
    // 写作统计（复用 writing 包）
    TotalDaysActive int `json:"total_days_active"`
    CurrentStreak   int `json:"current_streak"`
    LongestStreak   int `json:"longest_streak"`
}

type CharacterAppearance struct {
    CharacterID   int64  `json:"character_id"`
    CharacterName string `json:"character_name"`
    ChapterCount  int    `json:"chapter_count"`  // 出场章数
    LastChapter   int    `json:"last_chapter"`   // 最近出场章节号
    GapChapters   int    `json:"gap_chapters"`   // 距现在多少章没出现
}

type LocationUsage struct {
    LocationID   int64  `json:"location_id"`
    LocationName string `json:"location_name"`
    ChapterCount int    `json:"chapter_count"`
    LastChapter  int    `json:"last_chapter"`
    GapChapters  int    `json:"gap_chapters"`
}
```

## 角色出场统计的实现

角色没有直接绑定章节，需要通过 TimelineEntry 或正文关键词推断。**不做 AI 语义分析**，采用简单的引用统计：

1. 扫描 `timeline_entry` 中 `content` / `detail_json` 提到角色名的条目
2. 按 `source_chapter_id` 或关联章节计数
3. 方法不精确（名字可能写错/简称），但足够给作者一个大致参考

精确统计属于 P0 之后的事情，现阶段提供一个"合理估算"即可。

## 地点使用统计

同样基于 TimelineEntry 引用，以及 `Location` 本身的 `description` 更新频率来推测活跃度。

## MCP 工具（1 个）

| 工具 | 功能 |
|------|------|
| `get_stats` | 获取统计学要（novel_id，可选参数控制返回内容） |

参数：
```go
type GetStatsArgs struct {
    IncludeCharacters bool `json:"include_characters"` // 是否包含角色出场统计
    IncludeLocations  bool `json:"include_locations"`  // 是否包含地点使用统计
}
```

返回内容按参数控制，默认只返回 NovelStats（轻量），角色/地点统计默认不返回。

## 前端展示

- 移动端：小说详情页新增"统计" Tab
- 桌面端：侧边栏或弹窗展示
- 内容：进度条（伏笔回收率）、图表（每日字数趋势）、列表（角色出场排行）

## 不做功能

- **实时计算**：每次请求实时聚合（数据量小，SQL 查询 < 10ms）
- **图表库**：前端用简单 CSS 条形图，不引入 echarts 等重型库
- **导出报告**：属于独立功能

## 实现路线

1. `internal/stats/store.go` — 聚合查询函数（5-8 个 SQL）
2. `internal/mcp_tools/stats_tools.go` — get_stats 工具注册
3. 移动端/桌面端 UI 展示
