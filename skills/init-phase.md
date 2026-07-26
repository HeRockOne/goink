---
name: init-phase
description: 开书阶段工作流。首次对话必须先加载此 skill，填写全部核心设定后切换到 prepare 阶段。后续对话无需再加载。
category: 核心系统
mode: auto
---

# 开书阶段（init，仅新书首次）

## 创建顺序（依赖链约束，不可颠倒）

| # | 项目 | 工具 | required 字段 | 依赖 |
|---|------|------|--------------|------|
| 1 | **创建故事弧线**（至少1条主线） | create_story_arc | name(R), arc_type(R) | 无 |
| 2 | **创建核心地点**（至少3个） | create_location | name(R) | 无 |
| 3 | **创建核心角色**（至少2个，传 location_id） | create_character | name(R), description(R) | → location |
| 4 | **创建弧线节点**（至少2个，标记主线目标章节） | create_arc_node | story_arc_id(R), title(R), target_chapter(R) | → arc |
| 5 | **创建世界观**（至少1条，关联弧线） | create_lore | title(R), category(R), content(R), arc_id(R), reveal_chapter_id(R) | → arc |
| 6 | **创建物品**（重要物品，关联持有者+弧线） | create_item | name(R), arc_id(R), owner_id(R), narrative_role(R) | → arc, character |
| 7 | **创建偏好**（写作规则） | create_preference | category(R), content(R) | 无 |
| 8 | **创建伏笔**（至少3条，标记目标章节+重要度） | create_timeline_entry | title(R), category(R), target_chapter(R), importance(R) | → arc |

## 验证（逐项确认写入成功）

| # | 验证项 | 工具 | 通过条件 |
|---|--------|------|---------|
| 1 | 角色已创建 | get_characters | characters.length > 0 |
| 2 | 地点已创建 | get_locations | locations.length >= 3 |
| 3 | 弧线已创建 | get_story_arcs | arcs.length >= 1 |
| 4 | 世界观已创建 | get_lore | lore.length >= 1 |
| 5 | 物品已创建 | get_items | items.length >= 1 |
| 6 | 伏笔已创建 | get_timeline | timeline.pending.length >= 3 |
| 7 | 偏好已创建 | get_preferences | content 非空 |

全部验证通过后 → **set_phase("prepare")**
