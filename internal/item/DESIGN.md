# Item 设计文档

## 概述

Item 管理小说中的物品/法宝/道具系统。长篇创作（尤其玄幻修仙）中，物品是有独立身份、有历史、有能力的重要实体——主角获得一把剑、一枚丹药、一张地图，这些物品的特征和来历需要结构化存储，而非散落在章节正文里。

## 为什么需要独立模块

当前没有物品系统，Agent 只能把物品信息写在章节正文中。后续想查"天玄剑什么来历"时：

- ❌ **翻正文**：在第 5 章提到过"天玄剑，上古神兵"，第 50 章又提到一次——两条信息分散，无法聚合
- ❌ **全文检索**：搜"天玄剑"只能看到片段，没有结构化信息（品级、来历、能力）
- ✅ **Item 系统**：一条记录，随时查、随时更新，Agent 在写作时可引用

## 表结构

```go
type Item struct {
    ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
    NovelID     int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
    Name        string    `gorm:"column:name;not null;index"        json:"name"`        // 名称："天玄剑"
    ItemType    string    `gorm:"column:item_type;index"            json:"item_type"`   // 自由文本：法宝/丹药/灵药/功法/地图/信物/普通物品
    Grade       string    `gorm:"column:grade"                      json:"grade"`       // 品级：凡品/灵品/仙品/神品（自由文本，AI 自行定义体系）
    Description string    `gorm:"column:description"                json:"description"` // 外观/功能描述
    Lore        string    `gorm:"column:lore"                       json:"lore"`        // 来历/历史/传说（markdown 正文）
    Ability     string    `gorm:"column:ability"                    json:"ability"`     // 特殊能力/效果："持有者可感应灵脉""服用后突破金丹期瓶颈"
    OwnerID     *int64    `gorm:"column:owner_id;index"             json:"owner_id"`    // 当前持有者 character_id，NULL=无主/遗失
    LocationID  *int64    `gorm:"column:location_id;index"          json:"location_id"` // 当前位置 location_id，NULL=未知（被携带时 OwnerID 定位）
    Status      string    `gorm:"column:status;default:'active'"    json:"status"`      // active(现存)/consumed(已消耗)/destroyed(已毁)/lost(遗失)
    Tags        string    `gorm:"column:tags"                       json:"tags"`        // JSON 自由标签
    CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
    UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}
```

### 关键字段说明

| 字段 | 理由 |
|------|------|
| `ItemType` | 自由文本，不设枚举。AI 可填写法宝/丹药/灵药/功法/地图/信物/法器/铠甲/……与 `location_type` 设计一致 |
| `Grade` | 自由文本。不同小说体系不同（金丹/元婴 体系 → 下品/中品/上品/极品；也可以是 凡/灵/仙/神） |
| `Lore` | 物品的背景故事，与 Description（外观/功能描述）分离。一条物品可以同时有"外观黝黑无锋"（Description）和"上古剑神陨落时留下的佩剑"（Lore） |
| `Ability` | 特殊能力独立字段，便于 Agent 按能力搜索（"有没有能突破境界的东西"→搜 ability） |
| `OwnerID + LocationID` | 追踪物品当前位置。被角色携带时填 OwnerID，遗迹中时填 LocationID，都 NULL 表示遗失。Agent 可据此规划"主角需要去某个地点找某物品" |
| `Status` | 消耗品（丹药吃掉了）、毁坏品（剑断了）标记为 consumed/destroyed，不再参与活跃查询 |

### OwnerID / LocationID 追踪逻辑

物品的归属和位置是动态的。Agent 在写作中改变物品归属时调用 `update_item` 更新字段：

```
update_item(item_id=5, owner_id=12)
// 天玄剑现在在主角张三手里
```

不设变更历史（与 character_relation 的 append-only 不同），因为物品流转不频繁，正文中自有记载。

## MCP 工具（5 个）

| 工具 | 功能 |
|------|------|
| `get_items` | list（类型/品级/状态/搜索过滤）/ detail（完整信息 + 持有者名称 + 所在地点名称） |
| `create_item` | 新建设品条目（name 必填） |
| `update_item` | 更新物品（PATCH），可变更持有者/位置/状态 |
| `delete_item` | 删除物品条目 |
| `search_items` | 按名称/能力/描述/来历模糊搜索 |

## 与 Character / Location 的关系

Item 不设 FK 约束（`OwnerID` / `LocationID` 是软引用）：

- 删除角色时 item.owner_id 残留（标记为"持有人已不存在"）
- 删除地点时 item.location_id 残留（标记为"所在地已不存在"）
- Agent 可自行清理，系统不强删

## 不做功能

- **物品合成系统**：两个物品合成一个新物品——太复杂，现阶段 AI 在正文中描写即可
- **物品掉落/交易日志**：对于写作系统来说过于游戏化
- **物品图鉴/收集进度**：属于 UI 层，后续迭代

## 实现路线

### Phase 1 — 基础 CRUD
1. `internal/item/types.go` — Item 模型
2. `internal/item/store.go` — GORM CRUD + list/search
3. `internal/mcp_tools/item_tools.go` — 5 个工具注册
4. `internal/migrate/` — AutoMigrate 新增表

### Phase 2 — 关联查询
5. `get_characters` detail 模式返回持有物品列表
6. `get_locations` detail 模式返回存放物品列表

### Phase 3 — 前端展示
7. 移动端小说详情新增"物品" tab
8. 桌面端物品浏览/编辑界面
